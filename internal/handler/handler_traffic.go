package handler

import (
	"encoding/json"
	"net/http"
	"time"
)

func (s *Server) handleTraffic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		TenantID   int64  `json:"tenant_id"`
		InstanceID string `json:"instance_id"`
		VnicID     string `json:"vnic_id"`
		StartTime  string `json:"start_time"`
		EndTime    string `json:"end_time"`
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
	client, err := s.clientFor(tenant)
	if err != nil {
		jsonErr(w, "oci client: "+err.Error())
		return
	}

	// parse time range
	startTime, err := time.Parse(time.RFC3339, req.StartTime)
	if err != nil {
		jsonErr(w, "invalid start_time: "+err.Error())
		return
	}
	endTime, err := time.Parse(time.RFC3339, req.EndTime)
	if err != nil {
		jsonErr(w, "invalid end_time: "+err.Error())
		return
	}

	// if no VNIC specified, get the first VNIC of the instance
	vnicID := req.VnicID
	if vnicID == "" {
		vnics, err := client.GetInstanceVNICs(r.Context(), tenant.TenancyOCID, req.InstanceID)
		if err != nil || len(vnics) == 0 {
			jsonErr(w, "no VNIC found for instance")
			return
		}
		vnicID = *vnics[0].Id
	}

	data, err := client.GetVNICTtraffic(r.Context(), vnicID, startTime, endTime)
	if err != nil {
		jsonErr(w, "get traffic: "+err.Error())
		return
	}
	jsonOK(w, map[string]interface{}{"data": data, "vnic_id": vnicID})
}
