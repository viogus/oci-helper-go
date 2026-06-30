package handler

import (
	"net/http"
	"testing"

	"github.com/viogus/oci-helper-go/internal/db"
)

// Defense enable/disable handlers require real OCI credentials.
// These tests only validate input checking and blacklist queries.

func TestHandleDefense_Enable_MissingFields(t *testing.T) {
	_, _, ts, cleanup := setupTestServer(t)
	defer cleanup()

	resp := authedReq(t, ts, http.MethodPost, "/api/defense/enable", `{}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST /api/defense/enable (empty): %d, want 400", resp.StatusCode)
	}
	m := jsonMap(t, resp)
	if m["error"] == nil {
		t.Fatal("expected error for missing fields")
	}
}

func TestHandleDefense_Disable_MissingFields(t *testing.T) {
	_, _, ts, cleanup := setupTestServer(t)
	defer cleanup()

	resp := authedReq(t, ts, http.MethodPost, "/api/defense/disable", `{}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST /api/defense/disable (empty): %d, want 400", resp.StatusCode)
	}
	m := jsonMap(t, resp)
	if m["error"] == nil {
		t.Fatal("expected error for missing fields")
	}
}

func TestHandleIPBlacklist_List(t *testing.T) {
	_, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	tid := seedTenant(t, store)
	store.CreateIpData(&db.IpData{TenantID: tid, CIDR: "1.2.3.4/32", Label: "Bad IP", Type: "deny", Enabled: true})

	resp := authedReq(t, ts, http.MethodGet, "/api/ip-blacklist?tenant_id="+itoa(tid), "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/ip-blacklist: %d, want 200", resp.StatusCode)
	}

	m := jsonMap(t, resp)
	data, _ := m["data"].([]interface{})
	if len(data) != 1 {
		t.Fatalf("got %d blacklist entries, want 1", len(data))
	}
}

func TestHandleIPBlacklist_ByTenant(t *testing.T) {
	_, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	tid1 := seedTenant(t, store)
	ten2 := &db.Tenant{Name: "t2", Region: "us-ashburn-1", UserOCID: "u", TenancyOCID: "t", Status: "active"}
	store.CreateTenant(ten2)
	tid2 := seedTenant2(t, store)

	store.CreateIpData(&db.IpData{TenantID: tid1, CIDR: "1.1.1.1/32", Label: "A", Type: "deny", Enabled: true})
	store.CreateIpData(&db.IpData{TenantID: tid2, CIDR: "2.2.2.2/32", Label: "B", Type: "deny", Enabled: true})

	resp := authedReq(t, ts, http.MethodGet, "/api/ip-blacklist?tenant_id="+itoa(tid1), "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/ip-blacklist: %d", resp.StatusCode)
	}

	m := jsonMap(t, resp)
	data, _ := m["data"].([]interface{})
	if len(data) != 1 {
		t.Fatalf("got %d blacklist entries for tid1, want 1", len(data))
	}
}

// seedTenant2 creates a second tenant with a different name.
func seedTenant2(t *testing.T, store *db.Store) int64 {
	t.Helper()
	ten := &db.Tenant{
		Name: "test-tenant-2", Region: "us-ashburn-1",
		UserOCID: "u2", TenancyOCID: "t2", Status: "active",
	}
	if err := store.CreateTenant(ten); err != nil {
		t.Fatalf("seed tenant2: %v", err)
	}
	list, _ := store.ListTenants()
	for _, x := range list {
		if x.Name == "test-tenant-2" {
			return x.ID
		}
	}
	t.Fatal("could not find tenant 2")
	return 0
}
