// Package oci wraps the OCI Go SDK (v65) for compute, VCN, identity, block storage, monitoring, limits, and NLB operations.
package oci

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
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
	"github.com/oracle/oci-go-sdk/v65/ospgateway"
	"github.com/oracle/oci-go-sdk/v65/usageapi"

	"github.com/viogus/oci-helper-go/internal/db"
)

type Client struct {
	tenant       *db.Tenant
	rawCfg       common.ConfigurationProvider
	compute      core.ComputeClient
	vcn          core.VirtualNetworkClient
	identity     identity.IdentityClient
	bootVolume   core.BlockstorageClient
	monitoring   monitoring.MonitoringClient
	limits       limits.LimitsClient
	nlb          networkloadbalancer.NetworkLoadBalancerClient
	usageapi     usageapi.UsageapiClient
	subscription ospgateway.SubscriptionServiceClient

	mu sync.Mutex // guards interceptor mutations on all sub-clients
}

func NewClient(t *db.Tenant, proxyURL string) (*Client, error) {
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

	usage, err := usageapi.NewUsageapiClientWithConfigurationProvider(cfg)
	if err != nil {
		return nil, fmt.Errorf("usageapi client: %w", err)
	}

	sub, err := ospgateway.NewSubscriptionServiceClientWithConfigurationProvider(cfg)
	if err != nil {
		return nil, fmt.Errorf("ospgateway client: %w", err)
	}

	// Apply proxy transport to all sub-clients if a proxy URL is configured.
	if proxyURL != "" {
		parsed, parseErr := url.Parse(proxyURL)
		if parseErr != nil {
			return nil, fmt.Errorf("parse proxy URL %q: %w", proxyURL, parseErr)
		}
		proxyHTTPClient := &http.Client{
			Transport: &http.Transport{Proxy: http.ProxyURL(parsed)},
		}
		compute.HTTPClient = proxyHTTPClient
		vcn.HTTPClient = proxyHTTPClient
		id.HTTPClient = proxyHTTPClient
		bv.HTTPClient = proxyHTTPClient
		mon.HTTPClient = proxyHTTPClient
		lim.HTTPClient = proxyHTTPClient
		nlb.HTTPClient = proxyHTTPClient
		usage.HTTPClient = proxyHTTPClient
		sub.HTTPClient = proxyHTTPClient
	}

	return &Client{
		tenant:       t,
		rawCfg:       cfg,
		compute:      compute,
		vcn:          vcn,
		identity:     id,
		bootVolume:   bv,
		monitoring:   mon,
		limits:       lim,
		nlb:          nlb,
		usageapi:     usage,
		subscription: sub,
	}, nil
}

// SetRegion changes the region on all sub-clients in this Client.
// Client MUST NOT be shared across goroutines (current design creates a fresh
// Client per request, so this is safe for sequential region-switching within a
// single handler call). Mutating region is much cheaper than creating a new
// Client per region — avoids re-reading the key file and re-creating all SDK
// clients.
func (c *Client) SetRegion(region string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.compute.SetRegion(region)
	c.vcn.SetRegion(region)
	c.identity.SetRegion(region)
	c.bootVolume.SetRegion(region)
	c.monitoring.SetRegion(region)
	c.limits.SetRegion(region)
	c.nlb.SetRegion(region)
	c.usageapi.SetRegion(region)
	c.subscription.SetRegion(region)
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

func (c *Client) TerminateInstance(ctx context.Context, instanceID string, preserveBootVolume bool) error {
	req := core.TerminateInstanceRequest{
		InstanceId:         common.String(instanceID),
		PreserveBootVolume: common.Bool(preserveBootVolume),
	}
	_, err := c.compute.TerminateInstance(ctx, req)
	return err
}

// withSubtreeInterceptor sets an interceptor on the given embedded BaseClient
// field pointer that adds compartmentIdInSubtree=true to all requests. This
// enables recursive cross-compartment resource listing.
//
// The Client mutex guards interceptor setup so that concurrent callers do not
// clobber each other's interceptor before their OCI API call begins. The
// cleanup function returned by this method does NOT acquire the mutex; Clients
// SHOULD NOT be shared across goroutines (the current design creates a fresh
// Client per request via clientFor, so no sharing occurs in practice).
//
// Returns a cleanup function to restore the previous interceptor (or nil).
func (c *Client) withSubtreeInterceptor(interceptor *common.RequestInterceptor) func() {
	c.mu.Lock()
	defer c.mu.Unlock()
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
	defer c.withSubtreeInterceptor(&c.vcn.Interceptor)()
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
	defer c.withSubtreeInterceptor(&c.vcn.Interceptor)()
	resp, err := c.vcn.ListSubnets(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.Items, nil
}

// ValidateCredentials checks that the OCI credentials are valid by making a
// lightweight API call (ListAvailabilityDomains) before saving the tenant.
func (c *Client) ValidateCredentials(ctx context.Context, compartmentID string) error {
	_, err := c.ListAvailabilityDomains(ctx, compartmentID)
	return err
}

func (c *Client) ListAvailabilityDomains(ctx context.Context, compartmentID string) ([]identity.AvailabilityDomain, error) {
	defer c.withSubtreeInterceptor(&c.identity.Interceptor)()
	req := identity.ListAvailabilityDomainsRequest{
		CompartmentId: common.String(compartmentID),
	}
	resp, err := c.identity.ListAvailabilityDomains(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.Items, nil
}

// ListRegionSubscriptions returns subscribed regions for the tenancy.
func (c *Client) ListRegionSubscriptions(ctx context.Context) ([]identity.RegionSubscription, error) {
	defer c.withSubtreeInterceptor(&c.identity.Interceptor)()
	req := identity.ListRegionSubscriptionsRequest{
		TenancyId: common.String(c.tenant.TenancyOCID),
	}
	resp, err := c.identity.ListRegionSubscriptions(ctx, req)
	if err != nil {
		return nil, err
	}
	return resp.Items, nil
}

// ── Identity User Management ──────────────────────────────────────────

func (c *Client) ListUsers(ctx context.Context, compartmentID string) ([]identity.User, error) {
	defer c.withSubtreeInterceptor(&c.identity.Interceptor)()
	var all []identity.User
	page := common.String("")
	for {
		req := identity.ListUsersRequest{
			CompartmentId: common.String(compartmentID),
			Limit:         common.Int(100),
			Page:          page,
		}
		resp, err := c.identity.ListUsers(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("list users: %w", err)
		}
		all = append(all, resp.Items...)
		if resp.OpcNextPage == nil || *resp.OpcNextPage == "" {
			break
		}
		page = resp.OpcNextPage
	}
	return all, nil
}

func (c *Client) GetUser(ctx context.Context, userID string) (*identity.User, error) {
	defer c.withSubtreeInterceptor(&c.identity.Interceptor)()
	req := identity.GetUserRequest{
		UserId: common.String(userID),
	}
	resp, err := c.identity.GetUser(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("get user: %w", err)
	}
	return &resp.User, nil
}

func (c *Client) DeleteUser(ctx context.Context, userID string) error {
	defer c.withSubtreeInterceptor(&c.identity.Interceptor)()
	req := identity.DeleteUserRequest{
		UserId: common.String(userID),
	}
	_, err := c.identity.DeleteUser(ctx, req)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	return nil
}

func (c *Client) CreateOrResetUIPassword(ctx context.Context, userID string) (*identity.UiPassword, error) {
	defer c.withSubtreeInterceptor(&c.identity.Interceptor)()
	req := identity.CreateOrResetUIPasswordRequest{
		UserId: common.String(userID),
	}
	resp, err := c.identity.CreateOrResetUIPassword(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("create or reset UI password: %w", err)
	}
	return &resp.UiPassword, nil
}

func (c *Client) UpdateUser(ctx context.Context, userID string, email, description *string) (*identity.User, error) {
	defer c.withSubtreeInterceptor(&c.identity.Interceptor)()
	req := identity.UpdateUserRequest{
		UserId: common.String(userID),
		UpdateUserDetails: identity.UpdateUserDetails{
			Email:       email,
			Description: description,
		},
	}
	resp, err := c.identity.UpdateUser(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("update user: %w", err)
	}
	return &resp.User, nil
}

func (c *Client) ListMfaTotpDevices(ctx context.Context, userID string) ([]identity.MfaTotpDeviceSummary, error) {
	defer c.withSubtreeInterceptor(&c.identity.Interceptor)()
	req := identity.ListMfaTotpDevicesRequest{
		UserId: common.String(userID),
		Limit:  common.Int(100),
	}
	resp, err := c.identity.ListMfaTotpDevices(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("list MFA TOTP devices: %w", err)
	}
	return resp.Items, nil
}

func (c *Client) DeleteMfaTotpDevice(ctx context.Context, userID, deviceID string) error {
	defer c.withSubtreeInterceptor(&c.identity.Interceptor)()
	req := identity.DeleteMfaTotpDeviceRequest{
		UserId:          common.String(userID),
		MfaTotpDeviceId: common.String(deviceID),
	}
	_, err := c.identity.DeleteMfaTotpDevice(ctx, req)
	if err != nil {
		return fmt.Errorf("delete MFA TOTP device: %w", err)
	}
	return nil
}

func (c *Client) ListApiKeys(ctx context.Context, userID string) ([]identity.ApiKey, error) {
	defer c.withSubtreeInterceptor(&c.identity.Interceptor)()
	req := identity.ListApiKeysRequest{
		UserId: common.String(userID),
	}
	resp, err := c.identity.ListApiKeys(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("list API keys: %w", err)
	}
	return resp.Items, nil
}

func (c *Client) DeleteApiKey(ctx context.Context, userID, fingerprint string) error {
	defer c.withSubtreeInterceptor(&c.identity.Interceptor)()
	req := identity.DeleteApiKeyRequest{
		UserId:      common.String(userID),
		Fingerprint: common.String(fingerprint),
	}
	_, err := c.identity.DeleteApiKey(ctx, req)
	if err != nil {
		return fmt.Errorf("delete API key: %w", err)
	}
	return nil
}

func (c *Client) GetTenancy(ctx context.Context) (*identity.Tenancy, error) {
	defer c.withSubtreeInterceptor(&c.identity.Interceptor)()
	req := identity.GetTenancyRequest{
		TenancyId: common.String(c.tenant.TenancyOCID),
	}
	resp, err := c.identity.GetTenancy(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("get tenancy: %w", err)
	}
	return &resp.Tenancy, nil
}

func (c *Client) ListImages(ctx context.Context, compartmentID, os string) ([]core.Image, error) {
	defer c.withSubtreeInterceptor(&c.compute.Interceptor)()
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
	defer c.withSubtreeInterceptor(&c.compute.Interceptor)()
	var all []core.VnicAttachment
	page := common.String("")
	for {
		req := core.ListVnicAttachmentsRequest{
			CompartmentId: common.String(compartmentID),
			InstanceId:    common.String(instanceID),
			Limit:         common.Int(50),
			Page:          page,
		}
		resp, err := c.compute.ListVnicAttachments(ctx, req)
		if err != nil {
			return nil, err
		}
		all = append(all, resp.Items...)
		if resp.OpcNextPage == nil || *resp.OpcNextPage == "" {
			break
		}
		page = resp.OpcNextPage
	}
	return all, nil
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
	defer c.withSubtreeInterceptor(&c.vcn.Interceptor)()
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
	defer c.withSubtreeInterceptor(&c.compute.Interceptor)()
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
	defer c.withSubtreeInterceptor(&c.bootVolume.Interceptor)()
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
	defer c.withSubtreeInterceptor(&c.compute.Interceptor)()
	var all []core.BootVolumeAttachment
	page := common.String("")
	for {
		req := core.ListBootVolumeAttachmentsRequest{
			CompartmentId: common.String(compartmentID),
			InstanceId:    common.String(instanceID),
			Limit:         common.Int(50),
			Page:          page,
		}
		resp, err := c.compute.ListBootVolumeAttachments(ctx, req)
		if err != nil {
			return nil, err
		}
		all = append(all, resp.Items...)
		if resp.OpcNextPage == nil || *resp.OpcNextPage == "" {
			break
		}
		page = resp.OpcNextPage
	}
	return all, nil
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

func (c *Client) UpdateInstanceFreeformTags(ctx context.Context, instanceID string, tags map[string]string) error {
	details := core.UpdateInstanceDetails{
		FreeformTags: tags,
	}
	req := core.UpdateInstanceRequest{
		InstanceId:            common.String(instanceID),
		UpdateInstanceDetails: details,
	}
	_, err := c.compute.UpdateInstance(ctx, req)
	return err
}

func (c *Client) GetBootVolumeAttachment(ctx context.Context, compartmentID, instanceID string) (*core.BootVolumeAttachment, error) {
	defer c.withSubtreeInterceptor(&c.compute.Interceptor)()
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
	defer c.withSubtreeInterceptor(&c.compute.Interceptor)()
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
			log.Printf("[warn] get vnic %s: %v", *att.VnicId, err)
			continue
		}
		vnics = append(vnics, *vnic)
	}
	if len(vnics) == 0 && len(attachments) > 0 {
		return nil, fmt.Errorf("failed to fetch any VNICs for instance %s (%d attachments)", instanceID, len(attachments))
	}
	return vnics, nil
}

// EnableIPv6 provisions IPv6 end-to-end for the instance's primary VNIC: it
// ensures the VCN has an Oracle-allocated /56, carves a /64 for the subnet,
// adds a ::/0 default route to the internet gateway, mirrors IPv4 (0.0.0.0/0)
// ingress rules to IPv6 (::/0), then assigns an IPv6 address to the VNIC.
// Every step is idempotent, so it is safe to call repeatedly.
func (c *Client) EnableIPv6(ctx context.Context, instanceID string) (string, error) {
	compartmentID := c.tenant.TenancyOCID
	vnics, err := c.GetInstanceVNICs(ctx, compartmentID, instanceID)
	if err != nil {
		return "", fmt.Errorf("get vnics: %w", err)
	}
	if len(vnics) == 0 || vnics[0].Id == nil || vnics[0].SubnetId == nil {
		return "", fmt.Errorf("instance has no usable VNIC")
	}
	vnic := vnics[0]
	vnicID := *vnic.Id
	subnetID := *vnic.SubnetId

	// Already assigned → idempotent no-op.
	if len(vnic.Ipv6Addresses) > 0 {
		return vnic.Ipv6Addresses[0], nil
	}

	subnet, err := c.vcn.GetSubnet(ctx, core.GetSubnetRequest{SubnetId: common.String(subnetID)})
	if err != nil {
		return "", fmt.Errorf("get subnet: %w", err)
	}
	if subnet.VcnId == nil {
		return "", fmt.Errorf("subnet missing VCN id")
	}
	vcnID := *subnet.VcnId

	// 1. Ensure the VCN has an Oracle-allocated IPv6 /56.
	vcnResp, err := c.vcn.GetVcn(ctx, core.GetVcnRequest{VcnId: common.String(vcnID)})
	if err != nil {
		return "", fmt.Errorf("get vcn: %w", err)
	}
	if len(vcnResp.Ipv6CidrBlocks) == 0 {
		if _, err := c.vcn.AddIpv6VcnCidr(ctx, core.AddIpv6VcnCidrRequest{
			VcnId: common.String(vcnID),
			AddVcnIpv6CidrDetails: core.AddVcnIpv6CidrDetails{
				IsOracleGuaAllocationEnabled: common.Bool(true),
			},
		}); err != nil {
			return "", fmt.Errorf("add vcn ipv6 cidr: %w", err)
		}
		deadline := time.Now().Add(90 * time.Second)
		for len(vcnResp.Ipv6CidrBlocks) == 0 && time.Now().Before(deadline) {
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(3 * time.Second):
			}
			if vcnResp, err = c.vcn.GetVcn(ctx, core.GetVcnRequest{VcnId: common.String(vcnID)}); err != nil {
				return "", fmt.Errorf("poll vcn ipv6: %w", err)
			}
		}
		if len(vcnResp.Ipv6CidrBlocks) == 0 {
			return "", fmt.Errorf("VCN IPv6 CIDR not ready")
		}
	}
	vcnIpv6 := vcnResp.Ipv6CidrBlocks[0]

	// 2. Ensure the subnet has an IPv6 /64 carved from the VCN /56.
	if subnet.Ipv6CidrBlock == nil || *subnet.Ipv6CidrBlock == "" {
		// Collect already-used IPv6 CIDRs across all subnets in the VCN.
		var usedCIDRs []string
		existingSubnets, err := c.ListSubnets(ctx, compartmentID, vcnID)
		if err != nil {
			log.Printf("[warn] list subnets for VCN %s: %v", vcnID, err)
		} else {
			for _, s := range existingSubnets {
				if s.Ipv6CidrBlock != nil && *s.Ipv6CidrBlock != "" {
					usedCIDRs = append(usedCIDRs, *s.Ipv6CidrBlock)
				}
			}
		}
		subnetCidr, derr := deriveSubnetIpv6(vcnIpv6, usedCIDRs)
		if derr != nil {
			return "", derr
		}
		if _, err := c.vcn.AddIpv6SubnetCidr(ctx, core.AddIpv6SubnetCidrRequest{
			SubnetId: common.String(subnetID),
			AddSubnetIpv6CidrDetails: core.AddSubnetIpv6CidrDetails{
				Ipv6CidrBlock: common.String(subnetCidr),
			},
		}); err != nil {
			return "", fmt.Errorf("add subnet ipv6 cidr: %w", err)
		}
	}

	// 3. Ensure a ::/0 default route to the VCN's internet gateway.
	if err := c.ensureIPv6Route(ctx, vcnID, subnet.RouteTableId); err != nil {
		return "", err
	}

	// 4. Mirror IPv4 ingress and egress rules to IPv6 so the same ports are reachable.
	if err := c.mirrorToIPv6(ctx, subnet.SecurityListIds); err != nil {
		return "", err
	}

	// 5. Assign the IPv6 address to the VNIC.
	createResp, err := c.vcn.CreateIpv6(ctx, core.CreateIpv6Request{
		CreateIpv6Details: core.CreateIpv6Details{VnicId: common.String(vnicID)},
	})
	if err != nil {
		return "", fmt.Errorf("create ipv6: %w", err)
	}
	if createResp.IpAddress != nil && *createResp.IpAddress != "" {
		return *createResp.IpAddress, nil
	}

	// Rare propagation delay: fall back to listing.
	ipv6s, err := c.vcn.ListIpv6s(ctx, core.ListIpv6sRequest{VnicId: common.String(vnicID)})
	if err == nil {
		for _, ip := range ipv6s.Items {
			if ip.IpAddress != nil && *ip.IpAddress != "" {
				return *ip.IpAddress, nil
			}
		}
	}
	return "", fmt.Errorf("IPv6 address not available after assignment")
}

// DisableIPv6 removes IPv6 addresses from the instance's primary VNIC. Shared
// VCN/subnet/route/security-list resources are left intact (they may be used
// by other instances and can be reused if IPv6 is re-enabled).
func (c *Client) DisableIPv6(ctx context.Context, instanceID string) error {
	vnics, err := c.GetInstanceVNICs(ctx, c.tenant.TenancyOCID, instanceID)
	if err != nil {
		return fmt.Errorf("get vnics: %w", err)
	}
	if len(vnics) == 0 || vnics[0].Id == nil {
		return nil
	}
	vnicID := *vnics[0].Id
	ipv6s, err := c.vcn.ListIpv6s(ctx, core.ListIpv6sRequest{VnicId: common.String(vnicID)})
	if err != nil {
		return fmt.Errorf("list ipv6s: %w", err)
	}
	for _, ip := range ipv6s.Items {
		if ip.Id == nil {
			continue
		}
		if _, err := c.vcn.DeleteIpv6(ctx, core.DeleteIpv6Request{Ipv6Id: ip.Id}); err != nil {
			return fmt.Errorf("delete ipv6: %w", err)
		}
	}
	return nil
}

// deriveSubnetIpv6 carves a /64 from a VCN IPv6 /56 block, skipping
// already-used subnets. The VCN /56 has 256 /64 subnets (0-255).
func deriveSubnetIpv6(vcnCidr string, usedCIDRs []string) (string, error) {
	_, ipnet, err := net.ParseCIDR(vcnCidr)
	if err != nil {
		return "", fmt.Errorf("parse vcn ipv6 cidr %q: %w", vcnCidr, err)
	}
	used := make(map[string]bool, len(usedCIDRs))
	for _, cidr := range usedCIDRs {
		used[cidr] = true
	}
	ip := ipnet.IP.To16()
	if ip == nil {
		return "", fmt.Errorf("vcn ipv6 cidr %q is not a valid IPv6 address", vcnCidr)
	}
	// A /56 leaves the top 56 bits fixed. Subnet bits are bits 56-63 (the 8 bits
	// between /56 and /64). Iterate through all 256 possible subnets.
	mask := net.CIDRMask(56, 128)
	baseIP := ip.Mask(mask)
	for i := 0; i < 256; i++ {
		candidate := make(net.IP, len(baseIP))
		copy(candidate, baseIP)
		candidate[7] = byte(i) // bits 56-63 live in byte 7 (0-indexed).
		cidr := candidate.String() + "/64"
		if !used[cidr] {
			return cidr, nil
		}
	}
	return "", fmt.Errorf("all 256 /64 subnets in VCN /56 %s are already in use", vcnCidr)
}

// ensureIPv6Route adds a ::/0 → internet-gateway rule to the route table if absent.
func (c *Client) ensureIPv6Route(ctx context.Context, vcnID string, routeTableID *string) error {
	if routeTableID == nil || *routeTableID == "" {
		return fmt.Errorf("subnet has no route table")
	}
	igws, err := c.vcn.ListInternetGateways(ctx, core.ListInternetGatewaysRequest{
		CompartmentId: common.String(c.tenant.TenancyOCID),
		VcnId:         common.String(vcnID),
	})
	if err != nil {
		return fmt.Errorf("list internet gateways: %w", err)
	}
	if len(igws.Items) == 0 || igws.Items[0].Id == nil {
		return fmt.Errorf("no internet gateway in VCN")
	}
	igwID := *igws.Items[0].Id

	rt, err := c.vcn.GetRouteTable(ctx, core.GetRouteTableRequest{RtId: routeTableID})
	if err != nil {
		return fmt.Errorf("get route table: %w", err)
	}
	for _, r := range rt.RouteRules {
		if r.Destination != nil && *r.Destination == "::/0" {
			return nil // already present
		}
	}
	rules := append(rt.RouteRules, core.RouteRule{
		Destination:     common.String("::/0"),
		DestinationType: core.RouteRuleDestinationTypeCidrBlock,
		NetworkEntityId: common.String(igwID),
	})
	if _, err := c.vcn.UpdateRouteTable(ctx, core.UpdateRouteTableRequest{
		RtId:                    routeTableID,
		UpdateRouteTableDetails: core.UpdateRouteTableDetails{RouteRules: rules},
	}); err != nil {
		return fmt.Errorf("update route table: %w", err)
	}
	return nil
}

	// mirrorToIPv6 copies each 0.0.0.0/0 ingress and egress rule to a ::/0
	// equivalent, skipping duplicates. Applies the update per security list.
	func (c *Client) mirrorToIPv6(ctx context.Context, securityListIDs []string) error {
		for _, slID := range securityListIDs {
			sl, err := c.vcn.GetSecurityList(ctx, core.GetSecurityListRequest{SecurityListId: common.String(slID)})
			if err != nil {
				return fmt.Errorf("get security list: %w", err)
			}

			// --- Ingress: mirror 0.0.0.0/0 → ::/0 ---
			ingressRules := sl.IngressSecurityRules
			ingressExisting := map[string]bool{}
			for _, r := range ingressRules {
				if r.Source != nil && strings.Contains(*r.Source, ":") {
					ingressExisting[ingressRuleKey(r)] = true
				}
			}
			ingressAdded := false
			for _, r := range ingressRules {
				if r.Source == nil || *r.Source != "0.0.0.0/0" {
					continue
				}
				v6 := r
				v6.Source = common.String("::/0")
				v6.SourceType = core.IngressSecurityRuleSourceTypeCidrBlock
				if ingressExisting[ingressRuleKey(v6)] {
					continue
				}
				ingressRules = append(ingressRules, v6)
				ingressAdded = true
			}

			// --- Egress: mirror 0.0.0.0/0 → ::/0 ---
			egressRules := sl.EgressSecurityRules
			egressExisting := map[string]bool{}
			for _, r := range egressRules {
				if r.Destination != nil && strings.Contains(*r.Destination, ":") {
					egressExisting[egressRuleKey(r)] = true
				}
			}
			egressAdded := false
			for _, r := range egressRules {
				if r.Destination == nil || *r.Destination != "0.0.0.0/0" {
					continue
				}
				v6 := r
				v6.Destination = common.String("::/0")
				v6.DestinationType = core.EgressSecurityRuleDestinationTypeCidrBlock
				if egressExisting[egressRuleKey(v6)] {
					continue
				}
				egressRules = append(egressRules, v6)
				egressAdded = true
			}

			if !ingressAdded && !egressAdded {
				continue
			}
			if _, err := c.vcn.UpdateSecurityList(ctx, core.UpdateSecurityListRequest{
				SecurityListId: common.String(slID),
				UpdateSecurityListDetails: core.UpdateSecurityListDetails{
					IngressSecurityRules: ingressRules,
					EgressSecurityRules:  egressRules,
				},
			}); err != nil {
				return fmt.Errorf("update security list: %w", err)
			}
		}
		return nil
	}

// ingressRuleKey identifies an ingress rule by protocol, source and dest port range.
func ingressRuleKey(r core.IngressSecurityRule) string {
	proto, src, port := "", "", ""
	if r.Protocol != nil {
		proto = *r.Protocol
	}
	if r.Source != nil {
		src = *r.Source
	}
	if r.TcpOptions != nil && r.TcpOptions.DestinationPortRange != nil {
		pr := r.TcpOptions.DestinationPortRange
		port = fmt.Sprintf("tcp:%d-%d", ptrInt(pr.Min), ptrInt(pr.Max))
	} else if r.UdpOptions != nil && r.UdpOptions.DestinationPortRange != nil {
		pr := r.UdpOptions.DestinationPortRange
		port = fmt.Sprintf("udp:%d-%d", ptrInt(pr.Min), ptrInt(pr.Max))
	}
	return proto + "|" + src + "|" + port
}

// egressRuleKey identifies an egress rule by protocol, destination and dest port range.
func egressRuleKey(r core.EgressSecurityRule) string {
	proto, dst, port := "", "", ""
	if r.Protocol != nil {
		proto = *r.Protocol
	}
	if r.Destination != nil {
		dst = *r.Destination
	}
	if r.TcpOptions != nil && r.TcpOptions.DestinationPortRange != nil {
		pr := r.TcpOptions.DestinationPortRange
		port = fmt.Sprintf("tcp:%d-%d", ptrInt(pr.Min), ptrInt(pr.Max))
	} else if r.UdpOptions != nil && r.UdpOptions.DestinationPortRange != nil {
		pr := r.UdpOptions.DestinationPortRange
		port = fmt.Sprintf("udp:%d-%d", ptrInt(pr.Min), ptrInt(pr.Max))
	}
	return proto + "|" + dst + "|" + port
}

func ptrInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// NetworkStatus reports whether 500M (NLB) and IPv6 are enabled for an instance.
type NetworkStatus struct {
	NLBEnabled  bool   `json:"nlb_enabled"`
	NLBIP       string `json:"nlb_ip"`
	IPv6Enabled bool   `json:"ipv6_enabled"`
	IPv6Addr    string `json:"ipv6_addr"`
}

// GetNetworkStatus returns 500M/IPv6 status per instance ID. The NLB set is
// listed once and matched by freeform tag; IPv6 is read from each instance's
// primary VNIC. VNIC lookups run sequentially to avoid racing the shared
// request interceptor. Per-instance failures yield a zero (disabled) status.
func (c *Client) GetNetworkStatus(ctx context.Context, instanceIDs []string) map[string]NetworkStatus {
	compartmentID := c.tenant.TenancyOCID
	out := make(map[string]NetworkStatus, len(instanceIDs))

	nlbIP := map[string]string{}
	if listResp, err := c.nlb.ListNetworkLoadBalancers(ctx, networkloadbalancer.ListNetworkLoadBalancersRequest{
		CompartmentId: common.String(compartmentID),
		Limit:         common.Int(100),
	}); err == nil {
		for i := range listResp.Items {
			nlb := &listResp.Items[i]
			if nlb.FreeformTags != nil {
				if id := nlb.FreeformTags["oci-helper-instance-id"]; id != "" {
					nlbIP[id] = nlbPublicIP(nlb.IpAddresses)
				}
			}
		}
	}

	for _, id := range instanceIDs {
		st := NetworkStatus{}
		if ip, ok := nlbIP[id]; ok {
			st.NLBEnabled = true
			st.NLBIP = ip
		}
		if vnics, err := c.GetInstanceVNICs(ctx, compartmentID, id); err == nil && len(vnics) > 0 && len(vnics[0].Ipv6Addresses) > 0 {
			st.IPv6Enabled = true
			st.IPv6Addr = vnics[0].Ipv6Addresses[0]
		}
		out[id] = st
	}
	return out
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
				CompartmentIdInSubtree: common.Bool(compartmentID == c.tenant.TenancyOCID),
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
	)
	const thirty = 30 * day
	switch {
	case d <= seven:
		return "[1m]", time.Minute
	case d <= thirty:
		return "[5m]", 5 * time.Minute
	default:
		return "[1h]", time.Hour
	}
}

func (c *Client) GetVNICTtraffic(ctx context.Context, compartmentID, vnicID string, startTime, endTime time.Time) ([]TrafficDataPoint, error) {
	namespace := "oci_vcn"
	log.Printf("========== GetVNICTtraffic ==========")
	log.Printf("[GetVNICTtraffic] vnic=%s compartment=%s start=%s end=%s range=%v", vnicID, compartmentID, startTime.Format(time.RFC3339), endTime.Format(time.RFC3339), endTime.Sub(startTime))

	results := make(map[string][]float64)
	metricNames := []string{"VnicFromNetworkBytes", "VnicToNetworkBytes", "VnicFromNetworkPackets", "VnicToNetworkPackets"}

	totalDuration := endTime.Sub(startTime)
	intervalStr, step := intervalForDuration(totalDuration)
	log.Printf("[GetVNICTtraffic] vnic=%s compartment=%s range=%v interval=%s step=%v", vnicID, compartmentID, totalDuration, intervalStr, step)

	for _, name := range metricNames {
		query := fmt.Sprintf("%s%s{resourceId=\"%s\"}.mean()", name, intervalStr, vnicID)
		req := monitoring.SummarizeMetricsDataRequest{
			CompartmentId: common.String(compartmentID),
			CompartmentIdInSubtree: common.Bool(compartmentID == c.tenant.TenancyOCID),
			SummarizeMetricsDataDetails: monitoring.SummarizeMetricsDataDetails{
				Namespace: common.String(namespace),
				Query:     common.String(fmt.Sprintf("%s%s{resourceId=\"%s\"}.mean()", name, intervalStr, vnicID)),
				StartTime: &common.SDKTime{Time: startTime},
				EndTime:   &common.SDKTime{Time: endTime},
			},
		}
		log.Printf("[GetVNICTtraffic] query=%s compartment=%s", query, compartmentID)
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
		if len(resp.Items) > 0 {
			first := resp.Items[0]
			log.Printf("[GetVNICTtraffic] %s first item: name=%s, dims=%v, aggLen=%d",
				name,
				pointerToString(first.Name),
				first.Dimensions,
				len(first.AggregatedDatapoints))
		}
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
		if vals := results["VnicFromNetworkBytes"]; i < len(vals) {
			dp.BytesInPerSec = vals[i]
		}
		if vals := results["VnicToNetworkBytes"]; i < len(vals) {
			dp.BytesOutPerSec = vals[i]
		}
		if vals := results["VnicFromNetworkPackets"]; i < len(vals) {
			dp.PacketsInPerSec = vals[i]
		}
		if vals := results["VnicToNetworkPackets"]; i < len(vals) {
			dp.PacketsOutPerSec = vals[i]
		}
		data = append(data, dp)
	}
	log.Printf("[GetVNICTtraffic] returned %d data points (step=%v)", len(data), step)
	return data, nil
}

// FetchInstancesTrafficResult holds monthly traffic totals for one instance.
type FetchInstancesTrafficResult struct {
	InstanceCount    int    `json:"instanceCount"`
	InboundTraffic   string `json:"inboundTraffic"`
	OutboundTraffic  string `json:"outboundTraffic"`
}

// FetchInstancesTraffic sums traffic across all VNICs for all instances in a region
// over the given time range, returning human-readable totals.
func (c *Client) FetchInstancesTraffic(ctx context.Context, compartmentID, region string, startTime, endTime time.Time) (*FetchInstancesTrafficResult, error) {
	instances, err := c.ListInstances(ctx, compartmentID)
	if err != nil {
		return nil, fmt.Errorf("list instances: %w", err)
	}

	var totalIn, totalOut float64
	instanceCount := len(instances)
	totalDuration := endTime.Sub(startTime)
	intervalStr, step := intervalForDuration(totalDuration)
	namespace := "oci_vcn"

	for _, inst := range instances {
		vnics, err := c.GetInstanceVNICs(ctx, compartmentID, *inst.Id)
		if err != nil {
			continue
		}
		for _, vnic := range vnics {
			for _, metric := range []struct {
				name  string
				accum *float64
			}{
				{"VnicFromNetworkBytes", &totalIn},
				{"VnicToNetworkBytes", &totalOut},
			} {
				req := monitoring.SummarizeMetricsDataRequest{
					CompartmentId:          common.String(compartmentID),
					CompartmentIdInSubtree: common.Bool(true),
					SummarizeMetricsDataDetails: monitoring.SummarizeMetricsDataDetails{
						Namespace: common.String(namespace),
						Query:     common.String(fmt.Sprintf("%s%s{resourceId=\"%s\"}.mean()", metric.name, intervalStr, *vnic.Id)),
						StartTime: &common.SDKTime{Time: startTime},
						EndTime:   &common.SDKTime{Time: endTime},
					},
				}
				resp, err := c.monitoring.SummarizeMetricsData(ctx, req)
				if err != nil {
					continue
				}
				for _, item := range resp.Items {
					for _, dp := range item.AggregatedDatapoints {
						if dp.Value != nil {
							*metric.accum += *dp.Value * step.Minutes()
						}
					}
				}
			}
		}
	}

	return &FetchInstancesTrafficResult{
		InstanceCount:   instanceCount,
		InboundTraffic:  FormatBytes(totalIn),
		OutboundTraffic: FormatBytes(totalOut),
	}, nil
}

// FormatBytes converts bytes to a human-readable string (B/KB/MB/GB/TB).
func FormatBytes(bytes float64) string {
	units := []string{"B", "KB", "MB", "GB", "TB"}
	v := bytes
	idx := 0
	for idx < len(units)-1 && v >= 1024 {
		v /= 1024
		idx++
	}
	if idx == 0 {
		return fmt.Sprintf("%.0f %s", v, units[idx])
	}
	return fmt.Sprintf("%.2f %s", v, units[idx])
}

// helper for nil-safe pointer-to-string logging
func pointerToString(p *string) string {
	if p == nil {
		return "<nil>"
	}
	return *p
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
	defer c.withSubtreeInterceptor(&c.vcn.Interceptor)()
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
		portRange := &core.PortRange{
			Min: common.Int(minPort),
			Max: common.Int(maxPort),
		}
		if protocol == "UDP" {
			newRule.UdpOptions = &core.UdpOptions{DestinationPortRange: portRange}
		} else {
			newRule.TcpOptions = &core.TcpOptions{DestinationPortRange: portRange}
		}
	}
	// Dedup: skip if identical rule already exists.
	if !hasIngressRule(ingressRules, newRule) {
		ingressRules = append(ingressRules, newRule)
	}

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

// hasIngressRule checks whether an equivalent ingress rule already exists.
func hasIngressRule(rules []core.IngressSecurityRule, rule core.IngressSecurityRule) bool {
	for _, r := range rules {
		if !strPtrEq(r.Protocol, rule.Protocol) {
			continue
		}
		if !strPtrEq(r.Source, rule.Source) {
			continue
		}
		if !tcpOptionsEq(r.TcpOptions, rule.TcpOptions) {
			continue
		}
		if !udpOptionsEq(r.UdpOptions, rule.UdpOptions) {
			continue
		}
		return true
	}
	return false
}

// hasEgressRule checks whether an equivalent egress rule already exists.
func hasEgressRule(rules []core.EgressSecurityRule, rule core.EgressSecurityRule) bool {
	for _, r := range rules {
		if !strPtrEq(r.Protocol, rule.Protocol) {
			continue
		}
		if !strPtrEq(r.Destination, rule.Destination) {
			continue
		}
		if !tcpOptionsEq(r.TcpOptions, rule.TcpOptions) {
			continue
		}
		if !udpOptionsEq(r.UdpOptions, rule.UdpOptions) {
			continue
		}
		return true
	}
	return false
}

func strPtrEq(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func tcpOptionsEq(a, b *core.TcpOptions) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if (a.DestinationPortRange == nil) != (b.DestinationPortRange == nil) {
		return false
	}
	if a.DestinationPortRange != nil {
		if !intPtrEq(a.DestinationPortRange.Min, b.DestinationPortRange.Min) {
			return false
		}
		if !intPtrEq(a.DestinationPortRange.Max, b.DestinationPortRange.Max) {
			return false
		}
	}
	return true
}

func udpOptionsEq(a, b *core.UdpOptions) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if (a.DestinationPortRange == nil) != (b.DestinationPortRange == nil) {
		return false
	}
	if a.DestinationPortRange != nil {
		if !intPtrEq(a.DestinationPortRange.Min, b.DestinationPortRange.Min) {
			return false
		}
		if !intPtrEq(a.DestinationPortRange.Max, b.DestinationPortRange.Max) {
			return false
		}
	}
	return true
}

func intPtrEq(a, b *int) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
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
			portRange := &core.PortRange{
				Min: common.Int(minPort),
				Max: common.Int(maxPort),
			}
			if protocol == "UDP" {
				newRule.UdpOptions = &core.UdpOptions{DestinationPortRange: portRange}
			} else {
				newRule.TcpOptions = &core.TcpOptions{DestinationPortRange: portRange}
			}
	}
	// Dedup: skip if identical rule already exists.
	if !hasEgressRule(egressRules, newRule) {
		egressRules = append(egressRules, newRule)
	}

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
		hasAllowAll := false
		for _, r := range ingressRules {
			if r.Protocol != nil && *r.Protocol == "all" &&
				r.Source != nil && *r.Source == "0.0.0.0/0" {
				hasAllowAll = true
				break
			}
		}
		if !hasAllowAll {
			ingressRules = append(ingressRules, core.IngressSecurityRule{
				Protocol: common.String("all"),
				Source:   common.String("0.0.0.0/0"),
			})
		}
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

// LimitInfo represents a single OCI service limit/quota item.
type LimitInfo struct {
	ServiceName        string `json:"serviceName"`
	LimitName          string `json:"limitName"`
	Description        string `json:"description"`
	ScopeType          string `json:"scopeType"`
	AvailabilityDomain string `json:"availabilityDomain"`
	ServiceLimit       int64  `json:"serviceLimit"`
	Used               int64  `json:"used"`
	Available          int64  `json:"available"`
}

// ListServices returns all available service names for limit queries.
func (c *Client) ListServices(ctx context.Context) ([]string, error) {
	defer c.withSubtreeInterceptor(&c.limits.Interceptor)()
	req := limits.ListServicesRequest{
		CompartmentId: common.String(c.tenant.TenancyOCID),
		Limit:         common.Int(100),
	}
	resp, err := c.limits.ListServices(ctx, req)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, s := range resp.Items {
		if s.Name != nil && *s.Name != "" {
			names = append(names, *s.Name)
		}
	}
	return names, nil
}

// GetLimits queries limits for a tenant and optional service in a specific region.
// If serviceName is empty, queries ALL services.
func (c *Client) GetLimits(ctx context.Context, region, serviceName string) ([]LimitInfo, error) {
	defer c.withSubtreeInterceptor(&c.limits.Interceptor)()
	compartmentID := c.tenant.TenancyOCID

	// Determine which services to query.
	var services []string
	if serviceName != "" {
		services = []string{serviceName}
	} else {
		var err error
		services, err = c.ListServices(ctx)
		if err != nil {
			return nil, fmt.Errorf("list services: %w", err)
		}
	}

	var result []LimitInfo
	for _, svc := range services {
		// 1. List limit definitions for this service.
		defReq := limits.ListLimitDefinitionsRequest{
			CompartmentId: common.String(compartmentID),
			ServiceName:   common.String(svc),
			Limit:         common.Int(100),
		}
		defResp, err := c.limits.ListLimitDefinitions(ctx, defReq)
		if err != nil {
			continue
		}

		for _, def := range defResp.Items {
			scopeType := "REGION"
			if def.ScopeType != "" {
				scopeType = string(def.ScopeType)
			}

			desc := ""
			if def.Description != nil {
				desc = *def.Description
			}
			svcName := ""
			if def.ServiceName != nil {
				svcName = *def.ServiceName
			}
			limitName := ""
			if def.Name != nil {
				limitName = *def.Name
			}

			// 2. List limit values (may be AD-scoped).
			valReq := limits.ListLimitValuesRequest{
				CompartmentId: common.String(compartmentID),
				ServiceName:   common.String(svc),
				Limit:         common.Int(100),
			}
			valResp, err := c.limits.ListLimitValues(ctx, valReq)
			if err != nil {
				// No values — add placeholder row.
				result = append(result, LimitInfo{
					ServiceName: svcName,
					LimitName:   limitName,
					Description: desc,
					ScopeType:   scopeType,
				})
				continue
			}

			for _, val := range valResp.Items {
				ad := ""
				if val.AvailabilityDomain != nil {
					ad = *val.AvailabilityDomain
				}
				limit := int64(0)
				if val.Value != nil {
					limit = *val.Value
				}

				// 3. Get resource availability.
				availReq := limits.GetResourceAvailabilityRequest{
					ServiceName:        common.String(svc),
					LimitName:          common.String(limitName),
					CompartmentId:      common.String(compartmentID),
					AvailabilityDomain: common.String(ad),
				}
				availResp, err := c.limits.GetResourceAvailability(ctx, availReq)
				var used, available int64
				if err == nil {
					if availResp.Used != nil {
						used = *availResp.Used
					}
					if availResp.Available != nil {
						available = *availResp.Available
					}
				}

				info := LimitInfo{
					ServiceName:        svcName,
					LimitName:          limitName,
					Description:        desc,
					ScopeType:          scopeType,
					AvailabilityDomain: ad,
					ServiceLimit:       limit,
					Used:               used,
					Available:          available,
				}
				result = append(result, info)
			}
		}
	}

	return result, nil
}

// --- One-Click 500Mbps ---

// Enable500Mbps sets up a Network Load Balancer with NAT Gateway routing
// to bypass the 50Mbps bandwidth cap on free-tier AMD instances.
// findInstanceNLB returns the NLB tagged for the given instance, or nil if none exists.
func (c *Client) findInstanceNLB(ctx context.Context, compartmentID, instanceID string) (*networkloadbalancer.NetworkLoadBalancerSummary, error) {
	listResp, err := c.nlb.ListNetworkLoadBalancers(ctx, networkloadbalancer.ListNetworkLoadBalancersRequest{
		CompartmentId: common.String(compartmentID),
		Limit:         common.Int(100),
	})
	if err != nil {
		return nil, fmt.Errorf("list NLBs: %w", err)
	}
	for i := range listResp.Items {
		nlb := &listResp.Items[i]
		if nlb.FreeformTags != nil && nlb.FreeformTags["oci-helper-instance-id"] == instanceID {
			return nlb, nil
		}
	}
	return nil, nil
}

// nlbPublicIP returns the first public IP of an NLB, falling back to the first address.
func nlbPublicIP(addrs []networkloadbalancer.IpAddress) string {
	for _, a := range addrs {
		if a.IsPublic != nil && *a.IsPublic && a.IpAddress != nil {
			return *a.IpAddress
		}
	}
	if len(addrs) > 0 && addrs[0].IpAddress != nil {
		return *addrs[0].IpAddress
	}
	return ""
}

// ensureNatGateway finds or creates a NAT gateway in the VCN.
func (c *Client) ensureNatGateway(ctx context.Context, compartmentID, vcnID string) (*core.NatGateway, error) {
	listResp, err := c.vcn.ListNatGateways(ctx, core.ListNatGatewaysRequest{
		CompartmentId: common.String(compartmentID),
		VcnId:         common.String(vcnID),
		LifecycleState: core.NatGatewayLifecycleStateAvailable,
	})
	if err != nil {
		return nil, fmt.Errorf("list nat gateways: %w", err)
	}
	if len(listResp.Items) > 0 {
		log.Printf("[Enable500Mbps] reusing existing NAT gateway: %s", *listResp.Items[0].DisplayName)
		return &listResp.Items[0], nil
	}

	createResp, err := c.vcn.CreateNatGateway(ctx, core.CreateNatGatewayRequest{
		CreateNatGatewayDetails: core.CreateNatGatewayDetails{
			CompartmentId: common.String(compartmentID),
			VcnId:         common.String(vcnID),
			DisplayName:   common.String("nat-gateway"),
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create nat gateway: %w", err)
	}
	ngwID := *createResp.NatGateway.Id

	// Poll until available.
	deadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) {
		getResp, err := c.vcn.GetNatGateway(ctx, core.GetNatGatewayRequest{NatGatewayId: common.String(ngwID)})
		if err != nil {
			return nil, fmt.Errorf("poll nat gateway: %w", err)
		}
		if getResp.LifecycleState == core.NatGatewayLifecycleStateAvailable {
			log.Printf("[Enable500Mbps] NAT gateway created: %s", ngwID)
			return &getResp.NatGateway, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(5 * time.Second):
		}
	}
	return nil, fmt.Errorf("NAT gateway creation timed out")
}

// ensureNatRouteTable finds or creates a route table with 0.0.0.0/0 → NAT Gateway.
func (c *Client) ensureNatRouteTable(ctx context.Context, compartmentID, vcnID, natGWID string) (*core.RouteTable, error) {
	listResp, err := c.vcn.ListRouteTables(ctx, core.ListRouteTablesRequest{
		CompartmentId: common.String(compartmentID),
		VcnId:         common.String(vcnID),
	})
	if err != nil {
		return nil, fmt.Errorf("list route tables: %w", err)
	}
	for _, rt := range listResp.Items {
		for _, rule := range rt.RouteRules {
			if rule.NetworkEntityId != nil && *rule.NetworkEntityId == natGWID &&
				rule.Destination != nil && *rule.Destination == "0.0.0.0/0" {
				log.Printf("[Enable500Mbps] reusing existing NAT route table: %s", *rt.DisplayName)
				return &rt, nil
			}
		}
	}

	// Create a new route table with 0.0.0.0/0 → NAT Gateway.
	createResp, err := c.vcn.CreateRouteTable(ctx, core.CreateRouteTableRequest{
		CreateRouteTableDetails: core.CreateRouteTableDetails{
			CompartmentId: common.String(compartmentID),
			VcnId:         common.String(vcnID),
			DisplayName:   common.String("nat-route-table"),
			RouteRules: []core.RouteRule{{
				Destination:     common.String("0.0.0.0/0"),
				DestinationType: core.RouteRuleDestinationTypeCidrBlock,
				NetworkEntityId: common.String(natGWID),
			}},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("create route table: %w", err)
	}
	log.Printf("[Enable500Mbps] NAT route table created: %s", *createResp.RouteTable.Id)
	return &createResp.RouteTable, nil
}

func (c *Client) Enable500Mbps(ctx context.Context, instanceID string) (string, error) {
	compartmentID := c.tenant.TenancyOCID

	// Idempotent: if an NLB already exists for this instance, return its public IP.
	if existing, err := c.findInstanceNLB(ctx, compartmentID, instanceID); err != nil {
		return "", err
	} else if existing != nil {
		return nlbPublicIP(existing.IpAddresses), nil
	}

	vnics, err := c.GetInstanceVNICs(ctx, compartmentID, instanceID)
	if err != nil {
		return "", fmt.Errorf("get vnics: %w", err)
	}
	if len(vnics) == 0 {
		return "", fmt.Errorf("no VNIC found")
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
	vnicID := ""
	if vnic.Id != nil {
		vnicID = *vnic.Id
	}
	if privateIP == "" || subnetID == "" || vnicID == "" {
		return "", fmt.Errorf("instance VNIC missing private IP, subnet ID, or VNIC ID")
	}

	// Get subnet to find VCN ID.
	subnet, err := c.vcn.GetSubnet(ctx, core.GetSubnetRequest{SubnetId: common.String(subnetID)})
	if err != nil {
		return "", fmt.Errorf("get subnet: %w", err)
	}
	if subnet.VcnId == nil {
		return "", fmt.Errorf("subnet missing VCN ID")
	}
	vcnID := *subnet.VcnId

	// 1. Create or find NAT Gateway.
	natGW, err := c.ensureNatGateway(ctx, compartmentID, vcnID)
	if err != nil {
		return "", fmt.Errorf("nat gateway: %w", err)
	}
	natGWID := *natGW.Id

	// 2. Ensure route table has 0.0.0.0/0 → NAT Gateway.
	rt, err := c.ensureNatRouteTable(ctx, compartmentID, vcnID, natGWID)
	if err != nil {
		return "", fmt.Errorf("nat route table: %w", err)
	}
	rtID := *rt.Id

	// 3. Bind VNIC to NAT route table and skip source/dest check.
	if _, err := c.vcn.UpdateVnic(ctx, core.UpdateVnicRequest{
		VnicId: common.String(vnicID),
		UpdateVnicDetails: core.UpdateVnicDetails{
			SkipSourceDestCheck: common.Bool(true),
			RouteTableId:        common.String(rtID),
		},
	}); err != nil {
		return "", fmt.Errorf("update vnic: %w", err)
	}
	log.Printf("[Enable500Mbps] VNIC %s bound to NAT route table %s", vnicID, rtID)

	// 4. Release all-ports security to ensure NLB traffic is not blocked.
	_ = c.ReleaseAllPorts(ctx, vcnID)

	// 5. Create the Network Load Balancer.
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
						{IpAddress: common.String(privateIP), Port: common.Int(0), Name: common.String(instanceID)},
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
		return "", fmt.Errorf("create NLB: %w", err)
	}
	workReqID := resp.OpcWorkRequestId
	if workReqID == nil || *workReqID == "" {
		return "", fmt.Errorf("NLB created but no work request ID returned")
	}
	log.Printf("[Enable500Mbps] NLB work request: %s", *workReqID)

	pollInterval := 5 * time.Second
	deadline := time.Now().Add(5 * time.Minute)
	for time.Now().Before(deadline) {
		wr, err := c.nlb.GetWorkRequest(ctx, networkloadbalancer.GetWorkRequestRequest{WorkRequestId: workReqID})
		if err != nil {
			return "", fmt.Errorf("poll NLB work request: %w", err)
		}
		status := wr.Status
		pct := float32(0)
		if wr.PercentComplete != nil {
			pct = *wr.PercentComplete
		}
		log.Printf("[Enable500Mbps] status=%s %.0f%%", status, float64(pct))
		switch status {
		case networkloadbalancer.OperationStatusSucceeded:
			if nlb, err := c.findInstanceNLB(ctx, compartmentID, instanceID); err == nil && nlb != nil {
				log.Printf("[Enable500Mbps] done — NLB IP: %s", nlbPublicIP(nlb.IpAddresses))
				return nlbPublicIP(nlb.IpAddresses), nil
			}
			return "", nil
		case networkloadbalancer.OperationStatusFailed:
			return "", fmt.Errorf("NLB creation failed")
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-time.After(pollInterval):
		}
	}
	return "", fmt.Errorf("NLB creation timed out")
}

// Disable500Mbps deletes the NLB and unbinds the VNIC from the NAT route table.
func (c *Client) Disable500Mbps(ctx context.Context, instanceID string) error {
	compartmentID := c.tenant.TenancyOCID

	// Unbind VNIC from NAT route table if bound.
	vnics, err := c.GetInstanceVNICs(ctx, compartmentID, instanceID)
	if err == nil && len(vnics) > 0 && vnics[0].Id != nil && vnics[0].RouteTableId != nil && *vnics[0].RouteTableId != "" {
		if _, err := c.vcn.UpdateVnic(ctx, core.UpdateVnicRequest{
			VnicId: common.String(*vnics[0].Id),
			UpdateVnicDetails: core.UpdateVnicDetails{
				SkipSourceDestCheck: common.Bool(false),
				RouteTableId:        common.String(""),
			},
		}); err != nil {
			log.Printf("[Disable500Mbps] unbind VNIC from NAT route table: %v", err)
		} else {
			log.Printf("[Disable500Mbps] VNIC %s unbound from NAT route table", *vnics[0].Id)
		}
	}

	nlb, err := c.findInstanceNLB(ctx, compartmentID, instanceID)
	if err != nil {
		return err
	}
	if nlb == nil {
		return nil // idempotent: nothing to delete
	}
	nlbID := nlb.Id

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
	defer c.withSubtreeInterceptor(&c.compute.Interceptor)()
	defer c.withSubtreeInterceptor(&c.vcn.Interceptor)()
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

	// Create new public IP first (preserve old IP in case CIDR filter rejects).
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

	// Check CIDR filter before deleting the old IP.
	if len(cidrList) > 0 {
		matched := false
		for _, cidr := range cidrList {
			if ipInCIDR(newIP, cidr) {
				matched = true
				break
			}
		}
		if !matched {
			// CIDR mismatch: delete the unwanted new IP and return error.
			if createResp.PublicIp.Id != nil {
				c.vcn.DeletePublicIp(ctx, core.DeletePublicIpRequest{PublicIpId: createResp.PublicIp.Id})
			}
			return "", fmt.Errorf("new IP %s not in desired CIDR ranges: %v", newIP, cidrList)
		}
	}

	// CIDR OK (or no filter): now safe to delete the old public IP.
	var oldIPID string
	var page *string
	for {
		pipReq := core.ListPublicIpsRequest{
			Scope:         core.ListPublicIpsScopeRegion,
			CompartmentId: common.String(c.tenant.TenancyOCID),
			Limit:         common.Int(100),
			Page:          page,
		}
		pipResp, err := c.vcn.ListPublicIps(ctx, pipReq)
		if err != nil {
			return "", fmt.Errorf("list public IPs: %w", err)
		}
		for _, ip := range pipResp.Items {
			if ip.IpAddress != nil && *ip.IpAddress == oldIP {
				oldIPID = *ip.Id
				break
			}
		}
		if oldIPID != "" {
			break
		}
		if pipResp.OpcNextPage == nil || *pipResp.OpcNextPage == "" {
			break
		}
		page = pipResp.OpcNextPage
	}
	if oldIPID != "" {
		if _, err := c.vcn.DeletePublicIp(ctx, core.DeletePublicIpRequest{PublicIpId: common.String(oldIPID)}); err != nil {
			log.Printf("[ChangeInstanceIP] delete old IP %s: %v", oldIP, err)
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
	defer c.withSubtreeInterceptor(&c.compute.Interceptor)()
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

// ── Cost / Usage ────────────────────────────────────────────────────────

// CostAnalysisParams configures a cost/usage query.
type CostAnalysisParams struct {
	StartDate   string // yyyy-MM-dd
	EndDate     string // yyyy-MM-dd
	Granularity string // DAILY or MONTHLY
	QueryType   string // COST or USAGE
	ReportType  string // COST_BY_SERVICE, COST_BY_SERVICE_AND_DESCRIPTION, COST_BY_SERVICE_AND_SKU, COST_BY_SERVICE_AND_TAG, COST_BY_COMPARTMENT, MONTHLY_COST
}

// CostAnalysisResult wraps the full cost analysis response.
type CostAnalysisResult struct {
	Total     int         `json:"total"`
	TotalCost float64     `json:"totalCost"`
	Currency  string      `json:"currency"`
	Items     []CostItem  `json:"items"`
}

// CostItem represents a single cost/usage entry.
type CostItem struct {
	Service          string  `json:"service"`
	Description      string  `json:"description"`
	SkuName          string  `json:"skuName"`
	CompartmentName  string  `json:"compartmentName"`
	Region           string  `json:"region"`
	Date             string  `json:"date"`
	Cost             float64 `json:"cost"`
	ComputedQuantity float64 `json:"computedQuantity"`
	Currency         string  `json:"currency"`
	Unit             string  `json:"unit"`
}

// buildGroupBy maps report type to OCI group-by dimensions.
func buildGroupBy(reportType string) []string {
	switch reportType {
	case "COST_BY_SERVICE_AND_DESCRIPTION":
		return []string{"service", "skuPartNumber"}
	case "COST_BY_SERVICE_AND_SKU":
		return []string{"service", "skuName"}
	case "COST_BY_SERVICE_AND_TAG":
		return []string{"service", "tagNamespace", "tagKey"}
	case "COST_BY_COMPARTMENT":
		return []string{"compartmentName"}
	case "MONTHLY_COST":
		return []string{"service"}
	default:
		return []string{"service"}
	}
}

// CostAnalysis queries the OCI Usage API with full parameter support.
// It paginates through all results and returns a summary with items.
func (c *Client) CostAnalysis(ctx context.Context, params CostAnalysisParams) (*CostAnalysisResult, error) {
	// Parse dates; end date gets +1 day to cover the full day (matching Java behavior).
	sdkStart := common.SDKTime{Time: time.Now().AddDate(0, -1, 0)}
	sdkEnd := common.SDKTime{Time: time.Now()}
	if t, err := time.Parse("2006-01-02", params.StartDate); err == nil {
		sdkStart = common.SDKTime{Time: t}
	}
	if t, err := time.Parse("2006-01-02", params.EndDate); err == nil {
		sdkEnd = common.SDKTime{Time: t.AddDate(0, 0, 1)}
	}

	// Granularity.
	var granularity usageapi.RequestSummarizedUsagesDetailsGranularityEnum
	if strings.EqualFold(params.Granularity, "MONTHLY") {
		granularity = usageapi.RequestSummarizedUsagesDetailsGranularityMonthly
	} else {
		granularity = usageapi.RequestSummarizedUsagesDetailsGranularityDaily
	}

	// Query type.
	var queryType usageapi.RequestSummarizedUsagesDetailsQueryTypeEnum
	if strings.EqualFold(params.QueryType, "USAGE") {
		queryType = usageapi.RequestSummarizedUsagesDetailsQueryTypeUsage
	} else {
		queryType = usageapi.RequestSummarizedUsagesDetailsQueryTypeCost
	}

	groupBy := buildGroupBy(params.ReportType)
	isAggregateByTime := false

	details := usageapi.RequestSummarizedUsagesDetails{
		TenantId:         &c.tenant.TenancyOCID,
		Granularity:      granularity,
		GroupBy:          groupBy,
		TimeUsageStarted: &sdkStart,
		TimeUsageEnded:   &sdkEnd,
		QueryType:        queryType,
		IsAggregateByTime: &isAggregateByTime,
	}

	// Compartment dimension requires compartmentDepth.
	for _, g := range groupBy {
		if g == "compartmentName" || g == "compartmentId" || g == "compartmentPath" {
			depth := float32(1)
			details.CompartmentDepth = &depth
			break
		}
	}

	// Paginated fetch.
	var allItems []usageapi.UsageSummary
	var page *string
	for {
		req := usageapi.RequestSummarizedUsagesRequest{
			RequestSummarizedUsagesDetails: details,
			Page:                           page,
		}
		resp, err := c.usageapi.RequestSummarizedUsages(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("usage query: %w", err)
		}
		allItems = append(allItems, resp.UsageAggregation.Items...)
		page = resp.OpcNextPage
		if page == nil || *page == "" {
			break
		}
	}

	// Transform.
	result := &CostAnalysisResult{Currency: "USD"}
	dateFmt := "2006-01-02"
	if strings.EqualFold(params.Granularity, "MONTHLY") {
		dateFmt = "2006-01"
	}
	for _, u := range allItems {
		item := CostItem{}
		if u.Service != nil {
			item.Service = *u.Service
		}
		if u.SkuPartNumber != nil {
			item.Description = *u.SkuPartNumber
		}
		if u.SkuName != nil {
			item.SkuName = *u.SkuName
		}
		if u.CompartmentName != nil {
			item.CompartmentName = *u.CompartmentName
		}
		if u.Region != nil {
			item.Region = *u.Region
		}
		if u.Unit != nil {
			item.Unit = *u.Unit
		}
		if u.ComputedQuantity != nil {
			item.ComputedQuantity = float64(*u.ComputedQuantity)
		}
		if u.ComputedAmount != nil {
			item.Cost = float64(*u.ComputedAmount)
		}
		if u.Currency != nil {
			item.Currency = strings.TrimSpace(*u.Currency)
		}
		if u.TimeUsageStarted != nil {
			item.Date = u.TimeUsageStarted.Format(dateFmt)
		}
		// Capture first non-empty currency for summary.
		if result.Currency == "USD" && item.Currency != "" {
			result.Currency = item.Currency
		}
		result.TotalCost += item.Cost
		result.Items = append(result.Items, item)
	}
	result.Total = len(result.Items)

	return result, nil
}

// CostSummary is DEPRECATED; use CostAnalysis instead.
// Kept for backward compatibility — calls CostAnalysis with MONTHLY_COST defaults.
func (c *Client) CostSummary(ctx context.Context, startDate, endDate string) ([]CostItem, error) {
	r, err := c.CostAnalysis(ctx, CostAnalysisParams{
		StartDate:   startDate,
		EndDate:     endDate,
		Granularity: "MONTHLY",
		QueryType:   "COST",
		ReportType:  "MONTHLY_COST",
	})
	if err != nil {
		return nil, err
	}
	if r == nil {
		return nil, nil
	}
	return r.Items, nil
}

// Tenant returns the tenant config.
