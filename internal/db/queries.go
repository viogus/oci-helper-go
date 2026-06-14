package db

import (
	"database/sql"
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

// Import helpers (no auto-increment)

func (s *Store) CreateTenantImport(name, userOCID, tenancyOCID, region, fingerprint, keyFile string) error {
	_, err := s.db.Exec(
		`INSERT INTO tenants (name, user_ocid, tenancy_ocid, region, fingerprint, key_file)
		 VALUES (?,?,?,?,?,?)`,
		name, userOCID, tenancyOCID, region, fingerprint, keyFile)
	return err
}

func (s *Store) UpsertInstanceImport(id string, tenantID int64, name, ocid, shape, state, publicIP, privateIP string, ocpu, memoryGB float64, bootVolumeGB int64) error {
	_, err := s.db.Exec(
		`INSERT INTO instances (id, tenant_id, name, ocid, shape, ocpu, memory_gb, boot_volume_gb, public_ip, private_ip, state, synced_at)
		 VALUES (?,?,?,?,?,?,?,?,?,?,?,CURRENT_TIMESTAMP)
		 ON CONFLICT(id) DO UPDATE SET
		   name=excluded.name, shape=excluded.shape, ocpu=excluded.ocpu, memory_gb=excluded.memory_gb,
		   state=excluded.state, public_ip=excluded.public_ip, private_ip=excluded.private_ip,
		   synced_at=CURRENT_TIMESTAMP`,
		id, tenantID, name, ocid, shape, ocpu, memoryGB, bootVolumeGB, publicIP, privateIP, state)
	return err
}

// Config

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
