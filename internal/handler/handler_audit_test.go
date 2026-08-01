package handler

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func b64Decode(t *testing.T, s string) []byte {
	t.Helper()
	b, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		t.Fatalf("b64 decode: %v", err)
	}
	return b
}

func b64Encode(t *testing.T, b []byte) string {
	t.Helper()
	return base64.RawURLEncoding.EncodeToString(b)
}

// TestMemTasksGETValueCopy verifies the GET handler returns a stable snapshot
// even while worker goroutines mutate the live tasks (regression for the
// marshaling-outside-the-lock race).
func TestMemTasksGETValueCopy(t *testing.T) {
	_, _, ts, cleanup := setupTestServer(t)
	defer cleanup()

	// Seed an in-memory change-ip task directly (same package).
	memTasksMu.Lock()
	memTasks["test-1"] = &memTask{ID: "test-1", TaskType: "change_ip", Remark: "seed", Paused: false, Attempts: 3}
	memTasksMu.Unlock()
	defer func() {
		memTasksMu.Lock()
		delete(memTasks, "test-1")
		memTasksMu.Unlock()
	}()

	// Concurrently mutate the task while the handler marshals it.
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
				memTasksMu.Lock()
				if t, ok := memTasks["test-1"]; ok {
					t.Attempts++
				}
				memTasksMu.Unlock()
			}
		}
	}()

	for i := 0; i < 50; i++ {
		resp := authedReq(t, ts, http.MethodGet, "/api/mem-tasks/change-ip?page=1&size=20", "")
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET mem-tasks: status %d", resp.StatusCode)
		}
		var body struct {
			Data []map[string]interface{} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			resp.Body.Close()
			t.Fatalf("decode: %v", err)
		}
		resp.Body.Close()
		found := false
		for _, d := range body.Data {
			if d["id"] == "test-1" {
				found = true
				if _, ok := d["remark"]; !ok {
					t.Fatalf("task fields missing: %v", d)
				}
			}
		}
		if !found {
			t.Fatalf("seeded task not returned")
		}
	}
	close(stop)
	<-done
}

func TestTGSSHValidateCommand(t *testing.T) {
	allowed := []string{"ls -la", "cat /etc/os-release", "df -h | head -5", "uptime"}
	for _, c := range allowed {
		if msg := tgSSHValidateCommand(c); msg != "" {
			t.Errorf("command %q should be allowed, got rejection: %s", c, msg)
		}
	}

	blocked := []string{
		"top", "vi /etc/hosts", "tail -f /var/log/syslog", "htop",
		"echo x; top", "cmd && reboot", "a || b", "ls `id`", "echo $(rm -rf /)",
		"ls\nreboot", "echo hi\rrm -rf /", "echo x | bash", "sudo reboot",
	}
	for _, c := range blocked {
		if msg := tgSSHValidateCommand(c); msg == "" {
			t.Errorf("command %q should be rejected, was allowed", c)
		}
	}
}

// TestRestoreRefreshesMFACache verifies that restoring a backup that enables
// MFA updates the in-memory MFA cache (regression for the login bypass until
// restart).
func TestRestoreRefreshesMFACache(t *testing.T) {
	srv, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	// Build a backup with MFA enabled.
	store.SetConfig("mfa_enabled", "false")
	srv.refreshMFACache()

	resp := authedReq(t, ts, http.MethodPost, "/api/backup", `{"password":"pw"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("backup: %d", resp.StatusCode)
	}
	var b struct {
		Data string `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&b); err != nil {
		resp.Body.Close()
		t.Fatalf("decode backup: %v", err)
	}
	resp.Body.Close()

	// Enable MFA in the DB, then restore the (MFA-off) backup and confirm the
	// cache follows the restored value (false), not the pre-restore DB.
	store.SetConfig("mfa_enabled", "true")
	srv.refreshMFACache()
	srv.mfaCacheMu.Lock()
	before := srv.mfaCache.enabled
	srv.mfaCacheMu.Unlock()
	if !before {
		t.Fatalf("precondition: mfa cache should be true before restore")
	}

	resp = authedReq(t, ts, http.MethodPost, "/api/restore", `{"password":"pw","data":"`+b.Data+`"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("restore: %d", resp.StatusCode)
	}
	resp.Body.Close()

	srv.mfaCacheMu.Lock()
	after := srv.mfaCache.enabled
	srv.mfaCacheMu.Unlock()
	if after {
		t.Fatalf("mfa cache not refreshed after restore: still enabled")
	}
	got, _ := store.GetConfig("mfa_enabled")
	if got != "false" {
		t.Fatalf("db mfa_enabled = %q, want false", got)
	}
}

// TestRestoreRejectsTraversalKeyName guards the key-file path traversal fix.
func TestRestoreRejectsTraversalKeyName(t *testing.T) {
	_, _, ts, cleanup := setupTestServer(t)
	defer cleanup()

	// Build a backup payload manually with a traversal key file name.
	resp := authedReq(t, ts, http.MethodPost, "/api/backup", `{"password":"pw"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("backup: %d", resp.StatusCode)
	}
	var b struct {
		Data string `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&b); err != nil {
		resp.Body.Close()
		t.Fatalf("decode backup: %v", err)
	}
	resp.Body.Close()

	plain, err := decrypt(b64Decode(t, b.Data), "pw")
	if err != nil {
		t.Fatalf("decrypt: %v", err)
	}
	// Inject a traversal name into the payload.
	var payload map[string]interface{}
	if err := json.Unmarshal(plain, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	payload["key_files"] = []map[string]interface{}{{"name": "../../evil", "content": "data"}}
	evil, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal evil: %v", err)
	}
	enc, err := encrypt(evil, "pw")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}

	resp = authedReq(t, ts, http.MethodPost, "/api/restore", `{"password":"pw","data":"`+b64Encode(t, enc)+`"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("restore with traversal name: status %d, want 400", resp.StatusCode)
	}
	var e struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&e); err != nil {
		resp.Body.Close()
		t.Fatalf("decode error: %v", err)
	}
	resp.Body.Close()
	if !strings.Contains(e.Error, "invalid name") {
		t.Fatalf("unexpected error: %s", e.Error)
	}
}
