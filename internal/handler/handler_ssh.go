package handler

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"strconv"
	"strings"

	"github.com/viogus/oci-helper-go/internal/db"
	gossh "golang.org/x/crypto/ssh"
)

func (s *Server) handleSSHKeys(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		tenantID, _ := strconv.ParseInt(r.URL.Query().Get("tenant_id"), 10, 64)
		list, err := s.store.ListSSHKeys(tenantID)
		if err != nil {
			jsonErr(w, "list ssh keys: "+err.Error())
			return
		}
		if list == nil {
			list = []db.SSHKey{}
		}
		jsonOK(w, map[string]interface{}{"data": list})

	case http.MethodPost:
		var req struct {
			Action    string `json:"action"`
			TenantID  int64  `json:"tenant_id"`
			Name      string `json:"name"`
			PublicKey string `json:"public_key"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonErr(w, "invalid body: "+err.Error())
			return
		}

		if req.Action == "generate" {
			s.handleSSHKeyGenerate(w, r, req)
			return
		}
		if req.PublicKey == "" {
			jsonErr(w, "public_key required")
			return
		}

		// Parse and validate the public key
		pub, _, _, _, err := gossh.ParseAuthorizedKey([]byte(req.PublicKey))
		if err != nil {
			jsonErr(w, "invalid public key: "+err.Error())
			return
		}
		fingerprint := gossh.FingerprintSHA256(pub)

		key := &db.SSHKey{
			Name:        req.Name,
			PublicKey:   strings.TrimSpace(req.PublicKey),
			Fingerprint: fingerprint,
			TenantID:    req.TenantID,
		}
		if err := s.store.CreateSSHKey(key); err != nil {
			jsonErr(w, "create ssh key: "+err.Error())
			return
		}
		s.audit(req.TenantID, "ssh:key:add", fingerprint, r)
		jsonOK(w, key)

	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleSSHKeyGenerate(w http.ResponseWriter, r *http.Request, req struct {
	Action   string `json:"action"`
	TenantID int64  `json:"tenant_id"`
	Name     string `json:"name"`
	PublicKey string `json:"public_key"`
}) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		jsonErr(w, "generate key: "+err.Error())
		return
	}

	privPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	pub, err := gossh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		jsonErr(w, "public key: "+err.Error())
		return
	}
	pubBytes := gossh.MarshalAuthorizedKey(pub)
	fingerprint := gossh.FingerprintSHA256(pub)

	key := &db.SSHKey{
		Name:        req.Name,
		PublicKey:   strings.TrimSpace(string(pubBytes)),
		PrivateKey:  string(privPEM),
		Fingerprint: fingerprint,
		TenantID:    req.TenantID,
	}
	if err := s.store.CreateSSHKey(key); err != nil {
		jsonErr(w, "create ssh key: "+err.Error())
		return
	}

	// Sanitized hash for display
	hashDisplay := base64.RawStdEncoding.EncodeToString(sha256.New().Sum(nil))[:12]
	_ = hashDisplay

	s.audit(req.TenantID, "ssh:key:generate", fingerprint, r)
	jsonOK(w, map[string]interface{}{
		"id":          key.ID,
		"name":        key.Name,
		"fingerprint": fingerprint,
		"public_key":  string(pubBytes),
		"private_key": string(privPEM),
	})
}

func (s *Server) handleSSHKeyByID(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/api/ssh/keys/")
	idStr = strings.TrimSuffix(idStr, "/")
	if idStr == "" || idStr == "generate" {
		return
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		jsonErr(w, "invalid key id")
		return
	}
	if r.Method != http.MethodDelete {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := s.store.DeleteSSHKey(id); err != nil {
		jsonErr(w, "delete ssh key: "+err.Error())
		return
	}
	s.audit(0, "ssh:key:delete", strconv.FormatInt(id, 10), r)
	jsonOK(w, map[string]string{"status": "ok"})
}

// KeyPairToMap creates a map for JSON serialization
func keyPairToMap(id int64, name, fingerprint, pub, priv string) map[string]interface{} {
	m := map[string]interface{}{
		"id":          id,
		"name":        name,
		"fingerprint": fingerprint,
		"public_key":  pub,
	}
	if priv != "" {
		m["private_key"] = priv
	}
	return m
}
