package handler

import (
	"net/http"
	"testing"

	"github.com/viogus/oci-helper-go/internal/db"
)

func TestBackupRestoreIncludesAllTables(t *testing.T) {
	_, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	tid := seedTenant(t, store)
	_ = store.CreateUser(&db.User{Username: "admin", PasswordHash: "hash", Role: "admin", Email: "a@b.c"})
	_ = store.CreateCfCfg(&db.CfCfg{Name: "cf1", Token: "tok", ZoneID: "zone"})
	_ = store.CreateIpData(&db.IpData{TenantID: tid, CIDR: "1.2.3.4/32", Label: "x", Type: "pool", Enabled: true})
	_ = store.CreateSSHKey(&db.SSHKey{Name: "k", PublicKey: "pub", Fingerprint: "fp"})
	_ = store.CreateInstancePlan(&db.InstancePlan{Name: "p", TenantID: tid, Shape: "s", BootVolumeSizeGB: 50, OCPUs: 1, MemoryGB: 1})
	_ = store.CreateStockAlert(&db.StockAlert{TenantID: tid, Region: "us", Shape: "s"})
	_ = store.CreateTask(&db.Task{TenantID: tid, Type: "batch_start", Status: "completed", Payload: "{}"})

	resp := authedReq(t, ts, http.MethodPost, "/api/backup", `{"password":"secret"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("backup: %d, want 200", resp.StatusCode)
	}
	m := jsonMap(t, resp)
	data, _ := m["data"].(string)
	if data == "" {
		t.Fatal("backup data empty")
	}

	resp = authedReq(t, ts, http.MethodPost, "/api/restore", `{"password":"secret","data":"`+data+`"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("restore: %d, want 200", resp.StatusCode)
	}

	users, _ := store.ListUsers()
	if len(users) != 1 {
		t.Fatalf("expected 1 user after restore, got %d", len(users))
	}
	cfgs, _ := store.ListCfCfgs()
	if len(cfgs) != 1 {
		t.Fatalf("expected 1 cf config after restore, got %d", len(cfgs))
	}
	ipData, _ := store.ListIpData(0, "")
	if len(ipData) != 1 {
		t.Fatalf("expected 1 ip data after restore, got %d", len(ipData))
	}
	plans, _ := store.ListInstancePlans(0)
	if len(plans) != 1 {
		t.Fatalf("expected 1 plan after restore, got %d", len(plans))
	}
	tasks, _ := store.ListTasks()
	if len(tasks) != 1 {
		t.Fatalf("expected 1 task after restore, got %d", len(tasks))
	}
}
