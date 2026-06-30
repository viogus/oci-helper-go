package oci

import (
	"context"
	"fmt"
	"log"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/ospgateway"
)

// GetSubscriptionInfo queries the OSP Gateway for the tenant's subscription details.
// Requires home-region resolution; returns nil if no subscription found or unauthorized.
func (c *Client) GetSubscriptionInfo(ctx context.Context) (*ospgateway.Subscription, error) {
	// Resolve the home region from the subscribed regions list.
	regions, err := c.ListRegionSubscriptions(ctx)
	if err != nil {
		return nil, fmt.Errorf("list region subscriptions: %w", err)
	}
	var homeRegion string
	for _, r := range regions {
		if r.IsHomeRegion != nil && *r.IsHomeRegion {
			homeRegion = *r.RegionName
			break
		}
	}
	if homeRegion == "" {
		return nil, fmt.Errorf("home region not found")
	}

	// OSP Gateway must use the home region endpoint.
	// Save and restore the base client. Note: SetRegion may mutate additional
	// internal SDK state beyond BaseClient. Client MUST NOT be shared across
	// goroutines (current design creates a fresh Client per request).
	baseClient := c.subscription.BaseClient
	c.subscription.SetRegion(homeRegion)

	// List subscriptions for this tenancy.
	listReq := ospgateway.ListSubscriptionsRequest{
		CompartmentId: common.String(c.tenant.TenancyOCID),
		OspHomeRegion: common.String(homeRegion),
	}
	listResp, err := c.subscription.ListSubscriptions(ctx, listReq)
	if err != nil {
		log.Printf("[subscription] list failed for tenant %d: %v", c.tenant.ID, err)
		c.subscription.BaseClient = baseClient
		return nil, fmt.Errorf("list subscriptions: %w", err)
	}
	if len(listResp.SubscriptionCollection.Items) == 0 {
		log.Printf("[subscription] no subscriptions for tenant %d", c.tenant.ID)
		c.subscription.BaseClient = baseClient
		return nil, nil
	}

	// Get the first subscription's details.
	subscriptionID := *listResp.SubscriptionCollection.Items[0].Id
	getReq := ospgateway.GetSubscriptionRequest{
		SubscriptionId: common.String(subscriptionID),
		CompartmentId:  common.String(c.tenant.TenancyOCID),
		OspHomeRegion:  common.String(homeRegion),
	}
	getResp, err := c.subscription.GetSubscription(ctx, getReq)
	c.subscription.BaseClient = baseClient
	if err != nil {
		return nil, fmt.Errorf("get subscription: %w", err)
	}
	return &getResp.Subscription, nil
}
