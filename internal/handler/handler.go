package handler

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/viogus/oci-helper-go/internal/auth"
	"github.com/viogus/oci-helper-go/internal/ai"
	"github.com/viogus/oci-helper-go/internal/cloudflare"
	"github.com/viogus/oci-helper-go/internal/config"
	"github.com/viogus/oci-helper-go/internal/db"
	"github.com/viogus/oci-helper-go/internal/telegram"
	ociclient "github.com/viogus/oci-helper-go/internal/oci"
)

//go:embed all:dist/*
var staticFiles embed.FS

type Server struct {
	cfg    *config.Config
	store  *db.Store
	auth   *auth.Service
	mux    *http.ServeMux
	worker *Worker
}

func New(cfg *config.Config, store *db.Store) *Server {
	s := &Server{
		cfg:    cfg,
		store:  store,
		auth:   auth.New(cfg.Username, cfg.Password, cfg.MFASecret, cfg.MFA),
		mux:    http.NewServeMux(),
		worker: NewWorker(store),
	}
	s.routes()
	go s.worker.Run()
	return s
}

func (s *Server) routes() {
	// API — exact paths
	s.mux.HandleFunc("/api/login", s.handleLogin)
	s.mux.HandleFunc("/api/logout", s.handleLogout)
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

	// security rules
	s.mux.HandleFunc("/api/security-rules", s.withAuth(s.handleSecurityRules))

	// traffic & monitoring
	s.mux.HandleFunc("/api/traffic", s.withAuth(s.handleTraffic))
	s.mux.HandleFunc("/api/limits", s.withAuth(s.handleLimits))
	s.mux.HandleFunc("/api/logs", s.withAuth(s.handleLogs))

	// batch create tasks
	s.mux.HandleFunc("/api/instances/batch-create", s.withAuth(s.handleBatchCreate))
	s.mux.HandleFunc("/api/create-tasks", s.withAuth(s.handleCreateTasks))

	// in-memory tasks
	s.mux.HandleFunc("/api/mem-tasks/change-ip", s.withAuth(s.handleMemTasksChangeIP))
	s.mux.HandleFunc("/api/mem-tasks/update-cfg", s.withAuth(s.handleMemTasksUpdateCfg))

	// ip-info (no auth)
	s.mux.HandleFunc("/api/ip-info", s.handleIPInfo)

	// Wildcard routes (must come after exact paths)
	s.mux.HandleFunc("/api/tenants/", s.withAuth(s.handleTenantByID))
	s.mux.HandleFunc("/api/instances/", s.withAuth(s.handleInstanceAction))
	s.mux.HandleFunc("/api/shell/", s.withAuth(s.handleShell))
	s.mux.HandleFunc("/api/cloudflare/", s.withAuth(s.handleCloudflare))
	s.mux.HandleFunc("/api/public-ips/", s.withAuth(s.handlePublicIPByID))
	s.mux.HandleFunc("/api/boot-volumes/", s.withAuth(s.handleBootVolumeByID))
	s.mux.HandleFunc("/api/keys/", s.withAuth(s.handleKeyByID))
	s.mux.HandleFunc("/api/sync/", s.withAuth(s.handleSync))

	// Static files (frontend)
	staticFS, _ := fs.Sub(staticFiles, "dist")
	s.mux.Handle("/", http.FileServer(http.FS(staticFS)))
}

func (s *Server) Handler() http.Handler { return s.mux }

// --- auth ---

func (s *Server) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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
		MaxAge:   600,
	})
	redirectURL := "https://accounts.google.com/o/oauth2/v2/auth" +
		"?client_id=" + s.cfg.GoogleOAuth.ClientID +
		"&redirect_uri=" + s.cfg.GoogleOAuth.RedirectURL +
		"&response_type=code" +
		"&scope=openid+email+profile" +
		"&state=" + state
	http.Redirect(w, r, redirectURL, http.StatusFound)
}

func (s *Server) handleGoogleCallback(w http.ResponseWriter, r *http.Request) {
	if !s.cfg.GoogleOAuth.Enabled {
		jsonErr(w, "Google OAuth not configured")
		return
	}

	// verify state
	cookie, err := r.Cookie("oauth_state")
	if err != nil || cookie.Value != r.URL.Query().Get("state") {
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
	body := fmt.Sprintf("code=%s&client_id=%s&client_secret=%s&redirect_uri=%s&grant_type=authorization_code",
		code, s.cfg.GoogleOAuth.ClientID, s.cfg.GoogleOAuth.ClientSecret, s.cfg.GoogleOAuth.RedirectURL)

	resp, err := http.Post(tokenURL, "application/x-www-form-urlencoded", strings.NewReader(body))
	if err != nil {
		jsonErr(w, "token exchange: "+err.Error())
		return
	}
	defer resp.Body.Close()

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	json.NewDecoder(resp.Body).Decode(&tokenResp)
	if tokenResp.AccessToken == "" {
		jsonErr(w, "token exchange failed")
		return
	}

	// get user info
	userReq, _ := http.NewRequest("GET", "https://www.googleapis.com/oauth2/v2/userinfo", nil)
	userReq.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)
	userResp, err := http.DefaultClient.Do(userReq)
	if err != nil {
		jsonErr(w, "userinfo: "+err.Error())
		return
	}
	defer userResp.Body.Close()

	var userInfo struct {
		Email string `json:"email"`
	}
	json.NewDecoder(userResp.Body).Decode(&userInfo)

	// set session using signed cookie
	signedValue := s.auth.CreateSession(userInfo.Email)
	http.SetCookie(w, &http.Cookie{
		Name:     "oci_helper_session",
		Value:    signedValue,
		Path:     "/",
		HttpOnly: true,
		Secure:   r.TLS != nil,
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
		mfaEnabled, _ := s.store.GetConfig("mfa_enabled")
		jsonOK(w, map[string]string{
			"username": s.cfg.Username,
			"mfa":      mfaEnabled,
		})
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

func (s *Server) handleTenants(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		keyword := r.URL.Query().Get("keyword")
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		size, _ := strconv.Atoi(r.URL.Query().Get("size"))
		if size < 1 {
			size = 20
		}
		list, total, err := s.store.ListTenantsPaginated(keyword, page, size)
		if err != nil {
			jsonErr(w, "list tenants: "+err.Error())
			return
		}
		if list == nil {
			list = []db.Tenant{}
		}
		jsonOK(w, map[string]interface{}{"data": list, "total": total, "page": page, "size": size})
	case http.MethodPost:
		var t db.Tenant
		if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
			jsonErr(w, "invalid body: "+err.Error())
			return
		}
		if err := s.store.CreateTenant(&t); err != nil {
			jsonErr(w, "create tenant: "+err.Error())
			return
		}
		s.audit(t.ID, "tenant:create", t.Name, r)
		jsonOK(w, t)
	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleTenantByID(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/api/tenants/")
	idStr = strings.TrimSuffix(idStr, "/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		jsonErr(w, "invalid tenant id")
		return
	}

	switch r.Method {
	case http.MethodGet:
		t, _ := s.store.GetTenant(id)
		if t == nil {
			jsonErr(w, "not found")
			return
		}
		jsonOK(w, t)
	case http.MethodDelete:
		s.store.DeleteInstancesByTenant(id)
		s.store.DeleteTenant(id)
		s.audit(id, "tenant:delete", fmt.Sprintf("id=%d", id), r)
		jsonOK(w, map[string]string{"status": "ok"})
	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

// --- instances ---

func (s *Server) handleInstances(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		tenantID, _ := strconv.ParseInt(r.URL.Query().Get("tenant_id"), 10, 64)
		keyword := r.URL.Query().Get("keyword")
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		size, _ := strconv.Atoi(r.URL.Query().Get("size"))
		if size < 1 {
			size = 20
		}
		list, total, err := s.store.ListInstancesPaginated(tenantID, keyword, page, size)
		if err != nil {
			jsonErr(w, "list instances: "+err.Error())
			return
		}
		if list == nil {
			list = []db.Instance{}
		}
		jsonOK(w, map[string]interface{}{"data": list, "total": total, "page": page, "size": size})
	case http.MethodPost:
		s.createInstance(w, r)
	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) createInstance(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TenantID            int64   `json:"tenantId"`
		DisplayName         string  `json:"displayName"`
		ImageID             string  `json:"imageId"`
		Shape               string  `json:"shape"`
		SubnetID            string  `json:"subnetId"`
		AvailabilityDomain  string  `json:"availabilityDomain"`
		BootVolumeSizeGB    *int64  `json:"bootVolumeSizeGB"`
		OCPUs               *float32 `json:"ocpus"`
		MemoryGB            *float32 `json:"memoryGB"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}

	t, err := s.store.GetTenant(req.TenantID)
	if err != nil || t == nil {
		jsonErr(w, "tenant not found")
		return
	}

	client, err := ociclient.NewClient(t)
	if err != nil {
		jsonErr(w, "oci client: "+err.Error())
		return
	}

	launchReq := core.LaunchInstanceRequest{
		LaunchInstanceDetails: core.LaunchInstanceDetails{
			CompartmentId:      common.String(t.TenancyOCID),
			AvailabilityDomain: common.String(req.AvailabilityDomain),
			DisplayName:        common.String(req.DisplayName),
			ImageId:            common.String(req.ImageID),
			Shape:              common.String(req.Shape),
			SubnetId:           common.String(req.SubnetID),
			CreateVnicDetails: &core.CreateVnicDetails{
				SubnetId: common.String(req.SubnetID),
			},
			SourceDetails: core.InstanceSourceViaImageDetails{
				ImageId: common.String(req.ImageID),
			},
		},
	}
	if req.BootVolumeSizeGB != nil {
		launchReq.LaunchInstanceDetails.SourceDetails = core.InstanceSourceViaImageDetails{
			ImageId:           common.String(req.ImageID),
			BootVolumeSizeInGBs: req.BootVolumeSizeGB,
		}
	}
	if req.OCPUs != nil {
		launchReq.LaunchInstanceDetails.ShapeConfig = &core.LaunchInstanceShapeConfigDetails{
			Ocpus: req.OCPUs,
		}
	}
	if req.MemoryGB != nil {
		if launchReq.LaunchInstanceDetails.ShapeConfig == nil {
			launchReq.LaunchInstanceDetails.ShapeConfig = &core.LaunchInstanceShapeConfigDetails{}
		}
		launchReq.LaunchInstanceDetails.ShapeConfig.MemoryInGBs = req.MemoryGB
	}

	inst, err := client.LaunchInstance(r.Context(), launchReq)
	if err != nil {
		jsonErr(w, "launch: "+err.Error())
		return
	}

	s.store.UpsertInstance(&db.Instance{
		ID:       strOr(inst.Id, ""),
		TenantID: req.TenantID,
		Name:     strOr(inst.DisplayName, ""),
		OCID:     strOr(inst.Id, ""),
		Shape:    strOr(inst.Shape, ""),
		State:    string(inst.LifecycleState),
	})
	s.audit(req.TenantID, "instance:create", strOr(inst.DisplayName, ""), r)
	jsonOK(w, map[string]string{"status": "ok", "instanceId": strOr(inst.Id, "")})
}

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
	metrics, err := client.GetMetrics(r.Context(), instanceID)
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
	client, err := ociclient.NewClient(t)
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
	client, t, ok := s.ociClientFromQuery(w, r)
	if !ok {
		return
	}
	vcns, err := client.ListVCNs(r.Context(), t.TenancyOCID)
	if err != nil {
		jsonErr(w, "list vcns: "+err.Error())
		return
	}
	jsonOK(w, vcns)
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

// --- public IPs ---

func (s *Server) handlePublicIPs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		client, t, ok := s.ociClientFromQuery(w, r)
		if !ok {
			return
		}
		ips, err := client.ListPublicIPs(r.Context(), t.TenancyOCID)
		if err != nil {
			jsonErr(w, "list public ips: "+err.Error())
			return
		}
		jsonOK(w, ips)
	case http.MethodPost:
		var req struct {
			TenantID      int64  `json:"tenantId"`
			DisplayName   string `json:"displayName"`
			CompartmentID string `json:"compartmentId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonErr(w, "invalid body: "+err.Error())
			return
		}
		t, err := s.store.GetTenant(req.TenantID)
		if err != nil || t == nil {
			jsonErr(w, "tenant not found")
			return
		}
		client, err := ociclient.NewClient(t)
		if err != nil {
			jsonErr(w, "oci client: "+err.Error())
			return
		}
		lifetime := core.CreatePublicIpDetailsLifetimeReserved
		compartmentID := req.CompartmentID
		if compartmentID == "" {
			compartmentID = t.TenancyOCID
		}
		ip, err := client.CreatePublicIP(r.Context(), core.CreatePublicIpDetails{
			CompartmentId: common.String(compartmentID),
			DisplayName:   common.String(req.DisplayName),
			Lifetime:      lifetime,
		})
		if err != nil {
			jsonErr(w, "create public ip: "+err.Error())
			return
		}
		s.audit(req.TenantID, "publicip:create", strOr(ip.DisplayName, ""), r)
		jsonOK(w, ip)
	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handlePublicIPByID(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/api/public-ips/")
	idStr = strings.TrimSuffix(idStr, "/")

	if r.Method != http.MethodDelete {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	tenantID, _ := strconv.ParseInt(r.URL.Query().Get("tenant_id"), 10, 64)
	t, err := s.store.GetTenant(tenantID)
	if err != nil || t == nil {
		jsonErr(w, "tenant not found")
		return
	}
	client, err := ociclient.NewClient(t)
	if err != nil {
		jsonErr(w, "oci client: "+err.Error())
		return
	}
	if err := client.DeletePublicIP(r.Context(), idStr); err != nil {
		jsonErr(w, "delete public ip: "+err.Error())
		return
	}
	s.audit(tenantID, "publicip:delete", idStr, r)
	jsonOK(w, map[string]string{"status": "ok"})
}

func (s *Server) handleInstanceAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// /api/instances/{ocid}/action
	path := strings.TrimPrefix(r.URL.Path, "/api/instances/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 || parts[1] != "action" {
		jsonErr(w, "invalid path, expected /api/instances/{id}/action")
		return
	}
	instanceID := parts[0]

	var req struct {
		Action   string `json:"action"`
		TenantID int64  `json:"tenantId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}

	t, err := s.store.GetTenant(req.TenantID)
	if err != nil || t == nil {
		jsonErr(w, "tenant not found")
		return
	}

	client, err := ociclient.NewClient(t)
	if err != nil {
		jsonErr(w, "oci client: "+err.Error())
		return
	}

	ctx := r.Context()
	switch req.Action {
	case "terminate":
		if err := client.TerminateInstance(ctx, instanceID); err != nil {
			jsonErr(w, "terminate: "+err.Error())
			return
		}
		s.store.UpsertInstance(&db.Instance{ID: instanceID, TenantID: req.TenantID, State: "TERMINATING"})
	case "start":
		_, err := client.InstanceAction(ctx, instanceID, core.InstanceActionActionStart)
		if err != nil {
			jsonErr(w, "start: "+err.Error())
			return
		}
		s.store.UpsertInstance(&db.Instance{ID: instanceID, TenantID: req.TenantID, State: "STARTING"})
	case "stop":
		_, err := client.InstanceAction(ctx, instanceID, core.InstanceActionActionStop)
		if err != nil {
			jsonErr(w, "stop: "+err.Error())
			return
		}
		s.store.UpsertInstance(&db.Instance{ID: instanceID, TenantID: req.TenantID, State: "STOPPING"})
	case "reboot":
		_, err := client.InstanceAction(ctx, instanceID, core.InstanceActionActionReset)
		if err != nil {
			jsonErr(w, "reboot: "+err.Error())
			return
		}
		s.store.UpsertInstance(&db.Instance{ID: instanceID, TenantID: req.TenantID, State: "STARTING"})
	case "softstop":
		_, err := client.InstanceAction(ctx, instanceID, core.InstanceActionActionSoftstop)
		if err != nil {
			jsonErr(w, "softstop: "+err.Error())
			return
		}
		s.store.UpsertInstance(&db.Instance{ID: instanceID, TenantID: req.TenantID, State: "STOPPING"})
	case "softreset":
		_, err := client.InstanceAction(ctx, instanceID, core.InstanceActionActionSoftreset)
		if err != nil {
			jsonErr(w, "softreset: "+err.Error())
			return
		}
		s.store.UpsertInstance(&db.Instance{ID: instanceID, TenantID: req.TenantID, State: "STARTING"})
	default:
		jsonErr(w, "unknown action: "+req.Action+". use start|stop|reboot|softstop|softreset|terminate")
		return
	}

	s.audit(req.TenantID, "instance:"+req.Action, instanceID, r)
	jsonOK(w, map[string]string{"status": "ok", "action": req.Action, "instanceId": instanceID})
}

// --- batch start ---

func (s *Server) handleBatchStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		TenantID    int64    `json:"tenantId"`
		InstanceIDs []string `json:"instanceIds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}
	if req.TenantID == 0 || len(req.InstanceIDs) == 0 {
		jsonErr(w, "tenantId and instanceIds required")
		return
	}
	payload, _ := json.Marshal(req)
	task := &db.Task{
		TenantID: req.TenantID,
		Type:     "batch_start",
		Status:   "pending",
		Payload:  string(payload),
	}
	if err := s.store.CreateTask(task); err != nil {
		jsonErr(w, "create task: "+err.Error())
		return
	}
	s.audit(req.TenantID, "batch:start", fmt.Sprintf("%d instances", len(req.InstanceIDs)), r)
	jsonOK(w, task)
}

// --- tasks ---

func (s *Server) handleTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	keyword := r.URL.Query().Get("keyword")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if size < 1 {
		size = 20
	}
	list, total, err := s.store.ListTasksPaginated(keyword, page, size)
	if err != nil {
		jsonErr(w, "list tasks: "+err.Error())
		return
	}
	if list == nil {
		list = []db.Task{}
	}
	jsonOK(w, map[string]interface{}{"data": list, "total": total, "page": page, "size": size})
}

// --- audit ---

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	list, _ := s.store.ListAudit(100)
	if list == nil {
		list = []db.AuditLog{}
	}
	jsonOK(w, list)
}

// --- boot volumes ---

func (s *Server) handleBootVolumes(w http.ResponseWriter, r *http.Request) {
	client, t, ok := s.ociClientFromQuery(w, r)
	if !ok {
		return
	}
	vols, err := client.ListBootVolumes(r.Context(), t.TenancyOCID)
	if err != nil {
		jsonErr(w, "list boot volumes: "+err.Error())
		return
	}
	jsonOK(w, vols)
}

func (s *Server) handleBootVolumeByID(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/boot-volumes/")
	path = strings.TrimSuffix(path, "/")
	parts := strings.SplitN(path, "/", 2)
	bootVolumeID := parts[0]
	action := ""
	if len(parts) == 2 {
		action = parts[1]
	}

	if r.Method != http.MethodPost {
		// GET boot volume details
		client, _, ok := s.ociClientFromQuery(w, r)
		if !ok {
			return
		}
		vol, err := client.GetBootVolume(r.Context(), bootVolumeID)
		if err != nil {
			jsonErr(w, "get boot volume: "+err.Error())
			return
		}
		jsonOK(w, vol)
		return
	}

	var req struct {
		TenantID   int64  `json:"tenantId"`
		SizeInGBs  int64  `json:"sizeInGBs"`
		InstanceID string `json:"instanceId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}

	t, err := s.store.GetTenant(req.TenantID)
	if err != nil || t == nil {
		jsonErr(w, "tenant not found")
		return
	}
	client, err := ociclient.NewClient(t)
	if err != nil {
		jsonErr(w, "oci client: "+err.Error())
		return
	}

	switch action {
	case "resize":
		if req.SizeInGBs <= 0 {
			jsonErr(w, "sizeInGBs required")
			return
		}
		vol, err := client.UpdateBootVolume(r.Context(), bootVolumeID, req.SizeInGBs, "")
		if err != nil {
			jsonErr(w, "resize: "+err.Error())
			return
		}
		s.audit(req.TenantID, "bootvolume:resize", fmt.Sprintf("%s → %dGB", bootVolumeID, req.SizeInGBs), r)
		jsonOK(w, vol)
	case "attach":
		if req.InstanceID == "" {
			jsonErr(w, "instanceId required")
			return
		}
		att, err := client.AttachBootVolume(r.Context(), bootVolumeID, req.InstanceID)
		if err != nil {
			jsonErr(w, "attach: "+err.Error())
			return
		}
		s.audit(req.TenantID, "bootvolume:attach", fmt.Sprintf("%s → %s", bootVolumeID, req.InstanceID), r)
		jsonOK(w, att)
	case "detach":
		// find attachment ID
		attachments, err := client.ListBootVolumeAttachments(r.Context(), t.TenancyOCID, "")
		if err != nil {
			jsonErr(w, "list attachments: "+err.Error())
			return
		}
		var attachmentID string
		for _, a := range attachments {
			if strOr(a.BootVolumeId, "") == bootVolumeID {
				attachmentID = strOr(a.Id, "")
				break
			}
		}
		if attachmentID == "" {
			jsonErr(w, "no attachment found for boot volume")
			return
		}
		if err := client.DetachBootVolume(r.Context(), attachmentID); err != nil {
			jsonErr(w, "detach: "+err.Error())
			return
		}
		s.audit(req.TenantID, "bootvolume:detach", bootVolumeID, r)
		jsonOK(w, map[string]string{"status": "ok"})
	default:
		jsonErr(w, "unknown action: "+action+". use resize|attach|detach")
	}
}

// --- sync ---

func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	tenantIDStr := strings.TrimPrefix(r.URL.Path, "/api/sync/")
	tenantID, _ := strconv.ParseInt(tenantIDStr, 10, 64)

	t, err := s.store.GetTenant(tenantID)
	if err != nil || t == nil {
		jsonErr(w, "tenant not found")
		return
	}
	client, err := ociclient.NewClient(t)
	if err != nil {
		jsonErr(w, "oci client: "+err.Error())
		return
	}
	instances, err := client.ListInstances(r.Context(), t.TenancyOCID)
	if err != nil {
		jsonErr(w, "list instances: "+err.Error())
		return
	}
	for _, inst := range instances {
		if err := s.store.UpsertInstance(ociToDB(inst, tenantID)); err != nil {
			log.Printf("[sync] upsert %s: %v", strOr(inst.Id, ""), err)
		}
	}
	s.audit(tenantID, "sync", fmt.Sprintf("synced %d instances", len(instances)), r)
	jsonOK(w, map[string]int{"count": len(instances)})
}

func ociToDB(i core.Instance, tenantID int64) *db.Instance {
	return &db.Instance{
		ID:       strOr(i.Id, ""),
		TenantID: tenantID,
		Name:     strOr(i.DisplayName, ""),
		OCID:     strOr(i.Id, ""),
		Shape:    strOr(i.Shape, ""),
		State:    string(i.LifecycleState),
	}
}

func strOr(p *string, fallback string) string {
	if p != nil {
		return *p
	}
	return fallback
}

// --- ai ---

func (s *Server) handleAIChat(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
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

	client := ai.New(apiKey, "")
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

	client, err := ociclient.NewClient(t)
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

func (s *Server) handleTelegramWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	token, _ := s.store.GetConfig("telegram_token")
	if token == "" {
		jsonErr(w, "telegram_token not configured")
		return
	}

	var update telegram.Update
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		jsonErr(w, "invalid body")
		return
	}

	// ignore non-message updates (callback_query, channel_post, etc.)
	if update.Message.MessageID == 0 {
		jsonOK(w, map[string]string{"status": "ignored"})
		return
	}

	bot := telegram.New(token)
	text := update.Message.Text
	chatID := update.Message.Chat.ID

	var reply string
	switch {
	case text == "/start":
		reply = "oci-helper Telegram Bot\n/instances - List all instances\n/status - General status"
	case text == "/instances":
		instances, _ := s.store.ListInstances(0)
		infos := make([]telegram.InstanceInfo, 0, len(instances))
		for _, i := range instances {
			infos = append(infos, telegram.InstanceInfo{
				Name: i.Name, State: i.State, Shape: i.Shape,
				PublicIP: i.PublicIP, OCPU: i.OCPU, MemoryGB: i.MemoryGB,
			})
		}
		reply = telegram.FormatInstances(infos)
	case text == "/status":
		tenants, _ := s.store.ListTenants()
		instances, _ := s.store.ListInstances(0)
		reply = fmt.Sprintf("Tenants: %d\nInstances: %d", len(tenants), len(instances))
	default:
		reply = "Unknown command. /start for help."
	}

	if err := bot.SendMessage(chatID, reply); err != nil {
		log.Printf("[telegram] send: %v", err)
		jsonErr(w, "send failed")
		return
	}
	jsonOK(w, map[string]string{"status": "ok"})
}

// --- cloudflare ---

func (s *Server) handleCloudflare(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/cloudflare/")
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(path, "/")

	token, _ := s.store.GetConfig("cloudflare_token")
	if token == "" {
		jsonErr(w, "cloudflare_token not configured")
		return
	}
	cf := cloudflare.New(token)

	switch {
	case path == "zones" && r.Method == http.MethodGet:
		zones, err := cf.ListZones()
		if err != nil {
			jsonErr(w, "list zones: "+err.Error())
			return
		}
		jsonOK(w, zones)

	case len(parts) == 2 && parts[1] == "records" && r.Method == http.MethodGet:
		zoneID := parts[0]
		records, err := cf.ListDNSRecords(zoneID)
		if err != nil {
			jsonErr(w, "list records: "+err.Error())
			return
		}
		jsonOK(w, records)

	case len(parts) == 2 && parts[1] == "records" && r.Method == http.MethodPost:
		zoneID := parts[0]
		var record cloudflare.DNSRecord
		if err := json.NewDecoder(r.Body).Decode(&record); err != nil {
			jsonErr(w, "invalid body: "+err.Error())
			return
		}
		created, err := cf.CreateDNSRecord(zoneID, record)
		if err != nil {
			jsonErr(w, "create record: "+err.Error())
			return
		}
		s.audit(0, "cloudflare:record:create", record.Name, r)
		jsonOK(w, created)

	case len(parts) == 3 && parts[1] == "records" && r.Method == http.MethodPut:
		zoneID, recordID := parts[0], parts[2]
		var record cloudflare.DNSRecord
		if err := json.NewDecoder(r.Body).Decode(&record); err != nil {
			jsonErr(w, "invalid body: "+err.Error())
			return
		}
		updated, err := cf.UpdateDNSRecord(zoneID, recordID, record)
		if err != nil {
			jsonErr(w, "update record: "+err.Error())
			return
		}
		s.audit(0, "cloudflare:record:update", record.Name, r)
		jsonOK(w, updated)

	case len(parts) == 3 && parts[1] == "records" && r.Method == http.MethodDelete:
		zoneID, recordID := parts[0], parts[2]
		if err := cf.DeleteDNSRecord(zoneID, recordID); err != nil {
			jsonErr(w, "delete record: "+err.Error())
			return
		}
		s.audit(0, "cloudflare:record:delete", recordID, r)
		jsonOK(w, map[string]string{"status": "ok"})

	case path == "update-ip" && r.Method == http.MethodPost:
		var req struct {
			ZoneID string `json:"zoneId"`
			Name   string `json:"name"`
			NewIP  string `json:"newIp"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonErr(w, "invalid body: "+err.Error())
			return
		}
		if err := cf.UpdateDNSRecordIP(req.ZoneID, req.Name, req.NewIP); err != nil {
			jsonErr(w, "update ip: "+err.Error())
			return
		}
		s.audit(0, "cloudflare:ip:update", req.Name+" → "+req.NewIP, r)
		jsonOK(w, map[string]string{"status": "ok"})

	default:
		jsonErr(w, "unknown cloudflare endpoint")
	}
}

// --- helpers ---

func (s *Server) audit(tenantID int64, action, detail string, r *http.Request) {
	ip := r.RemoteAddr
	// Only trust X-Forwarded-For from localhost or private network
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" && isTrustedProxy(r.RemoteAddr) {
		ip = strings.Split(fwd, ",")[0]
	}
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

// --- key file management ---

func (s *Server) handleKeys(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		entries, err := os.ReadDir(s.cfg.KeysDir)
		if err != nil {
			jsonErr(w, "read keys dir: "+err.Error())
			return
		}
		var keys []map[string]interface{}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".pem") {
				continue
			}
			info, _ := e.Info()
			keys = append(keys, map[string]interface{}{
				"name": e.Name(),
				"size": info.Size(),
				"time": info.ModTime().Format("2006-01-02 15:04"),
			})
		}
		if keys == nil {
			keys = []map[string]interface{}{}
		}
		jsonOK(w, keys)

	case http.MethodPost:
		if err := r.ParseMultipartForm(32 << 20); err != nil {
			jsonErr(w, "parse multipart: "+err.Error())
			return
		}
		files := r.MultipartForm.File["files"]
		if len(files) == 0 {
			jsonErr(w, "no files uploaded (field name: files)")
			return
		}
		var saved []string
		for _, fh := range files {
			if !strings.HasSuffix(strings.ToLower(fh.Filename), ".pem") {
				jsonErr(w, "only .pem files allowed: "+fh.Filename)
				return
			}
			// sanitize: base name only
			name := filepath.Base(fh.Filename)
			dst := filepath.Join(s.cfg.KeysDir, name)
			src, err := fh.Open()
			if err != nil {
				jsonErr(w, "open upload: "+err.Error())
				return
			}
			out, err := os.Create(dst)
			if err != nil {
				src.Close()
				jsonErr(w, "create file: "+err.Error())
				return
			}
			if _, err := io.Copy(out, src); err != nil {
				out.Close()
				src.Close()
				jsonErr(w, "write file: "+err.Error())
				return
			}
			out.Close()
			src.Close()
			saved = append(saved, name)
		}
		s.audit(0, "keys:upload", strings.Join(saved, ","), r)
		jsonOK(w, map[string]interface{}{"saved": saved})

	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleKeyByID(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(r.URL.Path, "/api/keys/")
	name = strings.TrimSuffix(name, "/")
	name = filepath.Base(name)
	if name == "" || name == "." {
		jsonErr(w, "invalid key name")
		return
	}
	switch r.Method {
	case http.MethodDelete:
		path := filepath.Join(s.cfg.KeysDir, name)
		if err := os.Remove(path); err != nil {
			jsonErr(w, "delete key: "+err.Error())
			return
		}
		s.audit(0, "keys:delete", name, r)
		jsonOK(w, map[string]string{"status": "ok"})
	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

// --- Phase 1 stubs (implemented in later phases) ---

func (s *Server) handleChangeShape(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		TenantID   int64   `json:"tenant_id"`
		InstanceID string  `json:"instance_id"`
		Shape      string  `json:"shape"`
		Ocpus      float32 `json:"ocpus"`
		MemoryGB   float32 `json:"memory_gb"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}
	tenant, err := s.store.GetTenant(req.TenantID)
	if err != nil || tenant == nil {
		jsonErr(w, "tenant not found")
		return
	}
	client, err := ociclient.NewClient(tenant)
	if err != nil {
		jsonErr(w, "oci client: "+err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	if err := client.UpdateInstance(ctx, req.InstanceID, req.Shape, req.Ocpus, req.MemoryGB); err != nil {
		jsonErr(w, "update instance: "+err.Error())
		return
	}
	s.audit(req.TenantID, "instance:change-shape", req.InstanceID, r)
	jsonOK(w, map[string]string{"status": "ok"})
}
func (s *Server) handleChangeBootVolume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		TenantID   int64  `json:"tenant_id"`
		InstanceID string `json:"instance_id"`
		SizeGB     int64  `json:"size_gb"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}
	tenant, err := s.store.GetTenant(req.TenantID)
	if err != nil || tenant == nil {
		jsonErr(w, "tenant not found")
		return
	}
	client, err := ociclient.NewClient(tenant)
	if err != nil {
		jsonErr(w, "oci client: "+err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	attachment, err := client.GetBootVolumeAttachment(ctx, tenant.TenancyOCID, req.InstanceID)
	if err != nil {
		jsonErr(w, "get boot volume: "+err.Error())
		return
	}
	if _, err := client.UpdateBootVolume(ctx, *attachment.BootVolumeId, req.SizeGB, ""); err != nil {
		jsonErr(w, "update boot volume: "+err.Error())
		return
	}
	s.audit(req.TenantID, "instance:change-boot-volume", req.InstanceID, r)
	jsonOK(w, map[string]string{"status": "ok"})
}
func (s *Server) handleAttachIPv6(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		TenantID   int64  `json:"tenant_id"`
		InstanceID string `json:"instance_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}
	tenant, err := s.store.GetTenant(req.TenantID)
	if err != nil || tenant == nil {
		jsonErr(w, "tenant not found")
		return
	}
	client, err := ociclient.NewClient(tenant)
	if err != nil {
		jsonErr(w, "oci client: "+err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	vnics, err := client.GetInstanceVNICs(ctx, tenant.TenancyOCID, req.InstanceID)
	if err != nil {
		jsonErr(w, "list vnics: "+err.Error())
		return
	}
	if len(vnics) == 0 {
		jsonErr(w, "no VNIC found for instance")
		return
	}
	if err := client.AssignIPv6(ctx, *vnics[0].Id); err != nil {
		jsonErr(w, "assign ipv6: "+err.Error())
		return
	}
	s.audit(req.TenantID, "instance:attach-ipv6", req.InstanceID, r)
	jsonOK(w, map[string]string{"status": "ok"})
}
func (s *Server) handleUpdateInstanceName(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		TenantID   int64  `json:"tenant_id"`
		InstanceID string `json:"instance_id"`
		Name       string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}
	tenant, err := s.store.GetTenant(req.TenantID)
	if err != nil || tenant == nil {
		jsonErr(w, "tenant not found")
		return
	}
	client, err := ociclient.NewClient(tenant)
	if err != nil {
		jsonErr(w, "oci client: "+err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	if err := client.UpdateInstanceDisplayName(ctx, req.InstanceID, req.Name); err != nil {
		jsonErr(w, "update name: "+err.Error())
		return
	}
	s.audit(req.TenantID, "instance:update-name", req.InstanceID+" -> "+req.Name, r)
	jsonOK(w, map[string]string{"status": "ok"})
}
func (s *Server) handleChangeIP(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"status": "not implemented"})
}
func (s *Server) handleCheckAlive(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"status": "not implemented"})
}
func (s *Server) handleOneClick500M(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"status": "not implemented"})
}
func (s *Server) handleOneClickClose500M(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"status": "not implemented"})
}
func (s *Server) handleAutoRescue(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"status": "not implemented"})
}
func (s *Server) handleUpdateShape(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		TenantID   int64  `json:"tenant_id"`
		InstanceID string `json:"instance_id"`
		Shape      string `json:"shape"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}
	tenant, err := s.store.GetTenant(req.TenantID)
	if err != nil || tenant == nil {
		jsonErr(w, "tenant not found")
		return
	}
	client, err := ociclient.NewClient(tenant)
	if err != nil {
		jsonErr(w, "oci client: "+err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	if err := client.UpdateInstance(ctx, req.InstanceID, req.Shape, 0, 0); err != nil {
		jsonErr(w, "update shape: "+err.Error())
		return
	}
	s.audit(req.TenantID, "instance:update-shape", req.InstanceID, r)
	jsonOK(w, map[string]string{"status": "ok"})
}
func (s *Server) handleSecurityRules(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"status": "not implemented"})
}
func (s *Server) handleTraffic(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"status": "not implemented"})
}
func (s *Server) handleLimits(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"status": "not implemented"})
}
func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"status": "not implemented"})
}
func (s *Server) handleBatchCreate(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"status": "not implemented"})
}
func (s *Server) handleCreateTasks(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"status": "not implemented"})
}
func (s *Server) handleMemTasksChangeIP(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"status": "not implemented"})
}
func (s *Server) handleMemTasksUpdateCfg(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"status": "not implemented"})
}
func (s *Server) handleIPInfo(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"ip": r.RemoteAddr})
}

func jsonOK(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func jsonErr(w http.ResponseWriter, msg string) {
	log.Printf("ERROR: %s", msg)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

