package oci

import (
	"context"
	"fmt"
	"net"
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

	return &Client{
		tenant:     t,
		rawCfg:     cfg,
		compute:    compute,
		vcn:        vcn,
		identity:   id,
		bootVolume: bv,
		monitoring: mon,
		limits:     lim,
	}, nil
}

func (c *Client) Tenant() *db.Tenant { return c.tenant }

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

func (c *Client) ListVCNs(ctx context.Context, compartmentID string) ([]core.Vcn, error) {
	req := core.ListVcnsRequest{
		CompartmentId: common.String(compartmentID),
		Limit:         common.Int(1000),
	}
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
	resp, err := c.vcn.ListSubnets(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.Items, nil
}

func (c *Client) ListAvailabilityDomains(ctx context.Context, compartmentID string) ([]identity.AvailabilityDomain, error) {
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
		BootVolumeId:           common.String(id),
		UpdateBootVolumeDetails: details,
	}
	resp, err := c.bootVolume.UpdateBootVolume(ctx, req)
	if err != nil {
		return nil, err
	}
	return &resp.BootVolume, nil
}

func (c *Client) ListBootVolumeAttachments(ctx context.Context, compartmentID, instanceID string) ([]core.BootVolumeAttachment, error) {
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

func (c *Client) GetMetrics(ctx context.Context, instanceID string) (map[string]float64, error) {
	metricNames := []string{"CpuUtilization", "MemoryUtilization", "NetworkBytesIn", "NetworkBytesOut", "DiskBytesRead", "DiskBytesWrite"}
	result := make(map[string]float64)

	for _, name := range metricNames {
		req := monitoring.SummarizeMetricsDataRequest{
			SummarizeMetricsDataDetails: monitoring.SummarizeMetricsDataDetails{
				Namespace: common.String("oci_computeagent"),
				Query:     common.String(name + `[1m]{instanceId="` + instanceID + `"}.mean()`),
				StartTime: &common.SDKTime{Time: time.Now().Add(-5 * time.Minute)},
				EndTime:   &common.SDKTime{Time: time.Now()},
			},
		}
		resp, err := c.monitoring.SummarizeMetricsData(ctx, req)
		if err != nil {
			continue
		}
		if len(resp.Items) > 0 && len(resp.Items[0].AggregatedDatapoints) > 0 && resp.Items[0].AggregatedDatapoints[0].Value != nil {
			result[name] = *resp.Items[0].AggregatedDatapoints[0].Value
		}
	}
	return result, nil
}

// Traffic

type TrafficDataPoint struct {
	Timestamp       string  `json:"timestamp"`
	BytesInPerSec   float64 `json:"bytesInPerSec"`
	BytesOutPerSec  float64 `json:"bytesOutPerSec"`
	PacketsInPerSec float64 `json:"packetsInPerSec"`
	PacketsOutPerSec float64 `json:"packetsOutPerSec"`
}

func (c *Client) GetVNICTtraffic(ctx context.Context, vnicID string, startTime, endTime time.Time) ([]TrafficDataPoint, error) {
	namespace := "oci_vcn"

	results := make(map[string][]float64)
	metricNames := []string{"VnicBytesIn", "VnicBytesOut", "VnicPacketsIn", "VnicPacketsOut"}

	for _, name := range metricNames {
		req := monitoring.SummarizeMetricsDataRequest{
			SummarizeMetricsDataDetails: monitoring.SummarizeMetricsDataDetails{
				Namespace: common.String(namespace),
				Query:     common.String(fmt.Sprintf("%s[1m]{resourceId=\"%s\"}.mean()", name, vnicID)),
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
		results[name] = values
	}

	// Combine into time series
	var data []TrafficDataPoint
	maxLen := 0
	for _, v := range results {
		if len(v) > maxLen {
			maxLen = len(v)
		}
	}
	for i := 0; i < maxLen; i++ {
		dp := TrafficDataPoint{
			Timestamp: startTime.Add(time.Duration(i) * time.Minute).Format(time.RFC3339),
		}
		if i < len(results["VnicBytesIn"]) {
			dp.BytesInPerSec = results["VnicBytesIn"][i]
		}
		if i < len(results["VnicBytesOut"]) {
			dp.BytesOutPerSec = results["VnicBytesOut"][i]
		}
		if i < len(results["VnicPacketsIn"]) {
			dp.PacketsInPerSec = results["VnicPacketsIn"][i]
		}
		if i < len(results["VnicPacketsOut"]) {
			dp.PacketsOutPerSec = results["VnicPacketsOut"][i]
		}
		data = append(data, dp)
	}
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

func (c *Client) ListSecurityRules(ctx context.Context, vcnID, keyword string, page, size int) ([]SecurityRuleInfo, int64, error) {
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
				ID:       *sl.Id + "/ingress/" + *rule.Protocol,
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
				ID:       *sl.Id + "/egress/" + *rule.Protocol,
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
		Protocol:  common.String(protocol),
		Source:    common.String(source),
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

func (c *Client) RemoveSecurityRules(ctx context.Context, vcnID string, ruleIDs []string) error {
	return fmt.Errorf("not implemented: remove specific rules by ID")
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
			Protocol:  common.String("all"),
			Source:    common.String("0.0.0.0/0"),
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
			ServiceName:    common.String(serviceName),
			LimitName:      def.Name,
			CompartmentId:  common.String(compartmentID),
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
	_ = vnics
	return fmt.Errorf("NLB creation requires OCI network load balancer SDK (not yet integrated)")
}

// Disable500Mbps deletes the NLB associated with the instance.
func (c *Client) Disable500Mbps(ctx context.Context, instanceID string) error {
	return fmt.Errorf("NLB deletion requires OCI network load balancer SDK (not yet integrated)")
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
		Scope:         core.ListPublicIpsScopeAvailabilityDomain,
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
