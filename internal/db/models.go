package db

import "time"

// Tenant represents an OCI tenant / tenancy configuration.
type Tenant struct {
	ID            int64     `json:"id"`
	Name          string    `json:"name"`
	UserOCID      string    `json:"userOcid"`
	TenancyOCID   string    `json:"tenancyOcid"`
	Region        string    `json:"region"`
	Fingerprint   string    `json:"fingerprint"`
	KeyFile       string    `json:"keyFile"`
	Status        string    `json:"status"`
	HomeRegion    string    `json:"homeRegion,omitempty"`
	Subscribed    string    `json:"subscribed,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// Instance represents a single OCI compute instance, keyed by composite
// ID (tenantID:ocid). Synced by the sync worker.
type Instance struct {
	ID            string    `json:"id"`
	TenantID      int64     `json:"tenantId"`
	Name          string    `json:"name"`
	OCID          string    `json:"ocid"`
	Shape         string    `json:"shape"`
	OCPU          float64   `json:"ocpu"`
	MemoryGB      float64   `json:"memoryGB"`
	BootVolumeGB  int64     `json:"bootVolumeGB"`
	PublicIP      string    `json:"publicIp"`
	PrivateIP     string    `json:"privateIp"`
	State         string    `json:"state"`
	AvailabilityDomain string `json:"availabilityDomain"`
	FaultDomain   string    `json:"faultDomain"`
	ImageID       string    `json:"imageId"`
	SubnetID      string    `json:"subnetId"`
	CreatedAt     time.Time `json:"createdAt"`
	SyncedAt      time.Time `json:"syncedAt"`
}

// Task represents an asynchronous background operation (batch create, batch start, etc.).
type Task struct {
	ID          int64     `json:"id"`
	TenantID    int64     `json:"tenantId"`
	Type        string    `json:"type"`
	Status      string    `json:"status"`
	Progress    int       `json:"progress"`
	Message     string    `json:"message"`
	Payload     string    `json:"payload"`
	Result      string    `json:"result,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	StartedAt   *time.Time `json:"startedAt,omitempty"`
	FinishedAt  *time.Time `json:"finishedAt,omitempty"`
}

// AuditLog records an auditable action performed through the panel.
type AuditLog struct {
	ID        int64     `json:"id"`
	TenantID  int64     `json:"tenantId,omitempty"`
	Action    string    `json:"action"`
	Detail    string    `json:"detail"`
	IP        string    `json:"ip"`
	CreatedAt time.Time `json:"createdAt"`
}

// ConfigKV is a simple key-value store for runtime config (MFA secrets, API keys, etc.).
type ConfigKV struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// CfCfg stores a named Cloudflare configuration (token, zone, API key).
type CfCfg struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Token     string    `json:"token"`
	Email     string    `json:"email"`
	APIKey    string    `json:"apiKey"`
	ZoneID    string    `json:"zoneId"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// IpData stores IP address data (CIDR ranges) used for IP management and filtering.
type IpData struct {
	ID        int64     `json:"id"`
	TenantID  int64     `json:"tenantId"`
	CIDR      string    `json:"cidr"`
	Label     string    `json:"label"`
	Type      string    `json:"type"`
	Enabled   bool      `json:"enabled"`
	// Geolocation fields populated via IP lookup on creation.
	Lat       float64   `json:"lat,omitempty"`
	Lng       float64   `json:"lng,omitempty"`
	Country   string    `json:"country,omitempty"`
	Area      string    `json:"area,omitempty"`
	City      string    `json:"city,omitempty"`
	Org       string    `json:"org,omitempty"`
	Asn       string    `json:"asn,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

// SSHKey stores SSH public keys (and optionally private keys) for console connections.
type SSHKey struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	PublicKey   string    `json:"publicKey"`
	PrivateKey  string    `json:"privateKey,omitempty"`
	Fingerprint string    `json:"fingerprint"`
	TenantID    int64     `json:"tenantId,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
}

// InstancePlan is a saved launch template with predefined shape, image, subnet,
// and resource sizing for quick instance creation.
type InstancePlan struct {
	ID                int64     `json:"id"`
	Name              string    `json:"name"`
	TenantID          int64     `json:"tenantId"`
	Shape             string    `json:"shape"`
	ImageID           string    `json:"imageId"`
	SubnetID          string    `json:"subnetId"`
	AvailabilityDomain string   `json:"availabilityDomain"`
	BootVolumeSizeGB  int64     `json:"bootVolumeSizeGB"`
	OCPUs             float64   `json:"ocpus"`
	MemoryGB          float64   `json:"memoryGB"`
	CreatedAt         time.Time `json:"createdAt"`
}

// User represents an authenticated panel user with role-based access.
type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"`
	MFAEnabled   bool      `json:"mfaEnabled"`
	MFASecret    string    `json:"-"`
	Email        string    `json:"email"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}
