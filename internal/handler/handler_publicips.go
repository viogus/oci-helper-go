package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
)

func (s *Server) handlePublicIPs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		client, t, ok := s.ociClientFromQuery(w, r)
		if !ok {
			return
		}
		ips, err := client.ListPublicIPs(r.Context(), t.TenancyOCID)
		if err != nil {
			jsonErr(w, "list public ips: "+err.Error())
			return
		}
		jsonOK(w, ips)
	case http.MethodPost:
		var req struct {
			TenantID      int64  `json:"tenantId"`
			DisplayName   string `json:"displayName"`
			CompartmentID string `json:"compartmentId"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonErr(w, "invalid body: "+err.Error())
			return
		}
		client, t, ok := s.getTenantClient(req.TenantID, w)
		if !ok {
			return
		}
		lifetime := core.CreatePublicIpDetailsLifetimeReserved
		compartmentID := req.CompartmentID
		if compartmentID == "" {
			compartmentID = t.TenancyOCID
		}
		ip, err := client.CreatePublicIP(r.Context(), core.CreatePublicIpDetails{
			CompartmentId: common.String(compartmentID),
			DisplayName:   common.String(req.DisplayName),
			Lifetime:      lifetime,
		})
		if err != nil {
			jsonErr(w, "create public ip: "+err.Error())
			return
		}
		s.audit(req.TenantID, "publicip:create", strOr(ip.DisplayName, ""), r)
		jsonOK(w, ip)
	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handlePublicIPByID(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/api/public-ips/")
	idStr = strings.TrimSuffix(idStr, "/")

	if r.Method != http.MethodDelete {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	tenantID, _ := strconv.ParseInt(r.URL.Query().Get("tenant_id"), 10, 64)
	client, _, ok := s.getTenantClient(tenantID, w)
	if !ok {
		return
	}
	if err := client.DeletePublicIP(r.Context(), idStr); err != nil {
		jsonErr(w, "delete public ip: "+err.Error())
		return
	}
	s.audit(tenantID, "publicip:delete", idStr, r)
	jsonOK(w, map[string]string{"status": "ok"})
}
