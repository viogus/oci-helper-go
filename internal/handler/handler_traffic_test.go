package handler

import (
	"context"
	"math"
	"net/http"
	"strings"
	"testing"
	"time"
)

// TestTrafficAccountStatsValidation covers the request-validation branches that
// are reachable without OCI credentials.
func TestTrafficAccountStatsValidation(t *testing.T) {
	srv, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	tenantID := seedTenant(t, store)
	now := time.Now().UTC()

	cases := []struct {
		name string
		body string
		want string
	}{
		{"missing tenant", `{}`, "tenant_id required"},
		{"unknown tenant", `{"tenant_id":9999}`, "tenant not found"},
		{"invalid region", `{"tenant_id":` + itoa(tenantID) + `,"region":"not a region"}`, "invalid region"},
		{"bad start_time", `{"tenant_id":` + itoa(tenantID) + `,"start_time":"nope"}`, "invalid start_time"},
		{"bad end_time", `{"tenant_id":` + itoa(tenantID) + `,"end_time":"nope"}`, "invalid end_time"},
		{
			"end before start",
			`{"tenant_id":` + itoa(tenantID) + `,"start_time":"` + now.Format(time.RFC3339) + `","end_time":"` + now.Add(-time.Hour).Format(time.RFC3339) + `"}`,
			"end_time must be after start_time",
		},
		{
			"range too long",
			`{"tenant_id":` + itoa(tenantID) + `,"start_time":"` + now.Add(-90*24*time.Hour).Format(time.RFC3339) + `","end_time":"` + now.Format(time.RFC3339) + `"}`,
			"time range too long",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := authedReq(t, ts, http.MethodPost, "/api/traffic/accountStats", tc.body)
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", resp.StatusCode)
			}
			m := jsonMap(t, resp)
			if msg, _ := m["error"].(string); !strings.HasPrefix(msg, tc.want) {
				t.Fatalf("error = %v, want prefix %q", m["error"], tc.want)
			}
		})
	}

	if srv == nil {
		t.Fatal("server missing")
	}
}

// TestTrafficQuotaBytes checks the configurable free-allowance parsing.
func TestTrafficQuotaBytes(t *testing.T) {
	srv, store, _, cleanup := setupTestServer(t)
	defer cleanup()

	const gib = 1024 * 1024 * 1024
	if got, want := srv.trafficQuotaBytes(), float64(defaultTrafficQuotaGB)*gib; got != want {
		t.Fatalf("default quota = %v, want %v", got, want)
	}

	if err := store.SetConfig("traffic_quota_gb", "5120"); err != nil {
		t.Fatalf("set config: %v", err)
	}
	if got, want := srv.trafficQuotaBytes(), 5120.0*gib; got != want {
		t.Fatalf("configured quota = %v, want %v", got, want)
	}

	// Unparseable and non-positive values fall back to the OCI default rather
	// than disabling the allowance check.
	for _, bad := range []string{"", "abc", "0", "-5"} {
		if err := store.SetConfig("traffic_quota_gb", bad); err != nil {
			t.Fatalf("set config %q: %v", bad, err)
		}
		if got, want := srv.trafficQuotaBytes(), float64(defaultTrafficQuotaGB)*gib; math.Abs(got-want) > 1 {
			t.Fatalf("quota for %q = %v, want fallback %v", bad, got, want)
		}
	}
}

// TestTrafficAccountStatsMethodAndAuth guards the route: GET must not be
// accepted and anonymous callers must not reach the handler.
func TestTrafficAccountStatsMethodAndAuth(t *testing.T) {
	_, _, ts, cleanup := setupTestServer(t)
	defer cleanup()

	resp := authedReq(t, ts, http.MethodGet, "/api/traffic/accountStats", "")
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("GET status = %d, want 405", resp.StatusCode)
	}

	anon, err := ts.Client().Post(ts.URL+"/api/traffic/accountStats", "application/json", nil)
	if err != nil {
		t.Fatalf("anonymous post: %v", err)
	}
	defer anon.Body.Close()
	if anon.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous status = %d, want 401", anon.StatusCode)
	}
}

// TestParseTrafficWindow pins the window semantics every traffic view shares:
// both bounds default to the current billing month, and every view enforces the
// same ordering and span limits.
func TestParseTrafficWindow(t *testing.T) {
	t.Run("defaults to current month", func(t *testing.T) {
		before := time.Now()
		start, end, errMsg := parseTrafficWindow("", "")
		if errMsg != "" {
			t.Fatalf("errMsg = %q, want empty", errMsg)
		}
		wantStart := time.Date(before.Year(), before.Month(), 1, 0, 0, 0, 0, before.Location())
		if !start.Equal(wantStart) {
			t.Fatalf("start = %v, want %v", start, wantStart)
		}
		if end.Before(before) {
			t.Fatalf("end = %v, want >= %v", end, before)
		}
	})

	t.Run("explicit RFC3339 bounds", func(t *testing.T) {
		wantStart := time.Now().UTC().Add(-2 * time.Hour).Truncate(time.Second)
		wantEnd := wantStart.Add(time.Hour)
		start, end, errMsg := parseTrafficWindow(wantStart.Format(time.RFC3339), wantEnd.Format(time.RFC3339))
		if errMsg != "" {
			t.Fatalf("errMsg = %q, want empty", errMsg)
		}
		if !start.Equal(wantStart) || !end.Equal(wantEnd) {
			t.Fatalf("window = %v..%v, want %v..%v", start, end, wantStart, wantEnd)
		}
	})

	now := time.Now().UTC()
	cases := []struct {
		name    string
		start   string
		end     string
		wantErr string
	}{
		{"bad start", "nope", now.Format(time.RFC3339), "invalid start_time: "},
		{"bad end", now.Add(-time.Hour).Format(time.RFC3339), "nope", "invalid end_time: "},
		{"end equals start", now.Format(time.RFC3339), now.Format(time.RFC3339), "end_time must be after start_time"},
		{"span 33 days", now.Add(-33 * 24 * time.Hour).Format(time.RFC3339), now.Format(time.RFC3339), "time range too long"},
		// A window that starts past the 90-day retention is unusable, even
		// though its span is well within the 32-day cap.
		{"start beyond retention", now.Add(-120 * 24 * time.Hour).Format(time.RFC3339), now.Add(-100 * 24 * time.Hour).Format(time.RFC3339), "time range too old"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, errMsg := parseTrafficWindow(tc.start, tc.end)
			if !strings.HasPrefix(errMsg, tc.wantErr) {
				t.Fatalf("errMsg = %q, want prefix %q", errMsg, tc.wantErr)
			}
		})
	}

	// The span cap is inclusive: exactly maxTrafficStatsSpan is still allowed.
	t.Run("span at the cap", func(t *testing.T) {
		if _, _, errMsg := parseTrafficWindow(now.Add(-maxTrafficStatsSpan).Format(time.RFC3339), now.Format(time.RFC3339)); errMsg != "" {
			t.Fatalf("errMsg = %q, want empty", errMsg)
		}
	})
}

// TestCollectTrafficStatsFinalization covers the shared aggregation rollup
// without touching OCI: unusable regions must surface as errors, never as a
// clean "within allowance" answer.
func TestCollectTrafficStatsFinalization(t *testing.T) {
	srv, store, _, cleanup := setupTestServer(t)
	defer cleanup()

	tenantID := seedTenant(t, store)
	tenant, err := store.GetTenant(tenantID)
	if err != nil || tenant == nil {
		t.Fatalf("get tenant: %v", err)
	}
	start, end, errMsg := parseTrafficWindow("", "")
	if errMsg != "" {
		t.Fatalf("window: %q", errMsg)
	}

	t.Run("no regions", func(t *testing.T) {
		resp := srv.collectTrafficStats(context.Background(), tenant, nil, start, end)
		if resp.RegionCount != 0 || resp.InstanceCount != 0 || len(resp.Regions) != 0 {
			t.Fatalf("empty region list must roll up to zero, got %+v", resp)
		}
		if want := float64(defaultTrafficQuotaGB) * 1024 * 1024 * 1024; resp.QuotaBytes != want {
			t.Fatalf("quota = %v, want %v", resp.QuotaBytes, want)
		}
		if resp.QuotaPercent != 0 || resp.Exceeded || resp.Partial {
			t.Fatalf("zero usage must be within allowance, got %+v", resp)
		}
		if resp.TenantID != tenantID || resp.TenantName != tenant.Name {
			t.Fatalf("response tenant = %d/%q, want %d/%q", resp.TenantID, resp.TenantName, tenantID, tenant.Name)
		}
	})

	t.Run("unusable region", func(t *testing.T) {
		resp := srv.collectTrafficStats(context.Background(), tenant, []string{"not a region"}, start, end)
		if len(resp.Regions) != 1 || resp.Regions[0].Error != "invalid region" {
			t.Fatalf("regions = %+v, want one invalid-region error", resp.Regions)
		}
		if resp.RegionCount != 0 {
			t.Fatalf("RegionCount = %d, want 0", resp.RegionCount)
		}
		if len(resp.Errors) != 1 || !strings.HasPrefix(resp.Errors[0], "not a region: ") {
			t.Fatalf("Errors = %v, want one region-prefixed error", resp.Errors)
		}
	})
}
