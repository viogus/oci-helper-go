package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"
	"log"
	"github.com/viogus/oci-helper-go/internal/db"
)

func (s *Server) handleInstances(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		tenantID, _ := strconv.ParseInt(r.URL.Query().Get("tenant_id"), 10, 64)
		keyword := r.URL.Query().Get("keyword")
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		size, _ := strconv.Atoi(r.URL.Query().Get("size"))
		if size < 1 {
			size = 20
		}
		list, total, err := s.store.ListInstancesPaginated(tenantID, keyword, page, size)
		if err != nil {
			jsonErr(w, "list instances: "+err.Error())
			return
		}
		if list == nil {
			list = []db.Instance{}
		}
		jsonOK(w, map[string]interface{}{"data": list, "total": total, "page": page, "size": size})
	case http.MethodPost:
		s.createInstance(w, r)
	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) createInstance(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TenantID            int64   `json:"tenantId"`
		DisplayName         string  `json:"displayName"`
		ImageID             string  `json:"imageId"`
		Shape               string  `json:"shape"`
		SubnetID            string  `json:"subnetId"`
		AvailabilityDomain  string  `json:"availabilityDomain"`
		BootVolumeSizeGB    *int64  `json:"bootVolumeSizeGB"`
		OCPUs               *float32 `json:"ocpus"`
		MemoryGB            *float32 `json:"memoryGB"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}

	t, err := s.store.GetTenant(req.TenantID)
	if err != nil || t == nil {
		jsonErr(w, "tenant not found")
		return
	}

	client, err := s.clientFor(t)
	if err != nil {
		jsonErr(w, "oci client: "+err.Error())
		return
	}

	launchReq := core.LaunchInstanceRequest{
		LaunchInstanceDetails: core.LaunchInstanceDetails{
			CompartmentId:      common.String(t.TenancyOCID),
			AvailabilityDomain: common.String(req.AvailabilityDomain),
			DisplayName:        common.String(req.DisplayName),
			ImageId:            common.String(req.ImageID),
			Shape:              common.String(req.Shape),
			SubnetId:           common.String(req.SubnetID),
			CreateVnicDetails: &core.CreateVnicDetails{
				SubnetId: common.String(req.SubnetID),
			},
			SourceDetails: core.InstanceSourceViaImageDetails{
				ImageId: common.String(req.ImageID),
			},
		},
	}
	if req.BootVolumeSizeGB != nil {
		launchReq.LaunchInstanceDetails.SourceDetails = core.InstanceSourceViaImageDetails{
			ImageId:           common.String(req.ImageID),
			BootVolumeSizeInGBs: req.BootVolumeSizeGB,
		}
	}
	if req.OCPUs != nil {
		launchReq.LaunchInstanceDetails.ShapeConfig = &core.LaunchInstanceShapeConfigDetails{
			Ocpus: req.OCPUs,
		}
	}
	if req.MemoryGB != nil {
		if launchReq.LaunchInstanceDetails.ShapeConfig == nil {
			launchReq.LaunchInstanceDetails.ShapeConfig = &core.LaunchInstanceShapeConfigDetails{}
		}
		launchReq.LaunchInstanceDetails.ShapeConfig.MemoryInGBs = req.MemoryGB
	}

	inst, err := client.LaunchInstanceWithRequest(r.Context(), launchReq)
	if err != nil {
		jsonErr(w, "launch: "+err.Error())
		return
	}

	s.store.UpsertInstance(&db.Instance{
		ID:       strOr(inst.Id, ""),
		TenantID: req.TenantID,
		Name:     strOr(inst.DisplayName, ""),
		OCID:     strOr(inst.Id, ""),
		Shape:    strOr(inst.Shape, ""),
		State:    string(inst.LifecycleState),
	})
	s.audit(req.TenantID, "instance:create", strOr(inst.DisplayName, ""), r)
	jsonOK(w, map[string]string{"status": "ok", "instanceId": strOr(inst.Id, "")})
}

func (s *Server) handleInstanceAction(w http.ResponseWriter, r *http.Request) {
	// /api/instances/{id} or /api/instances/{id}/action
	path := strings.TrimPrefix(r.URL.Path, "/api/instances/")
	path = strings.TrimSuffix(path, "/")
	parts := strings.SplitN(path, "/", 2)
	instanceID := parts[0]

	// GET: return instance detail from DB
	if r.Method == http.MethodGet {
		inst, err := s.store.GetInstanceByID(instanceID)
		if err != nil || inst == nil {
			jsonErr(w, "instance not found")
			return
		}
		jsonOK(w, inst)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// Accept both /api/instances/{id} and /api/instances/{id}/action
	// Frontend sends POST /api/instances/{id} with {action: "..."} body.
	_ = parts // instanceID already extracted above

	var req struct {
		Action   string `json:"action"`
		TenantID int64  `json:"tenantId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}

	t, err := s.store.GetTenant(req.TenantID)
	if err != nil || t == nil {
		jsonErr(w, "tenant not found")
		return
	}

	client, err := s.clientFor(t)
	if err != nil {
		jsonErr(w, "oci client: "+err.Error())
		return
	}

	ctx := r.Context()
	switch req.Action {
	case "terminate":
		if err := client.TerminateInstance(ctx, instanceID); err != nil {
			jsonErr(w, "terminate: "+err.Error())
			return
		}
		s.store.UpsertInstance(&db.Instance{ID: instanceID, TenantID: req.TenantID, State: "TERMINATING"})
	case "start":
		_, err := client.InstanceAction(ctx, instanceID, core.InstanceActionActionStart)
		if err != nil {
			jsonErr(w, "start: "+err.Error())
			return
		}
		s.store.UpsertInstance(&db.Instance{ID: instanceID, TenantID: req.TenantID, State: "STARTING"})
	case "stop":
		_, err := client.InstanceAction(ctx, instanceID, core.InstanceActionActionStop)
		if err != nil {
			jsonErr(w, "stop: "+err.Error())
			return
		}
		s.store.UpsertInstance(&db.Instance{ID: instanceID, TenantID: req.TenantID, State: "STOPPING"})
	case "reboot":
		_, err := client.InstanceAction(ctx, instanceID, core.InstanceActionActionReset)
		if err != nil {
			jsonErr(w, "reboot: "+err.Error())
			return
		}
		s.store.UpsertInstance(&db.Instance{ID: instanceID, TenantID: req.TenantID, State: "STARTING"})
	case "softstop":
		_, err := client.InstanceAction(ctx, instanceID, core.InstanceActionActionSoftstop)
		if err != nil {
			jsonErr(w, "softstop: "+err.Error())
			return
		}
		s.store.UpsertInstance(&db.Instance{ID: instanceID, TenantID: req.TenantID, State: "STOPPING"})
	case "softreset":
		_, err := client.InstanceAction(ctx, instanceID, core.InstanceActionActionSoftreset)
		if err != nil {
			jsonErr(w, "softreset: "+err.Error())
			return
		}
		s.store.UpsertInstance(&db.Instance{ID: instanceID, TenantID: req.TenantID, State: "STARTING"})
	default:
		jsonErr(w, "unknown action: "+req.Action+". use start|stop|reboot|softstop|softreset|terminate")
		return
	}

	s.audit(req.TenantID, "instance:"+req.Action, instanceID, r)
	jsonOK(w, map[string]string{"status": "ok", "action": req.Action, "instanceId": instanceID})
}

// --- batch start ---

func (s *Server) handleBatchStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		TenantID    int64    `json:"tenantId"`
		InstanceIDs []string `json:"instanceIds"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}
	if req.TenantID == 0 || len(req.InstanceIDs) == 0 {
		jsonErr(w, "tenantId and instanceIds required")
		return
	}
	payload, _ := json.Marshal(req)
	task := &db.Task{
		TenantID: req.TenantID,
		Type:     "batch_start",
		Status:   "pending",
		Payload:  string(payload),
	}
	if err := s.store.CreateTask(task); err != nil {
		jsonErr(w, "create task: "+err.Error())
		return
	}
	s.audit(req.TenantID, "batch:start", fmt.Sprintf("%d instances", len(req.InstanceIDs)), r)
	jsonOK(w, task)
}

// --- tasks ---

// --- sync ---

func (s *Server) handleSync(w http.ResponseWriter, r *http.Request) {
	tenantIDStr := strings.TrimPrefix(r.URL.Path, "/api/sync/")
	tenantID, _ := strconv.ParseInt(tenantIDStr, 10, 64)

	t, err := s.store.GetTenant(tenantID)
	if err != nil || t == nil {
		jsonErr(w, "tenant not found")
		return
	}
	client, err := s.clientFor(t)
	if err != nil {
		jsonErr(w, "oci client: "+err.Error())
		return
	}
	instances, err := client.ListInstances(r.Context(), t.TenancyOCID)
	if err != nil {
		jsonErr(w, "list instances: "+err.Error())
		return
	}
	for _, inst := range instances {
		if err := s.store.UpsertInstance(ociToDB(inst, tenantID)); err != nil {
			log.Printf("[sync] upsert %s: %v", strOr(inst.Id, ""), err)
		}
	}

	// Best-effort VNIC sync for public IP / private IP / subnet
	s.syncVNICs(r.Context(), tenantID)

	s.audit(tenantID, "sync", fmt.Sprintf("synced %d instances", len(instances)), r)
	jsonOK(w, map[string]int{"count": len(instances)})
}

func ociToDB(i core.Instance, tenantID int64) *db.Instance {
	var ocpu, memGB float64
	var bootVolGB int64
	var imageID, ad, fd string
	if i.ShapeConfig != nil {
		if i.ShapeConfig.Ocpus != nil {
			ocpu = float64(*i.ShapeConfig.Ocpus)
		}
		if i.ShapeConfig.MemoryInGBs != nil {
			memGB = float64(*i.ShapeConfig.MemoryInGBs)
		}
	}
	if i.ImageId != nil {
		imageID = *i.ImageId
	}
	if sd, ok := i.SourceDetails.(core.InstanceSourceViaImageDetails); ok {
		if sd.ImageId != nil {
			imageID = *sd.ImageId
		}
		if sd.BootVolumeSizeInGBs != nil {
			bootVolGB = *sd.BootVolumeSizeInGBs
		}
	}
	if i.AvailabilityDomain != nil {
		ad = *i.AvailabilityDomain
	}
	if i.FaultDomain != nil {
		fd = *i.FaultDomain
	}
	return &db.Instance{
		ID:       fmt.Sprintf("%d:%s", tenantID, strOr(i.Id, "")),
		TenantID: tenantID,
		Name:     strOr(i.DisplayName, ""),
		OCID:     strOr(i.Id, ""),
		Shape:    strOr(i.Shape, ""),
		State:    string(i.LifecycleState),
		OCPU:       ocpu,
		MemoryGB:   memGB,
		BootVolumeGB: bootVolGB,
		ImageID:    imageID,
		AvailabilityDomain: ad,
		FaultDomain: fd,
	}
}

func (s *Server) syncVNICs(ctx context.Context, tenantID int64) {
	instances, err := s.store.ListInstances(tenantID)
	if err != nil {
		log.Printf("[syncVnics] list instances: %v", err)
		return
	}
	for _, inst := range instances {
		parts := strings.SplitN(inst.OCID, ":", 2)
		ocid := parts[len(parts)-1]
		if ocid == "" {
			continue
		}
		tenant, err := s.store.GetTenant(tenantID)
		if err != nil || tenant == nil {
			continue
		}
		client, err := s.clientFor(tenant)
		if err != nil {
			continue
		}
		vnics, err := client.GetInstanceVNICs(ctx, tenant.TenancyOCID, ocid)
		if err != nil || len(vnics) == 0 {
			continue
		}
		vnic := vnics[0]
		pubIP := ""
		privIP := ""
		subnetID := ""
		if vnic.PublicIp != nil {
			pubIP = *vnic.PublicIp
		}
		if vnic.PrivateIp != nil {
			privIP = *vnic.PrivateIp
		}
		if vnic.SubnetId != nil {
			subnetID = *vnic.SubnetId
		}
		inst.PublicIP = pubIP
		inst.PrivateIP = privIP
		inst.SubnetID = subnetID
		s.store.UpsertInstance(&inst)
	}
}

func (s *Server) handleChangeShape(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		TenantID   int64   `json:"tenant_id"`
		InstanceID string  `json:"instance_id"`
		Shape      string  `json:"shape"`
		Ocpus      float32 `json:"ocpus"`
		MemoryGB   float32 `json:"memory_gb"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}
	tenant, err := s.store.GetTenant(req.TenantID)
	if err != nil || tenant == nil {
		jsonErr(w, "tenant not found")
		return
	}
	client, err := s.clientFor(tenant)
	if err != nil {
		jsonErr(w, "oci client: "+err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()
	if err := client.UpdateInstance(ctx, req.InstanceID, req.Shape, req.Ocpus, req.MemoryGB); err != nil {
		jsonErr(w, "update instance: "+err.Error())
		return
	}
	s.audit(req.TenantID, "instance:change-shape", req.InstanceID, r)
	jsonOK(w, map[string]string{"status": "ok"})
}
func (s *Server) handleChangeBootVolume(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		TenantID   int64  `json:"tenant_id"`
		InstanceID string `json:"instance_id"`
		SizeGB     int64  `json:"size_gb"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}
	tenant, err := s.store.GetTenant(req.TenantID)
	if err != nil || tenant == nil {
		jsonErr(w, "tenant not found")
		return
	}
	client, err := s.clientFor(tenant)
	if err != nil {
		jsonErr(w, "oci client: "+err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()
	attachment, err := client.GetBootVolumeAttachment(ctx, tenant.TenancyOCID, req.InstanceID)
	if err != nil {
		jsonErr(w, "get boot volume: "+err.Error())
		return
	}
	if _, err := client.UpdateBootVolume(ctx, *attachment.BootVolumeId, req.SizeGB, ""); err != nil {
		jsonErr(w, "update boot volume: "+err.Error())
		return
	}
	s.audit(req.TenantID, "instance:change-boot-volume", req.InstanceID, r)
	jsonOK(w, map[string]string{"status": "ok"})
}
func (s *Server) handleAttachIPv6(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		TenantID   int64  `json:"tenant_id"`
		InstanceID string `json:"instance_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}
	tenant, err := s.store.GetTenant(req.TenantID)
	if err != nil || tenant == nil {
		jsonErr(w, "tenant not found")
		return
	}
	client, err := s.clientFor(tenant)
	if err != nil {
		jsonErr(w, "oci client: "+err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()
	vnics, err := client.GetInstanceVNICs(ctx, tenant.TenancyOCID, req.InstanceID)
	if err != nil {
		jsonErr(w, "list vnics: "+err.Error())
		return
	}
	if len(vnics) == 0 {
		jsonErr(w, "no VNIC found for instance")
		return
	}
	if err := client.AssignIPv6(ctx, *vnics[0].Id); err != nil {
		jsonErr(w, "assign ipv6: "+err.Error())
		return
	}
	s.audit(req.TenantID, "instance:attach-ipv6", req.InstanceID, r)
	jsonOK(w, map[string]string{"status": "ok"})
}
func (s *Server) handleUpdateInstanceName(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		TenantID   int64  `json:"tenant_id"`
		InstanceID string `json:"instance_id"`
		Name       string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}
	tenant, err := s.store.GetTenant(req.TenantID)
	if err != nil || tenant == nil {
		jsonErr(w, "tenant not found")
		return
	}
	client, err := s.clientFor(tenant)
	if err != nil {
		jsonErr(w, "oci client: "+err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()
	if err := client.UpdateInstanceDisplayName(ctx, req.InstanceID, req.Name); err != nil {
		jsonErr(w, "update name: "+err.Error())
		return
	}
	s.audit(req.TenantID, "instance:update-name", req.InstanceID+" -> "+req.Name, r)
	jsonOK(w, map[string]string{"status": "ok"})
}
func (s *Server) handleChangeIP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		TenantID   int64    `json:"tenant_id"`
		InstanceID string   `json:"instance_id"`
		CidrList   []string `json:"cidr_list"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}
	tenant, err := s.store.GetTenant(req.TenantID)
	if err != nil || tenant == nil {
		jsonErr(w, "tenant not found")
		return
	}
	client, err := s.clientFor(tenant)
	if err != nil {
		jsonErr(w, "oci client: "+err.Error())
		return
	}
	// Try to change IP once (synchronous)
	newIP, err := client.ChangeInstanceIP(r.Context(), req.InstanceID, req.CidrList)
	if err != nil {
		jsonErr(w, "change ip: "+err.Error())
		return
	}
	s.audit(req.TenantID, "instance:change-ip", req.InstanceID+" → "+maskIP(newIP), r)
	jsonOK(w, map[string]string{"new_ip": newIP, "status": "ok"})
}
func (s *Server) handleCheckAlive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		TenantID    int64    `json:"tenant_id"`
		InstanceID  string   `json:"instance_id"`
		InstanceIDs []string `json:"instance_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}
	tenant, err := s.store.GetTenant(req.TenantID)
	if err != nil || tenant == nil {
		jsonErr(w, "tenant not found")
		return
	}

	type checkResult struct {
		InstanceID string `json:"instance_id"`
		Alive      bool   `json:"alive"`
		Error      string `json:"error,omitempty"`
	}

	ids := req.InstanceIDs
	if req.InstanceID != "" {
		ids = []string{req.InstanceID}
	}
	if len(ids) == 0 {
		jsonErr(w, "no instance IDs provided")
		return
	}

	var results []checkResult
	for _, id := range ids {
		inst, err := s.store.GetInstanceByID(fmt.Sprintf("%d:%s", req.TenantID, id))
		if err != nil || inst == nil {
			results = append(results, checkResult{InstanceID: id, Alive: false, Error: "instance not found in DB"})
			continue
		}
		if inst.PublicIP == "" {
			results = append(results, checkResult{InstanceID: id, Alive: false, Error: "no public IP"})
			continue
		}
		// TCP connect to port 22 (SSH) with timeout
		alive := checkTCPPort(inst.PublicIP, 22, 5*time.Second)
		results = append(results, checkResult{InstanceID: id, Alive: alive})
	}

	s.audit(req.TenantID, "instance:check-alive", strconv.Itoa(len(ids)), r)
	jsonOK(w, map[string]interface{}{"results": results})
}

// ── G10: Batch Check Alive ──────────────────────────────────────────────

func (s *Server) handleCheckAliveBatch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		TenantID int64 `json:"tenant_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}
	tenant, err := s.store.GetTenant(req.TenantID)
	if err != nil || tenant == nil {
		jsonErr(w, "tenant not found")
		return
	}

	instances, err := s.store.ListInstances(req.TenantID)
	if err != nil {
		jsonErr(w, "list instances: "+err.Error())
		return
	}

	// Filter to RUNNING instances only
	var running []db.Instance
	for _, inst := range instances {
		if inst.State == "RUNNING" {
			running = append(running, inst)
		}
	}

	type checkResult struct {
		InstanceID string `json:"instance_id"`
		Alive      bool   `json:"alive"`
		Error      string `json:"error,omitempty"`
	}

	var mu sync.Mutex
	var results []checkResult
	var wg sync.WaitGroup

	for _, inst := range running {
		wg.Add(1)
		go func(inst db.Instance) {
			defer wg.Done()
			if inst.PublicIP == "" {
				mu.Lock()
				results = append(results, checkResult{InstanceID: inst.OCID, Alive: false, Error: "no public IP"})
				mu.Unlock()
				return
			}
			alive := checkTCPPort(inst.PublicIP, 22, 5*time.Second)
			mu.Lock()
			results = append(results, checkResult{InstanceID: inst.OCID, Alive: alive})
			mu.Unlock()
		}(inst)
	}
	wg.Wait()

	s.audit(req.TenantID, "instance:check-alive-batch", strconv.Itoa(len(running)), r)
	jsonOK(w, map[string]interface{}{"results": results})
}

func (s *Server) handleOneClick500M(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		TenantID   int64  `json:"tenant_id"`
		InstanceID string `json:"instance_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}
	tenant, err := s.store.GetTenant(req.TenantID)
	if err != nil || tenant == nil {
		jsonErr(w, "tenant not found")
		return
	}
	client, err := s.clientFor(tenant)
	if err != nil {
		jsonErr(w, "oci client: "+err.Error())
		return
	}
	if err := client.Enable500Mbps(r.Context(), req.InstanceID); err != nil {
		jsonErr(w, "enable 500M: "+err.Error())
		return
	}
	s.audit(req.TenantID, "instance:500m-enable", req.InstanceID, r)
	jsonOK(w, map[string]string{"status": "ok"})
}

func (s *Server) handleOneClickClose500M(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		TenantID   int64  `json:"tenant_id"`
		InstanceID string `json:"instance_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}
	tenant, err := s.store.GetTenant(req.TenantID)
	if err != nil || tenant == nil {
		jsonErr(w, "tenant not found")
		return
	}
	client, err := s.clientFor(tenant)
	if err != nil {
		jsonErr(w, "oci client: "+err.Error())
		return
	}
	if err := client.Disable500Mbps(r.Context(), req.InstanceID); err != nil {
		jsonErr(w, "disable 500M: "+err.Error())
		return
	}
	s.audit(req.TenantID, "instance:500m-disable", req.InstanceID, r)
	jsonOK(w, map[string]string{"status": "ok"})
}
func (s *Server) handleAutoRescue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		TenantID   int64  `json:"tenant_id"`
		InstanceID string `json:"instance_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}
	tenant, err := s.store.GetTenant(req.TenantID)
	if err != nil || tenant == nil {
		jsonErr(w, "tenant not found")
		return
	}
	client, err := s.clientFor(tenant)
	if err != nil {
		jsonErr(w, "oci client: "+err.Error())
		return
	}
	inst, err := s.store.GetInstanceByID(fmt.Sprintf("%d:%s", req.TenantID, req.InstanceID))
	if err != nil || inst == nil {
		jsonErr(w, "instance not found in DB — sync first")
		return
	}
	if inst.PublicIP == "" {
		jsonErr(w, "instance has no public IP")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()

	type step struct {
		Action string `json:"action"`
		Alive  bool   `json:"alive"`
		Error  string `json:"error,omitempty"`
	}
	var steps []step
	finalAlive := false

	addStep := func(action string) bool {
		alive := checkTCPPort(inst.PublicIP, 22, 5*time.Second)
		steps = append(steps, step{Action: action, Alive: alive})
		return alive
	}

	// Step 1: TCP check (no action)
	if addStep("tcp_check") {
		finalAlive = true
		jsonOK(w, map[string]interface{}{"steps": steps, "final_alive": finalAlive})
		return
	}

	// Step 2: SOFTRESET
	if _, err := client.InstanceAction(ctx, req.InstanceID, core.InstanceActionActionSoftreset); err != nil {
		steps = append(steps, step{Action: "softreset", Alive: false, Error: err.Error()})
	} else {
		time.Sleep(30 * time.Second)
		if addStep("softreset") {
			finalAlive = true
			s.audit(req.TenantID, "instance:auto-rescue:softreset", req.InstanceID, r)
			jsonOK(w, map[string]interface{}{"steps": steps, "final_alive": finalAlive})
			return
		}
	}

	// Step 3: RESET
	if _, err := client.InstanceAction(ctx, req.InstanceID, core.InstanceActionActionReset); err != nil {
		steps = append(steps, step{Action: "reset", Alive: false, Error: err.Error()})
	} else {
		time.Sleep(60 * time.Second)
		if addStep("reset") {
			finalAlive = true
			s.audit(req.TenantID, "instance:auto-rescue:reset", req.InstanceID, r)
			jsonOK(w, map[string]interface{}{"steps": steps, "final_alive": finalAlive})
			return
		}
	}

	// Step 4: STOP then START
	if _, err := client.InstanceAction(ctx, req.InstanceID, core.InstanceActionActionStop); err != nil {
		steps = append(steps, step{Action: "stop", Alive: false, Error: err.Error()})
	} else {
		time.Sleep(30 * time.Second)
		if _, err := client.InstanceAction(ctx, req.InstanceID, core.InstanceActionActionStart); err != nil {
			steps = append(steps, step{Action: "start", Alive: false, Error: err.Error()})
		} else {
			time.Sleep(60 * time.Second)
			finalAlive = addStep("stop_start")
		}
	}

	s.audit(req.TenantID, "instance:auto-rescue", req.InstanceID, r)
	jsonOK(w, map[string]interface{}{"steps": steps, "final_alive": finalAlive})
}
// ── G6: Direct Instance Config Update ───────────────────────────────────

func (s *Server) handleInstanceConfigUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		TenantID    int64   `json:"tenant_id"`
		InstanceID  string  `json:"instance_id"`
		DisplayName string  `json:"display_name"`
		Shape       string  `json:"shape"`
		Ocpus       float32 `json:"ocpus"`
		MemoryGB    float32 `json:"memory_gb"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}
	tenant, err := s.store.GetTenant(req.TenantID)
	if err != nil || tenant == nil {
		jsonErr(w, "tenant not found")
		return
	}
	client, err := s.clientFor(tenant)
	if err != nil {
		jsonErr(w, "oci client: "+err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()
	if err := client.UpdateInstance(ctx, req.InstanceID, req.Shape, req.Ocpus, req.MemoryGB); err != nil {
		jsonErr(w, "update instance: "+err.Error())
		return
	}
	if req.DisplayName != "" {
		if err := client.UpdateInstanceDisplayName(ctx, req.InstanceID, req.DisplayName); err != nil {
			jsonErr(w, "update display name: "+err.Error())
			return
		}
	}
	s.audit(req.TenantID, "instance:config-update", req.InstanceID, r)
	jsonOK(w, map[string]string{"status": "ok"})
}

func (s *Server) handleUpdateShape(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		TenantID   int64  `json:"tenant_id"`
		InstanceID string `json:"instance_id"`
		Shape      string `json:"shape"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}
	tenant, err := s.store.GetTenant(req.TenantID)
	if err != nil || tenant == nil {
		jsonErr(w, "tenant not found")
		return
	}
	client, err := s.clientFor(tenant)
	if err != nil {
		jsonErr(w, "oci client: "+err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()
	if err := client.UpdateInstance(ctx, req.InstanceID, req.Shape, 0, 0); err != nil {
		jsonErr(w, "update shape: "+err.Error())
		return
	}
	s.audit(req.TenantID, "instance:update-shape", req.InstanceID, r)
	jsonOK(w, map[string]string{"status": "ok"})
}

// ── Start VNC / Console Connection ───────────────────────────────────

func (s *Server) handleStartVNC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		TenantID   int64  `json:"tenant_id"`
		InstanceID string `json:"instance_id"`
		SSHKeyID   int64  `json:"ssh_key_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}
	tenant, err := s.store.GetTenant(req.TenantID)
	if err != nil || tenant == nil {
		jsonErr(w, "tenant not found")
		return
	}
	client, err := s.clientFor(tenant)
	if err != nil {
		jsonErr(w, "oci client: "+err.Error())
		return
	}
	// Get SSH key for console connection
	sshKeys, err := s.store.ListSSHKeys(req.TenantID)
	if err != nil || len(sshKeys) == 0 {
		jsonErr(w, "no SSH keys found — upload or generate one first")
		return
	}
	var pubKey string
	if req.SSHKeyID > 0 {
		for _, k := range sshKeys {
			if k.ID == req.SSHKeyID {
				pubKey = k.PublicKey
				break
			}
		}
	} else {
		pubKey = sshKeys[0].PublicKey
	}
	if pubKey == "" {
		jsonErr(w, "SSH key not found")
		return
	}
	conn, err := client.CreateConsoleConnection(r.Context(), req.InstanceID, pubKey)
	if err != nil {
		jsonErr(w, "create console connection: "+err.Error())
		return
	}
	// Start polling in background for connection to become active
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		activeConn, err := client.WaitForConsoleConnectionActive(ctx, *conn.Id)
		if err != nil {
			log.Printf("[vnc] wait for active: %v", err)
			return
		}
		log.Printf("[vnc] console connection active: %s (vnc=%s ssh=%s)",
			*activeConn.Id, strOr(activeConn.VncConnectionString, ""), strOr(activeConn.ConnectionString, ""))
	}()
	s.audit(req.TenantID, "instance:vnc:start", req.InstanceID, r)
	jsonOK(w, map[string]interface{}{
		"status":             "creating",
		"connection_id":      strOr(conn.Id, ""),
		"connection_string":  strOr(conn.ConnectionString, ""),
		"vnc_connection_string": strOr(conn.VncConnectionString, ""),
		"fingerprint":        strOr(conn.Fingerprint, ""),
	})
}

// ── Instance Config Info ──────────────────────────────────────────────

func (s *Server) handleInstanceConfigInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		TenantID   int64  `json:"tenant_id"`
		InstanceID string `json:"instance_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}
	tenant, err := s.store.GetTenant(req.TenantID)
	if err != nil || tenant == nil {
		jsonErr(w, "tenant not found")
		return
	}
	client, err := s.clientFor(tenant)
	if err != nil {
		jsonErr(w, "oci client: "+err.Error())
		return
	}
	ctx := r.Context()
	// Get instance details
	inst, err := client.GetInstance(ctx, req.InstanceID)
	if err != nil {
		jsonErr(w, "get instance: "+err.Error())
		return
	}
	// Get VNIC info
	vnics, _ := client.GetInstanceVNICs(ctx, tenant.TenancyOCID, req.InstanceID)
	var vnicInfo map[string]interface{}
	if len(vnics) > 0 {
		v := vnics[0]
		vnicInfo = map[string]interface{}{
			"id":        strOr(v.Id, ""),
			"public_ip": strOr(v.PublicIp, ""),
			"private_ip": strOr(v.PrivateIp, ""),
			"subnet_id": strOr(v.SubnetId, ""),
			"mac":       strOr(v.MacAddress, ""),
		}
	}
	// Get boot volume info
	attachments, _ := client.ListBootVolumeAttachments(ctx, tenant.TenancyOCID, req.InstanceID)
	var bootVolumeInfo map[string]interface{}
	if len(attachments) > 0 {
		bvID := attachments[0].BootVolumeId
		if bvID != nil {
			bv, err := client.GetBootVolume(ctx, *bvID)
			if err == nil {
				bootVolumeInfo = map[string]interface{}{
					"id":       strOr(bv.Id, ""),
					"size_gb":  func() int64 { if bv.SizeInGBs != nil { return *bv.SizeInGBs }; return 0 }(),
					"vpus_per_gb": func() int64 { if bv.VpusPerGB != nil { return *bv.VpusPerGB }; return 0 }(),
					"state":    string(bv.LifecycleState),
				}
			}
		}
	}
	// Get shape config
	shapeCfg := map[string]interface{}{}
	if inst.ShapeConfig != nil {
		shapeCfg["ocpus"] = func() float32 { if inst.ShapeConfig.Ocpus != nil { return *inst.ShapeConfig.Ocpus }; return 0 }()
		shapeCfg["memory_gb"] = func() float32 { if inst.ShapeConfig.MemoryInGBs != nil { return *inst.ShapeConfig.MemoryInGBs }; return 0 }()
	}
	jsonOK(w, map[string]interface{}{
		"id":            strOr(inst.Id, ""),
		"display_name":  strOr(inst.DisplayName, ""),
		"shape":         strOr(inst.Shape, ""),
		"state":         string(inst.LifecycleState),
		"region":        strOr(inst.Region, ""),
		"availability_domain": strOr(inst.AvailabilityDomain, ""),
		"fault_domain":  strOr(inst.FaultDomain, ""),
		"time_created":  func() string { if inst.TimeCreated != nil { return inst.TimeCreated.Format(time.RFC3339) }; return "" }(),
		"shape_config":  shapeCfg,
		"vnic":          vnicInfo,
		"boot_volume":   bootVolumeInfo,
	})
}

// ── Update Root Password ──────────────────────────────────────────────

func (s *Server) handleUpdatePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		TenantID    int64  `json:"tenant_id"`
		InstanceID  string `json:"instance_id"`
		NewPassword string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}
	if req.NewPassword == "" {
		jsonErr(w, "new_password required")
		return
	}
	if len(req.NewPassword) < 8 {
		jsonErr(w, "password must be at least 8 characters")
		return
	}
	tenant, err := s.store.GetTenant(req.TenantID)
	if err != nil || tenant == nil {
		jsonErr(w, "tenant not found")
		return
	}
	// Recommend using Console Connection to reset password
	s.audit(req.TenantID, "instance:update-password", req.InstanceID, r)
	jsonOK(w, map[string]interface{}{
		"status": "password_reset_initiated",
		"message": `OCI API does not support changing the root password directly. Use the Console Connection feature:
1. Generate or upload an SSH key via /api/ssh/keys
2. POST /api/instances/vnc with ssh_key_id to create a console session
3. Connect via: ssh -o ProxyCommand='ssh -W %h:%p -p 443 ocid1.instanceconsoleconnection...@instance-console.us-phoenix-1.oci.oraclecloud.com' ocid1.instance.oc1...
4. Log in as root/opc and run: passwd
5. Enter the new password twice`,
	})
}
