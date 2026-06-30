package handler

import (
	"net/http"
	"testing"
)

func TestHandleCheckAliveBatch_NoInstances(t *testing.T) {
	_, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	tid := seedTenant(t, store)
	// Tenant exists but has no instances — check-alive-batch should return empty results

	body := `{"tenant_id":` + itoa(tid) + `}`
	resp := authedReq(t, ts, http.MethodPost, "/api/instances/check-alive-batch", body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/instances/check-alive-batch: %d, want 200", resp.StatusCode)
	}

	m := jsonMap(t, resp)
	data, _ := m["data"].([]interface{})
	if len(data) != 0 {
		t.Fatalf("got %d results, want 0 (no running instances)", len(data))
	}
}

func TestHandleInstanceConfigUpdate_MissingBody(t *testing.T) {
	_, _, ts, cleanup := setupTestServer(t)
	defer cleanup()

	// Empty body should cause JSON decode error
	resp := authedReq(t, ts, http.MethodPost, "/api/instances/config-update", "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST /api/instances/config-update (empty body): %d, want 400", resp.StatusCode)
	}

	m := jsonMap(t, resp)
	if m["error"] == nil {
		t.Fatal("expected error for empty body")
	}
}
