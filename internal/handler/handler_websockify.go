package handler

import (
	"context"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var vncUpgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 32768,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true // same-origin requests, curl, wscat
		}
		if u, err := url.Parse(origin); err == nil && u.Host != "" {
			origin = u.Host
		}
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

// handleVNCProxy creates a WebSocket tunnel to an OCI VNC console connection.
// It creates a new console connection for the instance, waits for it to become
// ACTIVE, then proxies raw TCP between the WebSocket client and the VNC endpoint.
//
// GET /api/instances/vnc/proxy?tenant_id=X&instance_id=Y&ssh_key_id=Z
func (s *Server) handleVNCProxy(w http.ResponseWriter, r *http.Request) {
	tenantID, _ := strconv.ParseInt(r.URL.Query().Get("tenant_id"), 10, 64)
	instanceID := r.URL.Query().Get("instance_id")
	sshKeyID, _ := strconv.ParseInt(r.URL.Query().Get("ssh_key_id"), 10, 64)

	if tenantID == 0 || instanceID == "" {
		jsonErr(w, "tenant_id and instance_id are required")
		return
	}

	// Get OCI client with region set to the instance's actual region
	client, _, ok := s.clientForInstance(tenantID, instanceID, w)
	if !ok {
		return
	}

	// Get SSH public key — OCI CreateConsoleConnection requires a public key
	// even for VNC connections, though it is not used for VNC auth.
	sshKeys, err := s.store.ListSSHKeys(tenantID)
	if err != nil || len(sshKeys) == 0 {
		jsonErr(w, "no SSH keys found — upload or generate one first")
		return
	}
	var pubKey string
	if sshKeyID > 0 {
		for _, k := range sshKeys {
			if k.ID == sshKeyID {
				pubKey = k.PublicKey
				break
			}
		}
	} else {
		pubKey = sshKeys[0].PublicKey
	}
	if pubKey == "" {
		jsonErr(w, "SSH key not found")
		return
	}

	// Strip composite ID prefix (tenantID:ocid) to get bare OCID
	instID := instanceID
	if idx := strings.IndexByte(instID, ':'); idx >= 0 {
		instID = instID[idx+1:]
	}

	// Step 1: Create OCI instance console connection
	conn, err := client.CreateConsoleConnection(r.Context(), instID, pubKey)
	if err != nil {
		jsonErr(w, "create console connection: "+err.Error())
		return
	}
	consoleID := *conn.Id

	// Always clean up the console connection when this handler exits.
	// Uses a background context so cleanup still works if the request context
	// has been cancelled. Duplicate deletes are harmless — OCI returns 404.
	defer func() {
		if delErr := client.DeleteConsoleConnection(context.Background(), consoleID); delErr != nil {
			log.Printf("[vnc-proxy] cleanup console %s: %v", consoleID, delErr)
		}
	}()

	// Step 2: Poll until the console connection becomes ACTIVE
	log.Printf("[vnc-proxy] waiting for console connection %s to become active...", consoleID)
	activeConn, err := client.WaitForConsoleConnectionActive(r.Context(), consoleID)
	if err != nil {
		jsonErr(w, "console connection not ready: "+err.Error())
		return
	}

	vncConn := ""
	if activeConn.VncConnectionString != nil {
		vncConn = *activeConn.VncConnectionString
	}
	if vncConn == "" {
		jsonErr(w, "no VNC connection string in console connection")
		return
	}

	log.Printf("[vnc-proxy] VNC connection string: %s", vncConn)

	// Step 3: Parse the VNC URL to extract host and port
	host, port := parseVNCURL(vncConn)

	// Step 4: Establish a raw TCP connection to the VNC endpoint
	vncAddr := net.JoinHostPort(host, port)
	vncTCP, err := net.DialTimeout("tcp", vncAddr, 10*time.Second)
	if err != nil {
		jsonErr(w, "connect to VNC "+vncAddr+": "+err.Error())
		return
	}

	// Step 5: Upgrade the HTTP connection to a WebSocket
	wsConn, err := vncUpgrader.Upgrade(w, r, nil)
	if err != nil {
		vncTCP.Close()
		log.Printf("[vnc-proxy] ws upgrade error: %v", err)
		return
	}

	// Clear deadlines on the underlying connection for long-lived VNC session
	if nc, ok := wsConn.UnderlyingConn().(interface{ SetDeadline(time.Time) error }); ok {
		nc.SetDeadline(time.Time{})
	}
	// 1 MB max read frame — VNC framebuffer updates can be large
	wsConn.SetReadLimit(1 << 20)

	log.Printf("[vnc-proxy] connected instance=%s vnc=%s", instID, vncAddr)
	s.audit(tenantID, "vnc:proxy:connect", instanceID, r)

	// Step 6: Bidirectional copy between WebSocket and VNC TCP
	var wg sync.WaitGroup
	done := make(chan struct{})
	var closeDone sync.Once

	// VNC TCP → WebSocket (read raw bytes, send as binary frames)
	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, 32*1024)
		for {
			n, readErr := vncTCP.Read(buf)
			if n > 0 {
				if writeErr := wsConn.WriteMessage(websocket.BinaryMessage, buf[:n]); writeErr != nil {
					return
				}
			}
			if readErr != nil {
				closeDone.Do(func() { close(done) })
				return
			}
		}
	}()

	// WebSocket → VNC TCP (receive binary frames, write raw bytes)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			_, data, readErr := wsConn.ReadMessage()
			if readErr != nil {
				closeDone.Do(func() { close(done) })
				return
			}
			if _, writeErr := vncTCP.Write(data); writeErr != nil {
				return
			}
		}
	}()

	// Block until either side closes or the request context is cancelled
	select {
	case <-done:
	case <-r.Context().Done():
	}

	// Graceful shutdown — close WebSocket first to unblock ReadMessage,
	// then close TCP, then wait for goroutines to finish.
	wsConn.Close()
	vncTCP.Close()
	wg.Wait()

	s.audit(tenantID, "vnc:proxy:disconnect", instanceID, r)
}

// parseVNCURL extracts host and port from a VNC connection string.
//
// Supported formats:
//   - vnc://host:port
//   - vnc://host        (defaults to port 5900)
//   - vnc://[::1]:5900  (IPv6 bracket notation)
func parseVNCURL(vncURL string) (host string, port string) {
	s := strings.TrimPrefix(vncURL, "vnc://")

	// IPv6 bracket notation: [::1]:5900
	if strings.HasPrefix(s, "[") {
		if idx := strings.LastIndex(s, "]:"); idx >= 0 {
			return s[1:idx], s[idx+2:]
		}
		return strings.TrimSuffix(s[1:], "]"), "5900"
	}

	// host:port or host
	if idx := strings.LastIndex(s, ":"); idx >= 0 {
		return s[:idx], s[idx+1:]
	}
	return s, "5900"
}
