package oci

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/identity"
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
	mu         sync.Mutex
}

func NewClient(t *db.Tenant) (*Client, error) {
	cfg := common.NewRawConfigurationProvider(
		t.TenancyOCID, t.UserOCID, t.Region, t.Fingerprint,
		t.KeyFile, nil,
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

	return &Client{
		tenant:     t,
		rawCfg:     cfg,
		compute:    compute,
		vcn:        vcn,
		identity:   id,
		bootVolume: bv,
		monitoring: mon,
	}, nil
}

func (c *Client) Tenant() *db.Tenant { return c.tenant }

func (c *Client) ListInstances(ctx context.Context, compartmentID string) ([]core.Instance, error) {
	req := core.ListInstancesRequest{
		CompartmentId: common.String(compartmentID),
		Limit:         common.Int(100),
	}
	resp, err := c.compute.ListInstances(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("list instances: %w", err)
	}
	return resp.Items, nil
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

func (c *Client) LaunchInstance(ctx context.Context, req core.LaunchInstanceRequest) (*core.Instance, error) {
	resp, err := c.compute.LaunchInstance(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("launch instance: %w", err)
	}
	return &resp.Instance, nil
}

func (c *Client) TerminateInstance(ctx context.Context, instanceID string) error {
	req := core.TerminateInstanceRequest{InstanceId: common.String(instanceID)}
	_, err := c.compute.TerminateInstance(ctx, req)
	return err
}

func (c *Client) ListVCNs(ctx context.Context, compartmentID string) ([]core.Vcn, error) {
	req := core.ListVcnsRequest{
		CompartmentId: common.String(compartmentID),
		Limit:         common.Int(100),
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
		Limit:         common.Int(100),
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
		Limit:         common.Int(100),
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
		Limit:         common.Int(100),
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
		Limit:         common.Int(100),
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
