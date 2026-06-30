package handler

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"golang.org/x/crypto/pbkdf2"
)

type backupData struct {
	Tenants   []dbTenant   `json:"tenants"`
	Instances []dbInstance `json:"instances"`
	Config    []dbConfig   `json:"config"`
}

// lightweight copies to avoid import cycle (handler already imports db)
type dbTenant struct {
	Name, UserOCID, TenancyOCID, Region, Fingerprint, KeyFile, Status string
}

type dbInstance struct {
	ID, Name, OCID, Shape, State, PublicIP, PrivateIP string
	TenantID                                          int64
	OCPU, MemoryGB                                    float64
	BootVolumeGB                                      int64
}

type dbConfig struct {
	Key, Value string
}

func (s *Server) handleBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}
	if req.Password == "" {
		jsonErr(w, "password required")
		return
	}

	data := backupData{}

	tenants, err := s.store.ListTenants()
	if err != nil {
		jsonErr(w, "list tenants: "+err.Error())
		return
	}
	for _, t := range tenants {
		data.Tenants = append(data.Tenants, dbTenant{
			Name: t.Name, UserOCID: t.UserOCID, TenancyOCID: t.TenancyOCID,
			Region: t.Region, Fingerprint: t.Fingerprint, KeyFile: t.KeyFile, Status: t.Status,
		})
	}

	instances, err := s.store.ListInstances(0)
	if err != nil {
		jsonErr(w, "list instances: "+err.Error())
		return
	}
	for _, i := range instances {
		data.Instances = append(data.Instances, dbInstance{
			ID: i.ID, Name: i.Name, OCID: i.OCID, Shape: i.Shape,
			State: i.State, PublicIP: i.PublicIP, PrivateIP: i.PrivateIP,
			TenantID: i.TenantID, OCPU: i.OCPU, MemoryGB: i.MemoryGB, BootVolumeGB: i.BootVolumeGB,
		})
	}

	// export all config keys
	configList, err := s.store.ListAllConfig()
	if err != nil {
		jsonErr(w, "list config: "+err.Error())
		return
	}
	for _, c := range configList {
		data.Config = append(data.Config, dbConfig{Key: c.Key, Value: c.Value})
	}

	plain, _ := json.Marshal(data)
	encrypted, err := encrypt(plain, req.Password)
	if err != nil {
		jsonErr(w, "encrypt: "+err.Error())
		return
	}

	s.audit(0, "backup:export", "", r)
	jsonOK(w, map[string]string{"data": base64.RawURLEncoding.EncodeToString(encrypted)})
}

func (s *Server) handleRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Password string `json:"password"`
		Data     string `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}
	if req.Password == "" || req.Data == "" {
		jsonErr(w, "password and data required")
		return
	}

	encrypted, err := base64.RawURLEncoding.DecodeString(req.Data)
	if err != nil {
		jsonErr(w, "invalid data: "+err.Error())
		return
	}

	plain, err := decrypt(encrypted, req.Password)
	if err != nil {
		jsonErr(w, "decrypt failed: wrong password or corrupt data")
		return
	}

	var data backupData
	if err := json.Unmarshal(plain, &data); err != nil {
		jsonErr(w, "invalid backup: "+err.Error())
		return
	}

	// clear existing data before restore
	tx, err := s.store.BeginTx()
	if err != nil {
		jsonErr(w, "begin tx: "+err.Error())
		return
	}
	if err := s.store.ClearAllTx(tx); err != nil {
		tx.Rollback()
		jsonErr(w, "clear: "+err.Error())
		return
	}

	// restore tenants
	for _, t := range data.Tenants {
		if err := s.store.CreateTenantImportTx(tx, t.Name, t.UserOCID, t.TenancyOCID, t.Region, t.Fingerprint, t.KeyFile); err != nil {
			tx.Rollback()
			jsonErr(w, "restore tenant: "+err.Error())
			return
		}
	}

	// restore instances
	for _, i := range data.Instances {
		if err := s.store.UpsertInstanceImportTx(tx, i.ID, i.TenantID, i.Name, i.OCID, i.Shape, i.State, i.PublicIP, i.PrivateIP, i.OCPU, i.MemoryGB, i.BootVolumeGB); err != nil {
			tx.Rollback()
			jsonErr(w, "restore instance: "+err.Error())
			return
		}
	}

	// restore config
	for _, c := range data.Config {
		if err := s.store.SetConfigTx(tx, c.Key, c.Value); err != nil {
			tx.Rollback()
			jsonErr(w, "restore config: "+err.Error())
			return
		}
	}

	if err := tx.Commit(); err != nil {
		jsonErr(w, "commit: "+err.Error())
		return
	}

	s.audit(0, "backup:restore", fmt.Sprintf("%d tenants, %d instances", len(data.Tenants), len(data.Instances)), r)
	jsonOK(w, map[string]string{"status": "ok"})
}

func encrypt(plaintext []byte, password string) ([]byte, error) {
	salt := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}
	key := pbkdf2.Key([]byte(password), salt, 600000, 32, sha256.New)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	// format: salt + nonce + ciphertext
	out := make([]byte, 0, len(salt)+len(nonce)+len(plaintext)+16)
	out = append(out, salt...)
	out = append(out, nonce...)
	return gcm.Seal(out, nonce, plaintext, nil), nil
}

func decrypt(data []byte, password string) ([]byte, error) {
	const saltLen = 16
	if len(data) < saltLen+12 {
		return nil, fmt.Errorf("ciphertext too short")
	}
	salt := data[:saltLen]
	key := pbkdf2.Key([]byte(password), salt, 600000, 32, sha256.New)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonceSize := gcm.NonceSize()
	if len(data) < saltLen+nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce := data[saltLen : saltLen+nonceSize]
	ciphertext := data[saltLen+nonceSize:]
	return gcm.Open(nil, nonce, ciphertext, nil)
}
