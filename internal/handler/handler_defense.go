package handler

import (
	"encoding/json"
	"net/http"
	"strconv"

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
	ingressRules := sl.IngressSecurityRules

	// Add DENY rules for each blacklisted CIDR
	for _, cidr := range req.Blacklist {
		alreadyBlocked := false
		for _, existing := range ingressRules {
			if existing.Source != nil && *existing.Source == cidr && existing.Protocol != nil && *existing.Protocol == "all" {
				alreadyBlocked = true
				break
			}
		}
		if !alreadyBlocked {
			ingressRules = append(ingressRules, core.IngressSecurityRule{
				Protocol:    common.String("all"),
				Source:      common.String(cidr),
				IsStateless: common.Bool(false),
			})
		}
	}

	updateReq := core.UpdateSecurityListRequest{
		SecurityListId: sl.Id,
		UpdateSecurityListDetails: core.UpdateSecurityListDetails{
			IngressSecurityRules: ingressRules,
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

	var keepRules []core.IngressSecurityRule
	for _, rule := range sl.IngressSecurityRules {
		if rule.Protocol != nil && *rule.Protocol == "all" {
			// Skip DENY ALL rules (these are the defense mode rules)
			continue
		}
		keepRules = append(keepRules, rule)
	}

	updateReq := core.UpdateSecurityListRequest{
		SecurityListId: sl.Id,
		UpdateSecurityListDetails: core.UpdateSecurityListDetails{
			IngressSecurityRules: keepRules,
			EgressSecurityRules:  sl.EgressSecurityRules,
		},
	}
	if _, err := vcn.UpdateSecurityList(r.Context(), updateReq); err != nil {
		jsonErr(w, "update security list: "+err.Error())
		return
	}

	s.store.SetConfig("defense_enabled", "false")
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
