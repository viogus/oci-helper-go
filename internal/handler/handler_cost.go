package handler

import (
	"net/http"
	"strconv"
)

// handleCost returns monthly OCI cost summary grouped by service.
//
// GET /api/cost?tenant_id=X&start=YYYY-MM-DD&end=YYYY-MM-DD
func (s *Server) handleCost(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	tenantID, _ := strconv.ParseInt(r.URL.Query().Get("tenant_id"), 10, 64)
	if tenantID == 0 {
		jsonErr(w, "tenant_id required")
		return
	}

	tenant, err := s.store.GetTenant(tenantID)
	if err != nil || tenant == nil {
		jsonErr(w, "tenant not found")
		return
	}

	client, err := s.clientFor(tenant)
	if err != nil {
		jsonErr(w, "oci client: "+err.Error())
		return
	}

	startDate := r.URL.Query().Get("start")
	endDate := r.URL.Query().Get("end")
	if startDate == "" || endDate == "" {
		jsonErr(w, "start and end required (YYYY-MM-DD)")
		return
	}

	items, err := client.CostSummary(r.Context(), startDate, endDate)
	if err != nil {
		jsonErr(w, "cost query: "+err.Error())
		return
	}
	if items == nil {
		jsonOK(w, map[string]interface{}{
			"data":     []interface{}{},
			"start":    startDate,
			"end":      endDate,
			"tenantId": tenantID,
		})
		return
	}

	jsonOK(w, map[string]interface{}{
		"data":     items,
		"start":    startDate,
		"end":      endDate,
		"tenantId": tenantID,
	})
}
