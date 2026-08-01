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
	"os"
	"path/filepath"

	"golang.org/x/crypto/pbkdf2"

	"github.com/viogus/oci-helper-go/internal/db"
)

type backupData struct {
	Tenants       []dbTenant        `json:"tenants"`
	Instances     []dbInstance      `json:"instances"`
	Config        []dbConfig        `json:"config"`
	Users         []dbUser          `json:"users"`
	CfCfgs        []db.CfCfg        `json:"cf_configs"`
	IpData        []db.IpData       `json:"ip_data"`
	SSHKeys       []db.SSHKey       `json:"ssh_keys"`
	InstancePlans []db.InstancePlan `json:"instance_plans"`
	StockAlerts   []db.StockAlert   `json:"stock_alerts"`
	Tasks         []db.Task         `json:"tasks"`
	KeyFiles      []dbKeyFile       `json:"key_files"`
}

// lightweight copies to avoid import cycle (handler already imports db)
type dbTenant struct {
	ID                                                                int64
	Name, UserOCID, TenancyOCID, Region, Fingerprint, KeyFile, Status string
}

type dbInstance struct {
	ID, Name, OCID, Shape, State, PublicIP, PrivateIP, Region, AvailabilityDomain, FaultDomain, ImageID, SubnetID string
	TenantID                                                                                                      int64
	OCPU, MemoryGB                                                                                                float64
	BootVolumeGB                                                                                                  int64
}

type dbConfig struct {
	Key, Value string
}

type dbUser struct {
	Username     string
	PasswordHash string
	Role         string
	MFASecret    string
	Email        string
	MFAEnabled   bool
}

type dbKeyFile struct {
	Name    string
	Content []byte
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
			ID: t.ID, Name: t.Name, UserOCID: t.UserOCID, TenancyOCID: t.TenancyOCID,
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
			State: i.State, PublicIP: i.PublicIP, PrivateIP: i.PrivateIP, Region: i.Region,
			AvailabilityDomain: i.AvailabilityDomain, FaultDomain: i.FaultDomain, ImageID: i.ImageID, SubnetID: i.SubnetID,
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

	users, err := s.store.ListUsers()
	if err == nil {
		for _, u := range users {
			// Re-read full rows so password hashes and MFA secrets survive
			// backup/restore.
			full, err := s.store.GetUserByUsername(u.Username)
			if err == nil && full != nil {
				data.Users = append(data.Users, dbUser{
					Username:     full.Username,
					PasswordHash: full.PasswordHash,
					Role:         full.Role,
					MFASecret:    full.MFASecret,
					Email:        full.Email,
					MFAEnabled:   full.MFAEnabled,
				})
			}
		}
	}
	data.CfCfgs, _ = s.store.ListCfCfgs()
	data.IpData, _ = s.store.ListIpData(0, "")
	data.SSHKeys, _ = s.store.ListSSHKeys(0)
	data.InstancePlans, _ = s.store.ListInstancePlans(0)
	data.StockAlerts, _ = s.store.ListStockAlerts(0)
	data.Tasks, _ = s.store.ListTasks()
	if entries, err := os.ReadDir(s.cfg.KeysDir); err == nil {
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			content, err := os.ReadFile(filepath.Join(s.cfg.KeysDir, e.Name()))
			if err == nil {
				data.KeyFiles = append(data.KeyFiles, dbKeyFile{Name: e.Name(), Content: content})
			}
		}
	}

	plain, err := json.Marshal(data)
	if err != nil {
		jsonErr(w, "marshal backup: "+err.Error())
		return
	}
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

	nTenants, nInstances, err := s.restoreData(req.Password, req.Data)
	if err != nil {
		jsonErr(w, err.Error())
		return
	}
	s.audit(0, "backup:restore", fmt.Sprintf("%d tenants, %d instances", nTenants, nInstances), r)
	jsonOK(w, map[string]string{"status": "ok"})
}

// restoreData decrypts an encrypted backup payload and imports it into the
// database (wiping existing data first). Returns restored tenant/instance
// counts. Shared by the web restore endpoint and the Telegram restore flow.
func (s *Server) restoreData(password, data string) (int, int, error) {
	encrypted, err := base64.RawURLEncoding.DecodeString(data)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid data: %w", err)
	}

	plain, err := decrypt(encrypted, password)
	if err != nil {
		return 0, 0, fmt.Errorf("decrypt failed: wrong password or corrupt data")
	}

	var payload backupData
	if err := json.Unmarshal(plain, &payload); err != nil {
		return 0, 0, fmt.Errorf("invalid backup: %w", err)
	}

	// clear existing data before restore
	tx, err := s.store.BeginTx()
	if err != nil {
		return 0, 0, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() // safe no-op after successful Commit

	if err := s.store.ClearAllTx(tx); err != nil {
		return 0, 0, fmt.Errorf("clear: %w", err)
	}

	for _, t := range payload.Tenants {
		if err := s.store.CreateTenantImportWithIDTx(tx, t.ID, t.Name, t.UserOCID, t.TenancyOCID, t.Region, t.Fingerprint, t.KeyFile); err != nil {
			return 0, 0, fmt.Errorf("restore tenant: %w", err)
		}
	}
	for _, i := range payload.Instances {
		if err := s.store.UpsertInstanceImportTx(tx, i.ID, i.TenantID, i.Name, i.OCID, i.Shape, i.State, i.PublicIP, i.PrivateIP, i.Region, i.AvailabilityDomain, i.FaultDomain, i.ImageID, i.SubnetID, i.OCPU, i.MemoryGB, i.BootVolumeGB); err != nil {
			return 0, 0, fmt.Errorf("restore instance: %w", err)
		}
	}
	for _, c := range payload.Config {
		if err := s.store.SetConfigTx(tx, c.Key, c.Value); err != nil {
			return 0, 0, fmt.Errorf("restore config: %w", err)
		}
	}
	for _, u := range payload.Users {
		if err := s.store.CreateUserImportTx(tx, u.Username, u.PasswordHash, u.Role, u.MFASecret, u.Email, u.MFAEnabled); err != nil {
			return 0, 0, fmt.Errorf("restore user: %w", err)
		}
	}
	for i := range payload.CfCfgs {
		if err := s.store.CreateCfCfgImportTx(tx, &payload.CfCfgs[i]); err != nil {
			return 0, 0, fmt.Errorf("restore cf config: %w", err)
		}
	}
	for i := range payload.IpData {
		if err := s.store.CreateIpDataImportTx(tx, &payload.IpData[i]); err != nil {
			return 0, 0, fmt.Errorf("restore ip data: %w", err)
		}
	}
	for i := range payload.SSHKeys {
		if err := s.store.CreateSSHKeyImportTx(tx, &payload.SSHKeys[i]); err != nil {
			return 0, 0, fmt.Errorf("restore ssh key: %w", err)
		}
	}
	for i := range payload.InstancePlans {
		if err := s.store.CreateInstancePlanImportTx(tx, &payload.InstancePlans[i]); err != nil {
			return 0, 0, fmt.Errorf("restore instance plan: %w", err)
		}
	}
	for i := range payload.StockAlerts {
		if err := s.store.CreateStockAlertImportTx(tx, &payload.StockAlerts[i]); err != nil {
			return 0, 0, fmt.Errorf("restore stock alert: %w", err)
		}
	}
	for i := range payload.Tasks {
		if err := s.store.CreateTaskImportTx(tx, &payload.Tasks[i]); err != nil {
			return 0, 0, fmt.Errorf("restore task: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return 0, 0, fmt.Errorf("commit: %w", err)
	}
	if err := os.MkdirAll(s.cfg.KeysDir, 0700); err != nil {
		return 0, 0, fmt.Errorf("create keys dir: %w", err)
	}
	for _, kf := range payload.KeyFiles {
		if kf.Name == "" {
			continue
		}
		// Prevent path traversal: backup files must restore as plain names
		// inside the keys dir.
		if filepath.Base(kf.Name) != kf.Name {
			return 0, 0, fmt.Errorf("restore key file %q: invalid name", kf.Name)
		}
		if err := os.WriteFile(filepath.Join(s.cfg.KeysDir, kf.Name), kf.Content, 0600); err != nil {
			return 0, 0, fmt.Errorf("restore key file %s: %w", kf.Name, err)
		}
	}

	return len(payload.Tenants), len(payload.Instances), nil
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
