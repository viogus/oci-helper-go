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
	client, _, ok := s.getTenantClient(tenantID, w)
	if !ok {
		return
	}
	if err := client.DeleteVcn(r.Context(), idStr); err != nil {
		jsonErr(w, "delete vcn: "+err.Error())
		return
	}
	s.audit(tenantID, "vcn:delete", idStr, r)
	jsonOK(w, map[string]string{"status": "ok"})
}
