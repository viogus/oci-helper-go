package handler

import (
	"net/http"
	"testing"
)

func TestHandleUsers_List(t *testing.T) {
	_, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	seedUser(t, store, "alice", "pass1", "admin")
	seedUser(t, store, "bob", "pass2", "user")

	resp := authedReq(t, ts, http.MethodGet, "/api/users", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /api/users: %d, want 200", resp.StatusCode)
	}

	m := jsonMap(t, resp)
	data, ok := m["data"].([]interface{})
	if !ok {
		t.Fatalf("data is not array: %T", m["data"])
	}
	if len(data) < 2 {
		t.Fatalf("got %d users, want >= 2", len(data))
	}
}

func TestHandleUsers_Create(t *testing.T) {
	_, _, ts, cleanup := setupTestServer(t)
	defer cleanup()

	body := `{"username":"charlie","password":"secret","role":"user","email":"c@test.com"}`
	resp := authedReq(t, ts, http.MethodPost, "/api/users", body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST /api/users: %d, want 200", resp.StatusCode)
	}

	m := jsonMap(t, resp)
	if m["username"] != "charlie" {
		t.Fatalf("username = %v, want charlie", m["username"])
	}
	if m["role"] != "user" {
		t.Fatalf("role = %v, want user", m["role"])
	}
}

func TestHandleUsers_Create_MissingUsername(t *testing.T) {
	_, _, ts, cleanup := setupTestServer(t)
	defer cleanup()

	resp := authedReq(t, ts, http.MethodPost, "/api/users", `{"password":"x"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST /api/users (missing username): %d, want 400", resp.StatusCode)
	}
	m := jsonMap(t, resp)
	if m["error"] != "username and password required" {
		t.Fatalf("error = %v, want 'username and password required'", m["error"])
	}
}

func TestHandleUsers_Create_Duplicate(t *testing.T) {
	_, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	seedUser(t, store, "charlie", "x", "user")

	resp := authedReq(t, ts, http.MethodPost, "/api/users", `{"username":"charlie","password":"y"}`)
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("POST /api/users (duplicate): %d, want 400", resp.StatusCode)
	}
	m := jsonMap(t, resp)
	if m["error"] == nil {
		t.Fatal("expected error for duplicate username, got none")
	}
}

func TestHandleUserByID_Delete(t *testing.T) {
	_, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	u := seedUser(t, store, "deleteme", "x", "user")

	resp := authedReq(t, ts, http.MethodDelete, "/api/users/"+itoa(u.ID), "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE /api/users/%d: %d, want 200", u.ID, resp.StatusCode)
	}

	list, _ := store.ListUsers()
	for _, user := range list {
		if user.ID == u.ID {
			t.Fatal("user still exists after delete")
		}
	}
}

func TestHandleUserByID_ResetPassword(t *testing.T) {
	_, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	u := seedUser(t, store, "resetme", "oldpass", "user")

	resp := authedReq(t, ts, http.MethodDelete, "/api/users/"+itoa(u.ID)+"/reset-password", `{"password":"newpass"}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reset password: %d, want 200", resp.StatusCode)
	}
	m := jsonMap(t, resp)
	if m["status"] != "ok" {
		t.Fatalf("status = %v, want ok", m["status"])
	}
}

func TestHandleUserByID_ClearMFA(t *testing.T) {
	_, store, ts, cleanup := setupTestServer(t)
	defer cleanup()

	u := seedUser(t, store, "mfauser", "x", "user")
	if err := store.UpdateUserMFA(u.ID, "secret", true); err != nil {
		t.Fatalf("enable mfa: %v", err)
	}

	resp := authedReq(t, ts, http.MethodDelete, "/api/users/"+itoa(u.ID)+"/mfa", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("clear mfa: %d, want 200", resp.StatusCode)
	}
	m := jsonMap(t, resp)
	if m["status"] != "ok" {
		t.Fatalf("status = %v, want ok", m["status"])
	}
}

func TestHandleUserByID_InvalidID(t *testing.T) {
	_, _, ts, cleanup := setupTestServer(t)
	defer cleanup()

	// Deleting a non-existent user ID succeeds silently (0 rows affected).
	resp := authedReq(t, ts, http.MethodDelete, "/api/users/99999", "")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("DELETE invalid user: %d, want 200", resp.StatusCode)
	}
}
