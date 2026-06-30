package handler

import (
	"net/http"
	"testing"
)

func TestHandleMemTasks_Add(t *testing.T) {
	_, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	tid := seedTenant(t, store)

	body := `{"action":"add","tenant_id":` + itoa(tid) + `,"instance_id":"ocid1.inst.test","cidr_list":["10.0.0.0/24"]}`
	resp := authedReq(t, ts, http.MethodPost, "/api/mem-tasks/change-ip", body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/mem-tasks/change-ip add: %d, want 200", resp.StatusCode)
	}

	m := jsonMap(t, resp)
	if m["task_id"] == nil || m["task_id"] == "" {
		t.Fatal("task_id should not be empty")
	}
	if m["status"] != "started" {
		t.Fatalf("status = %v, want started", m["status"])
	}

	// Verify task appears in GET list
	resp2 := authedReq(t, ts, http.MethodGet, "/api/mem-tasks/change-ip", "")
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/mem-tasks/change-ip: %d, want 200", resp2.StatusCode)
	}
	m2 := jsonMap(t, resp2)
	data, _ := m2["data"].([]interface{})
	if len(data) != 1 {
		t.Fatalf("got %d tasks after add, want 1", len(data))
	}
}

func TestHandleMemTasks_List(t *testing.T) {
	_, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	tid := seedTenant(t, store)

	// Add 3 tasks
	for i := 0; i < 3; i++ {
		body := `{"action":"add","tenant_id":` + itoa(tid) + `,"instance_id":"ocid1.inst.` + itoa(int64(i)) + `"}`
		resp := authedReq(t, ts, http.MethodPost, "/api/mem-tasks/change-ip", body)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("add task %d: %d", i, resp.StatusCode)
		}
		resp.Body.Close()
	}

	resp := authedReq(t, ts, http.MethodGet, "/api/mem-tasks/change-ip?page=1&size=20", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/mem-tasks/change-ip: %d, want 200", resp.StatusCode)
	}

	m := jsonMap(t, resp)
	data, _ := m["data"].([]interface{})
	if len(data) != 3 {
		t.Fatalf("got %d tasks, want 3", len(data))
	}
	total, _ := m["total"].(float64)
	if int(total) != 3 {
		t.Fatalf("total = %v, want 3", total)
	}
}

func TestHandleMemTasks_Pause(t *testing.T) {
	_, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	tid := seedTenant(t, store)

	// Add a task
	addBody := `{"action":"add","tenant_id":` + itoa(tid) + `,"instance_id":"ocid1.inst.pause"}`
	resp := authedReq(t, ts, http.MethodPost, "/api/mem-tasks/change-ip", addBody)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("add task: %d", resp.StatusCode)
	}
	m := jsonMap(t, resp)
	taskID := m["task_id"].(string)
	resp.Body.Close()

	// Pause the task
	pauseBody := `{"action":"pause","task_ids":["` + taskID + `"]}`
	resp2 := authedReq(t, ts, http.MethodPost, "/api/mem-tasks/change-ip", pauseBody)
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("pause task: %d", resp2.StatusCode)
	}
	resp2.Body.Close()

	// Verify paused=true in list
	resp3 := authedReq(t, ts, http.MethodGet, "/api/mem-tasks/change-ip", "")
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("list after pause: %d", resp3.StatusCode)
	}
	m3 := jsonMap(t, resp3)
	data, _ := m3["data"].([]interface{})
	if len(data) < 1 {
		t.Fatal("no tasks in list after pause")
	}
	foundPaused := false
	for _, item := range data {
		task := item.(map[string]interface{})
		if task["id"] == taskID {
			paused, _ := task["paused"].(bool)
			if paused {
				foundPaused = true
			}
			break
		}
	}
	if !foundPaused {
		t.Fatal("task should be paused")
	}
}

func TestHandleMemTasks_Resume(t *testing.T) {
	_, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	tid := seedTenant(t, store)

	// Add a task
	addBody := `{"action":"add","tenant_id":` + itoa(tid) + `,"instance_id":"ocid1.inst.resume"}`
	resp := authedReq(t, ts, http.MethodPost, "/api/mem-tasks/change-ip", addBody)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("add task: %d", resp.StatusCode)
	}
	m := jsonMap(t, resp)
	taskID := m["task_id"].(string)
	resp.Body.Close()

	// Pause
	pauseBody := `{"action":"pause","task_ids":["` + taskID + `"]}`
	authedReq(t, ts, http.MethodPost, "/api/mem-tasks/change-ip", pauseBody).Body.Close()

	// Resume
	resumeBody := `{"action":"resume","task_ids":["` + taskID + `"]}`
	resp2 := authedReq(t, ts, http.MethodPost, "/api/mem-tasks/change-ip", resumeBody)
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("resume task: %d", resp2.StatusCode)
	}
	resp2.Body.Close()

	// Verify paused=false in list
	resp3 := authedReq(t, ts, http.MethodGet, "/api/mem-tasks/change-ip", "")
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("list after resume: %d", resp3.StatusCode)
	}
	m3 := jsonMap(t, resp3)
	data, _ := m3["data"].([]interface{})
	if len(data) < 1 {
		t.Fatal("no tasks in list after resume")
	}
	foundNotPaused := false
	for _, item := range data {
		task := item.(map[string]interface{})
		if task["id"] == taskID {
			paused, _ := task["paused"].(bool)
			if !paused {
				foundNotPaused = true
			}
			break
		}
	}
	if !foundNotPaused {
		t.Fatal("task should not be paused after resume")
	}
}

func TestHandleMemTasks_Delete(t *testing.T) {
	_, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	tid := seedTenant(t, store)

	// Add a task
	addBody := `{"action":"add","tenant_id":` + itoa(tid) + `,"instance_id":"ocid1.inst.del"}`
	resp := authedReq(t, ts, http.MethodPost, "/api/mem-tasks/change-ip", addBody)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("add task: %d", resp.StatusCode)
	}
	m := jsonMap(t, resp)
	taskID := m["task_id"].(string)
	resp.Body.Close()

	// Delete the task
	delBody := `{"action":"delete","task_ids":["` + taskID + `"]}`
	resp2 := authedReq(t, ts, http.MethodPost, "/api/mem-tasks/change-ip", delBody)
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("delete task: %d", resp2.StatusCode)
	}
	resp2.Body.Close()

	// Verify not in list
	resp3 := authedReq(t, ts, http.MethodGet, "/api/mem-tasks/change-ip", "")
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("list after delete: %d", resp3.StatusCode)
	}
	m3 := jsonMap(t, resp3)
	data, _ := m3["data"].([]interface{})
	if len(data) != 0 {
		t.Fatalf("got %d tasks after delete, want 0", len(data))
	}
}
