package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/viogus/oci-helper-go/internal/auth"
	"github.com/viogus/oci-helper-go/internal/db"
	"golang.org/x/crypto/bcrypt"
)

// requireAdmin checks the session role and writes a 403 error if not admin.
func (s *Server) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	sess := s.auth.GetSession(r)
	if sess == nil || sess.Role != "admin" {
		jsonErrStatus(w, "admin access required", http.StatusForbidden)
		return false
	}
	return true
}

func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	switch r.Method {
	case http.MethodGet:
		list, err := s.store.ListUsers()
		if err != nil {
			jsonErr(w, "list users: "+err.Error())
			return
		}
		if list == nil {
			list = []db.User{}
		}
		jsonOK(w, map[string]interface{}{"data": list})

	case http.MethodPost:
		var req struct {
			Username string `json:"username"`
			Password string `json:"password"`
			Email    string `json:"email"`
			Role     string `json:"role"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonErr(w, "invalid body: "+err.Error())
			return
		}
		if req.Username == "" || req.Password == "" {
			jsonErr(w, "username and password required")
			return
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
		if err != nil {
			jsonErr(w, "hash password: "+err.Error())
			return
		}
		if req.Role == "" {
			req.Role = "user"
		}
		u := &db.User{
			Username:     req.Username,
			PasswordHash: string(hash),
			Role:         req.Role,
			Email:        req.Email,
		}
		if err := s.store.CreateUser(u); err != nil {
			jsonErr(w, "create user: "+err.Error())
			return
		}
		s.audit(0, "user:create", req.Username, r)
		jsonOK(w, map[string]interface{}{
			"id":       u.ID,
			"username": u.Username,
			"role":     u.Role,
			"email":    u.Email,
		})

	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleUserByID(w http.ResponseWriter, r *http.Request) {
	if !s.requireAdmin(w, r) {
		return
	}
	idStr := strings.TrimPrefix(r.URL.Path, "/api/users/")
	idStr = strings.TrimSuffix(idStr, "/")
	parts := strings.SplitN(idStr, "/", 2)
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id <= 0 {
		jsonErr(w, "invalid user id")
		return
	}

	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	switch r.Method {
	case http.MethodDelete:
		if action == "mfa" || action == "mfa-device" {
			// Clear MFA for user
			if err := s.store.UpdateUserMFA(id, "", false); err != nil {
				jsonErr(w, "clear mfa: "+err.Error())
				return
			}
			s.audit(0, "user:mfa:clear", strconv.FormatInt(id, 10), r)
			jsonOK(w, map[string]string{"status": "ok"})
			return
		}
		if action == "reset-password" {
			var req struct{ Password string `json:"password"` }
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				jsonErr(w, "invalid body: "+err.Error())
				return
			}
			if req.Password == "" {
				jsonErr(w, "password required")
				return
			}
			hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
			if err != nil {
				jsonErr(w, "hash: "+err.Error())
				return
			}
			if err := s.store.UpdateUserPassword(id, string(hash)); err != nil {
				jsonErr(w, "update password: "+err.Error())
				return
			}
			s.audit(0, "user:password:reset", strconv.FormatInt(id, 10), r)
			jsonOK(w, map[string]string{"status": "ok"})
			return
		}
		if action == "" {
			// Prevent deleting the last admin user
			users, _ := s.store.ListUsers()
			adminCount := 0
			for _, u := range users {
				if u.Role == "admin" {
					adminCount++
				}
			}
			if adminCount <= 1 {
				// Check if target user is an admin via stored list
				for _, u := range users {
					if u.ID == id && u.Role == "admin" {
						jsonErr(w, "cannot delete the last admin user")
						return
					}
				}
			}
			if err := s.store.DeleteUser(id); err != nil {
				jsonErr(w, "delete user: "+err.Error())
				return
			}
			s.audit(0, "user:delete", strconv.FormatInt(id, 10), r)
			jsonOK(w, map[string]string{"status": "ok"})
			return
		}
		jsonErr(w, "unknown action: "+action)

	case http.MethodPost:
		switch action {
		case "mfa/setup":
			u, err := s.store.GetUserByID(id)
			if err != nil || u == nil {
				jsonErr(w, "user not found")
				return
			}
			secret := auth.GenerateMFA()
			if err := s.store.UpdateUserMFA(id, secret, false); err != nil {
				jsonErr(w, "save mfa secret: "+err.Error())
				return
			}
			uri := auth.TOTPURI(secret, u.Username, "oci-helper")
			s.audit(0, "user:mfa:setup", u.Username, r)
			jsonOK(w, map[string]string{"secret": secret, "uri": uri})
			return

		case "mfa/verify":
			var req struct {
				Code string `json:"code"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				jsonErr(w, "invalid body: "+err.Error())
				return
			}
			u, err := s.store.GetUserByID(id)
			if err != nil || u == nil {
				jsonErr(w, "user not found")
				return
			}
			if u.MFASecret == "" {
				jsonErr(w, "MFA not set up, call mfa/setup first")
				return
			}
			if !auth.ValidateTOTP(u.MFASecret, req.Code) {
				jsonErr(w, "invalid code")
				return
			}
			if err := s.store.UpdateUserMFA(id, u.MFASecret, true); err != nil {
				jsonErr(w, "enable mfa: "+err.Error())
				return
			}
			s.audit(0, "user:mfa:enabled", u.Username, r)
			jsonOK(w, map[string]string{"status": "ok"})
			return

		case "mfa/disable":
			var req struct {
				Code string `json:"code"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				jsonErr(w, "invalid body: "+err.Error())
				return
			}
			u, err := s.store.GetUserByID(id)
			if err != nil || u == nil {
				jsonErr(w, "user not found")
				return
			}
			if u.MFASecret == "" || !auth.ValidateTOTP(u.MFASecret, req.Code) {
				jsonErr(w, "valid TOTP code required to disable MFA")
				return
			}
			if err := s.store.UpdateUserMFA(id, "", false); err != nil {
				jsonErr(w, "disable mfa: "+err.Error())
				return
			}
			s.audit(0, "user:mfa:disabled", u.Username, r)
			jsonOK(w, map[string]string{"status": "ok"})
			return

		default:
			jsonErr(w, "unknown action: "+action)
		}

	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}
