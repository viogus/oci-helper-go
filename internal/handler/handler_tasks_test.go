package handler

import (
	"net/http"
	"testing"

	"github.com/viogus/oci-helper-go/internal/db"
)

func TestHandleCreateTasks_InvalidAction(t *testing.T) {
	_, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	tid := seedTenant(t, store)
	store.CreateTask(&db.Task{
		TenantID: tid,
		Type:     "batch_create",
		Status:   "pending",
		Payload:  "[]",
	})
	list, _ := store.ListTasks()
	if len(list) == 0 {
		t.Fatal("no task created")
	}
	taskID := list[0].ID

	// POST to /api/create-tasks/{id}/invalid-action
	resp := authedReq(t, ts, http.MethodPost, "/api/create-tasks/"+itoa(taskID)+"/invalid-action", "")
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST /api/create-tasks/%d/invalid-action: %d, want 400", taskID, resp.StatusCode)
	}

	m := jsonMap(t, resp)
	if m["error"] == nil {
		t.Fatal("expected error for invalid action")
	}
}
