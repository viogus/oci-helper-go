package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"

	ociclient "github.com/viogus/oci-helper-go/internal/oci"
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

	client, _, ok := s.getTenantClient(req.TenantID, w)
	if !ok {
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
	case "release": // alias for release_by_vcn, both call ReleaseAllPorts
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

// ── G7: Security Rule Batch Release ─────────────────────────────────────

func (s *Server) handleSecurityRuleRelease(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		TenantID int64    `json:"tenant_id"`
		VcnID    string   `json:"vcn_id"`
		Ports    []string `json:"ports"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}
	if req.VcnID == "" || len(req.Ports) == 0 {
		jsonErr(w, "vcn_id and ports required")
		return
	}
	client, _, ok := s.getTenantClient(req.TenantID, w)
	if !ok {
		return
	}
	added := 0
	var addedPorts []string
	for _, port := range req.Ports {
		if err := client.AddIngressRule(r.Context(), req.VcnID, "TCP", port, "0.0.0.0/0"); err != nil {
			// Rollback previously added rules on partial failure.
			if added > 0 {
				if rbErr := rollbackIngressRules(client, r.Context(), req.VcnID, addedPorts); rbErr != nil {
					log.Printf("[security] rollback failed after partial batch-release: %v (original: %v)", rbErr, err)
				}
			}
			jsonErr(w, fmt.Sprintf("add ingress rule port %s: %v", port, err))
			return
		}
		addedPorts = append(addedPorts, port)
		added++
	}
	s.audit(req.TenantID, "security-rule:batch-release", req.VcnID, r)
	jsonOK(w, map[string]interface{}{"status": "ok", "rules_added": added})
}

// rollbackIngressRules removes TCP ingress rules from 0.0.0.0/0 for given ports.
// Used to undo partial batch-release failures.
func rollbackIngressRules(client *ociclient.Client, ctx context.Context, vcnID string, ports []string) error {
	rules, _, err := client.ListSecurityRules(ctx, vcnID, "", 1, 100)
	if err != nil {
		return fmt.Errorf("list rules for rollback: %w", err)
	}
	var idsToRemove []string
	for _, rule := range rules {
		if rule.Type == "ingress" && rule.Source == "0.0.0.0/0" && rule.Protocol == "TCP" {
			for _, port := range ports {
				if rule.Port == port || rule.Port == port+"-"+port {
					idsToRemove = append(idsToRemove, rule.ID)
					break
				}
			}
		}
	}
	if len(idsToRemove) == 0 {
		return nil
	}
	return client.RemoveSecurityRules(ctx, vcnID, idsToRemove)
}
