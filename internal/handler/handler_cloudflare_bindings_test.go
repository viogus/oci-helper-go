package handler

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/viogus/oci-helper-go/internal/db"
)

func seedInstance(t *testing.T, store *db.Store) *db.Instance {
	t.Helper()
	tenantID := seedTenant(t, store)
	inst := &db.Instance{
		ID:       itoa(tenantID) + ":ocid1.instance.test",
		TenantID: tenantID,
		Name:     "myvps",
		OCID:     "ocid1.instance.test",
		State:    "RUNNING",
		Shape:    "VM.Standard.E2.1.Micro",
		PublicIP: "1.2.3.4",
	}
	if err := store.UpsertInstance(inst); err != nil {
		t.Fatalf("seed instance: %v", err)
	}
	return inst
}

func TestDNSBindingCRUD(t *testing.T) {
	_, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	// Create via API.
	inst := seedInstance(t, store)
	payload := `{"instanceId":"` + inst.ID + `","tenantId":` + itoa(inst.TenantID) + `,
		"name":"myvps.example.com","zoneId":"abc123","cfCfgId":0,"ttl":120,"proxied":false,"enabled":true}`
	resp := authedReq(t, ts, http.MethodPost, "/api/cloudflare/bindings", payload)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create binding: status %d", resp.StatusCode)
	}
	var created db.InstanceDNSBinding
	decodeJSON(t, resp, &created)
	if created.ID == 0 {
		t.Fatal("create binding: expected non-zero id")
	}
	if created.Name != "myvps.example.com" {
		t.Fatalf("create binding: name = %q", created.Name)
	}

	// List by instance.
	resp = authedReq(t, ts, http.MethodGet, "/api/cloudflare/bindings?instance_id="+inst.ID, "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list binding: status %d", resp.StatusCode)
	}
	var listResp struct {
		Data []db.InstanceDNSBinding `json:"data"`
	}
	decodeJSON(t, resp, &listResp)
	if len(listResp.Data) != 1 {
		t.Fatalf("list binding: got %d, want 1", len(listResp.Data))
	}

	// Update.
	upd := `{"instanceId":"` + inst.ID + `","tenantId":` + itoa(inst.TenantID) + `,
		"name":"myvps2.example.com","zoneId":"abc123","ttl":300,"proxied":true,"enabled":false}`
	resp = authedReq(t, ts, http.MethodPut, "/api/cloudflare/bindings/"+itoa(created.ID), upd)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("update binding: status %d", resp.StatusCode)
	}
	var updated db.InstanceDNSBinding
	decodeJSON(t, resp, &updated)
	if updated.Name != "myvps2.example.com" || updated.TTL != 300 || !updated.Proxied || updated.Enabled {
		t.Fatalf("update binding: unexpected value: %+v", updated)
	}

	// Delete.
	resp = authedReq(t, ts, http.MethodDelete, "/api/cloudflare/bindings/"+itoa(created.ID), "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete binding: status %d", resp.StatusCode)
	}
	resp = authedReq(t, ts, http.MethodGet, "/api/cloudflare/bindings?instance_id="+inst.ID, "")
	var after struct {
		Data []db.InstanceDNSBinding `json:"data"`
	}
	decodeJSON(t, resp, &after)
	if len(after.Data) != 0 {
		t.Fatalf("after delete: got %d bindings, want 0", len(after.Data))
	}
}

func TestDNSBindingValidation(t *testing.T) {
	_, _, ts, cleanup := setupTestServer(t)
	defer cleanup()

	cases := map[string]string{
		"missing_instance": `{"name":"x.example.com","zoneId":"abc"}`,
		"missing_name":     `{"instanceId":"1:ocid","zoneId":"abc"}`,
		"missing_zone":     `{"instanceId":"1:ocid","name":"x.example.com"}`,
		"invalid_name":     `{"instanceId":"1:ocid","name":"x ex ample.com","zoneId":"abc"}`,
		"wildcard_name":    `{"instanceId":"1:ocid","name":"*.example.com","zoneId":"abc"}`,
	}
	for label, body := range cases {
		resp := authedReq(t, ts, http.MethodPost, "/api/cloudflare/bindings", body)
		if resp.StatusCode == http.StatusOK {
			t.Fatalf("%s: expected non-200, got 200", label)
		}
		resp.Body.Close()
	}
}

// TestDNSBindingPersisted ensures the v10 migration table exists (no
// "no such table" error) and stores/retrieves via the store directly.
func TestDNSBindingPersisted(t *testing.T) {
	store, err := db.New(":memory:")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	b := &db.InstanceDNSBinding{
		InstanceID: "1:ocid1.instance.test",
		TenantID:   1,
		Name:       "a.example.com",
		ZoneID:     "z",
		TTL:        60,
	}
	if err := store.CreateInstanceDNSBinding(b); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := store.GetInstanceDNSBinding(b.ID)
	if err != nil || got == nil {
		t.Fatalf("get: err=%v got=%v", err, got)
	}
	if got.Name != "a.example.com" {
		t.Fatalf("got.Name = %q", got.Name)
	}

	// Delete by instance.
	if err := store.DeleteInstanceDNSBindingsByInstance("1:ocid1.instance.test"); err != nil {
		t.Fatalf("delete by instance: %v", err)
	}
	got, _ = store.GetInstanceDNSBinding(b.ID)
	if got != nil {
		t.Fatal("expected binding to be deleted")
	}
}

// Marshal sanity: the binding JSON round-trips with expected keys.
func TestDNSBindingJSON(t *testing.T) {
	b := db.InstanceDNSBinding{InstanceID: "1:ocid", Name: "x.y", ZoneID: "z", TTL: 60, Enabled: true}
	data, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back map[string]interface{}
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, k := range []string{"instanceId", "name", "zoneId", "cfCfgId", "ttl", "enabled"} {
		if _, ok := back[k]; !ok {
			t.Fatalf("missing json key %q in %s", k, data)
		}
	}
}
