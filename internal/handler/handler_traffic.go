package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
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

// ── POST /api/traffic/accountStats — per-account traffic, all regions ──

// defaultTrafficQuotaGB is OCI's free monthly egress allowance (10 TiB).
// It is only a default: the effective value comes from the traffic_quota_gb
// setting so users can correct it as OCI's terms change.
const defaultTrafficQuotaGB = 10240

// maxTrafficStatsSpan bounds the query window. A longer window would fall back
// to a coarse metric interval and multiply the number of monitoring calls past
// what a single synchronous request can afford.
const maxTrafficStatsSpan = 32 * 24 * time.Hour

// trafficQuotaBytes returns the configured monthly free egress allowance in
// bytes, falling back to the OCI default when unset or unparseable.
func (s *Server) trafficQuotaBytes() float64 {
	fallback := float64(defaultTrafficQuotaGB) * 1024 * 1024 * 1024
	raw, _ := s.store.GetConfig("traffic_quota_gb")
	gb, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || gb <= 0 {
		return fallback
	}
	return gb * 1024 * 1024 * 1024
}

type trafficAccountRegionStat struct {
	Region        string                    `json:"region"`
	InstanceCount int                       `json:"instanceCount"`
	InboundBytes  float64                   `json:"inboundBytes"`
	OutboundBytes float64                   `json:"outboundBytes"`
	Instances     []oci.InstanceTrafficStat `json:"instances,omitempty"`
	Error         string                    `json:"error,omitempty"`
	// Partial marks a region whose totals are understated because some
	// measurements failed. Callers must not treat it as "within allowance".
	Partial             bool `json:"partial,omitempty"`
	UnreadableInstances int  `json:"unreadableInstances,omitempty"`
	FailedMeasurements  int  `json:"failedMeasurements,omitempty"`
}

type trafficAccountStatsResponse struct {
	TenantID      int64                      `json:"tenant_id"`
	TenantName    string                     `json:"tenantName"`
	StartTime     string                     `json:"start_time"`
	EndTime       string                     `json:"end_time"`
	Regions       []trafficAccountRegionStat `json:"regions"`
	InstanceCount int                        `json:"instanceCount"`
	RegionCount   int                        `json:"regionCount"`
	InboundBytes  float64                    `json:"inboundBytes"`
	OutboundBytes float64                    `json:"outboundBytes"`
	QuotaBytes    float64                    `json:"quotaBytes"`
	QuotaPercent  float64                    `json:"quotaPercent"`
	Exceeded      bool                       `json:"exceeded"`
	// Partial is set when any region's totals are understated, so "within
	// allowance" must not be read as a definitive answer.
	Partial   bool     `json:"partial,omitempty"`
	Errors    []string `json:"errors,omitempty"`
	ElapsedMS int64    `json:"elapsedMs"`
}

func (s *Server) handleTrafficAccountStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		TenantID  int64  `json:"tenant_id"`
		StartTime string `json:"start_time"`
		EndTime   string `json:"end_time"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}
	if req.TenantID == 0 {
		jsonErr(w, "tenant_id required")
		return
	}

	tenant, err := s.store.GetTenant(req.TenantID)
	if err != nil || tenant == nil {
		jsonErr(w, "tenant not found")
		return
	}

	// Default window: from the 1st of the current month until now — the same
	// window OCI bills the free egress allowance against.
	now := time.Now()
	startTime := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	endTime := now
	if req.StartTime != "" {
		t, err := time.Parse(time.RFC3339, req.StartTime)
		if err != nil {
			jsonErr(w, "invalid start_time: "+err.Error())
			return
		}
		startTime = t
	}
	if req.EndTime != "" {
		t, err := time.Parse(time.RFC3339, req.EndTime)
		if err != nil {
			jsonErr(w, "invalid end_time: "+err.Error())
			return
		}
		endTime = t
	}
	if !endTime.After(startTime) {
		jsonErr(w, "end_time must be after start_time")
		return
	}
	if endTime.Sub(startTime) > maxTrafficStatsSpan {
		jsonErr(w, "time range too long: max "+strconv.Itoa(int(maxTrafficStatsSpan.Hours()/24))+" days")
		return
	}

	regions := getSubscribedRegions(tenant)
	if len(regions) == 0 {
		client, err := s.clientFor(tenant)
		if err != nil {
			s.clientSafeErr(w, "oci client init failed", err)
			return
		}
		discovered := discoverRegions(r.Context(), client)
		if len(discovered) == 0 {
			jsonErr(w, "no subscribed regions for tenant")
			return
		}
		regions = discovered
	}

	// Every region issues its own monitoring calls, so the whole aggregation is
	// kept inside a budget that still leaves time to write the response before
	// the server's 60s WriteTimeout.
	ctx, cancel := context.WithTimeout(r.Context(), 50*time.Second)
	defer cancel()

	started := time.Now()
	results := make([]trafficAccountRegionStat, len(regions))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 3) // 3 regions in flight; each builds its own client

	for i, region := range regions {
		if !validRegion.MatchString(region) {
			results[i] = trafficAccountRegionStat{Region: region, Error: "invalid region"}
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, region string) {
			defer wg.Done()
			defer func() { <-sem }()

			// SetRegion mutates every SDK client, so regions must never share one.
			regTenant := *tenant
			regTenant.Region = region
			client, err := s.clientFor(&regTenant)
			if err != nil {
				results[i] = trafficAccountRegionStat{Region: region, Error: "client init failed"}
				return
			}

			instances, err := client.ListInstances(ctx, tenant.TenancyOCID)
			if err != nil {
				results[i] = trafficAccountRegionStat{Region: region, Error: "list instances failed"}
				return
			}
			refs := make([]oci.InstanceTrafficRef, 0, len(instances))
			for _, inst := range instances {
				if inst.Id == nil {
					continue
				}
				refs = append(refs, oci.InstanceTrafficRef{
					OCID: *inst.Id,
					Name: strOr(inst.DisplayName, ""),
				})
			}

			detail, err := client.FetchInstancesTrafficDetail(ctx, tenant.TenancyOCID, region, refs, startTime, endTime)
			if err != nil {
				results[i] = trafficAccountRegionStat{Region: region, Error: "fetch traffic failed"}
				return
			}

			stat := trafficAccountRegionStat{
				Region:              region,
				InstanceCount:       len(detail.Stats),
				Instances:           detail.Stats,
				UnreadableInstances: detail.UnreadableVNICs,
				FailedMeasurements:  detail.FailedMetrics,
			}
			stat.Partial = detail.UnreadableVNICs > 0 || detail.FailedMetrics > 0 ||
				(ctx.Err() != nil && len(detail.Stats) > 0)
			for _, st := range detail.Stats {
				stat.InboundBytes += st.Inbound
				stat.OutboundBytes += st.Outbound
			}
			results[i] = stat
		}(i, region)
	}
	wg.Wait()

	resp := trafficAccountStatsResponse{
		TenantID:   tenant.ID,
		TenantName: tenant.Name,
		StartTime:  startTime.Format(time.RFC3339),
		EndTime:    endTime.Format(time.RFC3339),
		QuotaBytes: s.trafficQuotaBytes(),
		Regions:    results,
	}
	for _, rs := range results {
		if rs.Error != "" {
			resp.Errors = append(resp.Errors, rs.Region+": "+rs.Error)
			continue
		}
		resp.RegionCount++
		resp.InstanceCount += rs.InstanceCount
		resp.InboundBytes += rs.InboundBytes
		resp.OutboundBytes += rs.OutboundBytes
		if rs.Partial {
			// Understated totals must never pass as "within allowance".
			resp.Partial = true
			resp.Errors = append(resp.Errors, fmt.Sprintf("%s: partial data (%d unreadable instance(s), %d failed measurement(s))",
				rs.Region, rs.UnreadableInstances, rs.FailedMeasurements))
		}
	}
	// Heaviest egress first — the region closest to the quota is what matters.
	sort.SliceStable(resp.Regions, func(a, b int) bool {
		return resp.Regions[a].OutboundBytes > resp.Regions[b].OutboundBytes
	})
	if resp.QuotaBytes > 0 {
		resp.QuotaPercent = resp.OutboundBytes / resp.QuotaBytes * 100
	}
	resp.Exceeded = resp.QuotaBytes > 0 && resp.OutboundBytes > resp.QuotaBytes
	resp.ElapsedMS = time.Since(started).Milliseconds()

	jsonOK(w, resp)
}

// ── formatBytes helper exposed for handler reuse ────────────────────────

func formatBytes(bytes float64) string { return oci.FormatBytes(bytes) }

// validRegion matches OCI region identifiers (e.g. us-phoenix-1, eu-frankfurt-1).
var validRegion = regexp.MustCompile(`^[a-z]{2,}-[a-z]+-\d+$`)
