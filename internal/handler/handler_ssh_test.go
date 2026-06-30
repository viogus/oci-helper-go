package handler

import (
	"net/http"
	"testing"

	"github.com/viogus/oci-helper-go/internal/db"
)

func TestHandleSSHKeys_List(t *testing.T) {
	_, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	tid := seedTenant(t, store)
	createSSHKey(t, store, &db.SSHKey{Name: "my-key", PublicKey: "ssh-rsa AAAAtest", Fingerprint: "SHA256:abcd", TenantID: tid})
	createSSHKey(t, store, &db.SSHKey{Name: "prod-key", PublicKey: "ssh-ed25519 AAAAtest2", Fingerprint: "SHA256:ef01", TenantID: tid})

	resp := authedReq(t, ts, http.MethodGet, "/api/ssh/keys", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/ssh/keys: %d, want 200", resp.StatusCode)
	}

	m := jsonMap(t, resp)
	data, _ := m["data"].([]interface{})
	if len(data) != 2 {
		t.Fatalf("got %d keys, want 2", len(data))
	}
}

func TestHandleSSHKeys_Generate(t *testing.T) {
	_, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	tid := seedTenant(t, store)
	resp := authedReq(t, ts, http.MethodPost, "/api/ssh/keys", `{"name":"generated-key","type":"ed25519","action":"generate","tenant_id":`+itoa(tid)+`}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/ssh/keys generate: %d, want 200", resp.StatusCode)
	}

	m := jsonMap(t, resp)
	if m["name"] != "generated-key" {
		t.Fatalf("name = %v, want 'generated-key'", m["name"])
	}
	if m["public_key"] == nil || m["public_key"] == "" {
		t.Fatal("public_key should not be empty")
	}
}

func TestHandleSSHKeyByID_Delete(t *testing.T) {
	_, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	tid := seedTenant(t, store)
	k := createSSHKey(t, store, &db.SSHKey{Name: "del-key", PublicKey: "ssh-rsa test", Fingerprint: "SHA256:ff", TenantID: tid})

	resp := authedReq(t, ts, http.MethodDelete, "/api/ssh/keys/"+itoa(k.ID), "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE /api/ssh/keys/%d: %d, want 200", k.ID, resp.StatusCode)
	}

	m := jsonMap(t, resp)
	if m["status"] != "ok" {
		t.Fatalf("status = %v, want ok", m["status"])
	}
}
