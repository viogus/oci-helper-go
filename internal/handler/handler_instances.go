package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
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
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	// /api/instances/{ocid}/action
	path := strings.TrimPrefix(r.URL.Path, "/api/instances/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) != 2 || parts[1] != "action" {
		jsonErr(w, "invalid path, expected /api/instances/{id}/action")
		return
	}
	instanceID := parts[0]

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
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
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
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
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
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
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
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
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

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
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
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	if err := client.UpdateInstance(ctx, req.InstanceID, req.Shape, 0, 0); err != nil {
		jsonErr(w, "update shape: "+err.Error())
		return
	}
	s.audit(req.TenantID, "instance:update-shape", req.InstanceID, r)
	jsonOK(w, map[string]string{"status": "ok"})
}
