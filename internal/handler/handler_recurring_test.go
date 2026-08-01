package handler

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestRecurringCreateTaskLifecycle(t *testing.T) {
	_, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	tid := seedTenant(t, store)
	body := `{
		"tenant_id": ` + itoa(tid) + `,
		"region": "us-ashburn-1",
		"ocpus": 1,
		"memory_gb": 1,
		"disk": 50,
		"architecture": "AMD",
		"interval_seconds": 60,
		"create_numbers": 2,
		"operation_system": "Ubuntu",
		"root_password": "test123"
	}`
	resp := authedReq(t, ts, http.MethodPost, "/api/create-tasks/recurring", body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create recurring task: %d, want 200", resp.StatusCode)
	}
	tasks, err := store.ListCreateTasks(tid)
	if err != nil {
		t.Fatalf("list create tasks: %v", err)
	}
	if len(tasks) != 1 {
		t.Fatalf("expected 1 recurring task, got %d", len(tasks))
	}
	task := tasks[0]
	if task.CreateNumbers != 2 || task.IntervalSeconds != 60 || task.Architecture != "AMD" {
		t.Fatalf("unexpected task fields: %+v", task)
	}

	resp = authedReq(t, ts, http.MethodPost, "/api/create-tasks/recurring/"+itoa(task.ID)+"/pause", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("pause: %d, want 200", resp.StatusCode)
	}
	got, _ := store.GetCreateTask(task.ID)
	if got == nil || !got.Paused {
		t.Fatal("task should be paused")
	}

	resp = authedReq(t, ts, http.MethodPost, "/api/create-tasks/recurring/"+itoa(task.ID)+"/resume", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("resume: %d, want 200", resp.StatusCode)
	}
	got, _ = store.GetCreateTask(task.ID)
	if got == nil || got.Paused {
		t.Fatal("task should be resumed")
	}
}

func TestTenantAliveCheckReportsAllTenants(t *testing.T) {
	_, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	seedTenant(t, store)
	seedTenant(t, store)

	resp := authedReq(t, ts, http.MethodPost, "/api/tenants/check-alive", `{}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("check alive: %d, want 200", resp.StatusCode)
	}
	var out struct {
		Results []map[string]interface{} `json:"results"`
		Total   int                      `json:"total"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.Total != 2 {
		t.Fatalf("expected 2 tenant results, got %d", out.Total)
	}
}
