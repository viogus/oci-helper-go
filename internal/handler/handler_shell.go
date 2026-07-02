package handler

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	gossh "golang.org/x/crypto/ssh"

	"github.com/viogus/oci-helper-go/internal/db"
)

// wsMessage is the JSON protocol between frontend xterm.js and backend SSH.
type wsMessage struct {
	Type    string `json:"type"`              // "resize", "input", "output", "error", "ready"
	Rows    int    `json:"rows,omitempty"`     // terminal rows (resize)
	Cols    int    `json:"cols,omitempty"`     // terminal cols (resize)
	Data    string `json:"data,omitempty"`     // base64-encoded bytes
	Message string `json:"message,omitempty"`  // error/status text
}

// knownHostKeys stores SSH host keys for TOFU (Trust On First Use) verification.
// Keys are scoped to the server process lifetime.
var (
	knownHostKeys   = make(map[string]gossh.PublicKey)
	knownHostKeysMu sync.Mutex
)

// tofuHostKeyCallback returns a HostKeyCallback that implements TOFU:
// accepts the key on first connection, rejects on mismatch thereafter.
func tofuHostKeyCallback(host string) gossh.HostKeyCallback {
	return func(hostname string, remote net.Addr, key gossh.PublicKey) error {
		knownHostKeysMu.Lock()
		defer knownHostKeysMu.Unlock()
		stored, ok := knownHostKeys[host]
		if !ok {
			knownHostKeys[host] = key
			log.Printf("[shell] TOFU: accepted new host key for %s (%s)", host, gossh.FingerprintSHA256(key))
			return nil
		}
		if gossh.FingerprintSHA256(stored) != gossh.FingerprintSHA256(key) {
			return fmt.Errorf("host key mismatch for %s: expected %s, got %s",
				host, gossh.FingerprintSHA256(stored), gossh.FingerprintSHA256(key))
		}
		return nil
	}
}

var shellUpgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true // same-origin requests, curl, wscat
		}
		// Allow same host (including port differences for dev)
		host := r.Host
		if h, _, err := net.SplitHostPort(origin); err == nil {
			origin = h
		}
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		return origin == host
	},
}

// handleShellWS handles WebSocket upgrade and bridges SSH → browser terminal.
//
// GET /api/shell/ws?tenant_id=X&instance_id=X:ocid&ssh_key_id=X&rows=24&cols=80
func (s *Server) handleShellWS(w http.ResponseWriter, r *http.Request) {
	// ── Parse params ──────────────────────────────────────────────────
	tenantID, _ := strconv.ParseInt(r.URL.Query().Get("tenant_id"), 10, 64)
	instanceID := r.URL.Query().Get("instance_id")
	sshKeyID, _ := strconv.ParseInt(r.URL.Query().Get("ssh_key_id"), 10, 64)
	rows, _ := strconv.Atoi(r.URL.Query().Get("rows"))
	cols, _ := strconv.Atoi(r.URL.Query().Get("cols"))
	if rows < 1 {
		rows = 24
	}
	if cols < 1 {
		cols = 80
	}

	if tenantID == 0 || instanceID == "" || sshKeyID == 0 {
		jsonErr(w, "tenant_id, instance_id, and ssh_key_id are required")
		return
	}

	// ── Fetch tenant ──────────────────────────────────────────────────
	tenant, err := s.store.GetTenant(tenantID)
	if err != nil || tenant == nil {
		jsonErr(w, "tenant not found")
		return
	}

	// ── Look up instance in local DB for IPs ──────────────────────────
	inst, err := s.store.GetInstanceByID(instanceID)
	if err != nil || inst == nil {
		jsonErr(w, "instance not found in local DB — sync first")
		return
	}

	// ── Fetch SSH key with private key ────────────────────────────────
	sshKey, err := s.store.GetSSHKeyByID(sshKeyID)
	if err != nil || sshKey == nil {
		jsonErr(w, "SSH key not found")
		return
	}
	if sshKey.PrivateKey == "" {
		jsonErr(w, "SSH key has no private key — upload or generate a keypair")
		return
	}

	// ── Decrypt private key ───────────────────────────────────────────
	privPEM, err := decryptSSHPrivateKey(sshKey.PrivateKey)
	if err != nil {
		jsonErr(w, "decrypt ssh key: "+err.Error())
		return
	}

	// ── Upgrade to WebSocket ──────────────────────────────────────────
	wsConn, err := shellUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[shell] ws upgrade error: %v", err)
		return
	}
	defer wsConn.Close()

	// Clear deadlines on underlying connection for long-lived session
	if nc, ok := wsConn.UnderlyingConn().(interface{ SetDeadline(time.Time) error }); ok {
		nc.SetDeadline(time.Time{})
	}
	wsConn.SetReadLimit(65536)

	// ── Parse key and extract public key string ────────────────────────
	signer, err := gossh.ParsePrivateKey(privPEM)
	if err != nil {
		sendWSError(wsConn, "parse private key: "+err.Error())
		return
	}
	pubKeyStr := strings.TrimSpace(string(gossh.MarshalAuthorizedKey(signer.PublicKey())))

	// ── Establish SSH session ─────────────────────────────────────────
	sshClient, cleanup, err := s.connectSSH(tenant, inst, signer, pubKeyStr)
	if err != nil {
		sendWSError(wsConn, "SSH connection failed: "+err.Error())
		return
	}
	defer sshClient.Close()
	if cleanup != nil {
		defer cleanup()
	}

	session, err := sshClient.NewSession()
	if err != nil {
		sendWSError(wsConn, "session open failed: "+err.Error())
		return
	}
	defer session.Close()

	// ── Request PTY ───────────────────────────────────────────────────
	modes := gossh.TerminalModes{
		gossh.ECHO:          1,
		gossh.TTY_OP_ISPEED: 14400,
		gossh.TTY_OP_OSPEED: 14400,
	}
	if err := session.RequestPty("xterm-256color", rows, cols, modes); err != nil {
		sendWSError(wsConn, "PTY request failed: "+err.Error())
		return
	}

	stdinPipe, err := session.StdinPipe()
	if err != nil {
		sendWSError(wsConn, "stdin pipe: "+err.Error())
		return
	}

	stdoutPipe, err := session.StdoutPipe()
	if err != nil {
		sendWSError(wsConn, "stdout pipe: "+err.Error())
		return
	}

	stderrPipe, err := session.StderrPipe()
	if err != nil {
		sendWSError(wsConn, "stderr pipe: "+err.Error())
		return
	}

	// ── Start shell ───────────────────────────────────────────────────
	if err := session.Shell(); err != nil {
		sendWSError(wsConn, "shell start failed: "+err.Error())
		return
	}

	route := "direct"
	if cleanup != nil {
		route = "console-proxy"
	}
	log.Printf("[shell] connected to %s (via=%s user=%s)", inst.Name, route, sshClient.User())
	s.audit(tenantID, "shell:connect", inst.Name+" via="+route, r)

	// Signal ready
	sendWSMessage(wsConn, wsMessage{Type: "ready"})

	// ── Bidirectional I/O bridge ──────────────────────────────────────
	var wg sync.WaitGroup
	done := make(chan struct{})

	// stdout + stderr → WebSocket
	wg.Add(1)
	go func() {
		defer wg.Done()
		merged := io.MultiReader(stdoutPipe, stderrPipe)
		buf := make([]byte, 4096)
		for {
			n, readErr := merged.Read(buf)
			if n > 0 {
				encoded := base64.StdEncoding.EncodeToString(buf[:n])
				msg := wsMessage{Type: "output", Data: encoded}
				if writeErr := wsConn.WriteJSON(msg); writeErr != nil {
					return
				}
			}
			if readErr != nil {
				close(done)
				return
			}
		}
	}()

	// WebSocket → stdin + resize
	wg.Add(1)
	go func() {
		defer wg.Done()
		defer stdinPipe.Close()
		for {
			_, raw, readErr := wsConn.ReadMessage()
			if readErr != nil {
				close(done)
				return
			}
			var msg wsMessage
			if err := json.Unmarshal(raw, &msg); err != nil {
				continue
			}
			switch msg.Type {
			case "input":
				decoded, err := base64.StdEncoding.DecodeString(msg.Data)
				if err != nil {
					continue
				}
				if _, err := stdinPipe.Write(decoded); err != nil {
					return
				}
			case "resize":
				if msg.Rows > 0 && msg.Cols > 0 {
					session.WindowChange(msg.Rows, msg.Cols)
				}
			}
		}
	}()

	// Wait for either side to close
	select {
	case <-done:
	case <-r.Context().Done():
	}

	// Graceful shutdown
	session.Close()
	wg.Wait()

	s.audit(tenantID, "shell:disconnect", inst.Name, r)
}

// connectSSH establishes an SSH connection to the instance.
// Tries direct SSH to PublicIP first, then private IP, then OCI Console Connection proxy.
// Returns a cleanup function (non-nil only for console proxy path) that the caller
// MUST defer/run when done with the client.
func (s *Server) connectSSH(tenant *db.Tenant, inst *db.Instance, signer gossh.Signer, pubKeyStr string) (*gossh.Client, func(), error) {
	config := &gossh.ClientConfig{
		Auth:            []gossh.AuthMethod{gossh.PublicKeys(signer)},
		HostKeyCallback: tofuHostKeyCallback(inst.PublicIP),
		Timeout:         10 * time.Second,
	}

	// ── Strategy 1: Direct SSH to public IP ───────────────────────────
	if inst.PublicIP != "" {
		for _, user := range []string{"opc", "root", "ubuntu"} {
			cfg := *config
			cfg.User = user
			client, dialErr := gossh.Dial("tcp", net.JoinHostPort(inst.PublicIP, "22"), &cfg)
			if dialErr == nil {
				return client, nil, nil
			}
		}
		log.Printf("[shell] direct ssh to %s:22 failed, trying console proxy", inst.PublicIP)
	}

	// ── Strategy 2: Try private IP ────────────────────────────────────
	if inst.PrivateIP != "" && inst.PrivateIP != inst.PublicIP {
		for _, user := range []string{"opc", "root", "ubuntu"} {
			cfg := *config
			cfg.User = user
			client, dialErr := gossh.Dial("tcp", net.JoinHostPort(inst.PrivateIP, "22"), &cfg)
			if dialErr == nil {
				return client, nil, nil
			}
		}
	}

	// ── Strategy 3: OCI Console Connection proxy ──────────────────────
	return s.connectViaConsoleProxy(tenant, inst, config, pubKeyStr)
}

// connectViaConsoleProxy creates an OCI Console Connection and uses it as an SSH tunnel.
// On success, returns the instance SSH client and a cleanup function the caller MUST
// defer/run when done with the client. The cleanup closes the proxy SSH tunnel and
// deletes the OCI console connection.
func (s *Server) connectViaConsoleProxy(tenant *db.Tenant, inst *db.Instance, config *gossh.ClientConfig, pubKeyStr string) (*gossh.Client, func(), error) {
	client, err := s.clientFor(tenant)
	if err != nil {
		return nil, nil, fmt.Errorf("oci client: %w", err)
	}

	// Extract instance OCID from composite ID
	instanceOCID := inst.OCID

	log.Printf("[shell] creating console connection for %s...", inst.Name)
	conn, err := client.CreateConsoleConnection(nil, instanceOCID, pubKeyStr)
	if err != nil {
		return nil, nil, fmt.Errorf("create console connection: %w", err)
	}

	connID := *conn.Id

	// Build a cleanup function that accumulates resources as we acquire them.
	cleanup := func() {
		if delErr := client.DeleteConsoleConnection(nil, connID); delErr != nil {
			log.Printf("[shell] cleanup console connection %s: %v", connID, delErr)
		}
	}

	log.Printf("[shell] waiting for console connection %s to become active...", connID)
	activeConn, err := client.WaitForConsoleConnectionActive(nil, connID)
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("console connection not ready: %w", err)
	}

	connStr := ""
	if activeConn.ConnectionString != nil {
		connStr = *activeConn.ConnectionString
	}
	if connStr == "" {
		cleanup()
		return nil, nil, fmt.Errorf("no SSH connection string in console connection")
	}

	proxyInfo, err := parseConsoleConnectionString(connStr)
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("parse connection string: %w", err)
	}

	log.Printf("[shell] console proxy: %s:%d user=%s", proxyInfo.ProxyHost, proxyInfo.ProxyPort, proxyInfo.ProxyUser)

	// SSH to the OCI console proxy
	proxyConfig := &gossh.ClientConfig{
		User:            proxyInfo.ProxyUser,
		Auth:            config.Auth,
		HostKeyCallback: tofuHostKeyCallback(proxyInfo.ProxyHost),
		Timeout:         15 * time.Second,
	}

	proxyClient, err := gossh.Dial("tcp", net.JoinHostPort(proxyInfo.ProxyHost, strconv.Itoa(proxyInfo.ProxyPort)), proxyConfig)
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("connect to console proxy: %w", err)
	}

	// Extend cleanup to also close the proxy tunnel.
	prevCleanup := cleanup
	cleanup = func() {
		proxyClient.Close()
		prevCleanup()
	}

	// Use direct-tcpip through the proxy to reach the instance's SSH port
	targetAddr := net.JoinHostPort(proxyInfo.TargetHost, strconv.Itoa(proxyInfo.TargetPort))
	proxyConn, err := proxyClient.Dial("tcp", targetAddr)
	if err != nil {
		// Try common alternatives
		for _, alt := range []string{"localhost:22", "127.0.0.1:22"} {
			proxyConn, err = proxyClient.Dial("tcp", alt)
			if err == nil {
				break
			}
		}
		if err != nil {
			cleanup()
			return nil, nil, fmt.Errorf("proxy dial to %s: %w", targetAddr, err)
		}
	}

	// SSH handshake with the instance through the proxied connection
	for _, user := range []string{"opc", "root", "ubuntu"} {
		cfg := *config
		cfg.User = user
		sshConn, chans, reqs, sshErr := gossh.NewClientConn(proxyConn, targetAddr, &cfg)
		if sshErr == nil {
			return gossh.NewClient(sshConn, chans, reqs), cleanup, nil
		}
	}

	proxyConn.Close()
	cleanup()
	return nil, nil, fmt.Errorf("all auth attempts through console proxy failed")
}

// consoleProxyInfo holds parsed OCI console connection proxy details.
type consoleProxyInfo struct {
	ProxyHost  string
	ProxyPort  int
	ProxyUser  string
	TargetHost string
	TargetPort int
}

// parseConsoleConnectionString extracts proxy info from an OCI ConnectionString.
//
// Format: ssh -o ProxyCommand='ssh -W %h:%p -p 443 ocid1.console...@instance-console.region.oci.oraclecloud.com' ocid1.instance...
func parseConsoleConnectionString(s string) (*consoleProxyInfo, error) {
	info := &consoleProxyInfo{
		ProxyPort:  443,
		TargetHost: "localhost",
		TargetPort: 22,
	}

	// Extract proxy user@host:port from ProxyCommand='ssh -W %h:%p -p PORT USER@HOST'
	// Pattern: -p <port> <user>@<host>
	proxyCmdStart := strings.Index(s, "ProxyCommand=")
	if proxyCmdStart == -1 {
		return nil, fmt.Errorf("no ProxyCommand in connection string")
	}

	proxyCmd := s[proxyCmdStart:]

	// Find -p <port>
	portIdx := strings.Index(proxyCmd, "-p ")
	if portIdx != -1 {
		rest := proxyCmd[portIdx+3:]
		end := strings.IndexAny(rest, " '\"")
		if end > 0 {
			if p, err := strconv.Atoi(rest[:end]); err == nil {
				info.ProxyPort = p
			}
		}
	}

	// Find user@host
	atIdx := strings.Index(proxyCmd, "@")
	if atIdx == -1 {
		return nil, fmt.Errorf("no @ in proxy command")
	}

	// Walk back from @ to find start of username
	userStart := atIdx - 1
	for userStart >= 0 && proxyCmd[userStart] != ' ' && proxyCmd[userStart] != '\'' && proxyCmd[userStart] != '"' {
		userStart--
	}
	info.ProxyUser = strings.TrimSpace(proxyCmd[userStart+1 : atIdx])

	// Walk forward from @ to find end of host
	hostEnd := atIdx + 1
	for hostEnd < len(proxyCmd) && proxyCmd[hostEnd] != ' ' && proxyCmd[hostEnd] != '\'' && proxyCmd[hostEnd] != '"' {
		hostEnd++
	}
	info.ProxyHost = strings.TrimSpace(proxyCmd[atIdx+1 : hostEnd])

	if info.ProxyHost == "" || info.ProxyUser == "" {
		return nil, fmt.Errorf("could not parse proxy host/user from: %s", proxyCmd)
	}

	return info, nil
}

// ── WebSocket helpers ────────────────────────────────────────────────────

func sendWSMessage(conn *websocket.Conn, msg wsMessage) {
	if err := conn.WriteJSON(msg); err != nil {
		log.Printf("[shell] ws write error: %v", err)
	}
}

func sendWSError(conn *websocket.Conn, text string) {
	sendWSMessage(conn, wsMessage{Type: "error", Message: text})
}
