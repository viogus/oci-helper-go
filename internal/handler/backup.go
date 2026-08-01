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
	TenantID                                                   int64
	OCPU, MemoryGB                                             float64
	BootVolumeGB                                               int64
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
	defer tx.Rollback() // safe no-op after successful Commit

	if err := s.store.ClearAllTx(tx); err != nil {
		jsonErr(w, "clear: "+err.Error())
		return
	}

	// restore tenants
	for _, t := range data.Tenants {
		if err := s.store.CreateTenantImportWithIDTx(tx, t.ID, t.Name, t.UserOCID, t.TenancyOCID, t.Region, t.Fingerprint, t.KeyFile); err != nil {
			tx.Rollback()
			jsonErr(w, "restore tenant: "+err.Error())
			return
		}
	}

	// restore instances
	for _, i := range data.Instances {
		if err := s.store.UpsertInstanceImportTx(tx, i.ID, i.TenantID, i.Name, i.OCID, i.Shape, i.State, i.PublicIP, i.PrivateIP, i.Region, i.AvailabilityDomain, i.FaultDomain, i.ImageID, i.SubnetID, i.OCPU, i.MemoryGB, i.BootVolumeGB); err != nil {
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

	for _, u := range data.Users {
		if err := s.store.CreateUserImportTx(tx, u.Username, u.PasswordHash, u.Role, u.MFASecret, u.Email, u.MFAEnabled); err != nil {
			tx.Rollback()
			jsonErr(w, "restore user: "+err.Error())
			return
		}
	}
	for i := range data.CfCfgs {
		if err := s.store.CreateCfCfgImportTx(tx, &data.CfCfgs[i]); err != nil {
			tx.Rollback()
			jsonErr(w, "restore cf config: "+err.Error())
			return
		}
	}
	for i := range data.IpData {
		if err := s.store.CreateIpDataImportTx(tx, &data.IpData[i]); err != nil {
			tx.Rollback()
			jsonErr(w, "restore ip data: "+err.Error())
			return
		}
	}
	for i := range data.SSHKeys {
		if err := s.store.CreateSSHKeyImportTx(tx, &data.SSHKeys[i]); err != nil {
			tx.Rollback()
			jsonErr(w, "restore ssh key: "+err.Error())
			return
		}
	}
	for i := range data.InstancePlans {
		if err := s.store.CreateInstancePlanImportTx(tx, &data.InstancePlans[i]); err != nil {
			tx.Rollback()
			jsonErr(w, "restore instance plan: "+err.Error())
			return
		}
	}
	for i := range data.StockAlerts {
		if err := s.store.CreateStockAlertImportTx(tx, &data.StockAlerts[i]); err != nil {
			tx.Rollback()
			jsonErr(w, "restore stock alert: "+err.Error())
			return
		}
	}
	for i := range data.Tasks {
		if err := s.store.CreateTaskImportTx(tx, &data.Tasks[i]); err != nil {
			tx.Rollback()
			jsonErr(w, "restore task: "+err.Error())
			return
		}
	}

	if err := tx.Commit(); err != nil {
		jsonErr(w, "commit: "+err.Error())
		return
	}
	if err := os.MkdirAll(s.cfg.KeysDir, 0700); err != nil {
		jsonErr(w, "create keys dir: "+err.Error())
		return
	}
	for _, kf := range data.KeyFiles {
		if kf.Name == "" {
			continue
		}
		if err := os.WriteFile(filepath.Join(s.cfg.KeysDir, kf.Name), kf.Content, 0600); err != nil {
			jsonErr(w, "restore key file "+kf.Name+": "+err.Error())
			return
		}
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
