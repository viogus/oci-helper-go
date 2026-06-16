package handler

import (
	"net/http"
	"strconv"
	"strings"
)

func (s *Server) handleVCNByID(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/api/vcns/")
	idStr = strings.TrimSuffix(idStr, "/")
	if idStr == "" {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

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
	client, err := s.clientFor(t)
	if err != nil {
		jsonErr(w, "oci client: "+err.Error())
		return
	}
	if err := client.DeleteVcn(r.Context(), idStr); err != nil {
		jsonErr(w, "delete vcn: "+err.Error())
		return
	}
	s.audit(tenantID, "vcn:delete", idStr, r)
	jsonOK(w, map[string]string{"status": "ok"})
}
