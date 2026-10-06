package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/viogus/oci-helper-go/internal/db"
)

func setCfg(t *testing.T, store *db.Store, key, value string) {
	t.Helper()
	if err := store.SetConfig(key, value); err != nil {
		t.Fatalf("SetConfig(%s): %v", key, err)
	}
}

// TestTelegramWebhookFailClosed verifies the bot refuses to act unless the
// sender is the explicitly allowlisted chat. Without an allowlist, only the
// deny path is exercised here (the /start discovery reply needs network).
func TestTelegramWebhookFailClosed(t *testing.T) {
	_, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	setCfg(t, store, "telegram_token", "123:abc")
	setCfg(t, store, "telegram_webhook_secret", "s3cr3t")

	post := func(body string) map[string]string {
		req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/telegram/webhook", strings.NewReader(body))
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		req.Header.Set("X-Telegram-Bot-Api-Secret-Token", "s3cr3t")
		req.Header.Set("Content-Type", "application/json")
		resp, err := ts.Client().Do(req)
		if err != nil {
			t.Fatalf("webhook request: %v", err)
		}
		defer resp.Body.Close()
		var m map[string]string
		if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return m
	}

	// No allowlist configured: a normal command must be ignored.
	if m := post(`{"update_id":1,"message":{"message_id":5,"chat":{"id":999},"text":"/instances"}}`); m["status"] != "ignored" {
		t.Fatalf("no allowlist: got %v, want ignored", m)
	}

	// Allowlist configured: a different chat is ignored (command and callback).
	setCfg(t, store, "telegram_chat_id", "123")
	if m := post(`{"update_id":2,"message":{"message_id":6,"chat":{"id":999},"text":"/instances"}}`); m["status"] != "ignored" {
		t.Fatalf("foreign chat message: got %v, want ignored", m)
	}
	cb := `{"update_id":3,"callback_query":{"id":"c1","from":{"id":999},"message":{"message_id":7,"chat":{"id":999}},"data":"menu"}}`
	if m := post(cb); m["status"] != "ignored" {
		t.Fatalf("foreign chat callback: got %v, want ignored", m)
	}
}

// TestCloudflareCfgsMasked ensures list responses never leak CF credentials.
func TestCloudflareCfgsMasked(t *testing.T) {
	_, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	if err := store.CreateCfCfg(&db.CfCfg{
		Name:   "cf",
		Token:  "supersecrettokenvalue",
		APIKey: "api-key-value-123456",
		ZoneID: "zone1",
	}); err != nil {
		t.Fatalf("create cf cfg: %v", err)
	}

	resp := authedReq(t, ts, http.MethodGet, "/api/cloudflare/cfgs", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET cfgs: %d, want 200", resp.StatusCode)
	}
	m := jsonMap(t, resp)
	list, _ := m["data"].([]interface{})
	if len(list) != 1 {
		t.Fatalf("got %d cfgs, want 1", len(list))
	}
	cfg, _ := list[0].(map[string]interface{})
	tok, _ := cfg["token"].(string)
	key, _ := cfg["apiKey"].(string)
	if strings.Contains(tok, "supersecrettokenvalue") {
		t.Errorf("token leaked: %q", tok)
	}
	if strings.Contains(key, "api-key-value-123456") {
		t.Errorf("apiKey leaked: %q", key)
	}
}

// TestCreateTaskRootPasswordMaskedAndEncrypted verifies the recurring-task
// password is never returned over the API and is encrypted at rest.
func TestCreateTaskRootPasswordMaskedAndEncrypted(t *testing.T) {
	_, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	tid := seedTenant(t, store)
	body := `{"tenant_id":` + itoa(tid) + `,"root_password":"S3cretRootPw","interval_seconds":60,"create_numbers":1}`
	resp := authedReq(t, ts, http.MethodPost, "/api/create-tasks/recurring", body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create recurring task: %d, want 200", resp.StatusCode)
	}
	m := jsonMap(t, resp)
	if pw, _ := m["rootPassword"].(string); pw == "S3cretRootPw" {
		t.Fatal("create response leaked plaintext root password")
	}

	// Raw column must be encrypted (the handler installs a secret key).
	var stored string
	if err := store.DB().QueryRow(`SELECT root_password FROM create_tasks LIMIT 1`).Scan(&stored); err != nil {
		t.Fatalf("read raw root_password: %v", err)
	}
	if stored == "S3cretRootPw" {
		t.Fatal("root password stored in plaintext")
	}
	if !strings.HasPrefix(stored, "enc1:") {
		t.Fatalf("expected encrypted root_password, got %q", stored)
	}

	// GET list must be masked too.
	resp = authedReq(t, ts, http.MethodGet, "/api/create-tasks/recurring?tenant_id="+itoa(tid), "")
	m = jsonMap(t, resp)
	data, _ := m["data"].([]interface{})
	if len(data) != 1 {
		t.Fatalf("got %d tasks, want 1", len(data))
	}
	item, _ := data[0].(map[string]interface{})
	if item["rootPassword"] == "S3cretRootPw" {
		t.Fatal("GET list leaked plaintext root password")
	}
}

func TestValidRepoSpec(t *testing.T) {
	valid := []string{"owner/repo", "viogus/oci-helper-go", "a/b.c_d-e"}
	for _, s := range valid {
		if !validRepoSpec(s) {
			t.Errorf("validRepoSpec(%q) = false, want true", s)
		}
	}
	invalid := []string{"", "owner", "a/b/c", "owner/repo?x=1", "../../etc", "owner/../repo", "http://evil.com/x"}
	for _, s := range invalid {
		if validRepoSpec(s) {
			t.Errorf("validRepoSpec(%q) = true, want false", s)
		}
	}
}

func TestReadLastNLinesZero(t *testing.T) {
	if got := readLastNLines(nil, 0); got != nil {
		t.Fatalf("readLastNLines(nil, 0) = %v, want nil", got)
	}
}

// TestAPIErrSanitizes ensures internal error details are logged, not returned.
func TestAPIErrSanitizes(t *testing.T) {
	s := &Server{}
	rr := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/x", nil)
	s.apiErr(rr, r, "list things: ", errors.New("boom /secret/path"))

	body := rr.Body.String()
	if strings.Contains(body, "secret/path") || strings.Contains(body, "boom") {
		t.Fatalf("response leaked internal error: %s", body)
	}
	if !strings.Contains(body, "list things") {
		t.Fatalf("response missing public message: %s", body)
	}
}

func TestConfigMaskedWriteIgnored(t *testing.T) {
	_, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	setCfg(t, store, "telegram_token", "real-token-value-123")

	resp := authedReq(t, ts, http.MethodPost, "/api/config", `{"key":"telegram_token","value":"re***23"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("masked write: %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()
	if got, _ := store.GetConfig("telegram_token"); got != "real-token-value-123" {
		t.Fatalf("masked write clobbered secret: %q", got)
	}

	resp = authedReq(t, ts, http.MethodPost, "/api/config", `{"key":"telegram_token","value":"new-real-token"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("real write: %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()
	if got, _ := store.GetConfig("telegram_token"); got != "new-real-token" {
		t.Fatalf("real write not applied: %q", got)
	}
}

// TestBackupRestoreIncludesRecurringTasks ensures recurring create tasks are
// not silently dropped by a backup/restore cycle.
func TestBackupRestoreIncludesRecurringTasks(t *testing.T) {
	_, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	tid := seedTenant(t, store)
	if err := store.CreateCreateTask(&db.CreateTask{
		TenantID: tid, Region: "r", RootPassword: "RecurPw", IntervalSeconds: 60, CreateNumbers: 1,
	}); err != nil {
		t.Fatalf("create recurring task: %v", err)
	}

	resp := authedReq(t, ts, http.MethodPost, "/api/backup", `{"password":"pw"}`)
	m := jsonMap(t, resp)
	data, _ := m["data"].(string)
	if data == "" {
		t.Fatal("backup data empty")
	}

	if err := store.ClearAll(); err != nil {
		t.Fatalf("clear all: %v", err)
	}

	resp = authedReq(t, ts, http.MethodPost, "/api/restore", `{"password":"pw","data":"`+data+`"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("restore: %d, want 200", resp.StatusCode)
	}

	list, err := store.ListCreateTasks(0)
	if err != nil {
		t.Fatalf("list recurring tasks: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("recurring tasks after restore = %d, want 1", len(list))
	}
	if list[0].RootPassword != "RecurPw" {
		t.Fatalf("recurring task password lost: %q", list[0].RootPassword)
	}
}

// TestHandlerRecoversPanic ensures a handler panic returns a clean 500 instead
// of relying on net/http closing the connection.
func TestHandlerRecoversPanic(t *testing.T) {
	srv, _, _, cleanup := setupTestServer(t)
	defer cleanup()

	srv.mux.HandleFunc("/panic-test", func(http.ResponseWriter, *http.Request) { panic("boom") })

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/panic-test", nil)
	srv.Handler().ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("panic handler status = %d, want 500", rr.Code)
	}
}
