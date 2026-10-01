package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/viogus/oci-helper-go/internal/db"
	"github.com/viogus/oci-helper-go/internal/oci"
)

// The traffic page has three views — live VNIC query, single-account summary and
// per-account comparison. Each used to bring its own tenant lookup, region
// handling and time-window defaults, which drifted apart (the summary view
// hard-coded the current month on the server while the other two took a range
// from the client, and each fanned out over regions its own way).
//
// Everything below shares one core:
//   - parseTrafficWindow  → same window semantics for every view
//   - tenantRegionClient  → one OCI client per region (SetRegion mutates every
//     SDK client, so regions must never share one)
//   - collectTrafficStats → same per-region aggregation, failure counting and
//     free-allowance math
//
// Only the presentation differs per view.

// defaultTrafficQuotaGB is OCI's free monthly egress allowance (10 TiB).
// It is only a default: the effective value comes from the traffic_quota_gb
// setting so users can correct it as OCI's terms change.
const defaultTrafficQuotaGB = 10240

// maxTrafficStatsSpan bounds every traffic query window. A longer window would
// fall back to a coarse metric interval and multiply the number of monitoring
// calls past what a single synchronous request can afford.
const maxTrafficStatsSpan = 32 * 24 * time.Hour

const (
	// trafficStatsBudget leaves time to write the response before the server's
	// 60s WriteTimeout.
	trafficStatsBudget = 50 * time.Second
	// trafficRegionConcurrency is how many regions are read at once.
	trafficRegionConcurrency = 3
)

// errNoSubscribedRegions reports a tenant without any usable region list, as
// opposed to a failure while discovering them.
var errNoSubscribedRegions = errors.New("no subscribed regions for tenant")

// valueLabel is the {label,value} pair every frontend select consumes.
type valueLabel struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// ── shared query context ───────────────────────────────────────────────

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

// parseTrafficWindow resolves the query window shared by every traffic view:
// an omitted bound defaults to the current billing month, so the free egress
// allowance is measured over the period OCI bills it against. It returns an
// empty errMsg on success, otherwise a client-safe message.
func parseTrafficWindow(startRaw, endRaw string) (start, end time.Time, errMsg string) {
	now := time.Now()
	start = time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	end = now
	if startRaw != "" {
		t, err := time.Parse(time.RFC3339, startRaw)
		if err != nil {
			return start, end, "invalid start_time: " + err.Error()
		}
		start = t
	}
	if endRaw != "" {
		t, err := time.Parse(time.RFC3339, endRaw)
		if err != nil {
			return start, end, "invalid end_time: " + err.Error()
		}
		end = t
	}
	if !end.After(start) {
		return start, end, "end_time must be after start_time"
	}
	if end.Sub(start) > maxTrafficStatsSpan {
		return start, end, "time range too long: max " + strconv.Itoa(int(maxTrafficStatsSpan.Hours()/24)) + " days"
	}
	return start, end, ""
}

// tenantRegionClient returns a client pinned to region. An empty region (or the
// tenant's own) uses the tenant default. Never share the result across regions.
func (s *Server) tenantRegionClient(tenant *db.Tenant, region string) (*oci.Client, error) {
	if region == "" || region == tenant.Region {
		return s.clientFor(tenant)
	}
	regTenant := *tenant
	regTenant.Region = region
	return s.clientFor(&regTenant)
}

// tenantForRequest loads the tenant and builds a client pinned to region,
// writing the error response itself. Callers return immediately when ok is false.
func (s *Server) tenantForRequest(tenantID int64, region string, w http.ResponseWriter) (*oci.Client, *db.Tenant, bool) {
	tenant, err := s.store.GetTenant(tenantID)
	if err != nil || tenant == nil {
		jsonErr(w, "tenant not found")
		return nil, nil, false
	}
	client, err := s.tenantRegionClient(tenant, region)
	if err != nil {
		s.clientSafeErr(w, "OCI client error", err)
		return nil, nil, false
	}
	return client, tenant, true
}

// requestRegion validates an optional region and echoes it back.
func requestRegion(region string, w http.ResponseWriter) (string, bool) {
	if region == "" || validRegion.MatchString(region) {
		return region, true
	}
	jsonErr(w, "invalid region: "+region)
	return "", false
}

// trafficRegions resolves which regions a tenant's traffic can be read from:
// the cached subscription list first, then the Identity API.
func (s *Server) trafficRegions(ctx context.Context, tenant *db.Tenant) ([]string, error) {
	if regions := getSubscribedRegions(tenant); len(regions) > 0 {
		return regions, nil
	}
	client, err := s.clientFor(tenant)
	if err != nil {
		return nil, err
	}
	regions := discoverRegions(ctx, client)
	if len(regions) == 0 {
		return nil, errNoSubscribedRegions
	}
	return regions, nil
}

// listRegionInstances returns the instance select options for one region.
// A region that cannot be read yields no options, matching the previous
// best-effort cascade behaviour.
func (s *Server) listRegionInstances(ctx context.Context, tenant *db.Tenant, region string) []valueLabel {
	client, err := s.tenantRegionClient(tenant, region)
	if err != nil {
		return nil
	}
	instances, err := client.ListInstances(ctx, tenant.TenancyOCID)
	if err != nil {
		return nil
	}
	opts := make([]valueLabel, 0, len(instances))
	for _, inst := range instances {
		if inst.Id == nil {
			continue
		}
		opts = append(opts, valueLabel{Label: strOr(inst.DisplayName, ""), Value: *inst.Id})
	}
	return opts
}

// ── aggregation core ───────────────────────────────────────────────────

type trafficRegionStat struct {
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

// trafficStatsResponse is the single traffic aggregate served to the frontend;
// a single-region request is just a filtered instance of it.
type trafficStatsResponse struct {
	TenantID      int64               `json:"tenant_id"`
	TenantName    string              `json:"tenantName"`
	StartTime     string              `json:"start_time"`
	EndTime       string              `json:"end_time"`
	Regions       []trafficRegionStat `json:"regions"`
	InstanceCount int                 `json:"instanceCount"`
	RegionCount   int                 `json:"regionCount"`
	InboundBytes  float64             `json:"inboundBytes"`
	OutboundBytes float64             `json:"outboundBytes"`
	QuotaBytes    float64             `json:"quotaBytes"`
	QuotaPercent  float64             `json:"quotaPercent"`
	Exceeded      bool                `json:"exceeded"`
	// Partial is set when any region's totals are understated, so "within
	// allowance" must not be read as a definitive answer.
	Partial   bool     `json:"partial,omitempty"`
	Errors    []string `json:"errors,omitempty"`
	ElapsedMS int64    `json:"elapsedMs"`
}

// collectRegionTraffic reads every instance's traffic in one region. The
// failure counts travel with the stat so callers can tell "no traffic" apart
// from "could not measure".
func (s *Server) collectRegionTraffic(ctx context.Context, tenant *db.Tenant, region string, startTime, endTime time.Time) trafficRegionStat {
	stat := trafficRegionStat{Region: region}
	if !validRegion.MatchString(region) {
		stat.Error = "invalid region"
		return stat
	}
	client, err := s.tenantRegionClient(tenant, region)
	if err != nil {
		stat.Error = "client init failed"
		return stat
	}
	instances, err := client.ListInstances(ctx, tenant.TenancyOCID)
	if err != nil {
		stat.Error = "list instances failed"
		return stat
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
		stat.Error = "fetch traffic failed"
		return stat
	}

	stat.InstanceCount = len(detail.Stats)
	stat.Instances = detail.Stats
	stat.UnreadableInstances = detail.UnreadableVNICs
	stat.FailedMeasurements = detail.FailedMetrics
	stat.Partial = detail.UnreadableVNICs > 0 || detail.FailedMetrics > 0 ||
		(ctx.Err() != nil && len(detail.Stats) > 0)
	for _, st := range detail.Stats {
		stat.InboundBytes += st.Inbound
		stat.OutboundBytes += st.Outbound
	}
	return stat
}

// collectTrafficStats fans the per-region aggregation out with bounded
// concurrency and finalizes the response every traffic view renders.
func (s *Server) collectTrafficStats(ctx context.Context, tenant *db.Tenant, regions []string, startTime, endTime time.Time) trafficStatsResponse {
	ctx, cancel := context.WithTimeout(ctx, trafficStatsBudget)
	defer cancel()

	started := time.Now()
	results := make([]trafficRegionStat, len(regions))
	var wg sync.WaitGroup
	sem := make(chan struct{}, trafficRegionConcurrency)
	for i, region := range regions {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, region string) {
			defer wg.Done()
			defer func() { <-sem }()
			// Each region builds its own client: SetRegion mutates every SDK
			// client the tenant holds.
			results[i] = s.collectRegionTraffic(ctx, tenant, region, startTime, endTime)
		}(i, region)
	}
	wg.Wait()

	resp := trafficStatsResponse{
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
	return resp
}

// ── POST /api/traffic — VNIC traffic time series ────────────────────────

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

	region, ok := requestRegion(req.Region, w)
	if !ok {
		return
	}
	client, tenant, ok := s.tenantForRequest(req.TenantID, region, w)
	if !ok {
		return
	}

	// Same window semantics as the aggregate views.
	startTime, endTime, errMsg := parseTrafficWindow(req.StartTime, req.EndTime)
	if errMsg != "" {
		jsonErr(w, errMsg)
		return
	}

	vnicID := req.VnicID
	vnicCompartment := tenant.TenancyOCID
	if vnicID == "" {
		instanceID := bareOCID(req.InstanceID)
		vnics, err := client.GetInstanceVNICs(r.Context(), tenant.TenancyOCID, instanceID)
		if err != nil || len(vnics) == 0 {
			jsonErr(w, "no VNIC found for instance")
			return
		}
		vnicID = *vnics[0].Id
		if vnics[0].CompartmentId != nil {
			vnicCompartment = *vnics[0].CompartmentId
		}
	} else {
		vnicID = bareOCID(vnicID)
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

	regionOptions := make([]valueLabel, 0, len(regions))
	names := make([]string, 0, len(regions))
	for _, reg := range regions {
		if reg.RegionName == nil || *reg.RegionName == "" {
			continue
		}
		rn := *reg.RegionName
		names = append(names, rn)
		regionOptions = append(regionOptions, valueLabel{Label: rn, Value: rn})
	}

	// Instance lists are the slow part. Read them with the same bounded
	// fan-out (and the same one-client-per-region rule) the aggregation uses,
	// instead of walking every region serially on one shared client.
	ctx, cancel := context.WithTimeout(r.Context(), trafficStatsBudget)
	defer cancel()

	instanceOptions := make(map[string][]valueLabel, len(names))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, trafficRegionConcurrency)
	for _, region := range names {
		wg.Add(1)
		sem <- struct{}{}
		go func(region string) {
			defer wg.Done()
			defer func() { <-sem }()
			opts := s.listRegionInstances(ctx, tenant, region)
			mu.Lock()
			instanceOptions[region] = opts
			mu.Unlock()
		}(region)
	}
	wg.Wait()

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

	region, ok := requestRegion(region, w)
	if !ok {
		return
	}
	client, tenant, ok := s.tenantForRequest(tenantID, region, w)
	if !ok {
		return
	}

	vnics, err := client.GetInstanceVNICs(r.Context(), tenant.TenancyOCID, bareOCID(instanceID))
	if err != nil {
		jsonErr(w, "get vnics: "+err.Error())
		return
	}

	result := make([]valueLabel, 0, len(vnics))
	for _, v := range vnics {
		label := strOr(v.DisplayName, "")
		if v.PublicIp != nil && *v.PublicIp != "" {
			label += " (" + *v.PublicIp + ")"
		} else if v.PrivateIp != nil && *v.PrivateIp != "" {
			label += " (" + *v.PrivateIp + ")"
		}
		result = append(result, valueLabel{Label: label, Value: strOr(v.Id, "")})
	}

	jsonOK(w, result)
}

// ── POST /api/traffic/accountStats — aggregated traffic for one account ─

// handleTrafficAccountStats serves every aggregated traffic view: the monthly
// summary (one account, optionally one region) and the account comparison
// (one call per account, no region filter).
func (s *Server) handleTrafficAccountStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		TenantID  int64  `json:"tenant_id"`
		Region    string `json:"region"`
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

	startTime, endTime, errMsg := parseTrafficWindow(req.StartTime, req.EndTime)
	if errMsg != "" {
		jsonErr(w, errMsg)
		return
	}

	var regions []string
	if req.Region != "" {
		if !validRegion.MatchString(req.Region) {
			jsonErr(w, "invalid region: "+req.Region)
			return
		}
		regions = []string{req.Region}
	} else {
		regions, err = s.trafficRegions(r.Context(), tenant)
		if err != nil {
			if errors.Is(err, errNoSubscribedRegions) {
				jsonErr(w, err.Error())
			} else {
				s.clientSafeErr(w, "oci client init failed", err)
			}
			return
		}
	}

	jsonOK(w, s.collectTrafficStats(r.Context(), tenant, regions, startTime, endTime))
}

// validRegion matches OCI region identifiers (e.g. us-phoenix-1, eu-frankfurt-1).
var validRegion = regexp.MustCompile(`^[a-z]{2,}-[a-z]+-\d+$`)
