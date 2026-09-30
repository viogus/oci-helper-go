package handler

import (
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
