package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/viogus/oci-helper-go/internal/db"
)

func (s *Server) handleDefenseEnable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		TenantID  int64    `json:"tenant_id"`
		VcnID     string   `json:"vcn_id"`
		Blacklist []string `json:"blacklist"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}
	if req.TenantID == 0 || req.VcnID == "" || len(req.Blacklist) == 0 {
		jsonErr(w, "tenant_id, vcn_id, and blacklist required")
		return
	}

	client, tenant, ok := s.getTenantClient(req.TenantID, w)
	if !ok {
		return
	}
	vcn := client.VcnClient()

	slReq := core.ListSecurityListsRequest{
		CompartmentId: common.String(tenant.TenancyOCID),
		VcnId:         common.String(req.VcnID),
		Limit:         common.Int(1),
	}
	slResp, err := vcn.ListSecurityLists(r.Context(), slReq)
	if err != nil {
		jsonErr(w, "list security lists: "+err.Error())
		return
	}
	if len(slResp.Items) == 0 {
		jsonErr(w, "no security list found")
		return
	}

	sl := slResp.Items[0]

	// Filter OUT any ingress rule that ALLOWS traffic from blacklisted CIDRs.
	// OCI security lists use ALLOW semantics only, so to block an IP we
	// remove all existing rules that permit traffic from that source.
	var filteredRules []core.IngressSecurityRule
	for _, existing := range sl.IngressSecurityRules {
		remove := false
		for _, cidr := range req.Blacklist {
			if existing.Source != nil && *existing.Source == cidr {
				remove = true
				break
			}			
		}
		if !remove {
			filteredRules = append(filteredRules, existing)
		}
	}

	updateReq := core.UpdateSecurityListRequest{
		SecurityListId: sl.Id,
		UpdateSecurityListDetails: core.UpdateSecurityListDetails{
			IngressSecurityRules: filteredRules,
			EgressSecurityRules:  sl.EgressSecurityRules,
		},
	}
	if _, err := vcn.UpdateSecurityList(r.Context(), updateReq); err != nil {
		jsonErr(w, "update security list: "+err.Error())
		return
	}

	s.store.SetConfig("defense_enabled", "true")
	s.store.SetConfig("defense_tenant", strconv.FormatInt(req.TenantID, 10))
	s.store.SetConfig("defense_vcn", req.VcnID)
	s.store.SetConfig("defense_cidrs", strings.Join(req.Blacklist, ","))
	s.audit(req.TenantID, "defense:enable", strconv.Itoa(len(req.Blacklist))+" IPs blocked", r)
	jsonOK(w, map[string]interface{}{"status": "ok", "blocked": len(req.Blacklist)})
}

func (s *Server) handleDefenseDisable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		TenantID int64  `json:"tenant_id"`
		VcnID    string `json:"vcn_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}
	if req.TenantID == 0 || req.VcnID == "" {
		jsonErr(w, "tenant_id and vcn_id required")
		return
	}

	client, tenant, ok := s.getTenantClient(req.TenantID, w)
	if !ok {
		return
	}
	vcn := client.VcnClient()

	slReq := core.ListSecurityListsRequest{
		CompartmentId: common.String(tenant.TenancyOCID),
		VcnId:         common.String(req.VcnID),
		Limit:         common.Int(1),
	}
	slResp, err := vcn.ListSecurityLists(r.Context(), slReq)
	if err != nil {
		jsonErr(w, "list security lists: "+err.Error())
		return
	}
	if len(slResp.Items) == 0 {
		jsonErr(w, "no security list found")
		return
	}
	sl := slResp.Items[0]

	// Restore: add back an allow-all ingress rule to undo the blacklist
	restoredRules := append(sl.IngressSecurityRules, core.IngressSecurityRule{
		Protocol: common.String("all"),
		Source:   common.String("0.0.0.0/0"),
	})

	updateReq := core.UpdateSecurityListRequest{
		SecurityListId: sl.Id,
		UpdateSecurityListDetails: core.UpdateSecurityListDetails{
			IngressSecurityRules: restoredRules,
			EgressSecurityRules:  sl.EgressSecurityRules,
		},
	}
	if _, err := vcn.UpdateSecurityList(r.Context(), updateReq); err != nil {
		jsonErr(w, "update security list: "+err.Error())
		return
	}

	s.store.SetConfig("defense_enabled", "false")
	s.store.SetConfig("defense_cidrs", "")
	s.audit(req.TenantID, "defense:disable", req.VcnID, r)
	jsonOK(w, map[string]string{"status": "ok"})
}

func (s *Server) handleIPBlacklist(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		tenantID, _ := strconv.ParseInt(r.URL.Query().Get("tenant_id"), 10, 64)
		list, err := s.store.ListIpData(tenantID, "deny")
		if err != nil {
			jsonErr(w, "list blacklist: "+err.Error())
			return
		}
		if list == nil {
			list = []db.IpData{}
		}
		jsonOK(w, map[string]interface{}{"data": list})

	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}
