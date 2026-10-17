package handler

import (
	"net/http"
	"testing"
	"time"

	"github.com/viogus/oci-helper-go/internal/db"
	"github.com/viogus/oci-helper-go/internal/geoip"
)

func seedMapInstance(t *testing.T, store *db.Store, tenantID int64, name, publicIP string) {
	t.Helper()
	ocid := "ocid1.instance." + name
	inst := &db.Instance{
		ID:       itoa(tenantID) + ":" + ocid,
		TenantID: tenantID,
		Name:     name,
		OCID:     ocid,
		State:    "RUNNING",
		PublicIP: publicIP,
		Region:   "ap-tokyo-1",
	}
	if err := store.UpsertInstance(inst); err != nil {
		t.Fatalf("seed instance %s: %v", name, err)
	}
}

// Every instance gets a point, even when its IP cannot be geolocated: the SPA
// then falls back to the region coordinates instead of dropping the instance.
func TestHandleInstanceMap_OnePointPerInstance(t *testing.T) {
	_, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	tid := seedTenant(t, store)
	// Private IP: the lookup fails locally, without touching the network.
	seedMapInstance(t, store, tid, "private", "10.0.0.5")
	seedMapInstance(t, store, tid, "no-ip", "")

	resp := authedReq(t, ts, http.MethodGet, "/api/instances/map", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/instances/map: %d, want 200", resp.StatusCode)
	}

	m := jsonMap(t, resp)
	points, _ := m["points"].([]interface{})
	if len(points) != 2 {
		t.Fatalf("got %d points, want 2 (one per instance)", len(points))
	}
	for _, raw := range points {
		p, ok := raw.(map[string]interface{})
		if !ok {
			t.Fatalf("point is not an object: %T", raw)
		}
		if p["lat"].(float64) != 0 || p["lng"].(float64) != 0 {
			t.Fatalf("point %v: got coordinates for an unresolvable IP", p["name"])
		}
		if p["region"] != "ap-tokyo-1" {
			t.Fatalf("point %v: region = %v, want ap-tokyo-1", p["name"], p["region"])
		}
	}
	if pending, _ := m["pending"].(float64); pending != 0 {
		t.Fatalf("pending = %v, want 0 once a failed lookup is cached", m["pending"])
	}
}

func TestHandleInstanceMap_UsesCachedGeolocation(t *testing.T) {
	s, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	tid := seedTenant(t, store)
	seedMapInstance(t, store, tid, "tokyo", "203.0.113.10")

	// Pre-seed the cache so the test never performs a real lookup.
	s.geoMu.Lock()
	s.geoCache = map[string]geoEntry{
		"203.0.113.10": {info: &geoip.Info{Lat: 35.68, Lng: 139.76, Country: "Japan", City: "Tokyo"}, at: time.Now()},
	}
	s.geoMu.Unlock()

	resp := authedReq(t, ts, http.MethodGet, "/api/instances/map", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/instances/map: %d, want 200", resp.StatusCode)
	}

	m := jsonMap(t, resp)
	points, _ := m["points"].([]interface{})
	if len(points) != 1 {
		t.Fatalf("got %d points, want 1", len(points))
	}
	p := points[0].(map[string]interface{})
	if p["lat"].(float64) != 35.68 || p["lng"].(float64) != 139.76 {
		t.Fatalf("lat/lng = %v/%v, want 35.68/139.76", p["lat"], p["lng"])
	}
	if p["city"] != "Tokyo" || p["publicIp"] != "203.0.113.10" {
		t.Fatalf("city/publicIp = %v/%v, want Tokyo/203.0.113.10", p["city"], p["publicIp"])
	}
	if pending, _ := m["pending"].(float64); pending != 0 {
		t.Fatalf("pending = %v, want 0 for a cached IP", m["pending"])
	}
}

func TestHandleInstanceMap_MethodNotAllowed(t *testing.T) {
	_, _, ts, cleanup := setupTestServer(t)
	defer cleanup()

	resp := authedReq(t, ts, http.MethodPost, "/api/instances/map", "")
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("POST /api/instances/map: %d, want 405", resp.StatusCode)
	}
}
