package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/viogus/oci-helper-go/internal/oci"
)

// costAnalysisRequest is the POST body for /api/cost/analysis.
type costAnalysisRequest struct {
	TenantID    int64  `json:"tenant_id"`
	ReportType  string `json:"report_type"`
	StartDate   string `json:"start_date"`   // yyyy-MM-dd
	EndDate     string `json:"end_date"`     // yyyy-MM-dd
	Granularity string `json:"granularity"`  // DAILY or MONTHLY
	QueryType   string `json:"query_type"`   // COST or USAGE
}

// handleCostAnalysis handles POST /api/cost/analysis.
func (s *Server) handleCostAnalysis(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var req costAnalysisRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid request body: "+err.Error())
		return
	}

	if req.TenantID == 0 {
		jsonErr(w, "tenant_id required")
		return
	}
	if req.StartDate == "" || req.EndDate == "" {
		jsonErr(w, "start_date and end_date required (yyyy-MM-dd)")
		return
	}

	client, _, ok := s.getTenantClient(req.TenantID, w)
	if !ok {
		return
	}

	// Defaults.
	if req.ReportType == "" {
		req.ReportType = "MONTHLY_COST"
	}
	if req.Granularity == "" {
		req.Granularity = "MONTHLY"
	}
	if req.QueryType == "" {
		req.QueryType = "COST"
	}

	result, err := client.CostAnalysis(r.Context(), oci.CostAnalysisParams{
		StartDate:   req.StartDate,
		EndDate:     req.EndDate,
		Granularity: req.Granularity,
		QueryType:   req.QueryType,
		ReportType:  req.ReportType,
	})
	if err != nil {
		jsonErr(w, "cost query: "+err.Error())
		return
	}

	jsonOK(w, map[string]interface{}{
		"total":     result.Total,
		"totalCost": result.TotalCost,
		"currency":  result.Currency,
		"items":     result.Items,
		"startDate": req.StartDate,
		"endDate":   req.EndDate,
		"tenantId":  req.TenantID,
	})
}

// handleCost is the legacy GET /api/cost endpoint kept for backward compatibility.
// Prefer POST /api/cost/analysis for new clients.
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

	client, _, ok := s.getTenantClient(tenantID, w)
	if !ok {
		return
	}

	startDate := r.URL.Query().Get("start")
	endDate := r.URL.Query().Get("end")
	if startDate == "" || endDate == "" {
		jsonErr(w, "start and end required (YYYY-MM-DD)")
		return
	}

	result, err := client.CostAnalysis(r.Context(), oci.CostAnalysisParams{
		StartDate:   startDate,
		EndDate:     endDate,
		Granularity: "MONTHLY",
		QueryType:   "COST",
		ReportType:  "MONTHLY_COST",
	})
	if err != nil {
		jsonErr(w, "cost query: "+err.Error())
		return
	}
	if result == nil || len(result.Items) == 0 {
		jsonOK(w, map[string]interface{}{
			"data":     []interface{}{},
			"start":    startDate,
			"end":      endDate,
			"tenantId": tenantID,
		})
		return
	}

	jsonOK(w, map[string]interface{}{
		"data":     result.Items,
		"start":    startDate,
		"end":      endDate,
		"tenantId": tenantID,
	})
}
