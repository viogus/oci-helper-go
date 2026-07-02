// Package handler implements the HTTP API and SPA frontend server for oci-helper.
//
package handler

import (
	"crypto/subtle"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/viogus/oci-helper-go/internal/auth"
	"github.com/viogus/oci-helper-go/internal/ai"
	"github.com/viogus/oci-helper-go/internal/config"
	"github.com/viogus/oci-helper-go/internal/db"
	ociclient "github.com/viogus/oci-helper-go/internal/oci"
)

//go:embed all:dist/*
var staticFiles embed.FS

// version is the current application version, settable via -ldflags at build time.
// Defaults to "dev" for local development.
var version = "dev"

type Server struct {
	cfg      *config.Config
	store    *db.Store
	auth     *auth.Service
	mux      *http.ServeMux
	worker   *Worker
	ratelimit *loginRateLimiter
	startTime time.Time
}

func New(cfg *config.Config, store *db.Store) *Server {
	s := &Server{
		cfg:       cfg,
		store:     store,
		auth:      auth.New(cfg.Username, cfg.Password, cfg.MFASecret, cfg.MFA, cfg.SecureCookies),
		mux:       http.NewServeMux(),
		worker:    NewWorker(store, cfg.KeysDir),
		ratelimit: newLoginRateLimiter(),
		startTime: time.Now(),
	}
	s.routes()
	go s.worker.Run()
	return s
}

// clientForTenant resolves the tenant's key file path (may be relative filename)
// to an absolute path under keysDir before creating the OCI client.
// Shared by Server.clientFor and Worker.newClient.
func clientForTenant(t *db.Tenant, keysDir string) (*ociclient.Client, error) {
	keyPath := t.KeyFile
	if keyPath != "" && !filepath.IsAbs(keyPath) {
		keyPath = filepath.Join(keysDir, keyPath)
	}
	// Prevent path traversal: resolved path must stay within keysDir.
	if keyPath != "" {
		cleanKey := filepath.Clean(keyPath)
		cleanDir := filepath.Clean(keysDir)
		if !strings.HasPrefix(cleanKey, cleanDir+string(os.PathSeparator)) {
			return nil, fmt.Errorf("key file path escapes keys directory: %s", t.KeyFile)
		}
		if _, err := os.Stat(keyPath); err != nil {
			log.Printf("[clientFor] key file stat error: %v", err)
		}
	}

	resolved := *t
	resolved.KeyFile = keyPath
	return ociclient.NewClient(&resolved)
}

// clientFor delegates to clientForTenant with the server's configured KeysDir.
func (s *Server) clientFor(t *db.Tenant) (*ociclient.Client, error) {
	return clientForTenant(t, s.cfg.KeysDir)
}

// getTenantClient fetches the tenant and creates an OCI client for it.
// Writes an error response and returns (nil, nil, false) on failure.
// Caller must return immediately when ok is false.
func (s *Server) getTenantClient(tenantID int64, w http.ResponseWriter) (*ociclient.Client, *db.Tenant, bool) {
	tenant, err := s.store.GetTenant(tenantID)
	if err != nil || tenant == nil {
		jsonErr(w, "tenant not found")
		return nil, nil, false
	}
	client, err := s.clientFor(tenant)
	if err != nil {
		jsonErr(w, "oci client: "+err.Error())
		return nil, nil, false
	}
	return client, tenant, true
}

// clientForInstance gets the OCI client for the tenant, then sets its region
// to match the instance's actual region from the DB. Falls back to tenant's
// default region if the instance has no region stored or can't be found.
func (s *Server) clientForInstance(tenantID int64, instanceID string, w http.ResponseWriter) (*ociclient.Client, *db.Tenant, bool) {
	client, tenant, ok := s.getTenantClient(tenantID, w)
	if !ok {
		return nil, nil, false
	}
	if inst, err := s.store.GetInstanceByID(instanceID); err == nil && inst != nil && inst.Region != "" {
		client.SetRegion(inst.Region)
	}
	return client, tenant, true
}

func (s *Server) routes() {
	// API — exact paths
	s.mux.HandleFunc("/api/login", s.handleLogin)
	s.mux.HandleFunc("/api/logout", s.withAuth(s.handleLogout))
	s.mux.HandleFunc("/api/config", s.withAuth(s.handleConfig))
	s.mux.HandleFunc("/api/oauth/google/login", s.handleGoogleLogin)
	s.mux.HandleFunc("/api/oauth/google/callback", s.handleGoogleCallback)
	s.mux.HandleFunc("/api/mfa/setup", s.withAuth(s.handleMFASetup))
	s.mux.HandleFunc("/api/mfa/verify", s.withAuth(s.handleMFAVerify))
	s.mux.HandleFunc("/api/mfa/disable", s.withAuth(s.handleMFADisable))
	s.mux.HandleFunc("/api/tenants", s.withAuth(s.handleTenants))
	s.mux.HandleFunc("/api/instances", s.withAuth(s.handleInstances))
	s.mux.HandleFunc("/api/tasks", s.withAuth(s.handleTasks))
	s.mux.HandleFunc("/api/audit", s.withAuth(s.handleAudit))
	s.mux.HandleFunc("/api/ai/chat", s.withAuth(s.handleAIChat))
	s.mux.HandleFunc("/api/telegram/webhook", s.handleTelegramWebhook)
	s.mux.HandleFunc("/api/backup", s.withAuth(s.handleBackup))
	s.mux.HandleFunc("/api/restore", s.withAuth(s.handleRestore))
	s.mux.HandleFunc("/api/public-ips", s.withAuth(s.handlePublicIPs))
	s.mux.HandleFunc("/api/images", s.withAuth(s.handleListImages))
	s.mux.HandleFunc("/api/shapes", s.withAuth(s.handleListShapes))
	s.mux.HandleFunc("/api/vcns", s.withAuth(s.handleListVCNs))
	s.mux.HandleFunc("/api/subnets", s.withAuth(s.handleListSubnets))
	s.mux.HandleFunc("/api/availability-domains", s.withAuth(s.handleListADs))
	s.mux.HandleFunc("/api/instances/batch-start", s.withAuth(s.handleBatchStart))
	s.mux.HandleFunc("/api/metrics", s.withAuth(s.handleMetrics))
	s.mux.HandleFunc("/api/boot-volumes", s.withAuth(s.handleBootVolumes))
	s.mux.HandleFunc("/api/keys", s.withAuth(s.handleKeys))
	s.mux.HandleFunc("/api/dingtalk/notify", s.withAuth(s.handleDingTalkNotify))
	s.mux.HandleFunc("/api/dingtalk/test", s.withAuth(s.handleDingTalkTest))
	s.mux.HandleFunc("/api/update/check", s.withAuth(s.handleUpdateCheck))
	s.mux.HandleFunc("/api/update/now", s.withAuth(s.handleUpdateNow))
	s.mux.HandleFunc("/api/notify/test", s.withAuth(s.handleNotifyTest))
	// SSH keys
	s.mux.HandleFunc("/api/ssh/keys", s.withAuth(s.handleSSHKeys))
	// Users
	s.mux.HandleFunc("/api/users", s.withAuth(s.handleUsers))
	// Instance VNC & config
	s.mux.HandleFunc("/api/instances/vnc", s.withAuth(s.handleStartVNC))
	s.mux.HandleFunc("/api/instances/vnc/stop", s.withAuth(s.handleStopVNC))
	s.mux.HandleFunc("/api/instances/vnc/wait", s.withAuth(s.handleConsoleWait))
	s.mux.HandleFunc("/api/instances/config-info", s.withAuth(s.handleInstanceConfigInfo))
	s.mux.HandleFunc("/api/instances/update-password", s.withAuth(s.handleUpdatePassword))
	s.mux.HandleFunc("/api/shell/ws", s.withAuth(s.handleShellWS))
	s.mux.HandleFunc("/api/cost/analysis", s.withAuth(s.handleCostAnalysis))
	s.mux.HandleFunc("/api/cost", s.withAuth(s.handleCost))

	// NEW exact-path routes
	// instance mutations
	s.mux.HandleFunc("/api/instances/change-shape", s.withAuth(s.handleChangeShape))
	s.mux.HandleFunc("/api/instances/change-boot-volume", s.withAuth(s.handleChangeBootVolume))
	s.mux.HandleFunc("/api/instances/attach-ipv6", s.withAuth(s.handleAttachIPv6))
	s.mux.HandleFunc("/api/instances/update-name", s.withAuth(s.handleUpdateInstanceName))
	s.mux.HandleFunc("/api/instances/change-ip", s.withAuth(s.handleChangeIP))
	s.mux.HandleFunc("/api/instances/check-alive", s.withAuth(s.handleCheckAlive))
	s.mux.HandleFunc("/api/instances/one-click-500m", s.withAuth(s.handleOneClick500M))
	s.mux.HandleFunc("/api/instances/one-click-close-500m", s.withAuth(s.handleOneClickClose500M))
	s.mux.HandleFunc("/api/instances/auto-rescue", s.withAuth(s.handleAutoRescue))
	s.mux.HandleFunc("/api/instances/update-shape", s.withAuth(s.handleUpdateShape))
	// G6: config-update
	s.mux.HandleFunc("/api/instances/config-update", s.withAuth(s.handleInstanceConfigUpdate))
	// G10: batch check alive
	s.mux.HandleFunc("/api/instances/check-alive-batch", s.withAuth(s.handleCheckAliveBatch))

	// dashboard glance
	s.mux.HandleFunc("/api/glance", s.withAuth(s.handleGlance))

	s.mux.HandleFunc("/api/security-rules", s.withAuth(s.handleSecurityRules))
	// G7: security rule batch release
	s.mux.HandleFunc("/api/security-rules/release", s.withAuth(s.handleSecurityRuleRelease))

	// traffic & monitoring
	s.mux.HandleFunc("/api/traffic/getCondition", s.withAuth(s.handleTrafficCondition))
	s.mux.HandleFunc("/api/traffic/fetchVnics", s.withAuth(s.handleTrafficVnics))
	s.mux.HandleFunc("/api/traffic/fetchInstances", s.withAuth(s.handleTrafficInstances))
	s.mux.HandleFunc("/api/traffic", s.withAuth(s.handleTraffic))
	s.mux.HandleFunc("/api/limits/services", s.withAuth(s.handleLimitsServices))
	s.mux.HandleFunc("/api/limits", s.withAuth(s.handleLimits))
	s.mux.HandleFunc("/api/logs", s.withAuth(s.handleLogs))
	s.mux.HandleFunc("/api/logs/ws", s.withAuth(s.handleLogWS))

	// batch create tasks
	s.mux.HandleFunc("/api/instances/batch-create", s.withAuth(s.handleBatchCreate))
	s.mux.HandleFunc("/api/instance-plans", s.withAuth(s.handleInstancePlans))
	s.mux.HandleFunc("/api/ip-data", s.withAuth(s.handleIpData))
	s.mux.HandleFunc("/api/defense/enable", s.withAuth(s.handleDefenseEnable))
	s.mux.HandleFunc("/api/defense/disable", s.withAuth(s.handleDefenseDisable))
	s.mux.HandleFunc("/api/ip-blacklist", s.withAuth(s.handleIPBlacklist))
	s.mux.HandleFunc("/api/create-tasks", s.withAuth(s.handleCreateTasks))
	s.mux.HandleFunc("/api/create-tasks/", s.withAuth(s.handleCreateTasks))

	// in-memory tasks
	s.mux.HandleFunc("/api/mem-tasks/change-ip", s.withAuth(s.handleMemTasksChangeIP))
	s.mux.HandleFunc("/api/mem-tasks/update-cfg", s.withAuth(s.handleMemTasksUpdateCfg))

	// ip-info (no auth)
	s.mux.HandleFunc("/api/ip-info", s.handleIPInfo)

	// G9: tenant upload (BEFORE wildcard /api/tenants/)
	s.mux.HandleFunc("/api/tenants/upload", s.withAuth(s.handleTenantUpload))

	// G11: captcha send
	s.mux.HandleFunc("/api/captcha/send", s.withAuth(s.handleCaptchaSend))

	// G14: AI chat cache clear (BEFORE wildcards)
	s.mux.HandleFunc("/api/ai/chat/cache", s.withAuth(s.handleAIChatCacheClear))

	// Wildcard routes (must come after exact paths)
	s.mux.HandleFunc("/api/tenants/", s.withAuth(s.handleTenantByID))
	s.mux.HandleFunc("/api/instances/", s.withAuth(s.handleInstanceAction))
	s.mux.HandleFunc("/api/shell/", s.withAuth(s.handleShell))
	s.mux.HandleFunc("/api/cloudflare/", s.withAuth(s.handleCloudflare))
	s.mux.HandleFunc("/api/cloudflare/cfgs", s.withAuth(s.handleCloudflareCfgs))
	s.mux.HandleFunc("/api/cloudflare/cfgs/", s.withAuth(s.handleCloudflareCfgByID))
	s.mux.HandleFunc("/api/cloudflare/oci-sync", s.withAuth(s.handleCloudflareOCISync))
	s.mux.HandleFunc("/api/public-ips/", s.withAuth(s.handlePublicIPByID))
	s.mux.HandleFunc("/api/boot-volumes/", s.withAuth(s.handleBootVolumeByID))
	s.mux.HandleFunc("/api/keys/", s.withAuth(s.handleKeyByID))
	s.mux.HandleFunc("/api/vcns/", s.withAuth(s.handleVCNByID))
	s.mux.HandleFunc("/api/ssh/keys/", s.withAuth(s.handleSSHKeyByID))
	s.mux.HandleFunc("/api/instance-plans/", s.withAuth(s.handleInstancePlanByID))
	s.mux.HandleFunc("/api/ip-data/", s.withAuth(s.handleIpDataByID))
	s.mux.HandleFunc("/api/users/", s.withAuth(s.handleUserByID))
	s.mux.HandleFunc("/api/sync/", s.withAuth(s.handleSync))


		// Static files (frontend) with SPA fallback.
		// Client-side routes (/ssh-keys, /settings, etc.) get index.html
		// so the SPA router can handle them.
		staticFS, err := fs.Sub(staticFiles, "dist")
		if err != nil {
			log.Fatalf("embedded dist directory not found: %v", err)
		}
		fileFS := http.FS(staticFS)
		s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			// API paths that fell through — genuine 404.
			if strings.HasPrefix(r.URL.Path, "/api/") {
				http.NotFound(w, r)
				return
			}
			// Try to open the file; if it doesn't exist in the embedded FS,
			// serve index.html for SPA client-side routing.
			path := strings.TrimPrefix(r.URL.Path, "/")
			if path == "" {
				path = "index.html"
			}
			f, err := staticFS.Open(path)
			if err != nil {
				r.URL.Path = "/"
			} else {
				f.Close()
			}
			http.FileServer(fileFS).ServeHTTP(w, r)
		})
}

func (s *Server) Handler() http.Handler { return s.mux }

// --- auth ---

func (s *Server) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Limit request body to 10 MB to prevent memory exhaustion.
		r.Body = http.MaxBytesReader(w, r.Body, 10<<20)
		// Set baseline security headers.
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		if !s.auth.Authenticate(w, r) {
			return
		}
		next(w, r)
	}
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		jsonErr(w, "method not allowed")
		return
	}
	// Check rate limit before proceeding
	ip := extractIP(r)
	if !s.ratelimit.allow(ip) {
		log.Printf("[login] rate limit exceeded for %s", maskIP(ip))
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	// check MFA BEFORE setting session — prevents MFA status leak
	mfaEnabled, mfaErr := s.store.GetConfig("mfa_enabled")
	if mfaErr != nil {
		log.Printf("[login] config read error: %v", mfaErr)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	if mfaEnabled == "true" {
		totp := r.Header.Get("X-TOTP")
		secret, err := s.store.GetConfig("mfa_secret")
		if err != nil {
			log.Printf("[login] secret read error: %v", err)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		if totp == "" || !auth.ValidateTOTP(secret, totp) {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}
	if !s.auth.Login(w, r) {
		return
	}
	s.ratelimit.reset(ip)
	jsonOK(w, map[string]string{"status": "ok"})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	s.auth.Logout(w)
	jsonOK(w, map[string]string{"status": "ok"})
}

// --- google oauth ---

func (s *Server) handleGoogleLogin(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.GoogleOAuth.Enabled {
		jsonErr(w, "Google OAuth not configured")
		return
	}
	state := auth.GenerateMFA()[:32]
	http.SetCookie(w, &http.Cookie{
		Name:     "oauth_state",
		Value:    state,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cfg.SecureCookies && (r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   600,
	})
	// NOTE: redirect_uri must match the URI registered in Google Cloud Console
	redirectURL := "https://accounts.google.com/o/oauth2/v2/auth" +
		"?client_id=" + url.QueryEscape(s.cfg.GoogleOAuth.ClientID) +
		"&redirect_uri=" + url.QueryEscape(s.cfg.GoogleOAuth.RedirectURL) +
		"&response_type=code" +
		"&scope=openid+email+profile" +
		"&state=" + url.QueryEscape(state)
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

func (s *Server) handleGoogleCallback(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.GoogleOAuth.Enabled {
		jsonErr(w, "Google OAuth not configured")
		return
	}

	// verify state using timing-safe comparison
	cookie, err := r.Cookie("oauth_state")
	if err != nil || subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(r.URL.Query().Get("state"))) != 1 {
		jsonErr(w, "invalid state")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "oauth_state", Value: "", Path: "/", MaxAge: -1})

	code := r.URL.Query().Get("code")
	if code == "" {
		jsonErr(w, "missing code")
		return
	}

	// exchange code for token
	tokenURL := "https://oauth2.googleapis.com/token"
	v := url.Values{}
	v.Set("code", code)
	v.Set("client_id", s.cfg.GoogleOAuth.ClientID)
	v.Set("client_secret", s.cfg.GoogleOAuth.ClientSecret)
	v.Set("redirect_uri", s.cfg.GoogleOAuth.RedirectURL)
	v.Set("grant_type", "authorization_code")

	httpClient := &http.Client{Timeout: 15 * time.Second}
	resp, err := httpClient.Post(tokenURL, "application/x-www-form-urlencoded", strings.NewReader(v.Encode()))
	if err != nil {
		jsonErr(w, "token exchange: "+err.Error())
		return
	}
	defer resp.Body.Close()

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		jsonErr(w, "token decode: "+err.Error())
		return
	}
	if tokenResp.AccessToken == "" {
		jsonErr(w, "token exchange failed")
		return
	}

	// get user info
	userReq, err := http.NewRequest("GET", "https://www.googleapis.com/oauth2/v2/userinfo", nil)
	if err != nil {
		jsonErr(w, "userinfo request: "+err.Error())
		return
	}
	userReq.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)
	userResp, err := httpClient.Do(userReq)
	if err != nil {
		jsonErr(w, "userinfo: "+err.Error())
		return
	}
	defer userResp.Body.Close()

	var userInfo struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(userResp.Body).Decode(&userInfo); err != nil {
		jsonErr(w, "userinfo decode: "+err.Error())
		return
	}

	// set session using signed cookie
	// Look up user role from DB (default to "user" for new accounts).
	role := "user"
	if u, err := s.store.GetUserByUsername(userInfo.Email); err == nil && u != nil {
		role = u.Role
	}
	signedValue := s.auth.CreateSession(userInfo.Email, role)
	http.SetCookie(w, &http.Cookie{
		Name:     "oci_helper_session",
		Value:    signedValue,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.cfg.SecureCookies && (r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400,
	})
	s.audit(0, "oauth:google", userInfo.Email, r)
	http.Redirect(w, r, "/", http.StatusFound)
}

// --- config ---

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// Return all config keys the frontend Settings page expects.
		// Sensitive keys are masked — only first 2 + last 2 chars shown.
		keys := []string{
			"mfa_enabled", "telegram_token", "dingtalk_webhook",
			"google_client_id", "google_client_secret",
			"cloudflare_token", "siliconflow_key",
		}
		secretKeys := map[string]bool{
			"telegram_token": true, "cloudflare_token": true,
			"siliconflow_key": true, "google_client_secret": true,
		}
		out := map[string]string{"username": s.cfg.Username}
		for _, k := range keys {
			v, _ := s.store.GetConfig(k)
			if secretKeys[k] && len(v) > 8 {
				out[k] = v[:2] + "***" + v[len(v)-2:]
			} else {
				out[k] = v
			}
		}
		jsonOK(w, out)
	case http.MethodPost:
		var req struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonErr(w, "invalid body: "+err.Error())
			return
		}
		if req.Key == "" {
			jsonErr(w, "key required")
			return
		}
		if err := s.store.SetConfig(req.Key, req.Value); err != nil {
			jsonErr(w, "set config: "+err.Error())
			return
		}
		s.audit(0, "config:set", req.Key, r)
		jsonOK(w, map[string]string{"status": "ok"})
	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

// --- mfa ---

func (s *Server) handleMFASetup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	secret := auth.GenerateMFA()
	s.store.SetConfig("mfa_secret", secret)
	s.store.SetConfig("mfa_enabled", "false")
	uri := auth.TOTPURI(secret, s.cfg.Username, "oci-helper")
	s.audit(0, "mfa:setup", "generated new secret", r)
	jsonOK(w, map[string]string{"secret": secret, "uri": uri})
}

func (s *Server) handleMFAVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}
	secret, _ := s.store.GetConfig("mfa_secret")
	if secret == "" {
		jsonErr(w, "MFA not set up, call /api/mfa/setup first")
		return
	}
	if !auth.ValidateTOTP(secret, req.Code) {
		jsonErr(w, "invalid code")
		return
	}
	s.store.SetConfig("mfa_enabled", "true")
	s.audit(0, "mfa:enabled", "", r)
	jsonOK(w, map[string]string{"status": "ok"})
}

func (s *Server) handleMFADisable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}
	secret, _ := s.store.GetConfig("mfa_secret")
	if secret == "" || !auth.ValidateTOTP(secret, req.Code) {
		jsonErr(w, "valid TOTP code required to disable MFA")
		return
	}
	s.store.SetConfig("mfa_enabled", "false")
	s.audit(0, "mfa:disabled", "", r)
	jsonOK(w, map[string]string{"status": "ok"})
}

// --- tenants ---

// --- metrics ---

func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	client, _, ok := s.ociClientFromQuery(w, r)
	if !ok {
		return
	}
	instanceID := r.URL.Query().Get("instance_id")
	if instanceID == "" {
		jsonErr(w, "instance_id required")
		return
	}

	// Look up instance region from DB and set it on the client.
	if instDB, err := s.store.GetInstanceByID(instanceID); err == nil && instDB != nil && instDB.Region != "" {
		client.SetRegion(instDB.Region)
	}

	// Strip composite ID prefix (tenantID:ocid)
	if i := strings.IndexByte(instanceID, ':'); i >= 0 {
		instanceID = instanceID[i+1:]
	}
	inst, err := client.GetInstance(r.Context(), instanceID)
	if err != nil {
		jsonErr(w, "get instance: "+err.Error())
		return
	}
	metrics, err := client.GetMetrics(r.Context(), *inst.CompartmentId, instanceID)
	if err != nil {
		jsonErr(w, "metrics: "+err.Error())
		return
	}
	jsonOK(w, metrics)
}

// --- reference data ---

func (s *Server) ociClientFromQuery(w http.ResponseWriter, r *http.Request) (*ociclient.Client, *db.Tenant, bool) {
	tenantID, _ := strconv.ParseInt(r.URL.Query().Get("tenant_id"), 10, 64)
	t, err := s.store.GetTenant(tenantID)
	if err != nil || t == nil {
		jsonErr(w, "tenant not found")
		return nil, nil, false
	}
	client, err := s.clientFor(t)
	if err != nil {
		jsonErr(w, "oci client: "+err.Error())
		return nil, nil, false
	}
	return client, t, true
}

func (s *Server) handleListImages(w http.ResponseWriter, r *http.Request) {
	client, t, ok := s.ociClientFromQuery(w, r)
	if !ok {
		return
	}
	osFilter := r.URL.Query().Get("os")
	if osFilter == "" {
		osFilter = "Oracle Linux"
	}
	images, err := client.ListImages(r.Context(), t.TenancyOCID, osFilter)
	if err != nil {
		jsonErr(w, "list images: "+err.Error())
		return
	}
	jsonOK(w, images)
}

func (s *Server) handleListShapes(w http.ResponseWriter, r *http.Request) {
	client, t, ok := s.ociClientFromQuery(w, r)
	if !ok {
		return
	}
	imageID := r.URL.Query().Get("image_id")
	if imageID == "" {
		jsonErr(w, "image_id required")
		return
	}
	shapes, err := client.ListShapes(r.Context(), t.TenancyOCID, imageID)
	if err != nil {
		jsonErr(w, "list shapes: "+err.Error())
		return
	}
	jsonOK(w, shapes)
}

func (s *Server) handleListVCNs(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodDelete {
		return // handled by handleVCNByID wildcard route
	}
	client, t, ok := s.ociClientFromQuery(w, r)
	if !ok {
		return
	}
	vcns, err := client.ListVCNs(r.Context(), t.TenancyOCID)
	if err != nil {
		jsonErr(w, "list vcns: "+err.Error())
		return
	}

	// In-memory pagination
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if size < 1 {
		size = 20
	}
	start := (page - 1) * size
	end := start + size
	if start > len(vcns) {
		start = len(vcns)
	}
	if end > len(vcns) {
		end = len(vcns)
	}

	jsonOK(w, map[string]interface{}{
		"data":  vcns[start:end],
		"total": len(vcns),
		"page":  page,
		"size":  size,
	})
}

func (s *Server) handleListSubnets(w http.ResponseWriter, r *http.Request) {
	client, t, ok := s.ociClientFromQuery(w, r)
	if !ok {
		return
	}
	vcnID := r.URL.Query().Get("vcn_id")
	if vcnID == "" {
		jsonErr(w, "vcn_id required")
		return
	}
	subnets, err := client.ListSubnets(r.Context(), t.TenancyOCID, vcnID)
	if err != nil {
		jsonErr(w, "list subnets: "+err.Error())
		return
	}
	jsonOK(w, subnets)
}

func (s *Server) handleListADs(w http.ResponseWriter, r *http.Request) {
	client, t, ok := s.ociClientFromQuery(w, r)
	if !ok {
		return
	}
	ads, err := client.ListAvailabilityDomains(r.Context(), t.TenancyOCID)
	if err != nil {
		jsonErr(w, "list ads: "+err.Error())
		return
	}
	jsonOK(w, ads)
}

// --- ai ---

func (s *Server) handleAIChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	stream := r.URL.Query().Get("stream") == "true" || r.Header.Get("Accept") == "text/event-stream"

	apiKey, _ := s.store.GetConfig("siliconflow_key")
	if apiKey == "" {
		jsonErr(w, "siliconflow_key not configured")
		return
	}

	var req struct {
		Messages []ai.ChatMessage `json:"messages"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}

	model, _ := s.store.GetConfig("siliconflow_model")
	client := ai.New(apiKey, model)

	// Optionally search DuckDuckGo for context
	searchEnabled, _ := s.store.GetConfig("ai_search_enabled")
	if searchEnabled == "true" && len(req.Messages) > 0 {
		lastMsg := req.Messages[len(req.Messages)-1].Content
		if searchResults, err := ai.Search(lastMsg); err == nil && len(searchResults) > 0 {
			req.Messages = append([]ai.ChatMessage{
				{Role: "system", Content: "Search results for additional context:\n" + searchResults},
			}, req.Messages...)
		}
	}

	if stream {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher, ok := w.(http.Flusher)
		if !ok {
			jsonErr(w, "streaming not supported")
			return
		}
		ch, err := client.ChatStream(r.Context(), req.Messages)
		if err != nil {
			fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
			flusher.Flush()
			return
		}
		for token := range ch {
			fmt.Fprintf(w, "data: %s\n\n", token)
			flusher.Flush()
		}
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
		return
	}

	resp, err := client.Chat(req.Messages)
	if err != nil {
		jsonErr(w, "ai: "+err.Error())
		return
	}
	jsonOK(w, map[string]string{"reply": resp})
}

// --- shell ---

func (s *Server) handleShell(w http.ResponseWriter, r *http.Request) {
	instanceID := strings.TrimPrefix(r.URL.Path, "/api/shell/")
	instanceID = strings.TrimSuffix(instanceID, "/")

	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	tenantID, _ := strconv.ParseInt(r.URL.Query().Get("tenant_id"), 10, 64)
	t, err := s.store.GetTenant(tenantID)
	if err != nil || t == nil {
		jsonErr(w, "tenant not found")
		return
	}

	client, err := s.clientFor(t)
	if err != nil {
		jsonErr(w, "oci client: "+err.Error())
		return
	}

	// get instance to verify it exists
	inst, err := client.GetInstance(r.Context(), instanceID)
	if err != nil {
		jsonErr(w, "get instance: "+err.Error())
		return
	}

	jsonOK(w, map[string]interface{}{
		"instanceId":   instanceID,
		"instanceName": strOr(inst.DisplayName, ""),
		"state":        string(inst.LifecycleState),
		"message":      "Instance console access. Use OCI Console Connections API for interactive SSH/terminal.",
	})
}

// --- telegram ---

func (s *Server) audit(tenantID int64, action, detail string, r *http.Request) {
	ip := extractIP(r)
	s.store.AddAudit(&db.AuditLog{
		TenantID: tenantID,
		Action:   action,
		Detail:   detail,
		IP:       strings.TrimSpace(ip),
	})
}

func isTrustedProxy(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate()
}

func maskIP(s string) string {
	ip := net.ParseIP(s)
	if ip == nil {
		return s
	}
	if ip.To4() != nil {
		v4 := ip.To4()
		return fmt.Sprintf("%d.%d.%d.***", v4[0], v4[1], v4[2])
	}
	// IPv6: mask the last 80 bits (last 5 hex groups)
	s6 := ip.String()
	parts := strings.Split(s6, ":")
	if len(parts) >= 3 {
		parts[len(parts)-1] = "****"
		parts[len(parts)-2] = "****"
	}
	return strings.Join(parts, ":")
}

// --- key file management ---

// --- Phase 1 stubs (implemented in later phases) ---


func strOr(p *string, fallback string) string {
	if p != nil { return *p }
	return fallback
}

func checkTCPPort(ip string, port int, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(ip, strconv.Itoa(port)), timeout)
	if err != nil { return false }
	conn.Close()
	return true
}

func jsonOK(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func jsonErr(w http.ResponseWriter, msg string) {
	jsonErrStatus(w, msg, http.StatusBadRequest)
}

func jsonErrStatus(w http.ResponseWriter, msg string, status int) {
	log.Printf("ERROR: %s", msg)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// ── G14: AI Chat Cache Clear ────────────────────────────────────────────

// conversationCache holds in-memory AI conversation history, keyed by session.
var (
	conversationCache   = make(map[string][]ai.ChatMessage)
	conversationCacheMu sync.Mutex
)

func (s *Server) handleAIChatCacheClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	sessionID := r.URL.Query().Get("session_id")
	conversationCacheMu.Lock()
	var cleared int
	if sessionID != "" {
		if _, ok := conversationCache[sessionID]; ok {
			delete(conversationCache, sessionID)
			cleared = 1
		}
	} else {
		cleared = len(conversationCache)
		conversationCache = make(map[string][]ai.ChatMessage)
	}
	conversationCacheMu.Unlock()
	s.audit(0, "ai:cache-clear", fmt.Sprintf("cleared %d conversations (session=%s)", cleared, sessionID), r)
	jsonOK(w, map[string]interface{}{"status": "ok", "cleared": cleared})
}
