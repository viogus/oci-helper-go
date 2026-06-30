package handler

import (
	"net/http"
	"testing"

	"github.com/viogus/oci-helper-go/internal/db"
)

func TestHandleTenants_List(t *testing.T) {
	_, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	seedTenant(t, store)
	store.CreateTenant(&db.Tenant{Name: "tenant-b", Region: "us-ashburn-1", UserOCID: "u2", TenancyOCID: "t2", Status: "active"})
	store.CreateTenant(&db.Tenant{Name: "tenant-c", Region: "us-phoenix-1", UserOCID: "u3", TenancyOCID: "t3", Status: "active"})

	resp := authedReq(t, ts, http.MethodGet, "/api/tenants", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/tenants: %d, want 200", resp.StatusCode)
	}

	m := jsonMap(t, resp)
	data, ok := m["data"].([]interface{})
	if !ok {
		t.Fatalf("data is not array: %T", m["data"])
	}
	if len(data) < 3 {
		t.Fatalf("got %d tenants, want >= 3", len(data))
	}
}

func TestHandleTenants_Create(t *testing.T) {
	_, _, ts, cleanup := setupTestServer(t)
	defer cleanup()

	body := `{"name":"new-tenant","region":"us-ashburn-1","userOcid":"ocid1.user.test","tenancyOcid":"ocid1.tenancy.test","status":"active"}`
	resp := authedReq(t, ts, http.MethodPost, "/api/tenants", body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/tenants: %d, want 200", resp.StatusCode)
	}

	m := jsonMap(t, resp)
	if m["name"] != "new-tenant" {
		t.Fatalf("name = %v, want new-tenant", m["name"])
	}
}

func TestHandleTenantInfo(t *testing.T) {
	_, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	tid := seedTenant(t, store)
	// Add an instance to the tenant for instanceStats
	store.UpsertInstance(&db.Instance{
		ID:       itoa(tid) + ":ocid1.instance.test",
		TenantID: tid,
		OCID:     "ocid1.instance.test",
		Name:     "test-instance",
		State:    "RUNNING",
	})

	resp := authedReq(t, ts, http.MethodGet, "/api/tenants/"+itoa(tid)+"/info", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/tenants/%d/info: %d, want 200", tid, resp.StatusCode)
	}

	m := jsonMap(t, resp)
	// Without a real OCI client, regions should be [] and instanceStats should be {}
	if _, ok := m["regions"]; !ok {
		t.Fatal("response missing 'regions' field")
	}
	if _, ok := m["instanceStats"]; !ok {
		t.Fatal("response missing 'instanceStats' field")
	}
}

func TestHandleTenantUsers_List_RequiresAuth(t *testing.T) {
	_, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	tid := seedTenant(t, store)
	// Without auth cookie, the endpoint should return 401
	resp, err := http.Get(ts.URL + "/api/tenants/" + itoa(tid) + "/users")
	if err != nil {
		t.Fatalf("GET request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("GET /api/tenants/%d/users (no auth): %d, want 401", tid, resp.StatusCode)
	}
}

func TestHandleTenantMFAClear_MissingUserID(t *testing.T) {
	_, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	tid := seedTenant(t, store)

	// POST with empty body — should fail before OCI call with "user_id required"
	resp := authedReq(t, ts, http.MethodPost, "/api/tenants/"+itoa(tid)+"/mfa/clear", `{}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST /api/tenants/%d/mfa/clear (empty body): %d, want 400", tid, resp.StatusCode)
	}

	m := jsonMap(t, resp)
	if m["error"] == nil {
		t.Fatal("expected error for missing user_id")
	}
}

func TestHandleTenantPasswordPolicy(t *testing.T) {
	_, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	tid := seedTenant(t, store)

	body := `{"password_expires_after":90}`
	resp := authedReq(t, ts, http.MethodPost, "/api/tenants/"+itoa(tid)+"/password-policy", body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/tenants/%d/password-policy: %d, want 200", tid, resp.StatusCode)
	}

	m := jsonMap(t, resp)
	if m["status"] != "ok" {
		t.Fatalf("status = %v, want ok", m["status"])
	}
	if pe, ok := m["password_expires_after"].(float64); !ok || int(pe) != 90 {
		t.Fatalf("password_expires_after = %v, want 90", m["password_expires_after"])
	}

	// Verify config was stored
	val, _ := store.GetConfig("tenant_pwdexp_" + itoa(tid))
	if val != "90" {
		t.Fatalf("stored config = %q, want 90", val)
	}
}

func TestHandleTenantByID_Delete(t *testing.T) {
	_, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	tid := seedTenant(t, store)

	resp := authedReq(t, ts, http.MethodDelete, "/api/tenants/"+itoa(tid), "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE /api/tenants/%d: %d, want 200", tid, resp.StatusCode)
	}

	m := jsonMap(t, resp)
	if m["status"] != "ok" {
		t.Fatalf("status = %v, want ok", m["status"])
	}

	// Verify tenant is gone
	deleted, _ := store.GetTenant(tid)
	if deleted != nil {
		t.Fatal("tenant still exists after delete")
	}
}
