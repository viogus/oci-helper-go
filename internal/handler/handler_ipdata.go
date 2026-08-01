package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/viogus/oci-helper-go/internal/db"
	"github.com/viogus/oci-helper-go/internal/geoip"
)

func (s *Server) handleIpData(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		tenantID, _ := strconv.ParseInt(r.URL.Query().Get("tenant_id"), 10, 64)
		dataType := r.URL.Query().Get("type")
		list, err := s.store.ListIpData(tenantID, dataType)
		if err != nil {
			jsonErr(w, "list ip data: "+err.Error())
			return
		}
		if list == nil {
			list = []db.IpData{}
		}
		jsonOK(w, map[string]interface{}{"data": list})

	case http.MethodPost:
		var req struct {
			Action   string `json:"action"`
			TenantID int64  `json:"tenant_id"`
			CIDR     string `json:"cidr"`
			Label    string `json:"label"`
			Type     string `json:"type"`
			Enabled  bool   `json:"enabled"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonErr(w, "invalid body: "+err.Error())
			return
		}

		// Special action: load OCI instance IPs
		if req.Action == "load_oci" {
			if req.TenantID == 0 {
				go s.handleIpDataLoadOCIGlobal(r)
				jsonOK(w, map[string]string{"status": "started"})
				return
			}
			s.handleIpDataLoadOCI(w, r, req.TenantID)
			return
		}

		// Normal create
		if req.CIDR == "" {
			jsonErr(w, "cidr required")
			return
		}
		if req.Type == "" {
			req.Type = "pool"
		}

		// Extract bare IP from CIDR for geolocation lookup.
		bareIP := req.CIDR
		if idx := strings.IndexByte(bareIP, '/'); idx >= 0 {
			bareIP = bareIP[:idx]
		}

		data := &db.IpData{
			TenantID: req.TenantID,
			CIDR:     req.CIDR,
			Label:    req.Label,
			Type:     req.Type,
			Enabled:  req.Enabled,
		}

		// Best-effort geolocation lookup (non-blocking, non-fatal).
		if ip := net.ParseIP(bareIP); ip != nil && !ip.IsLoopback() && !ip.IsPrivate() && !ip.IsUnspecified() {
			if info, geoErr := geoip.Lookup(bareIP); geoErr == nil {
				data.Lat = info.Lat
				data.Lng = info.Lng
				data.Country = info.Country
				data.Area = info.Area
				data.City = info.City
				data.Org = info.Org
				data.Asn = info.Asn
			} else {
				log.Printf("[ip-data] geoip lookup for %s: %v", bareIP, geoErr)
			}
		}

		if err := s.store.CreateIpData(data); err != nil {
			jsonErr(w, "create ip data: "+err.Error())
			return
		}
		s.audit(data.TenantID, "ip-data:create", data.CIDR, r)
		jsonOK(w, data)

	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

// handleIpDataLoadOCIGlobal mirrors the Java loadOciIpData: clear old
// Oracle-typed entries, then refresh live public IPs from every configured
// tenant asynchronously.
func (s *Server) handleIpDataLoadOCIGlobal(r *http.Request) {
	_ = r
	ctx := context.Background()
	// Clear only entries previously imported from OCI.
	existing, _ := s.store.ListIpData(0, "oracle")
	for _, d := range existing {
		_ = s.store.DeleteIpData(d.ID)
	}
	tenants, err := s.store.ListTenants()
	if err != nil {
		log.Printf("[ip-data] load oci global: %v", err)
		return
	}
	added := 0
	for _, t := range tenants {
		client, err := s.clientFor(&t)
		if err != nil {
			continue
		}
		regions := discoverRegions(ctx, client)
		if len(regions) == 0 {
			regions = []string{t.Region}
		}
		for _, region := range regions {
			client.SetRegion(region)
			instances, err := client.ListInstances(ctx, t.TenancyOCID)
			if err != nil {
				continue
			}
			for _, inst := range instances {
				if inst.LifecycleState != core.InstanceLifecycleStateRunning {
					continue
				}
				vnics, err := client.GetInstanceVNICs(ctx, t.TenancyOCID, *inst.Id)
				if err != nil || len(vnics) == 0 || vnics[0].PublicIp == nil {
					continue
				}
				pub := *vnics[0].PublicIp
				d := &db.IpData{
					TenantID: t.ID,
					CIDR:     pub + "/32",
					Label:    strOr(inst.DisplayName, ""),
					Type:     "oracle",
					Enabled:  true,
				}
				if info, geoErr := geoip.Lookup(pub); geoErr == nil {
					d.Lat = info.Lat
					d.Lng = info.Lng
					d.Country = info.Country
					d.Area = info.Area
					d.City = info.City
					d.Org = info.Org
					d.Asn = info.Asn
				}
				if err := s.store.CreateIpData(d); err == nil {
					added++
				}
			}
		}
	}
	log.Printf("[ip-data] global oci sync complete: %d IPs", added)
}

func (s *Server) handleIpDataLoadOCI(w http.ResponseWriter, r *http.Request, tenantID int64) {
	// Verify tenant exists.
	t, err := s.store.GetTenant(tenantID)
	if err != nil || t == nil {
		jsonErr(w, "tenant not found")
		return
	}
	_ = t
	instances, err := s.store.ListInstances(tenantID)
	if err != nil {
		jsonErr(w, "list instances: "+err.Error())
		return
	}
	added := 0
	for _, inst := range instances {
		if inst.PublicIP == "" {
			continue
		}
		d := &db.IpData{
			TenantID: tenantID,
			CIDR:     inst.PublicIP + "/32",
			Label:    inst.Name,
			Type:     "pool",
			Enabled:  true,
		}
		// Best-effort geolocation lookup for public IPs.
		if info, geoErr := geoip.Lookup(inst.PublicIP); geoErr == nil {
			d.Lat = info.Lat
			d.Lng = info.Lng
			d.Country = info.Country
			d.Area = info.Area
			d.City = info.City
			d.Org = info.Org
			d.Asn = info.Asn
		}
		if err := s.store.CreateIpData(d); err != nil {
			continue
		}
		added++
	}
	s.audit(tenantID, "ip-data:load-oci", fmt.Sprintf("added %d IPs", added), r)
	jsonOK(w, map[string]interface{}{"added": added})
}

func (s *Server) handleIpDataByID(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/api/ip-data/")
	idStr = strings.TrimSuffix(idStr, "/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		jsonErr(w, "invalid ip data id")
		return
	}

	switch r.Method {
	case http.MethodPut:
		var data db.IpData
		if err := json.NewDecoder(r.Body).Decode(&data); err != nil {
			jsonErr(w, "invalid body: "+err.Error())
			return
		}
		data.ID = id
		if err := s.store.UpdateIpData(&data); err != nil {
			jsonErr(w, "update ip data: "+err.Error())
			return
		}
		s.audit(0, "ip-data:update", fmt.Sprintf("id:%d", id), r)
		jsonOK(w, data)

	case http.MethodDelete:
		if err := s.store.DeleteIpData(id); err != nil {
			jsonErr(w, "delete ip data: "+err.Error())
			return
		}
		s.audit(0, "ip-data:delete", fmt.Sprintf("id:%d", id), r)
		jsonOK(w, map[string]string{"status": "ok"})

	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}
