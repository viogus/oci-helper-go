package handler

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/oracle/oci-go-sdk/v65/core"

	"github.com/viogus/oci-helper-go/internal/db"
	ociclient "github.com/viogus/oci-helper-go/internal/oci"
)

func (s *Server) handleInstances(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		tenantID, _ := strconv.ParseInt(r.URL.Query().Get("tenant_id"), 10, 64)
		keyword := r.URL.Query().Get("keyword")
		state := r.URL.Query().Get("state")
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		size, _ := strconv.Atoi(r.URL.Query().Get("size"))
		if size < 1 {
			size = 20
		}
		list, total, err := s.store.ListInstancesPaginated(tenantID, keyword, state, page, size)
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
			Region              string   `json:"region"`
		MemoryGB            *float32 `json:"memoryGB"`
		SSHKeyID            int64   `json:"sshKeyId"`
		RootPassword        string  `json:"rootPassword"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}

	client, t, ok := s.getTenantClient(req.TenantID, w)
	if !ok {
		return
	}

		// If a region is specified, use it; otherwise use tenant's default.
		if req.Region != "" {
			client.SetRegion(req.Region)
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
			AgentConfig: &core.LaunchInstanceAgentConfigDetails{
				IsMonitoringDisabled: common.Bool(true),
			},
		},
	}
	bootVolSize := int64(50)
	if req.BootVolumeSizeGB != nil {
		bootVolSize = *req.BootVolumeSizeGB
	}
	launchReq.LaunchInstanceDetails.SourceDetails = core.InstanceSourceViaImageDetails{
		ImageId:             common.String(req.ImageID),
		BootVolumeSizeInGBs: common.Int64(bootVolSize),
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

	// SSH key and root password metadata (cloud-init).
	metadata := map[string]string{}
	if req.SSHKeyID > 0 || req.RootPassword != "" {
		sshKeys, listErr := s.store.ListSSHKeys(req.TenantID)
		if listErr != nil {
			log.Printf("[createInstance] list ssh keys: %v", listErr)
		}
		if req.SSHKeyID > 0 && len(sshKeys) > 0 {
			for _, k := range sshKeys {
				if k.ID == req.SSHKeyID && k.PublicKey != "" {
					metadata["ssh_authorized_keys"] = k.PublicKey
					break
				}
			}
		}
		if req.RootPassword != "" {
			script := buildCloudInit(req.RootPassword)
			metadata["user_data"] = base64.StdEncoding.EncodeToString([]byte(script))
		}
		if len(metadata) > 0 {
			launchReq.LaunchInstanceDetails.Metadata = metadata
		}
	}

	// Persist root password as freeform tag so it can be retrieved later.
	if req.RootPassword != "" {
		if launchReq.LaunchInstanceDetails.FreeformTags == nil {
			launchReq.LaunchInstanceDetails.FreeformTags = map[string]string{}
		}
		launchReq.LaunchInstanceDetails.FreeformTags["root_password"] = req.RootPassword
	}

	inst, err := client.LaunchInstanceWithRequest(r.Context(), launchReq)
	if err != nil {
		jsonErr(w, "launch: "+err.Error())
		return
	}

	region := req.Region
	if region == "" {
		region = t.Region
	}
	dbInst := &db.Instance{
		ID:       fmt.Sprintf("%d:%s", req.TenantID, strOr(inst.Id, "")),
		TenantID: req.TenantID,
		Name:     strOr(inst.DisplayName, ""),
		OCID:     strOr(inst.Id, ""),
		Shape:    strOr(inst.Shape, ""),
		State:    string(inst.LifecycleState),
		Region:   region,
	}
	if err := s.store.UpsertInstance(dbInst); err != nil {
		log.Printf("[createInstance] upsert instance: %v", err)
	}
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
		Action               string `json:"action"`
		TenantID             int64  `json:"tenantId"`
		PreserveBootVolume   bool   `json:"preserveBootVolume"`
		PreserveDataVolumes  bool   `json:"preserveDataVolumes"`
		CaptchaCode          string `json:"captchaCode"`
		CaptchaTarget        string `json:"captchaTarget"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}

	client, _, ok := s.clientForInstance(req.TenantID, instanceID, w)
	if !ok {
		return
	}
	if inst, err := s.store.GetInstanceByID(instanceID); err == nil && inst != nil && inst.Region != "" {
		client.SetRegion(inst.Region)
	}

	ctx := r.Context()
	switch req.Action {
	case "terminate":
		if req.CaptchaCode != "" && !verifyCaptcha(req.CaptchaTarget, req.CaptchaCode) {
			jsonErr(w, "invalid or expired captcha code")
			return
		}
		if err := client.TerminateInstance(ctx, bareOCID(instanceID), req.PreserveBootVolume, req.PreserveDataVolumes); err != nil {
			jsonErr(w, "terminate: "+err.Error())
			return
		}
		s.store.UpdateInstanceState(instanceID, "TERMINATING")
	case "start":
		_, err := client.InstanceAction(ctx, bareOCID(instanceID), core.InstanceActionActionStart)
		if err != nil {
			jsonErr(w, "start: "+err.Error())
			return
		}
		s.store.UpdateInstanceState(instanceID, "STARTING")
	case "stop":
		_, err := client.InstanceAction(ctx, bareOCID(instanceID), core.InstanceActionActionStop)
		if err != nil {
			jsonErr(w, "stop: "+err.Error())
			return
		}
		s.store.UpdateInstanceState(instanceID, "STOPPING")
	case "reboot":
		_, err := client.InstanceAction(ctx, bareOCID(instanceID), core.InstanceActionActionReset)
		if err != nil {
			jsonErr(w, "reboot: "+err.Error())
			return
		}
		s.store.UpdateInstanceState(instanceID, "STARTING")
	case "softstop":
		_, err := client.InstanceAction(ctx, bareOCID(instanceID), core.InstanceActionActionSoftstop)
		if err != nil {
			jsonErr(w, "softstop: "+err.Error())
			return
		}
		s.store.UpdateInstanceState(instanceID, "STOPPING")
	case "softreset":
		_, err := client.InstanceAction(ctx, bareOCID(instanceID), core.InstanceActionActionSoftreset)
		if err != nil {
			jsonErr(w, "softreset: "+err.Error())
			return
		}
		s.store.UpdateInstanceState(instanceID, "STARTING")
	case "stopChangeIp":
		memTasksMu.Lock()
		for id, t := range memTasks {
			if t.InstanceID == instanceID && t.TaskType == "change_ip" {
				close(t.Cancel)
				delete(memTasks, id)
			}
		}
		memTasksMu.Unlock()
		jsonOK(w, map[string]string{"status": "ok"})
		return
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

	client, t, ok := s.getTenantClient(tenantID, w)
	if !ok {
		return
	}

	// Discover subscribed regions from OCI Identity API.
	// Falls back to DB-cached regions, then tenant's home region.
	regions := discoverRegions(r.Context(), client)
	if len(regions) == 0 {
		regions = getSubscribedRegions(t)
	}
	if len(regions) == 0 {
		regions = []string{t.Region}
	}

	// Persist discovered regions back to tenant for next sync.
	updateTenantRegions(s.store, tenantID, regions)

	totalCount := 0
	for _, region := range regions {
		client.SetRegion(region)
		instances, err := client.ListInstances(r.Context(), t.TenancyOCID)
		if err != nil {
			log.Printf("[sync] region %s: list instances: %v", region, err)
			continue
		}
		for _, inst := range instances {
			if err := s.store.UpsertInstance(ociToDB(inst, tenantID, region)); err != nil {
				log.Printf("[sync] upsert %s: %v", strOr(inst.Id, ""), err)
			}
		}
		totalCount += len(instances)
	}

	// Best-effort VNIC sync for public IP / private IP / subnet
	s.syncVNICs(context.Background(), tenantID, regions)

	s.audit(tenantID, "sync", fmt.Sprintf("synced %d instances across %d regions", totalCount, len(regions)), r)
	jsonOK(w, map[string]interface{}{"count": totalCount, "regions": len(regions)})
}

func ociToDB(i core.Instance, tenantID int64, region string) *db.Instance {
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
		Region:     region,
	}
}

func (s *Server) syncVNICs(ctx context.Context, tenantID int64, regions []string) {
	instances, err := s.store.ListInstances(tenantID)
	if err != nil {
		log.Printf("[syncVnics] list instances: %v", err)
		return
	}
	// Fetch tenant and create OCI client once — not per instance.
	tenant, err := s.store.GetTenant(tenantID)
	if err != nil || tenant == nil {
		log.Printf("[syncVnics] tenant %d not found", tenantID)
		return
	}
	client, err := s.clientFor(tenant)
	if err != nil {
		log.Printf("[syncVnics] oci client: %v", err)
		return
	}
	for _, inst := range instances {
		parts := strings.SplitN(inst.OCID, ":", 2)
		ocid := parts[len(parts)-1]
		if ocid == "" {
			continue
		}
		region := inst.Region
		if region == "" {
			region = tenant.Region
		}
		client.SetRegion(region)
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
	client, _, ok := s.clientForInstance(req.TenantID, req.InstanceID, w)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()
	if err := client.UpdateInstance(ctx, bareOCID(req.InstanceID), req.Shape, req.Ocpus, req.MemoryGB); err != nil {
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
	client, tenant, ok := s.clientForInstance(req.TenantID, req.InstanceID, w)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()
	attachment, err := client.GetBootVolumeAttachment(ctx, tenant.TenancyOCID, bareOCID(req.InstanceID))
	if err != nil {
		jsonErr(w, "get boot volume: "+err.Error())
		return
	}
	if attachment.BootVolumeId == nil {
		jsonErr(w, "boot volume id not found in attachment")
		return
	}
	if _, err := client.UpdateBootVolume(ctx, *attachment.BootVolumeId, req.SizeGB, ""); err != nil {
		jsonErr(w, "update boot volume: "+err.Error())
		return
	}
	s.audit(req.TenantID, "instance:change-boot-volume", req.InstanceID, r)
	jsonOK(w, map[string]string{"status": "ok"})
}
// bareOCID strips a leading "tenantID:" prefix from a composite instance id,
// returning the raw OCID that OCI APIs expect. Bare OCIDs pass through unchanged
// (an OCID never contains a colon).
func bareOCID(id string) string {
	if i := strings.IndexByte(id, ':'); i >= 0 {
		return id[i+1:]
	}
	return id
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
	client, _, ok := s.clientForInstance(req.TenantID, req.InstanceID, w)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	addr, err := client.EnableIPv6(ctx, bareOCID(req.InstanceID))
	if err != nil {
		jsonErr(w, "enable ipv6: "+err.Error())
		return
	}
	s.audit(req.TenantID, "instance:attach-ipv6", req.InstanceID, r)
	jsonOK(w, map[string]string{"status": "ok", "ipv6": addr})
}

func (s *Server) handleDisableIPv6(w http.ResponseWriter, r *http.Request) {
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
	client, _, ok := s.clientForInstance(req.TenantID, req.InstanceID, w)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()
	if err := client.DisableIPv6(ctx, bareOCID(req.InstanceID)); err != nil {
		jsonErr(w, "disable ipv6: "+err.Error())
		return
	}
	s.audit(req.TenantID, "instance:ipv6-disable", req.InstanceID, r)
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
	client, _, ok := s.clientForInstance(req.TenantID, req.InstanceID, w)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()
	if err := client.UpdateInstanceDisplayName(ctx, bareOCID(req.InstanceID), req.Name); err != nil {
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
	client, _, ok := s.clientForInstance(req.TenantID, req.InstanceID, w)
	if !ok {
		return
	}
	// Try to change IP once (synchronous)
	newIP, err := client.ChangeInstanceIP(r.Context(), bareOCID(req.InstanceID), req.CidrList)
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
		// Handle both bare OCIDs and composite tenantID:ocid format.
		lookupID := id
		if !strings.Contains(id, ":") {
			lookupID = fmt.Sprintf("%d:%s", req.TenantID, id)
		}
		inst, err := s.store.GetInstanceByID(lookupID)
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
	sem := make(chan struct{}, 50) // max 50 concurrent TCP checks

	for _, inst := range running {
		wg.Add(1)
		sem <- struct{}{}
		go func(inst db.Instance) {
			defer wg.Done()
			defer func() { <-sem }()
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

// ── Shrink Boot Volume to 47GB ─────────────────────────────────────────

func (s *Server) handleShrinkDisk(w http.ResponseWriter, r *http.Request) {
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
	if req.TenantID == 0 || req.InstanceID == "" {
		jsonErr(w, "tenant_id and instance_id are required")
		return
	}

	client, tenant, ok := s.clientForInstance(req.TenantID, req.InstanceID, w)
	if !ok {
		return
	}
	ocid := bareOCID(req.InstanceID)

	// Step 1: Get instance details from OCI.
	ctx := r.Context()
	inst, err := client.GetInstance(ctx, ocid)
	if err != nil {
		jsonErr(w, "get instance: "+err.Error())
		return
	}
	compartmentID := strOr(inst.CompartmentId, tenant.TenancyOCID)
	ad := strOr(inst.AvailabilityDomain, "")
	if ad == "" {
		jsonErr(w, "instance has no availability domain")
		return
	}

	// Step 2: Get the boot volume attachment for this instance.
	attachments, err := client.ListBootVolumeAttachments(ctx, compartmentID, ocid)
	if err != nil {
		jsonErr(w, "list boot volume attachments: "+err.Error())
		return
	}
	if len(attachments) == 0 {
		jsonErr(w, "no boot volume attached to instance")
		return
	}
	attachment := attachments[0]
	oldVolumeID := strOr(attachment.BootVolumeId, "")
	attachmentID := strOr(attachment.Id, "")
	if oldVolumeID == "" || attachmentID == "" {
		jsonErr(w, "boot volume attachment missing IDs")
		return
	}

	// Step 3: Get old boot volume size.
	oldBV, err := client.GetBootVolume(ctx, oldVolumeID)
	if err != nil {
		jsonErr(w, "get boot volume: "+err.Error())
		return
	}
	oldSizeGB := int64(0)
	if oldBV.SizeInGBs != nil {
		oldSizeGB = *oldBV.SizeInGBs
	}
	const targetSizeGB = 47
	if oldSizeGB <= targetSizeGB {
		jsonErr(w, fmt.Sprintf("boot volume is already %dGB or smaller (current: %dGB)", targetSizeGB, oldSizeGB))
		return
	}

	// Step 4: Stop the instance if it is running.
	initialState := string(inst.LifecycleState)
	if initialState == "RUNNING" || initialState == "STARTING" {
		if _, err := client.InstanceAction(ctx, ocid, core.InstanceActionActionStop); err != nil {
			jsonErr(w, "stop instance: "+err.Error())
			return
		}
		log.Printf("[shrink-disk] stopping instance %s", ocid)
		if !client.WaitForState(ctx, ocid, "STOPPED", 5*time.Minute) {
			jsonErr(w, "timeout waiting for instance to stop")
			return
		}
	} else if initialState != "STOPPED" {
		jsonErr(w, fmt.Sprintf("instance must be RUNNING or STOPPED, current state: %s", initialState))
		return
	}

	// Step 5: Detach the old boot volume.
	log.Printf("[shrink-disk] detaching boot volume %s", oldVolumeID)
	if err := client.DetachBootVolume(ctx, attachmentID); err != nil {
		// Try to re-start the instance if it was stopped by us.
		if initialState == "RUNNING" {
			client.InstanceAction(context.Background(), ocid, core.InstanceActionActionStart)
		}
		jsonErr(w, "detach boot volume: "+err.Error())
		return
	}

	// Step 6: Create a new 47GB boot volume cloned from the old one.
	newDisplayName := fmt.Sprintf("%s-shrunk-47g", strOr(inst.DisplayName, ocid))
	if len(newDisplayName) > 255 {
		newDisplayName = newDisplayName[:255]
	}
	log.Printf("[shrink-disk] creating new %dGB boot volume from %s", targetSizeGB, oldVolumeID)
	newBV, err := client.CreateBootVolume(ctx, compartmentID, ad, oldVolumeID, newDisplayName, targetSizeGB)
	if err != nil {
		// Rollback: re-attach old boot volume.
		if _, attachErr := client.AttachBootVolume(ctx, oldVolumeID, ocid); attachErr != nil {
			log.Printf("[shrink-disk] rollback attach old BV failed: %v", attachErr)
		}
		if initialState == "RUNNING" {
			client.InstanceAction(context.Background(), ocid, core.InstanceActionActionStart)
		}
		jsonErr(w, "create boot volume: "+err.Error())
		return
	}
	newVolumeID := strOr(newBV.Id, "")
	if newVolumeID == "" {
		// Rollback: re-attach old boot volume.
		client.AttachBootVolume(ctx, oldVolumeID, ocid)
		if initialState == "RUNNING" {
			client.InstanceAction(context.Background(), ocid, core.InstanceActionActionStart)
		}
		jsonErr(w, "created boot volume has no ID")
		return
	}

	// Step 7: Wait for new boot volume to become AVAILABLE.
	log.Printf("[shrink-disk] waiting for new boot volume %s to be available", newVolumeID)
	{
		bvCtx, bvCancel := context.WithTimeout(ctx, 5*time.Minute)
		defer bvCancel()
		deadline := time.Now().Add(5 * time.Minute)
		for time.Now().Before(deadline) {
			bv, pollErr := client.GetBootVolume(bvCtx, newVolumeID)
			if pollErr != nil {
				client.DeleteBootVolume(context.Background(), newVolumeID)
				client.AttachBootVolume(context.Background(), oldVolumeID, ocid)
				if initialState == "RUNNING" {
					client.InstanceAction(context.Background(), ocid, core.InstanceActionActionStart)
				}
				jsonErr(w, "poll new boot volume: "+pollErr.Error())
				return
			}
			state := string(bv.LifecycleState)
			if state == "AVAILABLE" {
				break
			}
			if state == "FAULTY" || state == "TERMINATED" || state == "TERMINATING" {
				client.DeleteBootVolume(context.Background(), newVolumeID)
				client.AttachBootVolume(context.Background(), oldVolumeID, ocid)
				if initialState == "RUNNING" {
					client.InstanceAction(context.Background(), ocid, core.InstanceActionActionStart)
				}
				jsonErr(w, fmt.Sprintf("new boot volume entered %s state", state))
				return
			}
			select {
			case <-bvCtx.Done():
				client.DeleteBootVolume(context.Background(), newVolumeID)
				client.AttachBootVolume(context.Background(), oldVolumeID, ocid)
				if initialState == "RUNNING" {
					client.InstanceAction(context.Background(), ocid, core.InstanceActionActionStart)
				}
				jsonErr(w, "timeout waiting for new boot volume to become available")
				return
			case <-time.After(5 * time.Second):
			}
		}
	}

	// Step 8: Attach the new boot volume.
	log.Printf("[shrink-disk] attaching new boot volume %s to instance %s", newVolumeID, ocid)
	if _, err := client.AttachBootVolume(ctx, newVolumeID, ocid); err != nil {
		client.DeleteBootVolume(context.Background(), newVolumeID)
		client.AttachBootVolume(context.Background(), oldVolumeID, ocid)
		if initialState == "RUNNING" {
			client.InstanceAction(context.Background(), ocid, core.InstanceActionActionStart)
		}
		jsonErr(w, "attach new boot volume: "+err.Error())
		return
	}

	// Step 9: Delete the old boot volume.
	log.Printf("[shrink-disk] deleting old boot volume %s", oldVolumeID)
	if err := client.DeleteBootVolume(ctx, oldVolumeID); err != nil {
		log.Printf("[shrink-disk] delete old boot volume %s: %v (non-fatal)", oldVolumeID, err)
	}

	// Step 10: Start the instance if it was originally running.
	if initialState == "RUNNING" {
		log.Printf("[shrink-disk] starting instance %s", ocid)
		if _, err := client.InstanceAction(ctx, ocid, core.InstanceActionActionStart); err != nil {
			jsonErr(w, fmt.Sprintf("boot volume shrunk to %dGB but failed to start instance: %v", targetSizeGB, err))
			return
		}
	}

	// Step 11: Update the instance boot volume size in the DB.
	instID := fmt.Sprintf("%d:%s", req.TenantID, req.InstanceID)
	if dbInst, err := s.store.GetInstanceByID(instID); err == nil && dbInst != nil {
		dbInst.BootVolumeGB = targetSizeGB
		if err := s.store.UpsertInstance(dbInst); err != nil {
			log.Printf("[shrink-disk] update boot volume size in DB: %v", err)
		}
	}

	s.audit(req.TenantID, "instance:shrink-disk",
		fmt.Sprintf("%s: %dGB → %dGB", req.InstanceID, oldSizeGB, targetSizeGB), r)
	jsonOK(w, map[string]interface{}{
		"status":      "ok",
		"message":     "boot volume shrunk to 47GB, instance restarted",
		"old_size_gb": oldSizeGB,
		"new_size_gb": targetSizeGB,
	})
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
	client, _, ok := s.clientForInstance(req.TenantID, req.InstanceID, w)
	if !ok {
		return
	}
	nlbIP, err := client.Enable500Mbps(r.Context(), bareOCID(req.InstanceID))
	if err != nil {
		jsonErr(w, "enable 500M: "+err.Error())
		return
	}
	s.audit(req.TenantID, "instance:500m-enable", req.InstanceID, r)
	jsonOK(w, map[string]string{"status": "ok", "nlb_ip": nlbIP})
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
	client, _, ok := s.clientForInstance(req.TenantID, req.InstanceID, w)
	if !ok {
		return
	}
	if err := client.Disable500Mbps(r.Context(), bareOCID(req.InstanceID)); err != nil {
		jsonErr(w, "disable 500M: "+err.Error())
		return
	}
	s.audit(req.TenantID, "instance:500m-disable", req.InstanceID, r)
	jsonOK(w, map[string]string{"status": "ok"})
}
func (s *Server) handleNetworkStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		TenantID    int64    `json:"tenant_id"`
		InstanceIDs []string `json:"instance_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}
	client, _, ok := s.getTenantClient(req.TenantID, w)
	if !ok {
		return
	}
	// Group by region — a tenant's instances may span regions, and NLB/VNIC
	// queries are region-scoped. OCI is queried by bare OCID; results are keyed
	// back to the composite id the frontend uses.
	ocidToID := map[string]string{}
	byRegion := map[string][]string{}
	for _, id := range req.InstanceIDs {
		ocid := bareOCID(id)
		ocidToID[ocid] = id
		region := ""
		if inst, err := s.store.GetInstanceByID(id); err == nil && inst != nil {
			region = inst.Region
		}
		byRegion[region] = append(byRegion[region], ocid)
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	result := map[string]ociclient.NetworkStatus{}
	for region, ocids := range byRegion {
		if region != "" {
			client.SetRegion(region)
		}
		for ocid, st := range client.GetNetworkStatus(ctx, ocids) {
			result[ocidToID[ocid]] = st
		}
	}
	jsonOK(w, result)
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
	client, _, ok := s.clientForInstance(req.TenantID, req.InstanceID, w)
	if !ok {
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

	// Quick TCP check — if already alive, respond immediately.
	if checkTCPPort(inst.PublicIP, 22, 5*time.Second) {
		jsonOK(w, map[string]interface{}{"status": "ok", "alive": true})
		return
	}

	// Run rescue steps in background goroutine to avoid blocking HTTP handler.
	ocid := bareOCID(req.InstanceID)
	instanceID := req.InstanceID
	publicIP := inst.PublicIP

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		checkAlive := func() bool {
			return checkTCPPort(publicIP, 22, 5*time.Second)
		}

		// Step 2: SOFTRESET
		if _, err := client.InstanceAction(ctx, ocid, core.InstanceActionActionSoftreset); err != nil {
			log.Printf("[autoRescue] softreset %s: %v", instanceID, err)
		} else {
			time.Sleep(30 * time.Second)
			if checkAlive() {
				log.Printf("[autoRescue] %s recovered via softreset", instanceID)
				return
			}
		}

		// Step 3: RESET
		if _, err := client.InstanceAction(ctx, ocid, core.InstanceActionActionReset); err != nil {
			log.Printf("[autoRescue] reset %s: %v", instanceID, err)
		} else {
			time.Sleep(60 * time.Second)
			if checkAlive() {
				log.Printf("[autoRescue] %s recovered via reset", instanceID)
				return
			}
		}

		// Step 4: STOP then START
		if _, err := client.InstanceAction(ctx, ocid, core.InstanceActionActionStop); err != nil {
			log.Printf("[autoRescue] stop %s: %v", instanceID, err)
		} else {
			time.Sleep(30 * time.Second)
			if _, err := client.InstanceAction(ctx, ocid, core.InstanceActionActionStart); err != nil {
				log.Printf("[autoRescue] start %s: %v", instanceID, err)
			} else {
				time.Sleep(60 * time.Second)
				alive := checkAlive()
				log.Printf("[autoRescue] %s stop+start done, alive=%v", instanceID, alive)
			}
		}

		log.Printf("[autoRescue] %s rescue sequence complete", instanceID)
	}()

	s.audit(req.TenantID, "instance:auto-rescue:started", req.InstanceID, r)
	jsonOK(w, map[string]interface{}{"status": "running", "instance_id": req.InstanceID})
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
	client, _, ok := s.clientForInstance(req.TenantID, req.InstanceID, w)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()
	if err := client.UpdateInstance(ctx, bareOCID(req.InstanceID), req.Shape, req.Ocpus, req.MemoryGB); err != nil {
		jsonErr(w, "update instance: "+err.Error())
		return
	}
	if req.DisplayName != "" {
		if err := client.UpdateInstanceDisplayName(ctx, bareOCID(req.InstanceID), req.DisplayName); err != nil {
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
	client, _, ok := s.clientForInstance(req.TenantID, req.InstanceID, w)
	if !ok {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()
	if err := client.UpdateInstance(ctx, bareOCID(req.InstanceID), req.Shape, 0, 0); err != nil {
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
	client, _, ok := s.clientForInstance(req.TenantID, req.InstanceID, w)
	if !ok {
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
	conn, err := client.CreateConsoleConnection(r.Context(), bareOCID(req.InstanceID), pubKey)
	if err != nil {
		jsonErr(w, "create console connection: "+err.Error())
		return
	}
	// Start polling in background for connection to become active.
	// Track goroutine exit via log; 2-min timeout bounds any leak.
	go func() {
		defer log.Printf("[vnc] polling goroutine exited for conn=%s", strOr(conn.Id, ""))
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
	client, tenant, ok := s.clientForInstance(req.TenantID, req.InstanceID, w)
	if !ok {
		return
	}
	ctx := r.Context()
	// Get instance details
	inst, err := client.GetInstance(ctx, bareOCID(req.InstanceID))
	if err != nil {
		jsonErr(w, "get instance: "+err.Error())
		return
	}
	// Get VNIC info
	vnics, vnicErr := client.GetInstanceVNICs(ctx, tenant.TenancyOCID, bareOCID(req.InstanceID))
	if vnicErr != nil {
		log.Printf("[configInfo] get VNICs for %s: %v", req.InstanceID, vnicErr)
	}
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
	attachments, bvErr := client.ListBootVolumeAttachments(ctx, tenant.TenancyOCID, bareOCID(req.InstanceID))
	if bvErr != nil {
		log.Printf("[configInfo] get boot volumes for %s: %v", req.InstanceID, bvErr)
	}
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
	if len(req.NewPassword) < 8 {
		jsonErr(w, "password must be at least 8 characters")
		return
	}
	// Do NOT store the root password in OCI freeform tags — they are
	// visible to anyone with instances.read in OCI IAM. The password is
	// injected via cloud-init at launch time only and never persisted.
	//
	// For existing instances without cloud-init, the caller should use
	// the shell console to set the password interactively.
	s.audit(req.TenantID, "instance:update-password", req.InstanceID, r)
	jsonOK(w, map[string]string{"status": "ok", "warning": "password not stored in OCI tags; use cloud-init at launch or shell console for existing instances"})
}

// discoverRegions calls the OCI Identity API to list subscribed regions for the tenancy.
// Returns region names, or nil on any error (caller should fall back to cached/default).
func discoverRegions(ctx context.Context, client *ociclient.Client) []string {
	subs, err := client.ListRegionSubscriptions(ctx)
	if err != nil {
		log.Printf("[regions] ListRegionSubscriptions: %v", err)
		return nil
	}
	var regions []string
	for _, sub := range subs {
		if sub.RegionName != nil && *sub.RegionName != "" {
			regions = append(regions, *sub.RegionName)
		}
	}
	return regions
}

// getSubscribedRegions parses the tenant's subscribed JSON field into a string slice.
// Returns nil if the field is empty or unparseable.
func getSubscribedRegions(t *db.Tenant) []string {
	if t.Subscribed == "" {
		return nil
	}
	var regions []string
	if err := json.Unmarshal([]byte(t.Subscribed), &regions); err != nil {
		return nil
	}
	return regions
}

// updateTenantRegions persists the discovered region list to the tenant record.
// Uses a direct DB update to avoid a full tenant round-trip.
func updateTenantRegions(store *db.Store, tenantID int64, regions []string) {
	data, err := json.Marshal(regions)
	if err != nil {
		return
	}
	// Update the subscribed field via raw SQL since Store has no UpdateTenant method.
	// This is intentionally minimal to avoid adding a full UpdateTenant to Store.
	if err := store.UpdateTenantRegions(tenantID, string(data)); err != nil {
		log.Printf("[regions] update tenant %d regions: %v", tenantID, err)
	}
}

// buildCloudInit returns a cloud-init script that sets the root password
// and enables SSH password authentication. Mirrors Java's getPwdShell().
func buildCloudInit(password string) string {
	// Escape backslash and dollar signs for safe YAML embedding.
	safePwd := strings.ReplaceAll(password, `\`, `\\`)
	safePwd = strings.ReplaceAll(safePwd, `$`, `\$`)
	return "#cloud-config\n" +
		"ssh_pwauth: yes\n" +
		"chpasswd:\n" +
		"  list: |\n" +
		"    root:" + safePwd + "\n" +
		"  expire: false\n" +
		"write_files:\n" +
		"  - path: /tmp/setup_root_access.sh\n" +
		"    permissions: '0700'\n" +
		"    content: |\n" +
		"      #!/bin/bash\n" +
		"      if [ -f /etc/os-release ]; then\n" +
		"        . /etc/os-release\n" +
		"        OS=$ID\n" +
		"      else\n" +
		"        exit 1\n" +
		"      fi\n" +
		"      OS=$(echo \"$OS\" | tr '[:upper:]' '[:lower:]')\n" +
		"      sed -i 's/^#\\?PasswordAuthentication.*/PasswordAuthentication yes/' /etc/ssh/sshd_config\n" +
		"      sed -i 's/^#\\?PermitRootLogin.*/PermitRootLogin yes/' /etc/ssh/sshd_config\n" +
		"      if grep -q '^#\\?PrintMotd' /etc/ssh/sshd_config; then\n" +
		"        sed -i 's/^#\\?PrintMotd.*/PrintMotd no/' /etc/ssh/sshd_config\n" +
		"      else\n" +
		"        echo 'PrintMotd no' >> /etc/ssh/sshd_config\n" +
		"      fi\n" +
		"      if grep -q '^#\\?PrintLastLog' /etc/ssh/sshd_config; then\n" +
		"        sed -i 's/^#\\?PrintLastLog.*/PrintLastLog no/' /etc/ssh/sshd_config\n" +
		"      else\n" +
		"        echo 'PrintLastLog no' >> /etc/ssh/sshd_config\n" +
		"      fi\n" +
		"      case $OS in\n" +
		"        ubuntu|debian)\n" +
		"          if grep -q '^#\\?DenyUsers' /etc/ssh/sshd_config; then\n" +
		"            sed -i 's/^#\\?DenyUsers.*/DenyUsers ubuntu/' /etc/ssh/sshd_config\n" +
		"          else\n" +
		"            echo 'DenyUsers ubuntu' >> /etc/ssh/sshd_config\n" +
		"          fi\n" +
		"          ;;\n" +
		"        ol|rhel|centos|almalinux|rocky)\n" +
		"          if grep -q '^#\\?DenyUsers' /etc/ssh/sshd_config; then\n" +
		"            sed -i 's/^#\\?DenyUsers.*/DenyUsers opc/' /etc/ssh/sshd_config\n" +
		"          else\n" +
		"            echo 'DenyUsers opc' >> /etc/ssh/sshd_config\n" +
		"          fi\n" +
		"          ;;\n" +
		"      esac\n" +
		"      if command -v systemctl >/dev/null 2>&1; then\n" +
		"        systemctl restart sshd 2>/dev/null || systemctl restart ssh 2>/dev/null || true\n" +
		"      else\n" +
		"        service sshd restart 2>/dev/null || service ssh restart 2>/dev/null || true\n" +
		"      fi\n" +
		"runcmd:\n" +
		"  - [ bash, /tmp/setup_root_access.sh ]\n" +
		"  - echo 'Welcome to oci-helper managed instance' > /etc/motd\n" +
		"  - rm -f /tmp/setup_root_access.sh\n"
}
