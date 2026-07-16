package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/viogus/oci-helper-go/internal/db"
	ociclient "github.com/viogus/oci-helper-go/internal/oci"
)

// tenantInfoCache caches enriched tenant info responses for 10 minutes
// to avoid repeated OCI API calls.
var tenantInfoCache = struct {
	sync.RWMutex
	m map[int64]tenantInfoCacheEntry
}{m: make(map[int64]tenantInfoCacheEntry)}

type tenantInfoCacheEntry struct {
	data      map[string]interface{}
	expiresAt time.Time
}

// tenantUserInfo is a lightweight user representation for the tenant info response.
type tenantUserInfo struct {
	ID, Name, Email, LifecycleState string
	IsMFA, EmailVerified            bool
	TimeCreated, LastLogin          string
}

// --- tenants ---

func (s *Server) handleTenants(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		keyword := r.URL.Query().Get("keyword")
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		size, _ := strconv.Atoi(r.URL.Query().Get("size"))
		if size < 1 {
			size = 20
		}
		list, total, err := s.store.ListTenantsPaginated(keyword, page, size)
		if err != nil {
			jsonErr(w, "list tenants: "+err.Error())
			return
		}
		if list == nil {
			list = []db.Tenant{}
		}
		jsonOK(w, map[string]interface{}{"data": list, "total": total, "page": page, "size": size})
	case http.MethodPost:
		var t db.Tenant
		if err := json.NewDecoder(r.Body).Decode(&t); err != nil {
			jsonErr(w, "invalid body: "+err.Error())
			return
		}
		if t.Name == "" || t.TenancyOCID == "" || t.Region == "" || t.KeyFile == "" {
			jsonErr(w, "name, tenancyOcid, region, and keyFile are required")
			return
		}
		// Validate OCI connectivity before saving
		tCopy := t
		keyPath := t.KeyFile
		if !filepath.IsAbs(keyPath) {
			keyPath = filepath.Join(s.cfg.KeysDir, keyPath)
		}
		tCopy.KeyFile = keyPath
		client, err := ociclient.NewClient(&tCopy, "")
		if err != nil {
			jsonErr(w, "oci client: "+err.Error())
			return
		}
		if err := client.ValidateCredentials(r.Context(), t.TenancyOCID); err != nil {
			jsonErr(w, "OCI connectivity check failed: "+err.Error())
			return
		}
		if err := s.store.CreateTenant(&t); err != nil {
			jsonErr(w, "create tenant: "+err.Error())
			return
		}
		s.audit(t.ID, "tenant:create", t.Name, r)
		jsonOK(w, t)
	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleTenantByID(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/api/tenants/")
	idStr = strings.TrimSuffix(idStr, "/")

	// /api/tenants/{id}/info — enriched detail
	if strings.HasSuffix(idStr, "/info") {
		s.handleTenantInfo(w, r)
		return
	}

	// --- identity management sub-routes ---
	// /api/tenants/{id}/users/delete (must be before /users)
	if strings.HasSuffix(idStr, "/users/delete") {
		s.handleTenantUserDelete(w, r)
		return
	}
	if strings.HasSuffix(idStr, "/users/reset-password") {
		s.handleTenantUserResetPassword(w, r)
		return
	}
	if strings.HasSuffix(idStr, "/users/update") {
		s.handleTenantUserUpdate(w, r)
		return
	}
	// /api/tenants/{id}/users — list identity users
	if strings.HasSuffix(idStr, "/users") {
		s.handleTenantUsers(w, r)
		return
	}
	// /api/tenants/{id}/mfa/clear
	if strings.HasSuffix(idStr, "/mfa/clear") {
		s.handleTenantMFAClear(w, r)
		return
	}
	// /api/tenants/{id}/api-keys/clear
	if strings.HasSuffix(idStr, "/api-keys/clear") {
		s.handleTenantAPIKeysClear(w, r)
		return
	}
	// /api/tenants/{id}/password-policy
	if strings.HasSuffix(idStr, "/password-policy") {
		s.handleTenantPasswordPolicy(w, r)
		return
	}
	// G8: /api/tenants/{id}/proxy
	if strings.HasSuffix(idStr, "/proxy") {
		s.handleTenantProxy(w, r)
		return
	}
	// G15: /api/tenants/refresh-plan-type
	if strings.HasSuffix(idStr, "/refresh-plan-type") {
		s.handleRefreshPlanType(w, r)
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		jsonErr(w, "invalid tenant id")
		return
	}

	switch r.Method {
	case http.MethodPatch:
		var req struct {
			Name         string `json:"name"`
			NotifyTG     string `json:"notify_tg"`
			NotifyDingtalk string `json:"notify_dingtalk"`
			Region       string `json:"region"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonErr(w, "invalid body: "+err.Error())
			return
		}
		t, _ := s.store.GetTenant(id)
		if t == nil {
			jsonErr(w, "not found")
			return
		}
		if req.Name != "" {
			t.Name = req.Name
		}
		if req.Region != "" {
			t.Region = req.Region
		}
		// Notification settings stored in config table
		if req.NotifyTG != "" {
			s.store.SetConfig(fmt.Sprintf("tenant_ntg_%d", id), req.NotifyTG)
		}
		if req.NotifyDingtalk != "" {
			s.store.SetConfig(fmt.Sprintf("tenant_ndtalk_%d", id), req.NotifyDingtalk)
		}
		// Update tenant in DB
		if _, err := s.store.DB().Exec(`UPDATE tenants SET name=?, region=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
			t.Name, t.Region, id); err != nil {
			jsonErr(w, "update tenant: "+err.Error())
			return
		}
		s.audit(id, "tenant:update", t.Name, r)
		jsonOK(w, t)
	case http.MethodGet:
		t, _ := s.store.GetTenant(id)
		if t == nil {
			jsonErr(w, "not found")
			return
		}
		jsonOK(w, t)
	case http.MethodDelete:
			if err := s.store.DeleteTenantCascade(id); err != nil {
				jsonErr(w, "delete tenant: "+err.Error())
				return
			}
		s.audit(id, "tenant:delete", fmt.Sprintf("id=%d", id), r)
		jsonOK(w, map[string]string{"status": "ok"})
	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

// GET /api/tenants/{id}/info — enriched tenant detail with OCI data.
func (s *Server) handleTenantInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	idStr := strings.TrimPrefix(r.URL.Path, "/api/tenants/")
	idStr = strings.TrimSuffix(strings.TrimSuffix(idStr, "/info"), "/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		jsonErr(w, "invalid tenant id")
		return
	}

	// Check in-memory cache (10-minute TTL).
	tenantInfoCache.RLock()
	if entry, ok := tenantInfoCache.m[id]; ok && time.Now().Before(entry.expiresAt) {
		tenantInfoCache.RUnlock()
		jsonOK(w, entry.data)
		return
	}
	tenantInfoCache.RUnlock()

	t, _ := s.store.GetTenant(id)
	if t == nil {
		jsonErr(w, "not found")
		return
	}

	client, err := s.clientFor(t)
	if err != nil {
		jsonOK(w, map[string]interface{}{
			"tenant":        t,
			"regions":       []string{},
			"instanceStats": map[string]int{},
		})
		return
	}

	// Parallel OCI queries: region subscriptions + users + tenancy.
	var wg sync.WaitGroup
	var mu sync.Mutex
	var regionNames = make([]string, 0)
	var userList = make([]tenantUserInfo, 0)
	var userErr, subscriptionErr error
	var subscriptionResult map[string]interface{}

	wg.Add(3)
	// Goroutine 1: list region subscriptions.
	go func() {
		defer wg.Done()
		regions, err := client.ListRegionSubscriptions(r.Context())
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			log.Printf("[tenant-info] list regions for tenant %d: %v", id, err)
			return
		}
		for _, reg := range regions {
			if reg.RegionName != nil {
				regionNames = append(regionNames, *reg.RegionName)
			}
		}
	}()
	// Goroutine 2: list identity users.
	go func() {
		defer wg.Done()
		users, err := client.ListUsers(r.Context(), t.TenancyOCID)
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			userErr = err
			return
		}
		for _, u := range users {
			userList = append(userList, tenantUserInfo{
				ID:             safeStr(u.Id),
				Name:           safeStr(u.Name),
				Email:          safeStr(u.Email),
				LifecycleState: string(u.LifecycleState),
				IsMFA:          boolVal(u.IsMfaActivated),
				EmailVerified:  boolVal(u.EmailVerified),
				TimeCreated:    timeStr(u.TimeCreated),
				LastLogin:      timeStr(u.LastSuccessfulLoginTime),
			})
		}
	}()
	// Goroutine 3: subscription info via OSP Gateway.
	go func() {
		defer wg.Done()
		sub, err := client.GetSubscriptionInfo(r.Context())
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			subscriptionErr = err
			return
		}
		if sub != nil {
			subscriptionResult = map[string]interface{}{
				"planType":                     string(sub.PlanType),
				"accountType":                  string(sub.AccountType),
				"currencyCode":                 safeStr(sub.CurrencyCode),
				"upgradeState":                 string(sub.UpgradeState),
				"timeStart":                    timeStr(sub.TimeStart),
				"isIntentToPay":                boolVal(sub.IsIntentToPay),
				"isCorporateConversionAllowed": boolVal(sub.IsCorporateConversionAllowed),
			}
		}
	}()
	wg.Wait()

	// Log non-fatal errors from parallel queries.
	if userErr != nil {
		log.Printf("[tenant-info] list users for tenant %d: %v", id, userErr)
	}
	if subscriptionErr != nil {
		log.Printf("[tenant-info] subscription for tenant %d: %v", id, subscriptionErr)
	}

	// Instance stats (from local DB — already fast, no need to parallelise).
	instances, _ := s.store.ListInstances(id)
	stats := map[string]int{"total": 0, "RUNNING": 0, "STOPPED": 0, "TERMINATED": 0}
	totalOCPU := 0.0
	totalMem := 0.0
	for _, inst := range instances {
		stats["total"]++
		stats[inst.State]++
		totalOCPU += inst.OCPU
		totalMem += inst.MemoryGB
	}

	// Password policy from config table.
	passwordExpiresAfter := 0
	if v, err := s.store.GetConfig(fmt.Sprintf("tenant_pwdexp_%d", id)); err == nil {
		passwordExpiresAfter, _ = strconv.Atoi(v)
	}

	// Notification recipients from config table.
	notificationRecipients := []string{}
	if tg, err := s.store.GetConfig(fmt.Sprintf("tenant_ntg_%d", id)); err == nil && tg != "" {
		notificationRecipients = append(notificationRecipients, tg)
	}
	if dtalk, err := s.store.GetConfig(fmt.Sprintf("tenant_ndtalk_%d", id)); err == nil && dtalk != "" {
		notificationRecipients = append(notificationRecipients, dtalk)
	}

	// Notification test mode — always false for now (simplified).
	notificationTestModeEnabled := false

	// Account creation time: prefer OCI tenancy time, fall back to DB created_at.
	accountCreationTime := t.CreatedAt.Format(time.RFC3339)

	// Subscription info from OSP Gateway (nil if unavailable or unauthorized).
	subscription := interface{}(subscriptionResult)

	resp := map[string]interface{}{
		"tenant":                     t,
		"regions":                    regionNames,
		"instanceStats":              stats,
		"totalOCPU":                  totalOCPU,
		"totalMemoryGB":              totalMem,
		"users":                      userList,
		"passwordExpiresAfter":       passwordExpiresAfter,
		"notificationRecipients":     notificationRecipients,
		"notificationTestModeEnabled": notificationTestModeEnabled,
		"subscription":               subscription,
		"accountCreationTime":        accountCreationTime,
	}

	// Store in cache with 10-minute TTL.
	tenantInfoCache.Lock()
	tenantInfoCache.m[id] = tenantInfoCacheEntry{
		data:      resp,
		expiresAt: time.Now().Add(10 * time.Minute),
	}
	tenantInfoCache.Unlock()

	jsonOK(w, resp)
}

// --- instances ---

// tenantAndClient extracts the tenant ID from the URL, fetches the tenant from
// the store, and creates an OCI client. It writes an error response and returns
// false on failure.
func (s *Server) tenantAndClient(w http.ResponseWriter, r *http.Request, idStr string) (int64, *db.Tenant, *ociclient.Client, bool) {
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		jsonErr(w, "invalid tenant id")
		return 0, nil, nil, false
	}
	t, _ := s.store.GetTenant(id)
	if t == nil {
		jsonErr(w, "tenant not found")
		return 0, nil, nil, false
	}
	client, err := s.clientFor(t)
	if err != nil {
		jsonErr(w, "create OCI client: "+err.Error())
		return 0, nil, nil, false
	}
	return id, t, client, true
}

// trimTenantSuffix trims /api/tenants/ prefix and the given suffix from the URL
// path, returning the bare tenant ID string.
func trimTenantSuffix(path, suffix string) string {
	idStr := strings.TrimPrefix(path, "/api/tenants/")
	idStr = strings.TrimSuffix(idStr, suffix)
	return strings.TrimSuffix(idStr, "/")
}

// --- identity user management ---

// GET /api/tenants/{id}/users — list identity users for a tenancy.
func (s *Server) handleTenantUsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	idStr := trimTenantSuffix(r.URL.Path, "/users")
	_, t, client, ok := s.tenantAndClient(w, r, idStr)
	if !ok {
		return
	}
	users, err := client.ListUsers(r.Context(), t.TenancyOCID)
	if err != nil {
		jsonErr(w, "list users: "+err.Error())
		return
	}
	type userInfo struct {
		ID             string `json:"id"`
		Name           string `json:"name"`
		Email          string `json:"email"`
		LifecycleState string `json:"lifecycleState"`
		IsMFA          bool   `json:"isMfaActivated"`
		EmailVerified  bool   `json:"emailVerified"`
		TimeCreated    string `json:"timeCreated"`
		LastLogin      string `json:"lastSuccessfulLoginTime"`
	}
	var result []userInfo
	for _, u := range users {
		result = append(result, userInfo{
			ID:             safeStr(u.Id),
			Name:           safeStr(u.Name),
			Email:          safeStr(u.Email),
			LifecycleState: string(u.LifecycleState),
			IsMFA:          boolVal(u.IsMfaActivated),
			EmailVerified:  boolVal(u.EmailVerified),
			TimeCreated:    timeStr(u.TimeCreated),
			LastLogin:      timeStr(u.LastSuccessfulLoginTime),
		})
	}
	jsonOK(w, map[string]interface{}{"users": result})
}

// POST /api/tenants/{id}/users/delete — delete an identity user.
func (s *Server) handleTenantUserDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	idStr := trimTenantSuffix(r.URL.Path, "/users/delete")
	id, _, client, ok := s.tenantAndClient(w, r, idStr)
	if !ok {
		return
	}
	var body struct {
		UserID string `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}
	if body.UserID == "" {
		jsonErr(w, "user_id required")
		return
	}
	if err := client.DeleteUser(r.Context(), body.UserID); err != nil {
		jsonErr(w, "delete user: "+err.Error())
		return
	}
	s.audit(id, "user:delete", body.UserID, r)
	jsonOK(w, map[string]string{"status": "ok", "message": "User deleted"})
}

// POST /api/tenants/{id}/users/reset-password — reset/create UI password for a user.
func (s *Server) handleTenantUserResetPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	idStr := trimTenantSuffix(r.URL.Path, "/users/reset-password")
	id, _, client, ok := s.tenantAndClient(w, r, idStr)
	if !ok {
		return
	}
	var body struct {
		UserID string `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}
	if body.UserID == "" {
		jsonErr(w, "user_id required")
		return
	}
	// Attempt 1: classic IAM API (works for non-Identity-Domain users).
	resp, err := client.CreateOrResetUIPassword(r.Context(), body.UserID)
	if err == nil {
		pw := ""
		if resp.Password != nil {
			pw = *resp.Password
		}
		s.audit(id, "user:password:reset", body.UserID, r)
		jsonOK(w, map[string]string{"status": "ok", "password": pw})
		return
	}

	// Check if error is a 404/400 — user may be in Identity Domains only.
	var svcErr common.ServiceError
	if errors.As(err, &svcErr) {
		code := svcErr.GetHTTPStatusCode()
		if code == 404 || code == 400 {
			log.Printf("[password-reset] classic API returned %d, trying Identity Domains fallback for user %s", code, body.UserID)
			domainURL, domainErr := client.GetDomainURL(r.Context())
			if domainErr != nil {
				jsonErr(w, "reset password: classic API failed and no Identity Domain available: "+domainErr.Error())
				return
			}
			newPW, domainErr := client.ResetPasswordViaDomain(r.Context(), body.UserID, domainURL)
			if domainErr != nil {
				jsonErr(w, "reset password via Identity Domain: "+domainErr.Error())
				return
			}
			s.audit(id, "user:password:reset:domain", body.UserID, r)
			jsonOK(w, map[string]string{"status": "ok", "password": newPW})
			return
		}
	}
	jsonErr(w, "reset password: "+err.Error())
}

// POST /api/tenants/{id}/users/update — update user email and/or description.
func (s *Server) handleTenantUserUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	idStr := trimTenantSuffix(r.URL.Path, "/users/update")
	id, _, client, ok := s.tenantAndClient(w, r, idStr)
	if !ok {
		return
	}
	var body struct {
		UserID      string `json:"user_id"`
		Email       string `json:"email"`
		Description string `json:"description"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}
	if body.UserID == "" {
		jsonErr(w, "user_id required")
		return
	}
	if body.Email == "" && body.Description == "" {
		jsonErr(w, "at least one of email or description required")
		return
	}
	var emailPtr, descPtr *string
	if body.Email != "" {
		emailPtr = &body.Email
	}
	if body.Description != "" {
		descPtr = &body.Description
	}
	user, err := client.UpdateUser(r.Context(), body.UserID, emailPtr, descPtr)
	if err != nil {
		jsonErr(w, "update user: "+err.Error())
		return
	}
	s.audit(id, "user:update", body.UserID, r)
	jsonOK(w, user)
}

// POST /api/tenants/{id}/mfa/clear — clear all MFA TOTP devices for a user.
func (s *Server) handleTenantMFAClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	idStr := trimTenantSuffix(r.URL.Path, "/mfa/clear")
	id, _, client, ok := s.tenantAndClient(w, r, idStr)
	if !ok {
		return
	}
	var body struct {
		UserID string `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}
	if body.UserID == "" {
		jsonErr(w, "user_id required")
		return
	}
	devices, err := client.ListMfaTotpDevices(r.Context(), body.UserID)
	if err != nil {
		jsonErr(w, "list mfa devices: "+err.Error())
		return
	}
	var deleted int
	for _, d := range devices {
		if d.Id == nil {
			continue
		}
		if err := client.DeleteMfaTotpDevice(r.Context(), body.UserID, *d.Id); err != nil {
			jsonErr(w, fmt.Sprintf("delete mfa device %s: %v", *d.Id, err))
			return
		}
		deleted++
	}
	s.audit(id, "user:mfa:clear", body.UserID, r)
	jsonOK(w, map[string]interface{}{"status": "ok", "deleted": deleted})
}

// POST /api/tenants/{id}/api-keys/clear — clear all API keys for a user.
func (s *Server) handleTenantAPIKeysClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	idStr := trimTenantSuffix(r.URL.Path, "/api-keys/clear")
	id, _, client, ok := s.tenantAndClient(w, r, idStr)
	if !ok {
		return
	}
	var body struct {
		UserID string `json:"user_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}
	if body.UserID == "" {
		jsonErr(w, "user_id required")
		return
	}
	keys, err := client.ListApiKeys(r.Context(), body.UserID)
	if err != nil {
		jsonErr(w, "list api keys: "+err.Error())
		return
	}
	var deleted int
	for _, k := range keys {
		if k.Fingerprint == nil {
			continue
		}
		if err := client.DeleteApiKey(r.Context(), body.UserID, *k.Fingerprint); err != nil {
			jsonErr(w, fmt.Sprintf("delete api key %s: %v", *k.Fingerprint, err))
			return
		}
		deleted++
	}
	s.audit(id, "user:apikeys:clear", body.UserID, r)
	jsonOK(w, map[string]interface{}{"status": "ok", "deleted": deleted})
}

// POST /api/tenants/{id}/password-policy — store password expiration setting.
func (s *Server) handleTenantPasswordPolicy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	idStr := trimTenantSuffix(r.URL.Path, "/password-policy")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		jsonErr(w, "invalid tenant id")
		return
	}
	t, _ := s.store.GetTenant(id)
	if t == nil {
		jsonErr(w, "tenant not found")
		return
	}
	var body struct {
		PasswordExpiresAfter int `json:"password_expires_after"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}
	configKey := fmt.Sprintf("tenant_pwdexp_%d", id)
	if err := s.store.SetConfig(configKey, strconv.Itoa(body.PasswordExpiresAfter)); err != nil {
		jsonErr(w, "save password policy: "+err.Error())
		return
	}
	jsonOK(w, map[string]interface{}{
		"status":                 "ok",
		"password_expires_after": body.PasswordExpiresAfter,
	})
}

// ── G8: Proxy Configuration ─────────────────────────────────────────────

func (s *Server) handleTenantProxy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	idStr := strings.TrimPrefix(r.URL.Path, "/api/tenants/")
	idStr = strings.TrimSuffix(strings.TrimSuffix(idStr, "/proxy"), "/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		jsonErr(w, "invalid tenant id")
		return
	}
	t, _ := s.store.GetTenant(id)
	if t == nil {
		jsonErr(w, "tenant not found")
		return
	}
	var req struct {
		ProxyURL string `json:"proxy_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}
	configKey := fmt.Sprintf("tenant_proxy_%d", id)
	if err := s.store.SetConfig(configKey, req.ProxyURL); err != nil {
		jsonErr(w, "save proxy config: "+err.Error())
		return
	}
	s.audit(id, "tenant:proxy", req.ProxyURL, r)
	jsonOK(w, map[string]string{"status": "ok"})
}

// ── G9: Bulk Upload Config ──────────────────────────────────────────────

func (s *Server) handleTenantUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		jsonErr(w, "parse multipart form: "+err.Error())
		return
	}
	keyFile, handler, err := r.FormFile("key_file")
	if err != nil {
		jsonErr(w, "key_file required: "+err.Error())
		return
	}
	defer keyFile.Close()

	// Read the key file content
	buf := make([]byte, handler.Size)
	if _, err := keyFile.Read(buf); err != nil {
		jsonErr(w, "read key file: "+err.Error())
		return
	}

	// Generate unique filename if not provided
	filename := handler.Filename
	if filename == "" {
		filename = fmt.Sprintf("upload_%d.pem", time.Now().UnixNano())
	}
	keyPath := filepath.Join(s.cfg.KeysDir, filename)
	if err := os.WriteFile(keyPath, buf, 0600); err != nil {
		jsonErr(w, "save key file: "+err.Error())
		return
	}

	// Parse tenant fields from form
	tenant := &db.Tenant{
		Name:        r.FormValue("name"),
		TenancyOCID: r.FormValue("tenancy_ocid"),
		UserOCID:    r.FormValue("user_ocid"),
		Fingerprint: r.FormValue("fingerprint"),
		Region:      r.FormValue("region"),
		KeyFile:     filename,
	}
	if tenant.Name == "" || tenant.TenancyOCID == "" || tenant.UserOCID == "" || tenant.Fingerprint == "" || tenant.Region == "" {
		jsonErr(w, "all fields required: name, tenancy_ocid, user_ocid, fingerprint, region, key_file")
		return
	}

	// Validate OCI connectivity before saving
	tCopy := *tenant
	tCopy.KeyFile = keyPath
	client, err := ociclient.NewClient(&tCopy, "")
	if err != nil {
		jsonErr(w, "oci client: "+err.Error())
		return
	}
	if err := client.ValidateCredentials(r.Context(), tenant.TenancyOCID); err != nil {
		jsonErr(w, "OCI connectivity check failed: "+err.Error())
		return
	}

	if err := s.store.CreateTenant(tenant); err != nil {
		jsonErr(w, "create tenant: "+err.Error())
		return
	}
	s.audit(tenant.ID, "tenant:upload", tenant.Name, r)
	jsonOK(w, tenant)
}

// ── G15: Refresh Plan Type ──────────────────────────────────────────────

func (s *Server) handleRefreshPlanType(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		TenantID int64 `json:"tenant_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}
	t, _ := s.store.GetTenant(req.TenantID)
	if t == nil {
		jsonErr(w, "tenant not found")
		return
	}

	// Call OCI OSP Gateway to get the actual subscription plan type.
	planType := ""
	client, err := s.clientFor(t)
	if err != nil {
		log.Printf("[refresh-plan-type] client for tenant %d: %v", req.TenantID, err)
	} else {
		sub, err := client.GetSubscriptionInfo(r.Context())
		if err != nil {
			log.Printf("[refresh-plan-type] subscription for tenant %d: %v", req.TenantID, err)
		} else if sub != nil {
			planType = string(sub.PlanType)
		}
	}

	// Store plan type and refresh timestamp.
	if planType != "" {
		if err := s.store.SetConfig(fmt.Sprintf("tenant_plan_type_%d", req.TenantID), planType); err != nil {
			log.Printf("[refresh-plan-type] save plan type for tenant %d: %v", req.TenantID, err)
		}
	}
	configKey := fmt.Sprintf("tenant_plan_refresh_%d", req.TenantID)
	if err := s.store.SetConfig(configKey, time.Now().Format(time.RFC3339)); err != nil {
		jsonErr(w, "save refresh time: "+err.Error())
		return
	}
	s.audit(req.TenantID, "tenant:refresh-plan-type", "", r)
	jsonOK(w, map[string]string{
		"status":   "ok",
		"planType": planType,
	})
}

// --- helpers ---

func safeStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func boolVal(b *bool) bool {
	if b == nil {
		return false
	}
	return *b
}

func timeStr(t *common.SDKTime) string {
	if t == nil {
		return ""
	}
	return t.Format(time.RFC3339)
}

