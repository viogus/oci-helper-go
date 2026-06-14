package db

import "time"

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

type AuditLog struct {
	ID        int64     `json:"id"`
	TenantID  int64     `json:"tenantId,omitempty"`
	Action    string    `json:"action"`
	Detail    string    `json:"detail"`
	IP        string    `json:"ip"`
	CreatedAt time.Time `json:"createdAt"`
}

type ConfigKV struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}
