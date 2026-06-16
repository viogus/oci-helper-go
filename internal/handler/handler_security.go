package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
)

func (s *Server) handleSecurityRules(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Action   string   `json:"action"`
		TenantID int64    `json:"tenant_id"`
		VcnID    string   `json:"vcn_id"`
		Keyword  string   `json:"keyword"`
		Page     int      `json:"page"`
		Size     int      `json:"size"`
	// for add/remove
	Protocol string   `json:"protocol"`
	Port     string   `json:"port"`
	Source   string   `json:"source"`
	Dest     string   `json:"dest"`
	RuleIDs  []string `json:"rule_ids"`
	// for batch update
	IngressRules []json.RawMessage `json:"ingress_rules"`
	EgressRules  []json.RawMessage `json:"egress_rules"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}
	if req.Page < 1 {
		req.Page = 1
	}
	if req.Size < 1 || req.Size > 100 {
		req.Size = 20
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

	switch req.Action {
	case "page":
		rules, total, err := client.ListSecurityRules(r.Context(), req.VcnID, req.Keyword, req.Page, req.Size)
		if err != nil {
			jsonErr(w, "list security rules: "+err.Error())
			return
		}
		jsonOK(w, map[string]interface{}{"data": rules, "total": total, "page": req.Page, "size": req.Size})
	case "addIngress":
		if err := client.AddIngressRule(r.Context(), req.VcnID, req.Protocol, req.Port, req.Source); err != nil {
			jsonErr(w, "add ingress: "+err.Error())
			return
		}
		s.audit(req.TenantID, "security-rule:add-ingress", req.VcnID, r)
		jsonOK(w, map[string]string{"status": "ok"})
	case "addEgress":
		if err := client.AddEgressRule(r.Context(), req.VcnID, req.Protocol, req.Port, req.Dest); err != nil {
			jsonErr(w, "add egress: "+err.Error())
			return
		}
		s.audit(req.TenantID, "security-rule:add-egress", req.VcnID, r)
		jsonOK(w, map[string]string{"status": "ok"})
	case "remove":
		if err := client.RemoveSecurityRules(r.Context(), req.VcnID, req.RuleIDs); err != nil {
			jsonErr(w, "remove rules: "+err.Error())
			return
		}
		s.audit(req.TenantID, "security-rule:remove", strconv.Itoa(len(req.RuleIDs)), r)
		jsonOK(w, map[string]string{"status": "ok"})
	case "release":
		if err := client.ReleaseAllPorts(r.Context(), req.VcnID); err != nil {
			jsonErr(w, "release ports: "+err.Error())
			return
		}
		s.audit(req.TenantID, "security-rule:release", req.VcnID, r)
		jsonOK(w, map[string]string{"status": "ok"})
	case "release_by_vcn":
		if err := client.ReleaseAllPorts(r.Context(), req.VcnID); err != nil {
			jsonErr(w, "release by vcn: "+err.Error())
			return
		}
		s.audit(req.TenantID, "security-rule:release-by-vcn", req.VcnID, r)
		jsonOK(w, map[string]string{"status": "ok"})
	case "update_batch":
		if err := client.UpdateSecurityListBatch(r.Context(), req.VcnID, req.IngressRules, req.EgressRules); err != nil {
			jsonErr(w, "update batch: "+err.Error())
			return
		}
		s.audit(req.TenantID, "security-rule:update-batch", req.VcnID, r)
		jsonOK(w, map[string]string{"status": "ok"})
	default:
		jsonErr(w, "unknown action: "+req.Action)
	}
}
