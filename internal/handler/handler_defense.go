package handler

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/viogus/oci-helper-go/internal/db"
)

const defenseOriginalRulesPrefix = "defense_original_rules_"

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
		Limit:         common.Int(100),
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

	for _, sl := range slResp.Items {

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

		// Save the original rules so disable can restore them exactly.
		// Only save the first time — a second enable call sees already-filtered
		// rules and would overwrite the true originals.
		origKey := defenseOriginalRulesPrefix + req.VcnID + "_" + *sl.Id
		if origStr, _ := s.store.GetConfig(origKey); origStr == "" {
			origJSON, _ := json.Marshal(sl.IngressSecurityRules)
			s.store.SetConfig(origKey, string(origJSON))
		}
	}

	scope := strconv.FormatInt(req.TenantID, 10) + "_" + req.VcnID
	s.store.SetConfig("defense_enabled_"+scope, "true")
	s.store.SetConfig("defense_tenant_"+scope, strconv.FormatInt(req.TenantID, 10))
	s.store.SetConfig("defense_vcn_"+scope, req.VcnID)
	s.store.SetConfig("defense_cidrs_"+scope, strings.Join(req.Blacklist, ","))
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
		Limit:         common.Int(100),
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

	for _, sl := range slResp.Items {

		// Restore: load the original rules saved during enable.
		// If none saved (legacy), fall back to adding an allow-all rule.
		var restoredRules []core.IngressSecurityRule
		if origStr, err := s.store.GetConfig(defenseOriginalRulesPrefix + req.VcnID + "_" + *sl.Id); err == nil && origStr != "" {
			if err := json.Unmarshal([]byte(origStr), &restoredRules); err != nil {
				log.Printf("[defense] unmarshal original rules for %s: %v", *sl.Id, err)
				restoredRules = nil
			}
		}
		if restoredRules == nil {
			// Check if an allow-all rule already exists before appending.
			hasAllowAll := false
			for _, r := range sl.IngressSecurityRules {
				if r.Protocol != nil && *r.Protocol == "all" &&
					r.Source != nil && *r.Source == "0.0.0.0/0" {
					hasAllowAll = true
					break
				}
			}
			restoredRules = sl.IngressSecurityRules
			if !hasAllowAll {
				restoredRules = append(restoredRules, core.IngressSecurityRule{
					Protocol: common.String("all"),
					Source:   common.String("0.0.0.0/0"),
				})
			}
		}

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
	}

	scope := strconv.FormatInt(req.TenantID, 10) + "_" + req.VcnID
	s.store.SetConfig("defense_enabled_"+scope, "false")
	s.store.SetConfig("defense_tenant_"+scope, "")
	s.store.SetConfig("defense_vcn_"+scope, "")
	s.store.SetConfig("defense_cidrs_"+scope, "")
	for _, sl := range slResp.Items {
		s.store.SetConfig(defenseOriginalRulesPrefix+req.VcnID+"_"+*sl.Id, "")
	}
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
