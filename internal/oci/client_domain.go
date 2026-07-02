package oci

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/identity"
	"github.com/oracle/oci-go-sdk/v65/identitydomains"
)

// ListDomains returns all active identity domains in the tenancy.
func (c *Client) ListDomains(ctx context.Context) ([]identity.DomainSummary, error) {
	defer c.withSubtreeInterceptor(&c.identity.Interceptor)()
	req := identity.ListDomainsRequest{
		CompartmentId: common.String(c.tenant.TenancyOCID),
		Limit:         common.Int(100),
	}
	resp, err := c.identity.ListDomains(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("list domains: %w", err)
	}
	var active []identity.DomainSummary
	for _, d := range resp.Items {
		if d.LifecycleState == identity.DomainLifecycleStateActive {
			active = append(active, d)
		}
	}
	return active, nil
}

// GetDomainURL returns the URL of the first active identity domain, or empty string.
func (c *Client) GetDomainURL(ctx context.Context) (string, error) {
	domains, err := c.ListDomains(ctx)
	if err != nil {
		return "", err
	}
	if len(domains) == 0 {
		return "", fmt.Errorf("no active identity domain found")
	}
	if domains[0].Url == nil {
		return "", fmt.Errorf("domain URL is nil")
	}
	return *domains[0].Url, nil
}

// ResetPasswordViaDomain resets a user's password through the Identity Domains SCIM API.
// This is the fallback for users in new-style tenancies where the classic IAM
// CreateOrResetUIPassword returns 404 (user only exists in Identity Domains).
func (c *Client) ResetPasswordViaDomain(ctx context.Context, classicUserID, domainURL string) (string, error) {
	// Create a domain-specific client using the same credentials but the domain endpoint.
	domainsClient, err := identitydomains.NewIdentityDomainsClientWithConfigurationProvider(
		c.rawCfg, domainURL,
	)
	if err != nil {
		return "", fmt.Errorf("create identity domains client: %w", err)
	}

	// Step 1: resolve Classic OCID → Identity Domain SCIM UUID (SCIM id).
	listReq := identitydomains.ListUsersRequest{
		Filter:     common.String(fmt.Sprintf("ocid eq \"%s\"", classicUserID)),
		Attributes: common.String("id,ocid,userName"),
		Count:      common.Int(1),
	}
	listResp, err := domainsClient.ListUsers(ctx, listReq)
	if err != nil {
		return "", fmt.Errorf("identity domains list users by ocid: %w", err)
	}
	if listResp.Resources == nil || len(listResp.Resources) == 0 {
		return "", fmt.Errorf("user %s not found in identity domain", classicUserID)
	}
	scimUserID := *listResp.Resources[0].Id

	// Step 2: generate a new password and force-set it via UserPasswordChanger.
	newPassword, err := generateRandomPassword(16)
	if err != nil {
		return "", fmt.Errorf("generate password: %w", err)
	}
	body := identitydomains.UserPasswordChanger{
		Schemas:            []string{"urn:ietf:params:scim:schemas:oracle:idcs:UserPasswordChanger"},
		Password:           common.String(newPassword),
		BypassNotification: common.Bool(true),
	}
	putReq := identitydomains.PutUserPasswordChangerRequest{
		UserPasswordChangerId: common.String(scimUserID),
		UserPasswordChanger:   body,
	}
	_, err = domainsClient.PutUserPasswordChanger(ctx, putReq)
	if err != nil {
		return "", fmt.Errorf("put user password changer: %w", err)
	}
	return newPassword, nil
}

// generateRandomPassword creates a cryptographically random password of given length.
func generateRandomPassword(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*"
	b := make([]byte, length)
	for i := range b {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		b[i] = charset[n.Int64()]
	}
	return string(b), nil
}
