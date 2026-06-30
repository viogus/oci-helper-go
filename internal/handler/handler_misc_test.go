package handler

import (
	"net/http"
	"testing"

	"github.com/viogus/oci-helper-go/internal/db"
)

func TestHandleGlance(t *testing.T) {
	_, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	// Seed users
	seedUser(t, store, "admin2", "pass", "admin")
	seedUser(t, store, "viewer", "pass", "user")

	// Seed tenants
	tid1 := seedTenant(t, store)
	store.CreateTenant(&db.Tenant{Name: "t2", Region: "us-ashburn-1", UserOCID: "u2", TenancyOCID: "t2", Status: "active"})

	// Seed instances
	store.UpsertInstance(&db.Instance{
		ID:       itoa(tid1) + ":ocid1.inst.test1",
		TenantID: tid1,
		OCID:     "ocid1.inst.test1",
		Name:     "inst-1",
		State:    "RUNNING",
	})
	store.UpsertInstance(&db.Instance{
		ID:       itoa(tid1) + ":ocid1.inst.test2",
		TenantID: tid1,
		OCID:     "ocid1.inst.test2",
		Name:     "inst-2",
		State:    "STOPPED",
	})

	resp := authedReq(t, ts, http.MethodGet, "/api/glance", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/glance: %d, want 200", resp.StatusCode)
	}

	m := jsonMap(t, resp)

	users, _ := m["users"].(float64)
	if int(users) != 2 {
		t.Fatalf("users = %v, want 2", users)
	}

	tenants, _ := m["tenants"].(float64)
	if int(tenants) != 2 {
		t.Fatalf("tenants = %v, want 2", tenants)
	}

	instances, _ := m["instances"].(float64)
	if int(instances) != 2 {
		t.Fatalf("instances = %v, want 2", instances)
	}

	running, _ := m["runningInstances"].(float64)
	if int(running) != 1 {
		t.Fatalf("runningInstances = %v, want 1", running)
	}
}

func TestHandleCaptchaSend_NoBot(t *testing.T) {
	_, _, ts, cleanup := setupTestServer(t)
	defer cleanup()

	// No telegram_token or dingtalk webhook configured — should fail
	body := `{"recipient":"telegram","target":"12345678"}`
	resp := authedReq(t, ts, http.MethodPost, "/api/captcha/send", body)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST /api/captcha/send (no bot): %d, want 400", resp.StatusCode)
	}

	m := jsonMap(t, resp)
	if m["error"] == nil {
		t.Fatal("expected error for no notification channel")
	}
}
