package db

import (
	"database/sql"
	"fmt"
	"time"
)

// Tenant

func (s *Store) CreateTenant(t *Tenant) error {
	_, err := s.db.Exec(
		`INSERT INTO tenants (name, user_ocid, tenancy_ocid, region, fingerprint, key_file)
		 VALUES (?,?,?,?,?,?)`,
		t.Name, t.UserOCID, t.TenancyOCID, t.Region, t.Fingerprint, t.KeyFile)
	return err
}

func (s *Store) ListTenants() ([]Tenant, error) {
	rows, err := s.db.Query(`SELECT id, name, user_ocid, tenancy_ocid, region, fingerprint, key_file, status, coalesce(home_region,''), coalesce(subscribed,''), created_at, updated_at FROM tenants ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Tenant
	for rows.Next() {
		var t Tenant
		if err := rows.Scan(&t.ID, &t.Name, &t.UserOCID, &t.TenancyOCID, &t.Region, &t.Fingerprint, &t.KeyFile, &t.Status, &t.HomeRegion, &t.Subscribed, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, rows.Err()
}

func (s *Store) GetTenant(id int64) (*Tenant, error) {
	var t Tenant
	err := s.db.QueryRow(`SELECT id, name, user_ocid, tenancy_ocid, region, fingerprint, key_file, status, coalesce(home_region,''), coalesce(subscribed,''), created_at, updated_at FROM tenants WHERE id=?`, id).
		Scan(&t.ID, &t.Name, &t.UserOCID, &t.TenancyOCID, &t.Region, &t.Fingerprint, &t.KeyFile, &t.Status, &t.HomeRegion, &t.Subscribed, &t.CreatedAt, &t.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &t, err
}

func (s *Store) DeleteTenant(id int64) error {
	_, err := s.db.Exec(`DELETE FROM tenants WHERE id=?`, id)
	return err
}

// Instance

func (s *Store) UpsertInstance(inst *Instance) error {
	_, err := s.db.Exec(
		`INSERT INTO instances (id, tenant_id, name, ocid, shape, ocpu, memory_gb, boot_volume_gb, public_ip, private_ip, state, availability_domain, fault_domain, image_id, subnet_id, synced_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,CURRENT_TIMESTAMP)
		 ON CONFLICT(id) DO UPDATE SET
		   name=excluded.name, shape=excluded.shape, ocpu=excluded.ocpu, memory_gb=excluded.memory_gb,
		   boot_volume_gb=excluded.boot_volume_gb, public_ip=excluded.public_ip, private_ip=excluded.private_ip,
		   state=excluded.state, availability_domain=excluded.availability_domain, fault_domain=excluded.fault_domain,
		   synced_at=CURRENT_TIMESTAMP`,
		inst.ID, inst.TenantID, inst.Name, inst.OCID, inst.Shape, inst.OCPU, inst.MemoryGB, inst.BootVolumeGB, inst.PublicIP, inst.PrivateIP, inst.State, inst.AvailabilityDomain, inst.FaultDomain, inst.ImageID, inst.SubnetID)
	return err
}

func (s *Store) ListInstances(tenantID int64) ([]Instance, error) {
	rows, err := s.db.Query(`SELECT id, tenant_id, name, ocid, shape, ocpu, memory_gb, boot_volume_gb, public_ip, private_ip, state, availability_domain, fault_domain, image_id, subnet_id, created_at, synced_at FROM instances WHERE tenant_id=? OR ?=0 ORDER BY created_at DESC`, tenantID, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Instance
	for rows.Next() {
		var i Instance
		if err := rows.Scan(&i.ID, &i.TenantID, &i.Name, &i.OCID, &i.Shape, &i.OCPU, &i.MemoryGB, &i.BootVolumeGB, &i.PublicIP, &i.PrivateIP, &i.State, &i.AvailabilityDomain, &i.FaultDomain, &i.ImageID, &i.SubnetID, &i.CreatedAt, &i.SyncedAt); err != nil {
			return nil, err
		}
		list = append(list, i)
	}
	return list, rows.Err()
}

func (s *Store) DeleteInstancesByTenant(tenantID int64) error {
	_, err := s.db.Exec(`DELETE FROM instances WHERE tenant_id=?`, tenantID)
	return err
}

// Instance by OCID

func (s *Store) GetInstanceByID(id string) (*Instance, error) {
	var i Instance
	err := s.db.QueryRow(`SELECT id, tenant_id, name, ocid, shape, ocpu, memory_gb, boot_volume_gb, public_ip, private_ip, state, availability_domain, fault_domain, image_id, subnet_id, created_at, synced_at FROM instances WHERE id=?`, id).
		Scan(&i.ID, &i.TenantID, &i.Name, &i.OCID, &i.Shape, &i.OCPU, &i.MemoryGB, &i.BootVolumeGB, &i.PublicIP, &i.PrivateIP, &i.State, &i.AvailabilityDomain, &i.FaultDomain, &i.ImageID, &i.SubnetID, &i.CreatedAt, &i.SyncedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &i, err
}

// Task

func (s *Store) CreateTask(t *Task) error {
	_, err := s.db.Exec(`INSERT INTO tasks (tenant_id, type, status, payload) VALUES (?,?,?,?)`, t.TenantID, t.Type, t.Status, t.Payload)
	return err
}

func (s *Store) UpdateTaskStatus(id int64, status string, progress int, message string) error {
	now := time.Now()
	_, err := s.db.Exec(`UPDATE tasks SET status=?, progress=?, message=?, started_at=CASE WHEN started_at IS NULL AND ?='running' THEN ? ELSE started_at END, finished_at=CASE WHEN ? IN ('completed','failed') THEN ? ELSE finished_at END WHERE id=?`,
		status, progress, message, status, now, status, now, id)
	return err
}

func (s *Store) ListTasks() ([]Task, error) {
	rows, err := s.db.Query(`SELECT id, tenant_id, type, status, progress, message, payload, coalesce(result,''), created_at, started_at, finished_at FROM tasks ORDER BY id DESC LIMIT 200`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Task
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.TenantID, &t.Type, &t.Status, &t.Progress, &t.Message, &t.Payload, &t.Result, &t.CreatedAt, &t.StartedAt, &t.FinishedAt); err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, rows.Err()
}

// Audit

func (s *Store) AddAudit(log *AuditLog) error {
	_, err := s.db.Exec(`INSERT INTO audit_logs (tenant_id, action, detail, ip) VALUES (?,?,?,?)`, log.TenantID, log.Action, log.Detail, log.IP)
	return err
}

func (s *Store) ListAudit(limit int) ([]AuditLog, error) {
	rows, err := s.db.Query(`SELECT id, tenant_id, action, detail, ip, created_at FROM audit_logs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []AuditLog
	for rows.Next() {
		var l AuditLog
		if err := rows.Scan(&l.ID, &l.TenantID, &l.Action, &l.Detail, &l.IP, &l.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, l)
	}
	return list, rows.Err()
}

func (s *Store) ListAuditPaginated(keyword string, page, size int) ([]AuditLog, int64, error) {
	kw := "%" + keyword + "%"
	var total int64
	s.db.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE action LIKE ? OR detail LIKE ?`, kw, kw).Scan(&total)

	offset := (page - 1) * size
	rows, err := s.db.Query(`SELECT id, tenant_id, action, detail, ip, created_at FROM audit_logs
		WHERE action LIKE ? OR detail LIKE ?
		ORDER BY id DESC LIMIT ? OFFSET ?`,
		kw, kw, size, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var list []AuditLog
	for rows.Next() {
		var l AuditLog
		if err := rows.Scan(&l.ID, &l.TenantID, &l.Action, &l.Detail, &l.IP, &l.CreatedAt); err != nil {
			return nil, 0, err
		}
		list = append(list, l)
	}
	return list, total, rows.Err()
}

// ClearAllTx removes all data within a transaction (for restore)
func (s *Store) ClearAllTx(tx *sql.Tx) error {
	if _, err := tx.Exec(`DELETE FROM instances`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM tenants`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM config`); err != nil {
		return err
	}
	return nil
}

// ClearAll removes all data using a transaction (for restore)
func (s *Store) ClearAll() error {
	tx, err := s.BeginTx()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()
	if err := s.ClearAllTx(tx); err != nil {
		return err
	}
	return tx.Commit()
}

// CreateTenantImportTx inserts a tenant within a transaction.
func (s *Store) CreateTenantImportTx(tx *sql.Tx, name, userOCID, tenancyOCID, region, fingerprint, keyFile string) error {
	_, err := tx.Exec(
		`INSERT INTO tenants (name, user_ocid, tenancy_ocid, region, fingerprint, key_file)
		 VALUES (?,?,?,?,?,?)`,
		name, userOCID, tenancyOCID, region, fingerprint, keyFile)
	return err
}

// UpsertInstanceImportTx upserts an instance within a transaction.
func (s *Store) UpsertInstanceImportTx(tx *sql.Tx, id string, tenantID int64, name, ocid, shape, state, publicIP, privateIP string, ocpu, memoryGB float64, bootVolumeGB int64) error {
	_, err := tx.Exec(
		`INSERT INTO instances (id, tenant_id, name, ocid, shape, ocpu, memory_gb, boot_volume_gb, public_ip, private_ip, state, synced_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,CURRENT_TIMESTAMP)
		 ON CONFLICT(id) DO UPDATE SET
		   name=excluded.name, shape=excluded.shape, ocpu=excluded.ocpu, memory_gb=excluded.memory_gb,
		   state=excluded.state, public_ip=excluded.public_ip, private_ip=excluded.private_ip,
		   synced_at=CURRENT_TIMESTAMP`,
		id, tenantID, name, ocid, shape, ocpu, memoryGB, bootVolumeGB, publicIP, privateIP, state)
	return err
}

// SetConfigTx sets a config key-value pair within a transaction.
func (s *Store) SetConfigTx(tx *sql.Tx, key, value string) error {
	_, err := tx.Exec(`INSERT INTO config (key, value) VALUES (?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

// Config

func (s *Store) ListAllConfig() ([]ConfigKV, error) {
	rows, err := s.db.Query(`SELECT key, value FROM config ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []ConfigKV
	for rows.Next() {
		var c ConfigKV
		if err := rows.Scan(&c.Key, &c.Value); err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

func (s *Store) GetConfig(key string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM config WHERE key=?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return v, err
}

func (s *Store) SetConfig(key, value string) error {
	_, err := s.db.Exec(`INSERT INTO config (key, value) VALUES (?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

func (s *Store) ListInstancesPaginated(tenantID int64, keyword string, page, size int) ([]Instance, int64, error) {
	kw := "%" + keyword + "%"
	var total int64
	s.db.QueryRow(`SELECT COUNT(*) FROM instances WHERE (tenant_id=? OR ?=0) AND (name LIKE ? OR ocid LIKE ? OR public_ip LIKE ?)`,
		tenantID, tenantID, kw, kw, kw).Scan(&total)

	offset := (page - 1) * size
	rows, err := s.db.Query(`SELECT id, tenant_id, name, ocid, shape, ocpu, memory_gb, boot_volume_gb,
		public_ip, private_ip, state, availability_domain, fault_domain, image_id, subnet_id,
		created_at, synced_at FROM instances
		WHERE (tenant_id=? OR ?=0) AND (name LIKE ? OR ocid LIKE ? OR public_ip LIKE ?)
		ORDER BY created_at DESC LIMIT ? OFFSET ?`,
		tenantID, tenantID, kw, kw, kw, size, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var list []Instance
	for rows.Next() {
		var i Instance
		if err := rows.Scan(&i.ID, &i.TenantID, &i.Name, &i.OCID, &i.Shape, &i.OCPU, &i.MemoryGB, &i.BootVolumeGB,
			&i.PublicIP, &i.PrivateIP, &i.State, &i.AvailabilityDomain, &i.FaultDomain, &i.ImageID, &i.SubnetID,
			&i.CreatedAt, &i.SyncedAt); err != nil {
			return nil, 0, err
		}
		list = append(list, i)
	}
	return list, total, rows.Err()
}

func (s *Store) ListTenantsPaginated(keyword string, page, size int) ([]Tenant, int64, error) {
	kw := "%" + keyword + "%"
	var total int64
	s.db.QueryRow(`SELECT COUNT(*) FROM tenants WHERE name LIKE ? OR region LIKE ?`, kw, kw).Scan(&total)

	offset := (page - 1) * size
	rows, err := s.db.Query(`SELECT id, name, user_ocid, tenancy_ocid, region, fingerprint, key_file,
		status, coalesce(home_region,''), coalesce(subscribed,''), created_at, updated_at FROM tenants
		WHERE name LIKE ? OR region LIKE ?
		ORDER BY id DESC LIMIT ? OFFSET ?`,
		kw, kw, size, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var list []Tenant
	for rows.Next() {
		var t Tenant
		if err := rows.Scan(&t.ID, &t.Name, &t.UserOCID, &t.TenancyOCID, &t.Region, &t.Fingerprint, &t.KeyFile,
			&t.Status, &t.HomeRegion, &t.Subscribed, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, 0, err
		}
		list = append(list, t)
	}
	return list, total, rows.Err()
}

func (s *Store) ListTasksPaginated(keyword string, page, size int) ([]Task, int64, error) {
	kw := "%" + keyword + "%"
	var total int64
	s.db.QueryRow(`SELECT COUNT(*) FROM tasks WHERE type LIKE ? OR message LIKE ?`, kw, kw).Scan(&total)

	offset := (page - 1) * size
	rows, err := s.db.Query(`SELECT id, tenant_id, type, status, progress, message, payload,
		coalesce(result,''), created_at, started_at, finished_at FROM tasks
		WHERE type LIKE ? OR message LIKE ?
		ORDER BY id DESC LIMIT ? OFFSET ?`,
		kw, kw, size, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var list []Task
	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.TenantID, &t.Type, &t.Status, &t.Progress, &t.Message,
			&t.Payload, &t.Result, &t.CreatedAt, &t.StartedAt, &t.FinishedAt); err != nil {
			return nil, 0, err
		}
		list = append(list, t)
	}
	return list, total, rows.Err()
}

func (s *Store) UpdateTaskPayload(id int64, payload string) error {
	_, err := s.db.Exec(`UPDATE tasks SET payload=? WHERE id=?`, payload, id)
	return err
}

// ResetRunningTasks sets all "running" tasks back to "pending" so they are retried on restart.
// This implements checkpoint-resume: tasks that were interrupted by a server restart
// will be picked up again by the worker.
func (s *Store) ResetRunningTasks() (int64, error) {
	res, err := s.db.Exec(`UPDATE tasks SET status='pending', progress=0, message='restarting after server reboot' WHERE status='running'`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ── CfCfg ──────────────────────────────────────────────────────────────

func (s *Store) ListCfCfgs() ([]CfCfg, error) {
	rows, err := s.db.Query(`SELECT id, name, token, email, api_key, zone_id, enabled, created_at, updated_at FROM cf_configs ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []CfCfg
	for rows.Next() {
		var c CfCfg
		if err := rows.Scan(&c.ID, &c.Name, &c.Token, &c.Email, &c.APIKey, &c.ZoneID, &c.Enabled, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, rows.Err()
}

func (s *Store) GetCfCfg(id int64) (*CfCfg, error) {
	var c CfCfg
	err := s.db.QueryRow(`SELECT id, name, token, email, api_key, zone_id, enabled, created_at, updated_at FROM cf_configs WHERE id=?`, id).
		Scan(&c.ID, &c.Name, &c.Token, &c.Email, &c.APIKey, &c.ZoneID, &c.Enabled, &c.CreatedAt, &c.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &c, err
}

func (s *Store) CreateCfCfg(c *CfCfg) error {
	_, err := s.db.Exec(`INSERT INTO cf_configs (name, token, email, api_key, zone_id, enabled) VALUES (?,?,?,?,?,?)`,
		c.Name, c.Token, c.Email, c.APIKey, c.ZoneID, c.Enabled)
	return err
}

func (s *Store) UpdateCfCfg(c *CfCfg) error {
	_, err := s.db.Exec(`UPDATE cf_configs SET name=?, token=?, email=?, api_key=?, zone_id=?, enabled=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`,
		c.Name, c.Token, c.Email, c.APIKey, c.ZoneID, c.Enabled, c.ID)
	return err
}

func (s *Store) DeleteCfCfg(id int64) error {
	_, err := s.db.Exec(`DELETE FROM cf_configs WHERE id=?`, id)
	return err
}

// ── IpData ─────────────────────────────────────────────────────────────

func (s *Store) ListIpData(tenantID int64, dataType string) ([]IpData, error) {
	q := `SELECT id, tenant_id, cidr, label, type, enabled, created_at FROM ip_data WHERE (tenant_id=? OR ?=0)`
	args := []interface{}{tenantID, tenantID}
	if dataType != "" {
		q += ` AND type=?`
		args = append(args, dataType)
	}
	q += ` ORDER BY id DESC`
	rows, err := s.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []IpData
	for rows.Next() {
		var d IpData
		if err := rows.Scan(&d.ID, &d.TenantID, &d.CIDR, &d.Label, &d.Type, &d.Enabled, &d.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, d)
	}
	return list, rows.Err()
}

func (s *Store) CreateIpData(d *IpData) error {
	_, err := s.db.Exec(`INSERT INTO ip_data (tenant_id, cidr, label, type, enabled) VALUES (?,?,?,?,?)`,
		d.TenantID, d.CIDR, d.Label, d.Type, d.Enabled)
	return err
}

func (s *Store) UpdateIpData(d *IpData) error {
	_, err := s.db.Exec(`UPDATE ip_data SET cidr=?, label=?, type=?, enabled=? WHERE id=?`,
		d.CIDR, d.Label, d.Type, d.Enabled, d.ID)
	return err
}

func (s *Store) DeleteIpData(id int64) error {
	_, err := s.db.Exec(`DELETE FROM ip_data WHERE id=?`, id)
	return err
}

// ── SSH Keys ───────────────────────────────────────────────────────────

func (s *Store) ListSSHKeys(tenantID int64) ([]SSHKey, error) {
	rows, err := s.db.Query(`SELECT id, name, public_key, fingerprint, tenant_id, created_at FROM ssh_keys WHERE (tenant_id=? OR ?=0) ORDER BY id DESC`,
		tenantID, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []SSHKey
	for rows.Next() {
		var k SSHKey
		if err := rows.Scan(&k.ID, &k.Name, &k.PublicKey, &k.Fingerprint, &k.TenantID, &k.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, k)
	}
	return list, rows.Err()
}

func (s *Store) GetSSHKeyByID(id int64) (*SSHKey, error) {
	k := &SSHKey{}
	err := s.db.QueryRow(
		`SELECT id, name, public_key, COALESCE(private_key,''), fingerprint, COALESCE(tenant_id,0), created_at FROM ssh_keys WHERE id=?`,
		id,
	).Scan(&k.ID, &k.Name, &k.PublicKey, &k.PrivateKey, &k.Fingerprint, &k.TenantID, &k.CreatedAt)
	if err != nil {
		return nil, err
	}
	return k, nil
}

func (s *Store) CreateSSHKey(k *SSHKey) error {
	_, err := s.db.Exec(`INSERT INTO ssh_keys (name, public_key, private_key, fingerprint, tenant_id) VALUES (?,?,?,?,?)`,
		k.Name, k.PublicKey, k.PrivateKey, k.Fingerprint, k.TenantID)
	return err
}

func (s *Store) DeleteSSHKey(id int64) error {
	_, err := s.db.Exec(`DELETE FROM ssh_keys WHERE id=?`, id)
	return err
}

// ── Instance Plans ─────────────────────────────────────────────────────

func (s *Store) ListInstancePlans(tenantID int64) ([]InstancePlan, error) {
	rows, err := s.db.Query(`SELECT id, name, tenant_id, shape, image_id, subnet_id, availability_domain, boot_volume_size_gb, ocpus, memory_gb, created_at FROM instance_plans WHERE (tenant_id=? OR ?=0) ORDER BY id DESC`,
		tenantID, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []InstancePlan
	for rows.Next() {
		var p InstancePlan
		if err := rows.Scan(&p.ID, &p.Name, &p.TenantID, &p.Shape, &p.ImageID, &p.SubnetID, &p.AvailabilityDomain, &p.BootVolumeSizeGB, &p.OCPUs, &p.MemoryGB, &p.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, rows.Err()
}

func (s *Store) CreateInstancePlan(p *InstancePlan) error {
	_, err := s.db.Exec(`INSERT INTO instance_plans (name, tenant_id, shape, image_id, subnet_id, availability_domain, boot_volume_size_gb, ocpus, memory_gb) VALUES (?,?,?,?,?,?,?,?,?)`,
		p.Name, p.TenantID, p.Shape, p.ImageID, p.SubnetID, p.AvailabilityDomain, p.BootVolumeSizeGB, p.OCPUs, p.MemoryGB)
	return err
}

func (s *Store) UpdateInstancePlan(p *InstancePlan) error {
	_, err := s.db.Exec(`UPDATE instance_plans SET name=?, shape=?, image_id=?, subnet_id=?, availability_domain=?, boot_volume_size_gb=?, ocpus=?, memory_gb=? WHERE id=?`,
		p.Name, p.Shape, p.ImageID, p.SubnetID, p.AvailabilityDomain, p.BootVolumeSizeGB, p.OCPUs, p.MemoryGB, p.ID)
	return err
}

func (s *Store) DeleteInstancePlan(id int64) error {
	_, err := s.db.Exec(`DELETE FROM instance_plans WHERE id=?`, id)
	return err
}

// ── Users ──────────────────────────────────────────────────────────────

func (s *Store) ListUsers() ([]User, error) {
	rows, err := s.db.Query(`SELECT id, username, role, mfa_enabled, email, created_at, updated_at FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &u.MFAEnabled, &u.Email, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, u)
	}
	return list, rows.Err()
}

func (s *Store) GetUserByUsername(username string) (*User, error) {
	var u User
	err := s.db.QueryRow(`SELECT id, username, password_hash, role, mfa_enabled, mfa_secret, email, created_at, updated_at FROM users WHERE username=?`, username).
		Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.MFAEnabled, &u.MFASecret, &u.Email, &u.CreatedAt, &u.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &u, err
}

func (s *Store) CreateUser(u *User) error {
	_, err := s.db.Exec(`INSERT INTO users (username, password_hash, role, email) VALUES (?,?,?,?)`,
		u.Username, u.PasswordHash, u.Role, u.Email)
	return err
}

func (s *Store) UpdateUserPassword(id int64, hash string) error {
	_, err := s.db.Exec(`UPDATE users SET password_hash=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`, hash, id)
	return err
}

func (s *Store) UpdateUserMFA(id int64, secret string, enabled bool) error {
	_, err := s.db.Exec(`UPDATE users SET mfa_secret=?, mfa_enabled=?, updated_at=CURRENT_TIMESTAMP WHERE id=?`, secret, enabled, id)
	return err
}

func (s *Store) DeleteUser(id int64) error {
	_, err := s.db.Exec(`DELETE FROM users WHERE id=?`, id)
	return err
}
