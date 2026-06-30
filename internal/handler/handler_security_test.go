package handler

import (
	"net/http"
	"testing"
)

func TestHandleSecurityRuleRelease_MissingVCN(t *testing.T) {
	_, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	tid := seedTenant(t, store)

	// POST without vcn_id — should fail input validation before OCI call
	body := `{"tenant_id":` + itoa(tid) + `,"ports":["22"]}`
	resp := authedReq(t, ts, http.MethodPost, "/api/security-rules/release", body)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST /api/security-rules/release (missing vcn_id): %d, want 400", resp.StatusCode)
	}

	m := jsonMap(t, resp)
	if m["error"] == nil {
		t.Fatal("expected error for missing vcn_id")
	}
}
