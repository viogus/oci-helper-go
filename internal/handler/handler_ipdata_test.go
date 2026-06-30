package handler

import (
	"net/http"
	"testing"

	"github.com/viogus/oci-helper-go/internal/db"
)

func TestHandleIpData_List(t *testing.T) {
	_, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	tid := seedTenant(t, store)
	createIpData(t, store, &db.IpData{TenantID: tid, CIDR: "10.0.0.0/8", Label: "Private", Type: "pool", Enabled: true})
	createIpData(t, store, &db.IpData{TenantID: tid, CIDR: "192.168.0.0/16", Label: "Office", Type: "whitelist", Enabled: false})

	resp := authedReq(t, ts, http.MethodGet, "/api/ip-data?tenant_id="+itoa(tid), "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/ip-data: %d, want 200", resp.StatusCode)
	}

	m := jsonMap(t, resp)
	data, _ := m["data"].([]interface{})
	if len(data) != 2 {
		t.Fatalf("got %d ip data, want 2", len(data))
	}
}

func TestHandleIpData_List_ByType(t *testing.T) {
	_, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	tid := seedTenant(t, store)
	createIpData(t, store, &db.IpData{TenantID: tid, CIDR: "10.0.0.0/8", Label: "Pool", Type: "pool", Enabled: true})
	createIpData(t, store, &db.IpData{TenantID: tid, CIDR: "1.2.3.4/32", Label: "Bad", Type: "blacklist", Enabled: true})

	resp := authedReq(t, ts, http.MethodGet, "/api/ip-data?tenant_id="+itoa(tid)+"&type=pool", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/ip-data?type=pool: %d, want 200", resp.StatusCode)
	}

	m := jsonMap(t, resp)
	data, _ := m["data"].([]interface{})
	if len(data) != 1 {
		t.Fatalf("got %d pool ip data, want 1", len(data))
	}
}

func TestHandleIpData_Create(t *testing.T) {
	_, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	tid := seedTenant(t, store)
	body := `{"tenant_id":` + itoa(tid) + `,"cidr":"172.16.0.0/12","label":"Test","type":"pool","enabled":true}`

	resp := authedReq(t, ts, http.MethodPost, "/api/ip-data", body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/ip-data: %d, want 200", resp.StatusCode)
	}

	m := jsonMap(t, resp)
	if m["cidr"] != "172.16.0.0/12" {
		t.Fatalf("cidr = %v, want 172.16.0.0/12", m["cidr"])
	}
}

func TestHandleIpData_Create_MissingCIDR(t *testing.T) {
	_, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	tid := seedTenant(t, store)
	resp := authedReq(t, ts, http.MethodPost, "/api/ip-data", `{"tenant_id":`+itoa(tid)+`}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST /api/ip-data (missing cidr): %d, want 400", resp.StatusCode)
	}
	m := jsonMap(t, resp)
	if m["error"] == nil {
		t.Fatal("expected error for missing cidr")
	}
}

func TestHandleIpDataByID_Update(t *testing.T) {
	_, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	tid := seedTenant(t, store)
	d := createIpData(t, store, &db.IpData{TenantID: tid, CIDR: "10.0.0.0/8", Label: "Old", Type: "pool", Enabled: true})

	body := `{"cidr":"10.0.0.0/8","label":"Updated","type":"whitelist","enabled":false}`
	resp := authedReq(t, ts, http.MethodPut, "/api/ip-data/"+itoa(d.ID), body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT /api/ip-data/%d: %d, want 200", d.ID, resp.StatusCode)
	}
}

func TestHandleIpDataByID_Delete(t *testing.T) {
	_, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	tid := seedTenant(t, store)
	d := createIpData(t, store, &db.IpData{TenantID: tid, CIDR: "10.0.0.0/8", Label: "Del", Type: "pool", Enabled: false})

	resp := authedReq(t, ts, http.MethodDelete, "/api/ip-data/"+itoa(d.ID), "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE /api/ip-data/%d: %d, want 200", d.ID, resp.StatusCode)
	}

	m := jsonMap(t, resp)
	if m["status"] != "ok" {
		t.Fatalf("status = %v, want ok", m["status"])
	}
}

func TestHandleIpData_LoadOCI_NoInstances(t *testing.T) {
	_, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	tid := seedTenant(t, store)
	// Tenant exists but has no instances — load_oci should return added=0

	body := `{"action":"load_oci","tenant_id":` + itoa(tid) + `}`
	resp := authedReq(t, ts, http.MethodPost, "/api/ip-data", body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/ip-data load_oci: %d, want 200", resp.StatusCode)
	}

	m := jsonMap(t, resp)
	added, _ := m["added"].(float64)
	if int(added) != 0 {
		t.Fatalf("added = %v, want 0 (no instances)", added)
	}
}
