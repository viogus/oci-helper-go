package handler

import (
	"net/http"
	"testing"

	"github.com/viogus/oci-helper-go/internal/db"
)

func TestHandleInstancePlans_List(t *testing.T) {
	_, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	tid := seedTenant(t, store)
	createInstancePlan(t, store, &db.InstancePlan{
		Name: "Plan A", TenantID: tid, Shape: "VM.Standard.E3.Flex",
		ImageID: "img1", SubnetID: "sub1", AvailabilityDomain: "AD-1",
		BootVolumeSizeGB: 100, OCPUs: 2, MemoryGB: 16,
	})
	createInstancePlan(t, store, &db.InstancePlan{
		Name: "Plan B", TenantID: tid, Shape: "VM.Standard.E5.Flex",
		ImageID: "img2", SubnetID: "sub2", AvailabilityDomain: "AD-2",
		BootVolumeSizeGB: 200, OCPUs: 4, MemoryGB: 32,
	})

	resp := authedReq(t, ts, http.MethodGet, "/api/instance-plans", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/instance-plans: %d, want 200", resp.StatusCode)
	}

	m := jsonMap(t, resp)
	data, _ := m["data"].([]interface{})
	if len(data) != 2 {
		t.Fatalf("got %d plans, want 2", len(data))
	}
}

func TestHandleInstancePlans_List_ByTenant(t *testing.T) {
	_, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	tid1 := seedTenant(t, store)
	// Create second tenant.
	store.CreateTenant(&db.Tenant{Name: "t2", Region: "us-ashburn-1", UserOCID: "u2", TenancyOCID: "t2", Status: "active"})
	list, _ := store.ListTenants()
	var tid2 int64
	for _, x := range list {
		if x.Name == "t2" {
			tid2 = x.ID
			break
		}
	}

	createInstancePlan(t, store, &db.InstancePlan{
		Name: "Plan A", TenantID: tid1, Shape: "VM.Standard.E3.Flex",
		ImageID: "img", SubnetID: "sub", AvailabilityDomain: "AD-1",
		BootVolumeSizeGB: 50, OCPUs: 1, MemoryGB: 8,
	})
	createInstancePlan(t, store, &db.InstancePlan{
		Name: "Plan B", TenantID: tid2, Shape: "VM.Standard.E4.Flex",
		ImageID: "img", SubnetID: "sub", AvailabilityDomain: "AD-2",
		BootVolumeSizeGB: 50, OCPUs: 1, MemoryGB: 8,
	})

	resp := authedReq(t, ts, http.MethodGet, "/api/instance-plans?tenant_id="+itoa(tid1), "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/instance-plans?tenant_id=%d: %d, want 200", tid1, resp.StatusCode)
	}

	m := jsonMap(t, resp)
	data, _ := m["data"].([]interface{})
	if len(data) != 1 {
		t.Fatalf("got %d plans for tid1, want 1", len(data))
	}
}

func TestHandleInstancePlans_Create(t *testing.T) {
	_, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	tid := seedTenant(t, store)
	body := `{"name":"New Plan","tenant_id":` + itoa(tid) + `,"shape":"VM.Standard.E3.Flex","image_id":"img","subnet_id":"sub","availability_domain":"AD-1","boot_volume_size_gb":100,"ocpus":2,"memory_gb":16}`

	resp := authedReq(t, ts, http.MethodPost, "/api/instance-plans", body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/instance-plans: %d, want 200", resp.StatusCode)
	}

	m := jsonMap(t, resp)
	if m["name"] != "New Plan" {
		t.Fatalf("name = %v, want 'New Plan'", m["name"])
	}
}

func TestHandleInstancePlanByID_Update(t *testing.T) {
	_, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	tid := seedTenant(t, store)
	p := createInstancePlan(t, store, &db.InstancePlan{
		Name: "Old Name", TenantID: tid, Shape: "VM.Standard.E3.Flex",
		ImageID: "img", SubnetID: "sub", AvailabilityDomain: "AD-1",
		BootVolumeSizeGB: 50, OCPUs: 1, MemoryGB: 8,
	})

	body := `{"name":"Updated Plan","shape":"VM.Standard.E5.Flex","image_id":"img2","subnet_id":"sub2","availability_domain":"AD-2","boot_volume_size_gb":200,"ocpus":4,"memory_gb":32}`
	resp := authedReq(t, ts, http.MethodPut, "/api/instance-plans/"+itoa(p.ID), body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("PUT /api/instance-plans/%d: %d, want 200", p.ID, resp.StatusCode)
	}
}

func TestHandleInstancePlanByID_Delete(t *testing.T) {
	_, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	tid := seedTenant(t, store)
	p := createInstancePlan(t, store, &db.InstancePlan{
		Name: "To Delete", TenantID: tid, Shape: "VM.Standard.E3.Flex",
		ImageID: "img", SubnetID: "sub", AvailabilityDomain: "AD-1",
		BootVolumeSizeGB: 50, OCPUs: 1, MemoryGB: 8,
	})

	resp := authedReq(t, ts, http.MethodDelete, "/api/instance-plans/"+itoa(p.ID), "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE /api/instance-plans/%d: %d, want 200", p.ID, resp.StatusCode)
	}

	m := jsonMap(t, resp)
	if m["status"] != "ok" {
		t.Fatalf("status = %v, want ok", m["status"])
	}
}
