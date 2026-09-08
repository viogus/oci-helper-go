package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/viogus/oci-helper-go/internal/cloudflare"
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

// TestDNSBindingPartialUpdate: a PUT that only sends some fields must keep the
// untouched columns (name/zone/cfCfgId/proxied/enabled), while explicit zero
// values (cfCfgId:0, proxied:false, enabled:false) must still be honoured.
func TestDNSBindingPartialUpdate(t *testing.T) {
	_, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	inst := seedInstance(t, store)
	create := `{"instanceId":"` + inst.ID + `","tenantId":` + itoa(inst.TenantID) + `,
		"name":"a.example.com","zoneId":"zone1","cfCfgId":7,"ttl":120,"proxied":true,"enabled":true}`
	resp := authedReq(t, ts, http.MethodPost, "/api/cloudflare/bindings", create)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create: status %d", resp.StatusCode)
	}
	var created db.InstanceDNSBinding
	decodeJSON(t, resp, &created)

	// Only TTL provided — everything else must survive.
	resp = authedReq(t, ts, http.MethodPut, "/api/cloudflare/bindings/"+itoa(created.ID), `{"ttl":300}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("partial update: status %d", resp.StatusCode)
	}
	var updated db.InstanceDNSBinding
	decodeJSON(t, resp, &updated)
	if updated.Name != "a.example.com" || updated.ZoneID != "zone1" || updated.CfCfgID != 7 ||
		!updated.Proxied || !updated.Enabled || updated.TTL != 300 {
		t.Fatalf("partial update clobbered fields: %+v", updated)
	}

	// Explicit zero values must be applied.
	resp = authedReq(t, ts, http.MethodPut, "/api/cloudflare/bindings/"+itoa(created.ID),
		`{"cfCfgId":0,"proxied":false,"enabled":false}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("zero-value update: status %d", resp.StatusCode)
	}
	decodeJSON(t, resp, &updated)
	if updated.CfCfgID != 0 || updated.Proxied || updated.Enabled || updated.Name != "a.example.com" {
		t.Fatalf("zero-value update not honoured: %+v", updated)
	}

	// Moving a binding to an unknown instance must fail.
	resp = authedReq(t, ts, http.MethodPut, "/api/cloudflare/bindings/"+itoa(created.ID),
		`{"instanceId":"1:ocid1.instance.ghost"}`)
	if resp.StatusCode == http.StatusOK {
		t.Fatal("expected error when re-pointing at an unknown instance")
	}
}

// TestDNSBindingOwnership: bindings must reference an existing instance and
// carry that instance's tenant.
func TestDNSBindingOwnership(t *testing.T) {
	_, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	inst := seedInstance(t, store)
	// Unknown instance.
	resp := authedReq(t, ts, http.MethodPost, "/api/cloudflare/bindings",
		`{"instanceId":"1:ocid1.instance.ghost","tenantId":1,"name":"a.example.com","zoneId":"z"}`)
	if resp.StatusCode == http.StatusOK {
		t.Fatal("expected error for unknown instance")
	}
	resp.Body.Close()
	// Tenant mismatch.
	body := `{"instanceId":"` + inst.ID + `","tenantId":` + itoa(inst.TenantID+1) + `,
		"name":"a.example.com","zoneId":"z"}`
	resp = authedReq(t, ts, http.MethodPost, "/api/cloudflare/bindings", body)
	if resp.StatusCode == http.StatusOK {
		t.Fatal("expected error for tenant mismatch")
	}
	resp.Body.Close()
	// Omitted tenant is derived from the instance.
	body = `{"instanceId":"` + inst.ID + `","name":"ok.example.com","zoneId":"z"}`
	resp = authedReq(t, ts, http.MethodPost, "/api/cloudflare/bindings", body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create without tenantId: status %d", resp.StatusCode)
	}
	var created db.InstanceDNSBinding
	decodeJSON(t, resp, &created)
	if created.TenantID != inst.TenantID {
		t.Fatalf("derived tenant = %d, want %d", created.TenantID, inst.TenantID)
	}
}

// TestCfCfgPartialUpdate: PUT on a CF config only touches provided fields so
// a zone change no longer blanks token/email/api_key.
func TestCfCfgPartialUpdate(t *testing.T) {
	_, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	cfg := &db.CfCfg{Name: "Legacy", Token: "tok-abc", Email: "e@x.com", APIKey: "key-1", ZoneID: "zone-old", Enabled: true}
	if err := store.CreateCfCfg(cfg); err != nil {
		t.Fatalf("create cfg: %v", err)
	}
	// CreateCfCfg does not backfill the id; read it back.
	cfgs, err := store.ListCfCfgs()
	if err != nil || len(cfgs) != 1 {
		t.Fatalf("list cfgs: err=%v n=%d", err, len(cfgs))
	}
	cfgID := cfgs[0].ID

	// Only zone changes.
	resp := authedReq(t, ts, http.MethodPut, "/api/cloudflare/cfgs/"+itoa(cfgID), `{"zoneId":"zone-new"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("partial update cfg: status %d", resp.StatusCode)
	}
	var updated db.CfCfg
	decodeJSON(t, resp, &updated)
	if updated.ZoneID != "zone-new" || updated.Token != "tok-abc" || updated.Email != "e@x.com" ||
		updated.APIKey != "key-1" || updated.Name != "Legacy" {
		t.Fatalf("cfg partial update clobbered fields: %+v", updated)
	}

	// Name must stay non-empty.
	resp = authedReq(t, ts, http.MethodPut, "/api/cloudflare/cfgs/"+itoa(cfgID), `{"name":""}`)
	if resp.StatusCode == http.StatusOK {
		t.Fatal("expected error for empty name")
	}
}

// TestValidDNSName covers the strengthened record-name validation.
func TestValidDNSName(t *testing.T) {
	valid := []string{"@", "a.example.com", "vps1.example.com", "sub.domain.co.uk",
		"a_b.example.com", "A.example.com"}
	for _, s := range valid {
		if !validDNSName(s) {
			t.Errorf("validDNSName(%q) = false, want true", s)
		}
	}
	invalid := []string{"", "a b.com", "*.example.com", "中文.example.com", "a..b.com",
		".example.com", "example.com.", "x-.y", strings.Repeat("a", 254),
		"x." + strings.Repeat("a", 64) + ".com"}
	for _, s := range invalid {
		if validDNSName(s) {
			t.Errorf("validDNSName(%q) = true, want false", s)
		}
	}
}

// TestFindBindingTarget covers A-record matching and conflict reporting.
func TestFindBindingTarget(t *testing.T) {
	records := []cloudflare.DNSRecord{
		{ID: "1", Type: "A", Name: "z.example.com", Content: "1.1.1.1"},
		{ID: "2", Type: "CNAME", Name: "b.example.com", Content: "target"},
		{ID: "3", Type: "MX", Name: "example.com", Content: "10 mx"},
		{ID: "4", Type: "A", Name: "A.EXAMPLE.COM.", Content: "2.2.2.2"},
	}

	// Matches the A record (case/trailing-dot insensitive).
	got, conflict := findBindingTarget(records, "a.example.com")
	if conflict != "" {
		t.Fatalf("unexpected conflict %q", conflict)
	}
	if got == nil || got.ID != "4" {
		t.Fatalf("expected A record id 4, got %+v", got)
	}

	// No record at all.
	got, conflict = findBindingTarget(records, "nope.example.com")
	if got != nil || conflict != "" {
		t.Fatalf("expected no match, got %+v conflict %q", got, conflict)
	}

	// CNAME with the same name -> conflict.
	got, conflict = findBindingTarget(records, "b.example.com")
	if got != nil || conflict != "CNAME" {
		t.Fatalf("expected CNAME conflict, got %+v conflict %q", got, conflict)
	}

	// A + AAAA for the same name -> refuse to guess.
	both := append(append([]cloudflare.DNSRecord{}, records...),
		cloudflare.DNSRecord{ID: "5", Type: "A", Name: "c.example.com", Content: "3.3.3.3"},
		cloudflare.DNSRecord{ID: "6", Type: "AAAA", Name: "c.example.com", Content: "::1"})
	got, conflict = findBindingTarget(both, "c.example.com")
	if got != nil || conflict == "" {
		t.Fatalf("expected conflict for mixed A/AAAA, got %+v conflict %q", got, conflict)
	}
}
