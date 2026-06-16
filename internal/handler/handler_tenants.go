package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/viogus/oci-helper-go/internal/db"

)

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

