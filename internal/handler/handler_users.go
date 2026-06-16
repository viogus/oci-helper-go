package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/viogus/oci-helper-go/internal/db"
	"golang.org/x/crypto/bcrypt"
)

func (s *Server) handleUsers(w http.ResponseWriter, r *http.Request) {
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
			if err := s.store.DeleteUser(id); err != nil {
				jsonErr(w, "delete user: "+err.Error())
				return
			}
			s.audit(0, "user:delete", strconv.FormatInt(id, 10), r)
			jsonOK(w, map[string]string{"status": "ok"})
			return
		}
		jsonErr(w, "unknown action: "+action)

	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}
