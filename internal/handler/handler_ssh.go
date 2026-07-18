package handler

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
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
			KeyType   string `json:"key_type"`
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
	KeyType  string `json:"key_type"`
}) {
	var (
		pubBytes     []byte
		fingerprint  string
		privPEMBytes []byte
	)

	switch req.KeyType {
	case "ed25519":
		pub, priv, err := ed25519Generate()
		if err != nil {
			jsonErr(w, "generate ed25519 key: "+err.Error())
			return
		}
		privPEMBytes = priv
		pubBytes = pub
	default: // "rsa" or empty (backward compat)
		pub, priv, err := rsaGenerate()
		if err != nil {
			jsonErr(w, "generate rsa key: "+err.Error())
			return
		}
		privPEMBytes = priv
		pubBytes = pub
	}

	// Parse public key for fingerprint
	pk, _, _, _, err := gossh.ParseAuthorizedKey(pubBytes)
	if err != nil {
		jsonErr(w, "parse generated public key: "+err.Error())
		return
	}
	fingerprint = gossh.FingerprintSHA256(pk)

	encKey, err := s.getSSHEncryptionKey()
	if err != nil {
		s.audit(req.TenantID, "ssh:key:generate:error", "get encryption key: "+err.Error(), r)
		jsonErr(w, "get encryption key: "+err.Error())
		return
	}
	encryptedKey, err := encryptSSHPrivateKey(encKey, privPEMBytes)
	if err != nil {
		s.audit(req.TenantID, "ssh:key:generate:error", "encrypt failed: "+err.Error(), r)
		jsonErr(w, "encrypt private key: "+err.Error())
		return
	}
	key := &db.SSHKey{
		Name:        req.Name,
		PublicKey:   strings.TrimSpace(string(pubBytes)),
		PrivateKey:  encryptedKey,
		Fingerprint: fingerprint,
		TenantID:    req.TenantID,
	}
	if err := s.store.CreateSSHKey(key); err != nil {
		jsonErr(w, "create ssh key: "+err.Error())
		return
	}
	s.audit(req.TenantID, "ssh:key:generate", fingerprint, r)

	jsonOK(w, map[string]interface{}{
		"id":          key.ID,
		"name":        key.Name,
		"fingerprint": fingerprint,
		"public_key":  string(pubBytes),
	})
}

// ed25519Generate creates an ED25519 keypair. Returns authorized key bytes and PEM-encoded private key.
func ed25519Generate() (pub []byte, priv []byte, err error) {
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	sshPub, err := gossh.NewPublicKey(pubKey)
	if err != nil {
		return nil, nil, err
	}
	pub = gossh.MarshalAuthorizedKey(sshPub)

	privBytes, err := x509.MarshalPKCS8PrivateKey(privKey)
	if err != nil {
		return nil, nil, err
	}
	priv = pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privBytes,
	})
	return pub, priv, nil
}

// rsaGenerate creates an RSA 4096 keypair. Returns authorized key bytes and PEM-encoded private key.
func rsaGenerate() (pub []byte, priv []byte, err error) {
	privateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, nil, err
	}
	priv = pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})
	pubKey, err := gossh.NewPublicKey(&privateKey.PublicKey)
	if err != nil {
		return nil, nil, err
	}
	pub = gossh.MarshalAuthorizedKey(pubKey)
	return pub, priv, nil
}

func (s *Server) handleSSHKeyByID(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/api/ssh/keys/")
	idStr = strings.TrimSuffix(idStr, "/")
	if idStr == "" || idStr == "generate" {
		http.NotFound(w, r)
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
	if !s.requireAdmin(w, r) {
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
