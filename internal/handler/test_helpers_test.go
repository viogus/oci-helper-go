package handler

import (
"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/viogus/oci-helper-go/internal/config"
	"github.com/viogus/oci-helper-go/internal/db"
	"golang.org/x/crypto/bcrypt"
)

// setupTestServer creates an in-memory SQLite store, seeds the schema,
// wraps the Server in httptest.NewServer, and returns all three plus a
// cleanup function that closes the DB and test server.
func setupTestServer(t *testing.T) (*Server, *db.Store, *httptest.Server, func()) {
	t.Helper()

	store, err := db.New(":memory:")
	if err != nil {
		t.Fatalf("open in-memory store: %v", err)
	}

	cfg := &config.Config{
		Port:      "8818",
		Username:  "admin",
		Password:  "test",
		DBPath:    ":memory:",
		KeysDir:   "/tmp/oci-helper-test-keys",
		LogFile:   "",
		MFASecret: "",
		MFA:       false,
	}

	srv := New(cfg, store)
	ts := httptest.NewServer(srv.Handler())

	cleanup := func() {
		ts.Close()
		store.Close()
	}

	return srv, store, ts, cleanup
}

// mustLogin performs a Basic-Auth login against the test server and
// returns the session cookie for subsequent authenticated requests.
func mustLogin(t *testing.T, ts *httptest.Server) string {
	t.Helper()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/login", nil)
	req.SetBasicAuth("admin", "test")

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("login request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("login returned %d, want 200", resp.StatusCode)
	}

	for _, c := range resp.Cookies() {
		if c.Name == "oci_helper_session" {
			return c.Value
		}
	}
	t.Fatal("no oci_helper_session cookie in login response")
	return ""
}

// authedReq creates an HTTP request with the session cookie attached and
// returns the response. Use for GET/POST/DELETE against the test server.
func authedReq(t *testing.T, ts *httptest.Server, method, path, body string) *http.Response {
	t.Helper()

	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}

	req, err := http.NewRequest(method, ts.URL+path, r)
	if err != nil {
		t.Fatalf("new request %s %s: %v", method, path, err)
	}
	req.Header.Set("Content-Type", "application/json")

	cookie := mustLogin(t, ts)
	req.AddCookie(&http.Cookie{Name: "oci_helper_session", Value: cookie})

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return resp
}

func authedReqNoLogin(t *testing.T, ts *httptest.Server, method, path, body, cookie string) *http.Response {
	t.Helper()

	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}

	req, err := http.NewRequest(method, ts.URL+path, r)
	if err != nil {
		t.Fatalf("new request %s %s: %v", method, path, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{Name: "oci_helper_session", Value: cookie})

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return resp
}

// decodeJSON decodes a JSON response body into v.
func decodeJSON(t *testing.T, resp *http.Response, v interface{}) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

// jsonMap decodes the response body into a map[string]interface{}.
func jsonMap(t *testing.T, resp *http.Response) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	decodeJSON(t, resp, &m)
	return m
}

// seedUser creates a test user and returns it. Panics on failure.
func seedUser(t *testing.T, store *db.Store, username, password, role string) *db.User {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("seed user hash: %v", err)
	}
	u := &db.User{
		Username:     username,
		PasswordHash: string(hash),
		Role:         role,
	}
	if err := store.CreateUser(u); err != nil {
		t.Fatalf("seed user create: %v", err)
	}
	// CreateUser does not set the ID. Find the user by username.
	found, err := store.GetUserByUsername(username)
	if err != nil || found == nil {
		t.Fatalf("seed user: could not retrieve ID (err=%v, found=%v)", err, found)
	}
	return found
}

// itoa converts an int64 to its decimal string representation.
func itoa(i int64) string { return fmt.Sprintf("%d", i) }

// seedTenant creates a test tenant and returns its ID.
func seedTenant(t *testing.T, store *db.Store) int64 {
	t.Helper()
	ten := &db.Tenant{
		Name:        "test-tenant",
		Region:      "us-phoenix-1",
		UserOCID:    "ocid1.user.test",
		TenancyOCID: "ocid1.tenancy.test",
		Fingerprint: "aa:bb:cc",
		KeyFile:     "",
		Status:      "active",
	}
	if err := store.CreateTenant(ten); err != nil {
		t.Fatalf("seed tenant: %v", err)
	}
	// CreateTenant does not set the ID. Fetch the last inserted tenant.
	list, err := store.ListTenants()
	if err != nil || len(list) == 0 {
		t.Fatalf("seed tenant: could not retrieve ID (err=%v, len=%d)", err, len(list))
	}
	return list[0].ID
}

// createIpData creates IpData and returns it with the auto-generated ID.
func createIpData(t *testing.T, store *db.Store, d *db.IpData) *db.IpData {
	t.Helper()
	if err := store.CreateIpData(d); err != nil {
		t.Fatalf("create ip data: %v", err)
	}
	list, _ := store.ListIpData(d.TenantID, "")
	for _, x := range list {
		if x.CIDR == d.CIDR && x.Label == d.Label {
			return &x
		}
	}
	if len(list) > 0 {
		return &list[len(list)-1]
	}
	t.Fatal("ip data not found after creation")
	return nil
}

// createSSHKey creates an SSH key and returns it with the auto-generated ID.
func createSSHKey(t *testing.T, store *db.Store, k *db.SSHKey) *db.SSHKey {
	t.Helper()
	if err := store.CreateSSHKey(k); err != nil {
		t.Fatalf("create ssh key: %v", err)
	}
	list, _ := store.ListSSHKeys(0)
	for _, x := range list {
		if x.Name == k.Name {
			return &x
		}
	}
	if len(list) > 0 {
		return &list[len(list)-1]
	}
	t.Fatal("ssh key not found after creation")
	return nil
}

// createInstancePlan creates a plan and returns it with the auto-generated ID.
func createInstancePlan(t *testing.T, store *db.Store, p *db.InstancePlan) *db.InstancePlan {
	t.Helper()
	if err := store.CreateInstancePlan(p); err != nil {
		t.Fatalf("create plan: %v", err)
	}
	list, _ := store.ListInstancePlans(p.TenantID)
	for _, x := range list {
		if x.Name == p.Name {
			return &x
		}
	}
	if len(list) > 0 {
		return &list[len(list)-1]
	}
	t.Fatal("plan not found after creation")
	return nil
}
