package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/viogus/oci-helper-go/internal/db"
)

// handleTenantAliveCheck verifies each selected (or all) tenant's OCI API
// credentials by listing availability domains, updates account_status, and
// sends a summary notification. This mirrors Java's /api/oci/checkAlive and
// checkAliveBatch endpoints.
func (s *Server) handleTenantAliveCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		TenantIDs []int64 `json:"tenant_ids"`
	}
	// A malformed body must not silently fall through to checking ALL tenants.
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}

	var tenants []db.Tenant
	var err error
	if len(req.TenantIDs) == 0 {
		tenants, err = s.store.ListTenants()
	} else {
		for _, id := range req.TenantIDs {
			t, e := s.store.GetTenant(id)
			if e != nil {
				err = e
				break
			}
			if t != nil {
				tenants = append(tenants, *t)
			}
		}
	}
	if err != nil {
		jsonErr(w, "list tenants: "+err.Error())
		return
	}

	type result struct {
		TenantID int64  `json:"tenant_id"`
		Name     string `json:"name"`
		Status   string `json:"status"`
		Error    string `json:"error,omitempty"`
	}
	results := make([]result, 0, len(tenants))
	failNames := make([]string, 0)
	for _, t := range tenants {
		res := result{TenantID: t.ID, Name: t.Name}
		client, err := s.clientFor(&t)
		if err != nil {
			res.Status = "INACTIVE"
			res.Error = err.Error()
			if err := s.store.SetTenantAccountStatus(t.ID, "INACTIVE"); err != nil {
				log.Printf("[alive] SetTenantAccountStatus: %v", err)
			}
			failNames = append(failNames, t.Name)
		} else if err := client.ValidateCredentials(r.Context(), t.TenancyOCID); err != nil {
			res.Status = "INACTIVE"
			res.Error = err.Error()
			if err := s.store.SetTenantAccountStatus(t.ID, "INACTIVE"); err != nil {
				log.Printf("[alive] SetTenantAccountStatus: %v", err)
			}
			failNames = append(failNames, t.Name)
		} else {
			res.Status = "ACTIVE"
			if err := s.store.SetTenantAccountStatus(t.ID, "ACTIVE"); err != nil {
				log.Printf("[alive] SetTenantAccountStatus: %v", err)
			}
		}
		results = append(results, res)
	}
	failText := "无"
	if len(failNames) > 0 {
		failText = strings.Join(failNames, "、")
	}
	s.notifyGlobal(fmt.Sprintf("【API测活结果】\n总配置数：%d\n有效配置数：%d\n失效配置数：%d\n失效配置：%s",
		len(results), len(results)-len(failNames), len(failNames), failText))
	s.audit(0, "tenant:check-alive", fmt.Sprintf("%d tenants, %d failed", len(results), len(failNames)), r)
	jsonOK(w, map[string]interface{}{"results": results, "total": len(results), "failed": len(failNames)})
}

// handleUpdateRootPassword sets or removes the root-password freeform tag on an
// instance.
func (s *Server) handleUpdateRootPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		TenantID   int64  `json:"tenant_id"`
		InstanceID string `json:"instance_id"`
		Password   string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}
	if req.TenantID == 0 || req.InstanceID == "" {
		jsonErr(w, "tenant_id and instance_id required")
		return
	}
	client, _, ok := s.clientForInstance(req.TenantID, req.InstanceID, w)
	if !ok {
		return
	}
	if err := client.UpdateRootPasswordTag(r.Context(), bareOCID(req.InstanceID), req.Password); err != nil {
		jsonErr(w, "update root password tag: "+err.Error())
		return
	}
	action := "set"
	if req.Password == "" {
		action = "clear"
	}
	s.audit(req.TenantID, "instance:root-password:"+action, req.InstanceID, r)
	jsonOK(w, map[string]string{"status": "ok", "action": action})
}
