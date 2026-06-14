package oci

import (
	"context"
	"fmt"
	"sync"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/oracle/oci-go-sdk/v65/identity"

	"github.com/viogus/oci-helper-go/internal/db"
)

type Client struct {
	tenant     *db.Tenant
	rawCfg     common.ConfigurationProvider
	compute    core.ComputeClient
	vcn        core.VirtualNetworkClient
	identity   identity.IdentityClient
	bootVolume core.BlockstorageClient
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

	return &Client{
		tenant:     t,
		rawCfg:     cfg,
		compute:    compute,
		vcn:        vcn,
		identity:   id,
		bootVolume: bv,
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
