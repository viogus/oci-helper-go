// Package oci wraps the OCI Go SDK (v65) for compute, VCN, identity, block storage, monitoring, limits, and NLB operations.
package oci

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/identity"
	"github.com/oracle/oci-go-sdk/v65/limits"
	"github.com/oracle/oci-go-sdk/v65/monitoring"
	"github.com/oracle/oci-go-sdk/v65/networkloadbalancer"

	"github.com/viogus/oci-helper-go/internal/db"
)

type Client struct {
	tenant     *db.Tenant
	rawCfg     common.ConfigurationProvider
	compute    core.ComputeClient
	vcn        core.VirtualNetworkClient
	identity   identity.IdentityClient
	bootVolume core.BlockstorageClient
	monitoring monitoring.MonitoringClient
	limits     limits.LimitsClient
	nlb        networkloadbalancer.NetworkLoadBalancerClient
	mu         sync.Mutex
}

func NewClient(t *db.Tenant) (*Client, error) {
	pemData, err := os.ReadFile(t.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("read key file %s: %w", t.KeyFile, err)
	}
	cfg := common.NewRawConfigurationProvider(
		t.TenancyOCID, t.UserOCID, t.Region, t.Fingerprint,
		string(pemData), nil,
	)

	compute, err := core.NewComputeClientWithConfigurationProvider(cfg)
	if err != nil {
		return nil, fmt.Errorf("compute client: %w", err)
	}

	vcn, err := core.NewVirtualNetworkClientWithConfigurationProvider(cfg)
	if err != nil {
		return nil, fmt.Errorf("vcn client: %w", err)
	}

	id, err := identity.NewIdentityClientWithConfigurationProvider(cfg)
	if err != nil {
		return nil, fmt.Errorf("identity client: %w", err)
	}

	bv, err := core.NewBlockstorageClientWithConfigurationProvider(cfg)
	if err != nil {
		return nil, fmt.Errorf("blockstorage client: %w", err)
	}

	mon, err := monitoring.NewMonitoringClientWithConfigurationProvider(cfg)
	if err != nil {
		return nil, fmt.Errorf("monitoring client: %w", err)
	}

	lim, err := limits.NewLimitsClientWithConfigurationProvider(cfg)
	if err != nil {
		return nil, fmt.Errorf("limits client: %w", err)
	}

	nlb, err := networkloadbalancer.NewNetworkLoadBalancerClientWithConfigurationProvider(cfg)
	if err != nil {
		return nil, fmt.Errorf("nlb client: %w", err)
	}

	return &Client{
		tenant:     t,
		rawCfg:     cfg,
		compute:    compute,
		vcn:        vcn,
		identity:   id,
		bootVolume: bv,
		monitoring: mon,
		limits:     lim,
		nlb:        nlb,
	}, nil
}

func (c *Client) ListInstances(ctx context.Context, compartmentID string) ([]core.Instance, error) {
	var all []core.Instance
	page := common.String("")
	for {
		req := core.ListInstancesRequest{
			CompartmentId: common.String(compartmentID),
			Limit:         common.Int(1000),
			Page:          page,
		}
		resp, err := c.compute.ListInstances(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("list instances: %w", err)
		}
		all = append(all, resp.Items...)
		if resp.OpcNextPage == nil || *resp.OpcNextPage == "" {
			break
		}
		page = resp.OpcNextPage
	}
	return all, nil
}

func (c *Client) GetInstance(ctx context.Context, instanceID string) (*core.Instance, error) {
	req := core.GetInstanceRequest{InstanceId: common.String(instanceID)}
	resp, err := c.compute.GetInstance(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("get instance: %w", err)
	}
	return &resp.Instance, nil
}

func (c *Client) InstanceAction(ctx context.Context, instanceID string, action core.InstanceActionActionEnum) (*core.Instance, error) {
	req := core.InstanceActionRequest{
		InstanceId: common.String(instanceID),
		Action:     action,
	}
	resp, err := c.compute.InstanceAction(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("instance action %s: %w", action, err)
	}
	return &resp.Instance, nil
}

func (c *Client) LaunchInstanceWithRequest(ctx context.Context, req core.LaunchInstanceRequest) (*core.Instance, error) {
	resp, err := c.compute.LaunchInstance(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("launch instance: %w", err)
	}
	return &resp.Instance, nil
}

// LaunchInstance creates a compute instance from individual parameters (convenience wrapper).
func (c *Client) LaunchInstance(ctx context.Context, region, ad, shape, imageID, subnetID, displayName string, bootVolumeSizeGB int64) error {
	details := core.LaunchInstanceDetails{
		AvailabilityDomain: common.String(ad),
		CompartmentId:      common.String(c.tenant.TenancyOCID),
		Shape:              common.String(shape),
		DisplayName:        common.String(displayName),
		Metadata:           map[string]string{},
		SourceDetails: core.InstanceSourceViaImageDetails{
			ImageId:             common.String(imageID),
			BootVolumeSizeInGBs: common.Int64(bootVolumeSizeGB),
		},
		CreateVnicDetails: &core.CreateVnicDetails{
			SubnetId: common.String(subnetID),
		},
	}
	// For AMD micro instances, constrain shape config
	if strings.Contains(strings.ToLower(shape), "micro") {
		details.ShapeConfig = &core.LaunchInstanceShapeConfigDetails{
			Ocpus: common.Float32(1),
		}
	}
	req := core.LaunchInstanceRequest{
		LaunchInstanceDetails: details,
	}
	_, err := c.compute.LaunchInstance(ctx, req)
	return err
}

func (c *Client) TerminateInstance(ctx context.Context, instanceID string) error {
	req := core.TerminateInstanceRequest{InstanceId: common.String(instanceID)}
	_, err := c.compute.TerminateInstance(ctx, req)
	return err
}

// withSubtreeInterceptor sets an interceptor on the given embedded BaseClient
// field pointer that adds compartmentIdInSubtree=true to all requests. This
// enables recursive cross-compartment resource listing.
// Returns a cleanup function to restore the previous interceptor (or nil).
func withSubtreeInterceptor(interceptor *common.RequestInterceptor) func() {
	prev := *interceptor
	*interceptor = func(r *http.Request) error {
		q := r.URL.Query()
		q.Set("compartmentIdInSubtree", "true")
		r.URL.RawQuery = q.Encode()
		return nil
	}
	if prev != nil {
		return func() { *interceptor = prev }
	}
	return func() { *interceptor = nil }
}

func (c *Client) ListVCNs(ctx context.Context, compartmentID string) ([]core.Vcn, error) {
	req := core.ListVcnsRequest{
		CompartmentId: common.String(compartmentID),
		Limit:         common.Int(1000),
	}
	defer withSubtreeInterceptor(&c.vcn.Interceptor)()
	resp, err := c.vcn.ListVcns(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.Items, nil
}

func (c *Client) ListSubnets(ctx context.Context, compartmentID, vcnID string) ([]core.Subnet, error) {
	req := core.ListSubnetsRequest{
		CompartmentId: common.String(compartmentID),
		VcnId:         common.String(vcnID),
		Limit:         common.Int(1000),
	}
	defer withSubtreeInterceptor(&c.vcn.Interceptor)()
	resp, err := c.vcn.ListSubnets(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.Items, nil
}

func (c *Client) ListAvailabilityDomains(ctx context.Context, compartmentID string) ([]identity.AvailabilityDomain, error) {
	defer withSubtreeInterceptor(&c.identity.Interceptor)()
	req := identity.ListAvailabilityDomainsRequest{
		CompartmentId: common.String(compartmentID),
	}
	resp, err := c.identity.ListAvailabilityDomains(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.Items, nil
}

func (c *Client) ListImages(ctx context.Context, compartmentID, os string) ([]core.Image, error) {
	defer withSubtreeInterceptor(&c.compute.Interceptor)()
	req := core.ListImagesRequest{
		CompartmentId:   common.String(compartmentID),
		OperatingSystem: common.String(os),
		Limit:           common.Int(100),
		SortBy:          core.ListImagesSortByTimecreated,
		SortOrder:       core.ListImagesSortOrderDesc,
	}
	resp, err := c.compute.ListImages(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.Items, nil
}

// VNIC

func (c *Client) ListVnicAttachments(ctx context.Context, compartmentID, instanceID string) ([]core.VnicAttachment, error) {
	defer withSubtreeInterceptor(&c.compute.Interceptor)()
	req := core.ListVnicAttachmentsRequest{
		CompartmentId: common.String(compartmentID),
		InstanceId:    common.String(instanceID),
		Limit:         common.Int(50),
	}
	resp, err := c.compute.ListVnicAttachments(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.Items, nil
}

func (c *Client) GetVnic(ctx context.Context, vnicID string) (*core.Vnic, error) {
	req := core.GetVnicRequest{VnicId: common.String(vnicID)}
	resp, err := c.vcn.GetVnic(ctx, req)
	if err != nil {
		return nil, err
	}
	return &resp.Vnic, nil
}

// Public IP

func (c *Client) ListPublicIPs(ctx context.Context, compartmentID string) ([]core.PublicIp, error) {
	defer withSubtreeInterceptor(&c.vcn.Interceptor)()
	req := core.ListPublicIpsRequest{
		CompartmentId: common.String(compartmentID),
		Scope:         core.ListPublicIpsScopeRegion,
		Limit:         common.Int(1000),
	}
	resp, err := c.vcn.ListPublicIps(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.Items, nil
}

func (c *Client) CreatePublicIP(ctx context.Context, details core.CreatePublicIpDetails) (*core.PublicIp, error) {
	req := core.CreatePublicIpRequest{CreatePublicIpDetails: details}
	resp, err := c.vcn.CreatePublicIp(ctx, req)
	if err != nil {
		return nil, err
	}
	return &resp.PublicIp, nil
}

func (c *Client) DeletePublicIP(ctx context.Context, publicIPID string) error {
	req := core.DeletePublicIpRequest{PublicIpId: common.String(publicIPID)}
	_, err := c.vcn.DeletePublicIp(ctx, req)
	return err
}

func (c *Client) GetPublicIP(ctx context.Context, publicIPID string) (*core.PublicIp, error) {
	req := core.GetPublicIpRequest{PublicIpId: common.String(publicIPID)}
	resp, err := c.vcn.GetPublicIp(ctx, req)
	if err != nil {
		return nil, err
	}
	return &resp.PublicIp, nil
}

func (c *Client) ListShapes(ctx context.Context, compartmentID, imageID string) ([]core.Shape, error) {
	defer withSubtreeInterceptor(&c.compute.Interceptor)()
	req := core.ListShapesRequest{
		CompartmentId: common.String(compartmentID),
		ImageId:       common.String(imageID),
		Limit:         common.Int(1000),
	}
	resp, err := c.compute.ListShapes(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.Items, nil
}

// Boot Volumes

func (c *Client) ListBootVolumes(ctx context.Context, compartmentID string) ([]core.BootVolume, error) {
	defer withSubtreeInterceptor(&c.bootVolume.Interceptor)()
	req := core.ListBootVolumesRequest{
		CompartmentId: common.String(compartmentID),
		Limit:         common.Int(1000),
	}
	resp, err := c.bootVolume.ListBootVolumes(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.Items, nil
}

func (c *Client) GetBootVolume(ctx context.Context, id string) (*core.BootVolume, error) {
	req := core.GetBootVolumeRequest{BootVolumeId: common.String(id)}
	resp, err := c.bootVolume.GetBootVolume(ctx, req)
	if err != nil {
		return nil, err
	}
	return &resp.BootVolume, nil
}

func (c *Client) UpdateBootVolume(ctx context.Context, id string, sizeInGBs int64, displayName string) (*core.BootVolume, error) {
	details := core.UpdateBootVolumeDetails{}
	if sizeInGBs > 0 {
		details.SizeInGBs = common.Int64(sizeInGBs)
	}
	if displayName != "" {
		details.DisplayName = common.String(displayName)
	}
	req := core.UpdateBootVolumeRequest{
		BootVolumeId:            common.String(id),
		UpdateBootVolumeDetails: details,
	}
	resp, err := c.bootVolume.UpdateBootVolume(ctx, req)
	if err != nil {
		return nil, err
	}
	return &resp.BootVolume, nil
}

func (c *Client) ListBootVolumeAttachments(ctx context.Context, compartmentID, instanceID string) ([]core.BootVolumeAttachment, error) {
	defer withSubtreeInterceptor(&c.compute.Interceptor)()
	req := core.ListBootVolumeAttachmentsRequest{
		CompartmentId: common.String(compartmentID),
		InstanceId:    common.String(instanceID),
		Limit:         common.Int(50),
	}
	resp, err := c.compute.ListBootVolumeAttachments(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.Items, nil
}

func (c *Client) AttachBootVolume(ctx context.Context, bootVolumeID, instanceID string) (*core.BootVolumeAttachment, error) {
	req := core.AttachBootVolumeRequest{
		AttachBootVolumeDetails: core.AttachBootVolumeDetails{
			BootVolumeId: common.String(bootVolumeID),
			InstanceId:   common.String(instanceID),
		},
	}
	resp, err := c.compute.AttachBootVolume(ctx, req)
	if err != nil {
		return nil, err
	}
	return &resp.BootVolumeAttachment, nil
}

func (c *Client) DetachBootVolume(ctx context.Context, attachmentID string) error {
	req := core.DetachBootVolumeRequest{
		BootVolumeAttachmentId: common.String(attachmentID),
	}
	_, err := c.compute.DetachBootVolume(ctx, req)
	return err
}

// --- Instance Mutations ---

func (c *Client) UpdateInstance(ctx context.Context, instanceID, shape string, ocpus, memoryGB float32) error {
	details := core.UpdateInstanceDetails{}
	if shape != "" {
		details.Shape = common.String(shape)
	}
	if ocpus > 0 || memoryGB > 0 {
		details.ShapeConfig = &core.UpdateInstanceShapeConfigDetails{}
		if ocpus > 0 {
			details.ShapeConfig.Ocpus = common.Float32(ocpus)
		}
		if memoryGB > 0 {
			details.ShapeConfig.MemoryInGBs = common.Float32(memoryGB)
		}
	}
	req := core.UpdateInstanceRequest{
		InstanceId:            common.String(instanceID),
		UpdateInstanceDetails: details,
	}
	_, err := c.compute.UpdateInstance(ctx, req)
	return err
}

func (c *Client) UpdateInstanceDisplayName(ctx context.Context, instanceID, displayName string) error {
	details := core.UpdateInstanceDetails{
		DisplayName: common.String(displayName),
	}
	req := core.UpdateInstanceRequest{
		InstanceId:            common.String(instanceID),
		UpdateInstanceDetails: details,
	}
	_, err := c.compute.UpdateInstance(ctx, req)
	return err
}

func (c *Client) GetBootVolumeAttachment(ctx context.Context, compartmentID, instanceID string) (*core.BootVolumeAttachment, error) {
	defer withSubtreeInterceptor(&c.compute.Interceptor)()
	req := core.ListBootVolumeAttachmentsRequest{
		CompartmentId: common.String(compartmentID),
		InstanceId:    common.String(instanceID),
		Limit:         common.Int(10),
	}
	resp, err := c.compute.ListBootVolumeAttachments(ctx, req)
	if err != nil {
		return nil, err
	}
	if len(resp.Items) == 0 {
		return nil, fmt.Errorf("no boot volume attached")
	}
	return &resp.Items[0], nil
}

func (c *Client) GetInstanceVNICs(ctx context.Context, compartmentID, instanceID string) ([]core.Vnic, error) {
	defer withSubtreeInterceptor(&c.compute.Interceptor)()
	attachments, err := c.ListVnicAttachments(ctx, compartmentID, instanceID)
	if err != nil {
		return nil, err
	}
	var vnics []core.Vnic
	for _, att := range attachments {
		if att.VnicId == nil {
			continue
		}
		vnic, err := c.GetVnic(ctx, *att.VnicId)
		if err != nil {
			return nil, err
		}
		vnics = append(vnics, *vnic)
	}
	return vnics, nil
}

func (c *Client) AssignIPv6(ctx context.Context, vnicID string) error {
	req := core.CreateIpv6Request{
		CreateIpv6Details: core.CreateIpv6Details{
			VnicId: common.String(vnicID),
		},
	}
	_, err := c.vcn.CreateIpv6(ctx, req)
	return err
}

// Metrics

type MetricValue struct {
	Value *float64 `json:"value,omitempty"`
	Unit  string   `json:"unit"`
	Error string   `json:"error,omitempty"`
}

type InstanceMetrics struct {
	CPU        MetricValue `json:"cpu"`
	Memory     MetricValue `json:"memory"`
	NetworkIn  MetricValue `json:"networkIn"`
	NetworkOut MetricValue `json:"networkOut"`
	DiskRead   MetricValue `json:"diskRead"`
	DiskWrite  MetricValue `json:"diskWrite"`
	Updated    time.Time   `json:"updated"`
}

func (c *Client) GetMetrics(ctx context.Context, compartmentID, instanceID string) (*InstanceMetrics, error) {
	log.Printf("[GetMetrics] compartment=%s instance=%s", compartmentID, instanceID)
	type queryDef struct {
		key  string
		name string
		unit string
	}

	queries := []queryDef{
		{"cpu", "CpuUtilization", "%"},
		{"memory", "MemoryUtilization", "%"},
		{"networkIn", "NetworkBytesIn", "bytes/s"},
		{"networkOut", "NetworkBytesOut", "bytes/s"},
		{"diskRead", "DiskBytesRead", "bytes/s"},
		{"diskWrite", "DiskBytesWrite", "bytes/s"},
	}

	results := make(map[string]MetricValue)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for _, q := range queries {
		wg.Add(1)
		q := q
		go func() {
			defer wg.Done()

			mv := MetricValue{Unit: q.unit}
			req := monitoring.SummarizeMetricsDataRequest{
				CompartmentId: common.String(compartmentID),
				SummarizeMetricsDataDetails: monitoring.SummarizeMetricsDataDetails{
					Namespace: common.String("oci_computeagent"),
					Query:     common.String(fmt.Sprintf(`%s[1m]{instanceId="%s"}.mean()`, q.name, instanceID)),
					StartTime: &common.SDKTime{Time: time.Now().Add(-30 * time.Minute)},
					EndTime:   &common.SDKTime{Time: time.Now()},
				},
			}
			resp, err := c.monitoring.SummarizeMetricsData(ctx, req)
			if err != nil {
				mv.Error = err.Error()
				log.Printf("[GetMetrics] %s ERROR: %v", q.name, err)
			} else if len(resp.Items) > 0 && len(resp.Items[0].AggregatedDatapoints) > 0 && resp.Items[0].AggregatedDatapoints[0].Value != nil {
				log.Printf("[GetMetrics] %s value=%.2f items=%d", q.name, *resp.Items[0].AggregatedDatapoints[0].Value, len(resp.Items))
				mv.Value = resp.Items[0].AggregatedDatapoints[0].Value
			} else {
				log.Printf("[GetMetrics] %s no data items=%d", q.name, len(resp.Items))
			}

			mu.Lock()
			results[q.key] = mv
			mu.Unlock()
		}()
	}

	wg.Wait()

	m := &InstanceMetrics{Updated: time.Now()}
	if v, ok := results["cpu"]; ok {
		m.CPU = v
	}
	if v, ok := results["memory"]; ok {
		m.Memory = v
	}
	if v, ok := results["networkIn"]; ok {
		m.NetworkIn = v
	}
	if v, ok := results["networkOut"]; ok {
		m.NetworkOut = v
	}
	if v, ok := results["diskRead"]; ok {
		m.DiskRead = v
	}
	if v, ok := results["diskWrite"]; ok {
		m.DiskWrite = v
	}
	return m, nil
}

// Traffic

type TrafficDataPoint struct {
	Timestamp        string  `json:"timestamp"`
	BytesInPerSec    float64 `json:"bytesInPerSec"`
	BytesOutPerSec   float64 `json:"bytesOutPerSec"`
	PacketsInPerSec  float64 `json:"packetsInPerSec"`
	PacketsOutPerSec float64 `json:"packetsOutPerSec"`
}

// intervalForDuration returns an OCI monitoring query interval that is
// compatible with the given time range. 1-minute intervals are limited
// to roughly 7 days, 5-minute to 30 days, and 1-hour beyond that.
func intervalForDuration(d time.Duration) (string, time.Duration) {
	const (
		day    = 24 * time.Hour
		seven  = 7 * day
		thirty = 30 * day
	)
	switch {
	case d <= seven:
		return "[1m]", time.Minute
	case d <= thirty:
		return "[5m]", 5 * time.Minute
	default:
		return "[1h]", time.Hour
	}
}

// minDataPoints estimates the number of data points for a given duration and
// aggregation step. Used to synthesise a regular interval in the response.
func minDataPoints(totalDuration, step time.Duration) int {
	if step <= 0 {
		return 0
	}
	n := int(totalDuration / step)
	if n < 1 {
		n = 1
	}
	return n
}

func (c *Client) GetVNICTtraffic(ctx context.Context, compartmentID, vnicID string, startTime, endTime time.Time) ([]TrafficDataPoint, error) {
	namespace := "oci_vcn"

	results := make(map[string][]float64)
	metricNames := []string{"VnicBytesIn", "VnicBytesOut", "VnicPacketsIn", "VnicPacketsOut"}

	totalDuration := endTime.Sub(startTime)
	intervalStr, step := intervalForDuration(totalDuration)
	log.Printf("[GetVNICTtraffic] compartment=%s range=%v interval=%s step=%v", compartmentID, totalDuration, intervalStr, step)

	for _, name := range metricNames {
		req := monitoring.SummarizeMetricsDataRequest{
			CompartmentId: common.String(compartmentID),
			SummarizeMetricsDataDetails: monitoring.SummarizeMetricsDataDetails{
				Namespace: common.String(namespace),
				Query:     common.String(fmt.Sprintf("%s%s{resourceId=\"%s\"}.mean()", name, intervalStr, vnicID)),
				StartTime: &common.SDKTime{Time: startTime},
				EndTime:   &common.SDKTime{Time: endTime},
			},
		}
		resp, err := c.monitoring.SummarizeMetricsData(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("%s query: %w", name, err)
		}
		var values []float64
		for _, item := range resp.Items {
			if item.AggregatedDatapoints != nil {
				for _, dp := range item.AggregatedDatapoints {
					if dp.Value != nil {
						values = append(values, *dp.Value)
					}
				}
			}
		}
		log.Printf("[GetVNICTtraffic] %s items=%d datapoints=%d", name, len(resp.Items), len(values))
		results[name] = values
	}

	// Build aligned time series using the chosen step
	maxLen := 0
	for _, v := range results {
		if len(v) > maxLen {
			maxLen = len(v)
		}
	}
	var data []TrafficDataPoint
	for i := 0; i < maxLen; i++ {
		dp := TrafficDataPoint{
			Timestamp: startTime.Add(time.Duration(i) * step).Format(time.RFC3339),
		}
		if vals := results["VnicBytesIn"]; i < len(vals) {
			dp.BytesInPerSec = vals[i]
		}
		if vals := results["VnicBytesOut"]; i < len(vals) {
			dp.BytesOutPerSec = vals[i]
		}
		if vals := results["VnicPacketsIn"]; i < len(vals) {
			dp.PacketsInPerSec = vals[i]
		}
		if vals := results["VnicPacketsOut"]; i < len(vals) {
			dp.PacketsOutPerSec = vals[i]
		}
		data = append(data, dp)
	}
	log.Printf("[GetVNICTtraffic] returned %d data points (step=%v)", len(data), step)
	return data, nil
}

// --- Security Rules ---

type SecurityRuleInfo struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Protocol string `json:"protocol"`
	Source   string `json:"source"`
	Dest     string `json:"dest"`
	Port     string `json:"port"`
	Type     string `json:"type"` // "ingress" or "egress"
}

func portRangeKey(opts *core.TcpOptions) string {
	if opts == nil || opts.DestinationPortRange == nil {
		return "all"
	}
	min := *opts.DestinationPortRange.Min
	max := *opts.DestinationPortRange.Max
	if min == max {
		return strconv.Itoa(min)
	}
	return strconv.Itoa(min) + "-" + strconv.Itoa(max)
}

func securityRuleID(slID, direction string, protocol, source, dest *string, tcpOpts *core.TcpOptions) string {
	id := slID + "/" + direction + "/"
	if protocol != nil {
		id += *protocol
	} else {
		id += "all"
	}
	if direction == "ingress" && source != nil {
		id += "/" + strings.ReplaceAll(*source, "/", "_")
	} else if direction == "egress" && dest != nil {
		id += "/" + strings.ReplaceAll(*dest, "/", "_")
	} else {
		id += "/0.0.0.0/0"
	}
	if protocol != nil && (*protocol == "TCP" || *protocol == "UDP") {
		id += "/" + portRangeKey(tcpOpts)
	}
	return id
}

func (c *Client) ListSecurityRules(ctx context.Context, vcnID, keyword string, page, size int) ([]SecurityRuleInfo, int64, error) {
	defer withSubtreeInterceptor(&c.vcn.Interceptor)()
	compartmentID := c.tenant.TenancyOCID
	req := core.ListSecurityListsRequest{
		CompartmentId: common.String(compartmentID),
		VcnId:         common.String(vcnID),
		Limit:         common.Int(100),
	}
	resp, err := c.vcn.ListSecurityLists(ctx, req)
	if err != nil {
		return nil, 0, err
	}

	var all []SecurityRuleInfo
	for _, sl := range resp.Items {
		for _, rule := range sl.IngressSecurityRules {
			info := SecurityRuleInfo{
				ID:       securityRuleID(*sl.Id, "ingress", rule.Protocol, rule.Source, nil, rule.TcpOptions),
				Name:     *sl.DisplayName,
				Protocol: "",
				Source:   "",
				Port:     "",
				Type:     "ingress",
			}
			if rule.Protocol != nil {
				info.Protocol = *rule.Protocol
			}
			if rule.Source != nil {
				info.Source = *rule.Source
			}
			if rule.TcpOptions != nil && rule.TcpOptions.DestinationPortRange != nil {
				info.Port = strconv.Itoa(*rule.TcpOptions.DestinationPortRange.Min) + "-" + strconv.Itoa(*rule.TcpOptions.DestinationPortRange.Max)
			}
			all = append(all, info)
		}
		for _, rule := range sl.EgressSecurityRules {
			info := SecurityRuleInfo{
				ID:       securityRuleID(*sl.Id, "egress", rule.Protocol, nil, rule.Destination, rule.TcpOptions),
				Name:     *sl.DisplayName,
				Protocol: "",
				Dest:     "",
				Port:     "",
				Type:     "egress",
			}
			if rule.Protocol != nil {
				info.Protocol = *rule.Protocol
			}
			if rule.Destination != nil {
				info.Dest = *rule.Destination
			}
			if rule.TcpOptions != nil && rule.TcpOptions.DestinationPortRange != nil {
				info.Port = strconv.Itoa(*rule.TcpOptions.DestinationPortRange.Min) + "-" + strconv.Itoa(*rule.TcpOptions.DestinationPortRange.Max)
			}
			all = append(all, info)
		}
	}

	// filter by keyword
	kw := strings.ToLower(keyword)
	var filtered []SecurityRuleInfo
	for _, r := range all {
		if kw == "" || strings.Contains(strings.ToLower(r.Protocol), kw) ||
			strings.Contains(strings.ToLower(r.Source), kw) || strings.Contains(strings.ToLower(r.Port), kw) {
			filtered = append(filtered, r)
		}
	}
	total := int64(len(filtered))

	// paginate
	start := (page - 1) * size
	if start >= len(filtered) {
		return []SecurityRuleInfo{}, total, nil
	}
	end := start + size
	if end > len(filtered) {
		end = len(filtered)
	}
	return filtered[start:end], total, nil
}

func (c *Client) AddIngressRule(ctx context.Context, vcnID, protocol, port, source string) error {
	req := core.ListSecurityListsRequest{
		CompartmentId: common.String(c.tenant.TenancyOCID),
		VcnId:         common.String(vcnID),
		Limit:         common.Int(1),
	}
	resp, err := c.vcn.ListSecurityLists(ctx, req)
	if err != nil {
		return err
	}
	if len(resp.Items) == 0 {
		return fmt.Errorf("no security list found")
	}

	sl := resp.Items[0]
	ingressRules := sl.IngressSecurityRules
	newRule := core.IngressSecurityRule{
		Protocol: common.String(protocol),
		Source:   common.String(source),
	}
	if protocol == "TCP" || protocol == "UDP" {
		parts := strings.Split(port, "-")
		minPort, _ := strconv.Atoi(parts[0])
		maxPort := minPort
		if len(parts) > 1 {
			maxPort, _ = strconv.Atoi(parts[1])
		}
		newRule.TcpOptions = &core.TcpOptions{
			DestinationPortRange: &core.PortRange{
				Min: common.Int(minPort),
				Max: common.Int(maxPort),
			},
		}
	}
	ingressRules = append(ingressRules, newRule)

	updateReq := core.UpdateSecurityListRequest{
		SecurityListId: sl.Id,
		UpdateSecurityListDetails: core.UpdateSecurityListDetails{
			IngressSecurityRules: ingressRules,
			EgressSecurityRules:  sl.EgressSecurityRules,
		},
	}
	_, err = c.vcn.UpdateSecurityList(ctx, updateReq)
	return err
}

func (c *Client) AddEgressRule(ctx context.Context, vcnID, protocol, port, dest string) error {
	req := core.ListSecurityListsRequest{
		CompartmentId: common.String(c.tenant.TenancyOCID),
		VcnId:         common.String(vcnID),
		Limit:         common.Int(1),
	}
	resp, err := c.vcn.ListSecurityLists(ctx, req)
	if err != nil {
		return err
	}
	if len(resp.Items) == 0 {
		return fmt.Errorf("no security list found")
	}

	sl := resp.Items[0]
	egressRules := sl.EgressSecurityRules
	newRule := core.EgressSecurityRule{
		Protocol:    common.String(protocol),
		Destination: common.String(dest),
	}
	if protocol == "TCP" || protocol == "UDP" {
		parts := strings.Split(port, "-")
		minPort, _ := strconv.Atoi(parts[0])
		maxPort := minPort
		if len(parts) > 1 {
			maxPort, _ = strconv.Atoi(parts[1])
		}
		newRule.TcpOptions = &core.TcpOptions{
			DestinationPortRange: &core.PortRange{
				Min: common.Int(minPort),
				Max: common.Int(maxPort),
			},
		}
	}
	egressRules = append(egressRules, newRule)

	updateReq := core.UpdateSecurityListRequest{
		SecurityListId: sl.Id,
		UpdateSecurityListDetails: core.UpdateSecurityListDetails{
			IngressSecurityRules: sl.IngressSecurityRules,
			EgressSecurityRules:  egressRules,
		},
	}
	_, err = c.vcn.UpdateSecurityList(ctx, updateReq)
	return err
}

type ruleFilter struct {
	direction    string
	protocol     string
	sourceOrDest string
	portMin      int
	portMax      int
	hasPort      bool
}

func (c *Client) RemoveSecurityRules(ctx context.Context, vcnID string, ruleIDs []string) error {
	if len(ruleIDs) == 0 {
		return fmt.Errorf("no rule IDs provided")
	}

	// Parse rule IDs: {slId}/{direction}/{protocol}[/{sourceOrDest}[/{portMin}[-{portMax}]]]
	filtersBySL := make(map[string][]ruleFilter)
	for _, rid := range ruleIDs {
		parts := strings.SplitN(rid, "/", 6)
		if len(parts) < 3 {
			return fmt.Errorf("invalid rule ID format: %s", rid)
		}
		slID := parts[0]
		f := ruleFilter{direction: parts[1], protocol: parts[2]}
		if len(parts) >= 5 {
			f.sourceOrDest = parts[3]
			if parts[4] != "all" {
				f.hasPort = true
				portParts := strings.SplitN(parts[4], "-", 2)
				f.portMin, _ = strconv.Atoi(portParts[0])
				if len(portParts) > 1 {
					f.portMax, _ = strconv.Atoi(portParts[1])
				} else {
					f.portMax = f.portMin
				}
			}
		}
		filtersBySL[slID] = append(filtersBySL[slID], f)
	}

	// List all security lists for the VCN
	req := core.ListSecurityListsRequest{
		CompartmentId: common.String(c.tenant.TenancyOCID),
		VcnId:         common.String(vcnID),
		Limit:         common.Int(100),
	}
	resp, err := c.vcn.ListSecurityLists(ctx, req)
	if err != nil {
		return fmt.Errorf("list security lists: %w", err)
	}

	for _, sl := range resp.Items {
		slID := *sl.Id
		filters, ok := filtersBySL[slID]
		if !ok {
			continue
		}

		// Filter ingress rules
		var newIngress []core.IngressSecurityRule
		for _, rule := range sl.IngressSecurityRules {
			if !matchesIngressFilter(rule, filters) {
				newIngress = append(newIngress, rule)
			}
		}

		// Filter egress rules
		var newEgress []core.EgressSecurityRule
		for _, rule := range sl.EgressSecurityRules {
			if !matchesEgressFilter(rule, filters) {
				newEgress = append(newEgress, rule)
			}
		}

		updateReq := core.UpdateSecurityListRequest{
			SecurityListId: sl.Id,
			UpdateSecurityListDetails: core.UpdateSecurityListDetails{
				IngressSecurityRules: newIngress,
				EgressSecurityRules:  newEgress,
			},
		}
		if _, err := c.vcn.UpdateSecurityList(ctx, updateReq); err != nil {
			return fmt.Errorf("update security list %s: %w", slID, err)
		}
	}
	return nil
}

func matchesIngressFilter(rule core.IngressSecurityRule, filters []ruleFilter) bool {
	for _, f := range filters {
		if f.direction != "ingress" {
			continue
		}
		if rule.Protocol == nil || *rule.Protocol != f.protocol {
			continue
		}
		if f.sourceOrDest != "" {
			if rule.Source == nil || *rule.Source != f.sourceOrDest {
				continue
			}
		}
		if f.hasPort {
			if rule.TcpOptions == nil || rule.TcpOptions.DestinationPortRange == nil {
				continue
			}
			min := *rule.TcpOptions.DestinationPortRange.Min
			max := *rule.TcpOptions.DestinationPortRange.Max
			if min != f.portMin || max != f.portMax {
				continue
			}
		}
		return true
	}
	return false
}

func matchesEgressFilter(rule core.EgressSecurityRule, filters []ruleFilter) bool {
	for _, f := range filters {
		if f.direction != "egress" {
			continue
		}
		if rule.Protocol == nil || *rule.Protocol != f.protocol {
			continue
		}
		if f.sourceOrDest != "" {
			if rule.Destination == nil || *rule.Destination != f.sourceOrDest {
				continue
			}
		}
		if f.hasPort {
			if rule.TcpOptions == nil || rule.TcpOptions.DestinationPortRange == nil {
				continue
			}
			min := *rule.TcpOptions.DestinationPortRange.Min
			max := *rule.TcpOptions.DestinationPortRange.Max
			if min != f.portMin || max != f.portMax {
				continue
			}
		}
		return true
	}
	return false
}

func (c *Client) ReleaseAllPorts(ctx context.Context, vcnID string) error {
	req := core.ListSecurityListsRequest{
		CompartmentId: common.String(c.tenant.TenancyOCID),
		VcnId:         common.String(vcnID),
		Limit:         common.Int(100),
	}
	resp, err := c.vcn.ListSecurityLists(ctx, req)
	if err != nil {
		return err
	}
	for _, sl := range resp.Items {
		ingressRules := sl.IngressSecurityRules
		ingressRules = append(ingressRules, core.IngressSecurityRule{
			Protocol: common.String("all"),
			Source:   common.String("0.0.0.0/0"),
		})
		updateReq := core.UpdateSecurityListRequest{
			SecurityListId: sl.Id,
			UpdateSecurityListDetails: core.UpdateSecurityListDetails{
				IngressSecurityRules: ingressRules,
				EgressSecurityRules:  sl.EgressSecurityRules,
			},
		}
		if _, err := c.vcn.UpdateSecurityList(ctx, updateReq); err != nil {
			return err
		}
	}
	return nil
}

// --- Limits ---

type LimitInfo struct {
	ServiceName string `json:"serviceName"`
	Name        string `json:"name"`
	Used        int64  `json:"used"`
	Available   int64  `json:"available"`
	Max         int64  `json:"max"`
}

func (c *Client) GetLimits(ctx context.Context, tenantID int64, serviceName string) ([]LimitInfo, error) {
	defer withSubtreeInterceptor(&c.limits.Interceptor)()
	compartmentID := c.tenant.TenancyOCID
	req := limits.ListLimitDefinitionsRequest{
		CompartmentId: common.String(compartmentID),
		ServiceName:   common.String(serviceName),
		Limit:         common.Int(100),
	}
	resp, err := c.limits.ListLimitDefinitions(ctx, req)
	if err != nil {
		return nil, err
	}
	var result []LimitInfo
	for _, def := range resp.Items {
		valReq := limits.GetResourceAvailabilityRequest{
			ServiceName:        common.String(serviceName),
			LimitName:          def.Name,
			CompartmentId:      common.String(compartmentID),
			AvailabilityDomain: common.String(c.tenant.Region),
		}
		valResp, err := c.limits.GetResourceAvailability(ctx, valReq)
		if err != nil {
			continue
		}
		info := LimitInfo{
			ServiceName: *def.ServiceName,
			Name:        *def.Name,
		}
		if valResp.Available != nil {
			info.Available = *valResp.Available
		}
		if valResp.Used != nil {
			info.Used = *valResp.Used
		}
		if valResp.EffectiveQuotaValue != nil {
			info.Max = int64(*valResp.EffectiveQuotaValue)
		}
		result = append(result, info)
	}
	return result, nil
}

// --- One-Click 500Mbps ---

// Enable500Mbps creates a Network Load Balancer for the instance to achieve 500Mbps bandwidth.
// Note: This requires the OCI network load balancer API which may need additional SDK imports.
func (c *Client) Enable500Mbps(ctx context.Context, instanceID string) error {
	compartmentID := c.tenant.TenancyOCID
	vnics, err := c.GetInstanceVNICs(ctx, compartmentID, instanceID)
	if err != nil {
		return fmt.Errorf("get vnics: %w", err)
	}
	if len(vnics) == 0 {
		return fmt.Errorf("no VNIC found")
	}
	vnic := vnics[0]
	privateIP := ""
	if vnic.PrivateIp != nil {
		privateIP = *vnic.PrivateIp
	}
	subnetID := ""
	if vnic.SubnetId != nil {
		subnetID = *vnic.SubnetId
	}
	if privateIP == "" || subnetID == "" {
		return fmt.Errorf("instance VNIC missing private IP or subnet ID")
	}

	displayName := "nlb-" + instanceID
	if len(displayName) > 100 {
		displayName = displayName[:100]
	}
	bsName := "bs-" + instanceID

	createReq := networkloadbalancer.CreateNetworkLoadBalancerRequest{
		CreateNetworkLoadBalancerDetails: networkloadbalancer.CreateNetworkLoadBalancerDetails{
			CompartmentId:               common.String(compartmentID),
			DisplayName:                 common.String(displayName),
			SubnetId:                    common.String(subnetID),
			IsPreserveSourceDestination: common.Bool(true),
			IsPrivate:                   common.Bool(false),
			FreeformTags:                map[string]string{"oci-helper-instance-id": instanceID},
			BackendSets: map[string]networkloadbalancer.BackendSetDetails{
				bsName: {
					Policy: networkloadbalancer.NetworkLoadBalancingPolicyFiveTuple,
					HealthChecker: &networkloadbalancer.HealthChecker{
						Protocol: networkloadbalancer.HealthCheckProtocolsTcp,
						Port:     common.Int(22),
					},
					Backends: []networkloadbalancer.Backend{
						{IpAddress: common.String(privateIP), Port: common.Int(22), Name: common.String(instanceID)},
					},
				},
			},
			Listeners: map[string]networkloadbalancer.ListenerDetails{
				"listener-all": {
					Name:                  common.String("listener-all"),
					DefaultBackendSetName: common.String(bsName),
					Port:                  common.Int(0),
					Protocol:              networkloadbalancer.ListenerProtocolsAny,
				},
			},
		},
	}
	resp, err := c.nlb.CreateNetworkLoadBalancer(ctx, createReq)
	if err != nil {
		return fmt.Errorf("create NLB: %w", err)
	}
	workReqID := resp.OpcWorkRequestId
	if workReqID == nil || *workReqID == "" {
		return fmt.Errorf("NLB created but no work request ID returned")
	}
	log.Printf("[Enable500Mbps] NLB work request: %s", *workReqID)

	pollInterval := 5 * time.Second
	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		wr, err := c.nlb.GetWorkRequest(ctx, networkloadbalancer.GetWorkRequestRequest{WorkRequestId: workReqID})
		if err != nil {
			return fmt.Errorf("poll NLB work request: %w", err)
		}
		status := wr.Status
		pct := float32(0)
		if wr.PercentComplete != nil {
			pct = *wr.PercentComplete
		}
		log.Printf("[Enable500Mbps] status=%s %.0f%%", status, float64(pct))
		switch status {
		case networkloadbalancer.OperationStatusSucceeded:
			return nil
		case networkloadbalancer.OperationStatusFailed:
			return fmt.Errorf("NLB creation failed")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
	}
	return fmt.Errorf("NLB creation timed out")
}

// Disable500Mbps deletes the NLB associated with the instance.
func (c *Client) Disable500Mbps(ctx context.Context, instanceID string) error {
	compartmentID := c.tenant.TenancyOCID

	listReq := networkloadbalancer.ListNetworkLoadBalancersRequest{
		CompartmentId: common.String(compartmentID),
		Limit:         common.Int(100),
	}
	listResp, err := c.nlb.ListNetworkLoadBalancers(ctx, listReq)
	if err != nil {
		return fmt.Errorf("list NLBs: %w", err)
	}

	var nlbID *string
	for _, nlb := range listResp.Items {
		if nlb.FreeformTags != nil && nlb.FreeformTags["oci-helper-instance-id"] == instanceID {
			nlbID = nlb.Id
			break
		}
	}
	if nlbID == nil {
		return fmt.Errorf("no NLB found for instance %s", instanceID)
	}

	delResp, err := c.nlb.DeleteNetworkLoadBalancer(ctx, networkloadbalancer.DeleteNetworkLoadBalancerRequest{
		NetworkLoadBalancerId: nlbID,
	})
	if err != nil {
		return fmt.Errorf("delete NLB: %w", err)
	}
	workReqID := delResp.OpcWorkRequestId
	if workReqID == nil || *workReqID == "" {
		return nil
	}
	log.Printf("[Disable500Mbps] NLB delete work request: %s", *workReqID)

	pollInterval := 5 * time.Second
	deadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) {
		wr, err := c.nlb.GetWorkRequest(ctx, networkloadbalancer.GetWorkRequestRequest{WorkRequestId: workReqID})
		if err != nil {
			return fmt.Errorf("poll NLB delete: %w", err)
		}
		status := wr.Status
		switch status {
		case networkloadbalancer.OperationStatusSucceeded:
			log.Printf("[Disable500Mbps] NLB deleted")
			return nil
		case networkloadbalancer.OperationStatusFailed:
			return fmt.Errorf("NLB deletion failed")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(pollInterval):
		}
	}
	return fmt.Errorf("NLB deletion timed out")
}

// ChangeInstanceIP replaces the ephemeral public IP of an instance.
func (c *Client) ChangeInstanceIP(ctx context.Context, instanceID string, cidrList []string) (string, error) {
	// Get current VNIC
	attReq := core.ListVnicAttachmentsRequest{
		CompartmentId: common.String(c.tenant.TenancyOCID),
		InstanceId:    common.String(instanceID),
		Limit:         common.Int(10),
	}
	attResp, err := c.compute.ListVnicAttachments(ctx, attReq)
	if err != nil {
		return "", fmt.Errorf("list vnic attachments: %w", err)
	}
	if len(attResp.Items) == 0 {
		return "", fmt.Errorf("no VNIC attached")
	}

	vnicID := attResp.Items[0].VnicId
	vnicReq := core.GetVnicRequest{VnicId: vnicID}
	vnicResp, err := c.vcn.GetVnic(ctx, vnicReq)
	if err != nil {
		return "", fmt.Errorf("get vnic: %w", err)
	}

	// Get current public IP
	oldIP := ""
	if vnicResp.PublicIp != nil {
		oldIP = *vnicResp.PublicIp
	}

	// If no public IP, create one
	if oldIP == "" {
		return "", fmt.Errorf("instance has no public IP to replace")
	}

	// Find the private IP OCID for this VNIC to use when creating the new public IP
	privReq := core.ListPrivateIpsRequest{
		VnicId: vnicID,
		Limit:  common.Int(10),
	}
	privResp, err := c.vcn.ListPrivateIps(ctx, privReq)
	if err != nil {
		return "", fmt.Errorf("list private IPs: %w", err)
	}
	var privateIPID *string
	for _, p := range privResp.Items {
		if p.IsPrimary != nil && *p.IsPrimary {
			privateIPID = p.Id
			break
		}
	}
	if privateIPID == nil {
		return "", fmt.Errorf("no primary private IP found for VNIC")
	}

	// Delete old public IP
	// Find the public IP OCID by listing
	pipReq := core.ListPublicIpsRequest{
		Scope:         core.ListPublicIpsScopeRegion,
		CompartmentId: common.String(c.tenant.TenancyOCID),
		Limit:         common.Int(100),
	}
	pipResp, err := c.vcn.ListPublicIps(ctx, pipReq)
	if err != nil {
		return "", fmt.Errorf("list public IPs: %w", err)
	}

	var oldIPID string
	for _, ip := range pipResp.Items {
		if ip.IpAddress != nil && *ip.IpAddress == oldIP {
			oldIPID = *ip.Id
			break
		}
	}
	if oldIPID == "" {
		return "", fmt.Errorf("public IP not found in tenancy")
	}

	// Delete the old public IP
	delReq := core.DeletePublicIpRequest{PublicIpId: common.String(oldIPID)}
	if _, err := c.vcn.DeletePublicIp(ctx, delReq); err != nil {
		return "", fmt.Errorf("delete old IP: %w", err)
	}

	// Create new public IP
	createReq := core.CreatePublicIpRequest{
		CreatePublicIpDetails: core.CreatePublicIpDetails{
			CompartmentId: common.String(c.tenant.TenancyOCID),
			Lifetime:      core.CreatePublicIpDetailsLifetimeEphemeral,
			PrivateIpId:   privateIPID,
		},
	}
	createResp, err := c.vcn.CreatePublicIp(ctx, createReq)
	if err != nil {
		return "", fmt.Errorf("create new IP: %w", err)
	}

	newIP := ""
	if createResp.PublicIp.IpAddress != nil {
		newIP = *createResp.PublicIp.IpAddress
	}

	// Check CIDR filter if specified
	if len(cidrList) > 0 {
		matched := false
		for _, cidr := range cidrList {
			if ipInCIDR(newIP, cidr) {
				matched = true
				break
			}
		}
		if !matched {
			return "", fmt.Errorf("new IP %s not in desired CIDR ranges: %v", newIP, cidrList)
		}
	}

	return newIP, nil
}

// ipInCIDR checks if an IP is in a CIDR range
func ipInCIDR(ipStr, cidrStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	_, cidr, err := net.ParseCIDR(cidrStr)
	if err != nil {
		return false
	}
	return cidr.Contains(ip)
}

// ── Console Connection ────────────────────────────────────────────────

const (
	consoleConnectionPollInterval = 5 * time.Second
	consoleConnectionTimeout      = 2 * time.Minute
)

func (c *Client) CreateConsoleConnection(ctx context.Context, instanceID, publicKey string) (*core.InstanceConsoleConnection, error) {
	req := core.CreateInstanceConsoleConnectionRequest{
		CreateInstanceConsoleConnectionDetails: core.CreateInstanceConsoleConnectionDetails{
			InstanceId: common.String(instanceID),
			PublicKey:  common.String(publicKey),
		},
	}
	resp, err := c.compute.CreateInstanceConsoleConnection(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("create console connection: %w", err)
	}
	return &resp.InstanceConsoleConnection, nil
}

func (c *Client) GetConsoleConnection(ctx context.Context, connectionID string) (*core.InstanceConsoleConnection, error) {
	req := core.GetInstanceConsoleConnectionRequest{
		InstanceConsoleConnectionId: common.String(connectionID),
	}
	resp, err := c.compute.GetInstanceConsoleConnection(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("get console connection: %w", err)
	}
	return &resp.InstanceConsoleConnection, nil
}

func (c *Client) DeleteConsoleConnection(ctx context.Context, connectionID string) error {
	req := core.DeleteInstanceConsoleConnectionRequest{
		InstanceConsoleConnectionId: common.String(connectionID),
	}
	_, err := c.compute.DeleteInstanceConsoleConnection(ctx, req)
	return err
}

func (c *Client) ListConsoleConnections(ctx context.Context, instanceID string) ([]core.InstanceConsoleConnection, error) {
	defer withSubtreeInterceptor(&c.compute.Interceptor)()
	req := core.ListInstanceConsoleConnectionsRequest{
		CompartmentId: common.String(c.tenant.TenancyOCID),
		InstanceId:    common.String(instanceID),
	}
	resp, err := c.compute.ListInstanceConsoleConnections(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("list console connections: %w", err)
	}
	return resp.Items, nil
}

func (c *Client) WaitForConsoleConnectionActive(ctx context.Context, connectionID string) (*core.InstanceConsoleConnection, error) {
	deadline := time.Now().Add(consoleConnectionTimeout)
	for time.Now().Before(deadline) {
		conn, err := c.GetConsoleConnection(ctx, connectionID)
		if err != nil {
			return nil, err
		}
		if conn.LifecycleState == core.InstanceConsoleConnectionLifecycleStateActive {
			return conn, nil
		}
		if conn.LifecycleState == core.InstanceConsoleConnectionLifecycleStateFailed {
			return nil, fmt.Errorf("console connection failed")
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(consoleConnectionPollInterval):
		}
	}
	return nil, fmt.Errorf("console connection timed out after %v", consoleConnectionTimeout)
}

// ── Boot Volume Operations ────────────────────────────────────────────

func (c *Client) DeleteBootVolume(ctx context.Context, bootVolumeID string) error {
	req := core.DeleteBootVolumeRequest{
		BootVolumeId: common.String(bootVolumeID),
	}
	_, err := c.bootVolume.DeleteBootVolume(ctx, req)
	if err != nil {
		return fmt.Errorf("delete boot volume: %w", err)
	}
	return nil
}

func (c *Client) UpdateBootVolumeWithVPU(ctx context.Context, id string, sizeInGBs int64, displayName string, vpusPerGB int64) (*core.BootVolume, error) {
	details := core.UpdateBootVolumeDetails{}
	if sizeInGBs > 0 {
		details.SizeInGBs = common.Int64(sizeInGBs)
	}
	if displayName != "" {
		details.DisplayName = common.String(displayName)
	}
	if vpusPerGB > 0 {
		details.VpusPerGB = common.Int64(vpusPerGB)
	}
	req := core.UpdateBootVolumeRequest{
		BootVolumeId:            common.String(id),
		UpdateBootVolumeDetails: details,
	}
	resp, err := c.bootVolume.UpdateBootVolume(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("update boot volume: %w", err)
	}
	return &resp.BootVolume, nil
}

// ── VCN Operations ────────────────────────────────────────────────────

func (c *Client) DeleteVcn(ctx context.Context, vcnID string) error {
	req := core.DeleteVcnRequest{
		VcnId: common.String(vcnID),
	}
	_, err := c.vcn.DeleteVcn(ctx, req)
	if err != nil {
		return fmt.Errorf("delete vcn: %w", err)
	}
	return nil
}

// UpdateSecurityListBatch replaces the entire security list rules with the provided ones.
func (c *Client) UpdateSecurityListBatch(ctx context.Context, vcnID string, ingressRaw, egressRaw []json.RawMessage) error {
	req := core.ListSecurityListsRequest{
		CompartmentId: common.String(c.tenant.TenancyOCID),
		VcnId:         common.String(vcnID),
		Limit:         common.Int(1),
	}
	resp, err := c.vcn.ListSecurityLists(ctx, req)
	if err != nil {
		return fmt.Errorf("list security lists: %w", err)
	}
	if len(resp.Items) == 0 {
		return fmt.Errorf("no security list found for VCN")
	}

	sl := resp.Items[0]

	// Parse ingress rules
	var ingressRules []core.IngressSecurityRule
	for _, raw := range ingressRaw {
		var rule core.IngressSecurityRule
		if err := json.Unmarshal(raw, &rule); err != nil {
			return fmt.Errorf("parse ingress rule: %w", err)
		}
		ingressRules = append(ingressRules, rule)
	}

	// Parse egress rules
	var egressRules []core.EgressSecurityRule
	for _, raw := range egressRaw {
		var rule core.EgressSecurityRule
		if err := json.Unmarshal(raw, &rule); err != nil {
			return fmt.Errorf("parse egress rule: %w", err)
		}
		egressRules = append(egressRules, rule)
	}

	updateReq := core.UpdateSecurityListRequest{
		SecurityListId: sl.Id,
		UpdateSecurityListDetails: core.UpdateSecurityListDetails{
			IngressSecurityRules: ingressRules,
			EgressSecurityRules:  egressRules,
		},
	}
	_, err = c.vcn.UpdateSecurityList(ctx, updateReq)
	return err
}

// VcnClient returns the raw VCN client for direct SDK access.
func (c *Client) VcnClient() core.VirtualNetworkClient { return c.vcn }

// ComputeClient returns the raw Compute client for direct SDK access.
func (c *Client) ComputeClient() core.ComputeClient { return c.compute }

// Tenant returns the tenant config.
