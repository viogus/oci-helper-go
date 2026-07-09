package handler

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/viogus/oci-helper-go/internal/oci"
)

// ── POST /api/traffic — query traffic data for a VNIC ──────────────────

func (s *Server) handleTraffic(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		TenantID   int64  `json:"tenant_id"`
		Region     string `json:"region"`
		InstanceID string `json:"instance_id"`
		VnicID     string `json:"vnic_id"`
		StartTime  string `json:"start_time"`
		EndTime    string `json:"end_time"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}

	client, tenant, ok := s.getTenantClient(req.TenantID, w)
	if !ok {
		return
	}
	if req.Region != "" {
		if !validRegion.MatchString(req.Region) {
			jsonErr(w, "invalid region: "+req.Region)
			return
		}
		client.SetRegion(req.Region)
	}

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

	vnicID := req.VnicID
	vnicCompartment := tenant.TenancyOCID
	if vnicID == "" {
		instanceID := req.InstanceID
		if i := strings.IndexByte(instanceID, ':'); i >= 0 {
			instanceID = instanceID[i+1:]
		}
		vnics, err := client.GetInstanceVNICs(r.Context(), tenant.TenancyOCID, instanceID)
		if err != nil || len(vnics) == 0 {
			jsonErr(w, "no VNIC found for instance")
			return
		}
		vnicID = *vnics[0].Id
		if vnics[0].CompartmentId != nil {
			vnicCompartment = *vnics[0].CompartmentId
		}
	} else if i := strings.IndexByte(vnicID, ':'); i >= 0 {
		vnicID = vnicID[i+1:]
	}

	data, err := client.GetVNICTtraffic(r.Context(), vnicCompartment, vnicID, startTime, endTime)
	if err != nil {
		jsonErr(w, "get traffic: "+err.Error())
		return
	}
	jsonOK(w, map[string]interface{}{"data": data, "vnic_id": vnicID})
}

// ── GET /api/traffic/getCondition — region + instance cascade ──────────

func (s *Server) handleTrafficCondition(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	tenantID, _ := strconv.ParseInt(r.URL.Query().Get("tenant_id"), 10, 64)
	if tenantID == 0 {
		jsonErr(w, "tenant_id required")
		return
	}

	client, tenant, ok := s.getTenantClient(tenantID, w)
	if !ok {
		return
	}

	// List region subscriptions.
	regions, err := client.ListRegionSubscriptions(r.Context())
	if err != nil {
		jsonErr(w, "list regions: "+err.Error())
		return
	}

	type ValueLabel struct {
		Label string `json:"label"`
		Value string `json:"value"`
	}

	regionOptions := make([]ValueLabel, 0, len(regions))
	instanceOptions := make(map[string][]ValueLabel)

	for _, reg := range regions {
		rn := *reg.RegionName
		regionOptions = append(regionOptions, ValueLabel{Label: rn, Value: rn})

		// Reuse the existing client — only switch region (avoids re-reading key file
		// and re-creating all SDK clients per region).
		client.SetRegion(rn)
		instances, err := client.ListInstances(r.Context(), tenant.TenancyOCID)
		if err != nil {
			continue
		}
		var instOpts []ValueLabel
		for _, inst := range instances {
			name := ""
			if inst.DisplayName != nil {
				name = *inst.DisplayName
			}
			id := ""
			if inst.Id != nil {
				id = *inst.Id
			}
			instOpts = append(instOpts, ValueLabel{Label: name, Value: id})
		}
		instanceOptions[rn] = instOpts
	}

	jsonOK(w, map[string]interface{}{
		"regionOptions":   regionOptions,
		"instanceOptions": instanceOptions,
	})
}

// ── GET /api/traffic/fetchVnics — list VNICs for an instance ───────────

func (s *Server) handleTrafficVnics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	tenantID, _ := strconv.ParseInt(r.URL.Query().Get("tenant_id"), 10, 64)
	instanceID := r.URL.Query().Get("instance_id")
	region := r.URL.Query().Get("region")
	if tenantID == 0 || instanceID == "" {
		jsonErr(w, "tenant_id and instance_id required")
		return
	}

	client, tenant, ok := s.getTenantClient(tenantID, w)
	if !ok {
		return
	}
	if region != "" {
		if !validRegion.MatchString(region) {
			jsonErr(w, "invalid region: "+region)
			return
		}
		client.SetRegion(region)
	}

	if i := strings.IndexByte(instanceID, ':'); i >= 0 {
		instanceID = instanceID[i+1:]
	}

	vnics, err := client.GetInstanceVNICs(r.Context(), tenant.TenancyOCID, instanceID)
	if err != nil {
		jsonErr(w, "get vnics: "+err.Error())
		return
	}

	type ValueLabel struct {
		Label string `json:"label"`
		Value string `json:"value"`
	}
	var result []ValueLabel
	for _, v := range vnics {
		label := ""
		if v.DisplayName != nil {
			label = *v.DisplayName
		}
		val := ""
		if v.Id != nil {
			val = *v.Id
		}
		if v.PublicIp != nil && *v.PublicIp != "" {
			label += " (" + *v.PublicIp + ")"
		} else if v.PrivateIp != nil && *v.PrivateIp != "" {
			label += " (" + *v.PrivateIp + ")"
		}
		result = append(result, ValueLabel{Label: label, Value: val})
	}

	jsonOK(w, result)
}

// ── GET /api/traffic/fetchInstances — monthly traffic summary per region ──

func (s *Server) handleTrafficInstances(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	tenantID, _ := strconv.ParseInt(r.URL.Query().Get("tenant_id"), 10, 64)
	region := r.URL.Query().Get("region")
	if tenantID == 0 || region == "" {
		jsonErr(w, "tenant_id and region required")
		return
	}

	tenant, err := s.store.GetTenant(tenantID)
	if err != nil || tenant == nil {
		jsonErr(w, "tenant not found")
		return
	}

	// Use tenant with specific region.
	regTenant := *tenant
	regTenant.Region = region
	client, err := s.clientFor(&regTenant)
	if err != nil {
		jsonErr(w, "oci client: "+err.Error())
		return
	}

	// Default: current month.
	now := time.Now()
	startTime := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	endTime := now

	result, err := client.FetchInstancesTraffic(r.Context(), tenant.TenancyOCID, region, startTime, endTime)
	if err != nil {
		jsonErr(w, "fetch instances: "+err.Error())
		return
	}

	jsonOK(w, result)
}

// ── formatBytes helper exposed for handler reuse ────────────────────────

func formatBytes(bytes float64) string { return oci.FormatBytes(bytes) }

// validRegion matches OCI region identifiers (e.g. us-phoenix-1, eu-frankfurt-1).
var validRegion = regexp.MustCompile(`^[a-z]{2,}-[a-z]+-\d+$`)
