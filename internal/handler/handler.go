package handler

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/viogus/oci-helper-go/internal/auth"
	"github.com/viogus/oci-helper-go/internal/config"
	"github.com/viogus/oci-helper-go/internal/db"
	ociclient "github.com/viogus/oci-helper-go/internal/oci"
)

//go:embed all:dist/*
var staticFiles embed.FS

type Server struct {
	cfg   *config.Config
	store *db.Store
	auth  *auth.Service
	mux   *http.ServeMux
	// ociClients maps tenant ID to OCI client (not used in cmd yet)
}

func New(cfg *config.Config, store *db.Store) *Server {
	s := &Server{
		cfg:   cfg,
		store: store,
		auth:  auth.New(cfg.Username, cfg.Password, cfg.MFASecret, cfg.MFA),
		mux:   http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	// API
	s.mux.HandleFunc("/api/login", s.handleLogin)
	s.mux.HandleFunc("/api/logout", s.handleLogout)
	s.mux.HandleFunc("/api/config", s.withAuth(s.handleConfig))
	s.mux.HandleFunc("/api/tenants", s.withAuth(s.handleTenants))
	s.mux.HandleFunc("/api/tenants/", s.withAuth(s.handleTenantByID))
	s.mux.HandleFunc("/api/instances", s.withAuth(s.handleInstances))
	s.mux.HandleFunc("/api/tasks", s.withAuth(s.handleTasks))
	s.mux.HandleFunc("/api/audit", s.withAuth(s.handleAudit))
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
	if !s.auth.Login(w, r) {
		return
	}
	jsonOK(w, map[string]string{"status": "ok"})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	s.auth.Logout(w)
	jsonOK(w, map[string]string{"status": "ok"})
}

// --- config ---

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		jsonOK(w, map[string]string{
			"username": s.cfg.Username,
			"mfa":      strconv.FormatBool(s.cfg.MFA),
		})
	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

// --- tenants ---

func (s *Server) handleTenants(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		list, _ := s.store.ListTenants()
		if list == nil {
			list = []db.Tenant{}
		}
		jsonOK(w, list)
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
	id, _ := strconv.ParseInt(idStr, 10, 64)

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
	tenantID, _ := strconv.ParseInt(r.URL.Query().Get("tenant_id"), 10, 64)
	list, _ := s.store.ListInstances(tenantID)
	if list == nil {
		list = []db.Instance{}
	}
	jsonOK(w, list)
}

// --- tasks ---

func (s *Server) handleTasks(w http.ResponseWriter, r *http.Request) {
	list, _ := s.store.ListTasks()
	if list == nil {
		list = []db.Task{}
	}
	jsonOK(w, list)
}

// --- audit ---

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	list, _ := s.store.ListAudit(100)
	if list == nil {
		list = []db.AuditLog{}
	}
	jsonOK(w, list)
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
		s.store.UpsertInstance(ociToDB(inst, tenantID))
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

// --- helpers ---

func (s *Server) audit(tenantID int64, action, detail string, r *http.Request) {
	ip := r.RemoteAddr
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		ip = strings.Split(fwd, ",")[0]
	}
	s.store.AddAudit(&db.AuditLog{
		TenantID: tenantID,
		Action:   action,
		Detail:   detail,
		IP:       strings.TrimSpace(ip),
	})
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

func init() {
	// ensure keys dir exists
	if err := os.MkdirAll(filepath.Join("/app", "oci-helper", "keys"), 0700); err != nil {
		log.Printf("warn: cannot create keys dir: %v", err)
	}
}
