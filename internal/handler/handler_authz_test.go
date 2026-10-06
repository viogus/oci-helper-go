package handler

import (
	"net/http"
	"strings"
	"testing"
)

// TestOAuthEmailAllowed covers the Google OAuth fail-closed allowlist.
func TestOAuthEmailAllowed(t *testing.T) {
	cases := []struct {
		name      string
		allowlist string
		email     string
		want      bool
	}{
		{"empty allowlist denies", "", "victim@gmail.com", false},
		{"blank allowlist denies", "  ,, ", "victim@gmail.com", false},
		{"exact match", "me@example.com", "me@example.com", true},
		{"case-insensitive", "Me@Example.com", "me@example.com", true},
		{"whitespace tolerant", " a@x.com , me@example.com ", "me@example.com", true},
		{"non-listed denies", "other@example.com", "victim@gmail.com", false},
		{"empty email denies", "me@example.com", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := oauthEmailAllowed(tc.allowlist, tc.email); got != tc.want {
				t.Fatalf("oauthEmailAllowed(%q, %q) = %v, want %v", tc.allowlist, tc.email, got, tc.want)
			}
		})
	}
}

// TestNonAdminDeniedSensitiveEndpoints verifies that a session with role
// "user" cannot reach admin-only sensitive endpoints.
func TestNonAdminDeniedSensitiveEndpoints(t *testing.T) {
	srv, _, ts, cleanup := setupTestServer(t)
	defer cleanup()

	cookie := srv.auth.CreateSession("bob", "user")

	endpoints := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodPost, "/api/backup", `{"password":"x"}`},
		{http.MethodPost, "/api/restore", `{"password":"x","data":"x"}`},
		{http.MethodGet, "/api/keys", ""},
		{http.MethodGet, "/api/ssh/keys", ""},
		{http.MethodGet, "/api/admin/blacklist", ""},
		{http.MethodPost, "/api/admin/blacklist/clear", `{"ip":""}`},
		{http.MethodPost, "/api/mfa/setup", ""},
		{http.MethodPost, "/api/mfa/verify", `{"code":"000000"}`},
	}
	for _, ep := range endpoints {
		resp := authedReqNoLogin(t, ts, ep.method, ep.path, ep.body, cookie)
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("%s %s as role=user: got %d, want 403", ep.method, ep.path, resp.StatusCode)
		}
	}
}

// TestAdminAllowedSensitiveEndpoints is the positive control: the same
// endpoints accept an admin session (they may still fail on validation, but
// must not be rejected with 403).
func TestAdminAllowedSensitiveEndpoints(t *testing.T) {
	srv, _, ts, cleanup := setupTestServer(t)
	defer cleanup()

	cookie := srv.auth.CreateSession("admin", "admin")

	for _, path := range []string{"/api/keys", "/api/ssh/keys", "/api/admin/blacklist"} {
		resp := authedReqNoLogin(t, ts, http.MethodGet, path, "", cookie)
		resp.Body.Close()
		if resp.StatusCode == http.StatusForbidden {
			t.Errorf("GET %s as admin: got 403, want non-403", path)
		}
	}
}

// TestLogoutRevokesOnlyOwnSession ensures logging out one session does not
// invalidate every other session.
func TestLogoutRevokesOnlyOwnSession(t *testing.T) {
	srv, _, ts, cleanup := setupTestServer(t)
	defer cleanup()

	c1 := srv.auth.CreateSession("admin", "admin")
	c2 := srv.auth.CreateSession("admin", "admin")

	// Sanity: both sessions work.
	for i, c := range []string{c1, c2} {
		resp := authedReqNoLogin(t, ts, http.MethodGet, "/api/config", "", c)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("session %d before logout: got %d, want 200", i+1, resp.StatusCode)
		}
	}

	// Log out session 1.
	resp := authedReqNoLogin(t, ts, http.MethodPost, "/api/logout", "", c1)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("logout: got %d, want 200", resp.StatusCode)
	}

	// Session 1 must now be rejected...
	resp = authedReqNoLogin(t, ts, http.MethodGet, "/api/config", "", c1)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked session: got %d, want 401", resp.StatusCode)
	}

	// ...while session 2 stays valid.
	resp = authedReqNoLogin(t, ts, http.MethodGet, "/api/config", "", c2)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("other session after peer logout: got %d, want 200", resp.StatusCode)
	}
}

// TestCSRFRequiredOnStateChanging ensures a valid session alone is not enough;
// the CSRF token is mandatory for mutating methods.
func TestCSRFRequiredOnStateChanging(t *testing.T) {
	srv, _, ts, cleanup := setupTestServer(t)
	defer cleanup()

	cookie := srv.auth.CreateSession("admin", "admin")

	req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/config",
		strings.NewReader(`{"key":"k","value":"v"}`))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "oci_helper_session", Value: cookie})
	// Deliberately no X-CSRF-Token header.
	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("POST without CSRF token: got %d, want 403", resp.StatusCode)
	}
}

// TestBuildTrustedProxies verifies CIDR and bare-IP parsing.
func TestBuildTrustedProxies(t *testing.T) {
	nets := buildTrustedProxies([]string{"10.0.0.0/8", "192.168.1.5", "::1", "not-a-cidr", ""})
	if len(nets) != 3 {
		t.Fatalf("parsed %d networks, want 3", len(nets))
	}

	// Explicit allowlist must override the loopback/private default.
	old := trustedProxyNets
	trustedProxyNets = nets
	defer func() { trustedProxyNets = old }()

	if !isTrustedProxy("10.1.2.3:5555") {
		t.Error("10.1.2.3 should be trusted via 10.0.0.0/8")
	}
	if !isTrustedProxy("192.168.1.5:1") {
		t.Error("192.168.1.5 should be trusted")
	}
	if isTrustedProxy("203.0.113.9:1") {
		t.Error("203.0.113.9 must not be trusted")
	}
}
