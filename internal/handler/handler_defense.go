package handler

import (
	"context"
	"encoding/json"
	"fmt"
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

	n, err := s.enableDefense(r.Context(), req.TenantID, req.VcnID, req.Blacklist)
	if err != nil {
		jsonErr(w, err.Error())
		return
	}
	s.audit(req.TenantID, "defense:enable", strconv.Itoa(n)+" IPs blocked", r)
	jsonOK(w, map[string]interface{}{"status": "ok", "blocked": n})
}

// enableDefense blocks the given CIDRs by removing ALLOW rules matching them
// from every security list in the VCN, and saves the original rules so
// disableDefense can restore them. Returns the number of CIDRs blocked.
func (s *Server) enableDefense(ctx context.Context, tenantID int64, vcnID string, blacklist []string) (int, error) {
	tenant, err := s.store.GetTenant(tenantID)
	if err != nil || tenant == nil {
		return 0, fmt.Errorf("tenant not found")
	}
	client, err := s.clientFor(tenant)
	if err != nil {
		return 0, fmt.Errorf("oci client: %w", err)
	}
	vcn := client.VcnClient()

	slReq := core.ListSecurityListsRequest{
		CompartmentId: common.String(tenant.TenancyOCID),
		VcnId:         common.String(vcnID),
		Limit:         common.Int(100),
	}
	slResp, err := vcn.ListSecurityLists(ctx, slReq)
	if err != nil {
		return 0, fmt.Errorf("list security lists: %w", err)
	}
	if len(slResp.Items) == 0 {
		return 0, fmt.Errorf("no security list found")
	}

	for _, sl := range slResp.Items {
		if sl.Id == nil {
			log.Printf("[defense] skipping security list with nil Id in VCN %s", vcnID)
			continue
		}

		// Filter OUT any ingress rule that ALLOWS traffic from blacklisted CIDRs.
		// OCI security lists use ALLOW semantics only, so to block an IP we
		// remove all existing rules that permit traffic from that source.
		var filteredRules []core.IngressSecurityRule
		for _, existing := range sl.IngressSecurityRules {
			remove := false
			for _, cidr := range blacklist {
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
		if _, err := vcn.UpdateSecurityList(ctx, updateReq); err != nil {
			return 0, fmt.Errorf("update security list: %w", err)
		}

		// Save the original rules so disable can restore them exactly.
		// Only save the first time — a second enable call sees already-filtered
		// rules and would overwrite the true originals.
		origKey := defenseOriginalRulesPrefix + vcnID + "_" + *sl.Id
		if origStr, _ := s.store.GetConfig(origKey); origStr == "" {
			origJSON, _ := json.Marshal(sl.IngressSecurityRules)
			s.setConfig(origKey, string(origJSON))
		}
	}

	scope := strconv.FormatInt(tenantID, 10) + "_" + vcnID
	s.setConfig("defense_enabled_"+scope, "true")
	s.setConfig("defense_tenant_"+scope, strconv.FormatInt(tenantID, 10))
	s.setConfig("defense_vcn_"+scope, vcnID)
	s.setConfig("defense_cidrs_"+scope, strings.Join(blacklist, ","))
	return len(blacklist), nil
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

	if err := s.disableDefense(r.Context(), req.TenantID, req.VcnID); err != nil {
		jsonErr(w, err.Error())
		return
	}
	s.audit(req.TenantID, "defense:disable", req.VcnID, r)
	jsonOK(w, map[string]string{"status": "ok"})
}

// disableDefense restores the original ingress rules saved by enableDefense
// (falling back to an allow-all rule for legacy configs).
func (s *Server) disableDefense(ctx context.Context, tenantID int64, vcnID string) error {
	tenant, err := s.store.GetTenant(tenantID)
	if err != nil || tenant == nil {
		return fmt.Errorf("tenant not found")
	}
	client, err := s.clientFor(tenant)
	if err != nil {
		return fmt.Errorf("oci client: %w", err)
	}
	vcn := client.VcnClient()

	slReq := core.ListSecurityListsRequest{
		CompartmentId: common.String(tenant.TenancyOCID),
		VcnId:         common.String(vcnID),
		Limit:         common.Int(100),
	}
	slResp, err := vcn.ListSecurityLists(ctx, slReq)
	if err != nil {
		return fmt.Errorf("list security lists: %w", err)
	}
	if len(slResp.Items) == 0 {
		return fmt.Errorf("no security list found")
	}

	for _, sl := range slResp.Items {
		if sl.Id == nil {
			log.Printf("[defense] skipping security list with nil Id in VCN %s", vcnID)
			continue
		}

		// Restore: load the original rules saved during enable.
		// If none saved (legacy), fall back to adding an allow-all rule.
		var restoredRules []core.IngressSecurityRule
		if origStr, err := s.store.GetConfig(defenseOriginalRulesPrefix + vcnID + "_" + *sl.Id); err == nil && origStr != "" {
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
		if _, err := vcn.UpdateSecurityList(ctx, updateReq); err != nil {
			return fmt.Errorf("update security list: %w", err)
		}
	}

	scope := strconv.FormatInt(tenantID, 10) + "_" + vcnID
	s.setConfig("defense_enabled_"+scope, "false")
	s.setConfig("defense_tenant_"+scope, "")
	s.setConfig("defense_vcn_"+scope, "")
	s.setConfig("defense_cidrs_"+scope, "")
	for _, sl := range slResp.Items {
		if sl.Id == nil {
			continue
		}
		s.setConfig(defenseOriginalRulesPrefix+vcnID+"_"+*sl.Id, "")
	}
	return nil
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
