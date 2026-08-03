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
		TenantID           int64    `json:"tenantId"`
		DisplayName        string   `json:"displayName"`
		ImageID            string   `json:"imageId"`
		Shape              string   `json:"shape"`
		SubnetID           string   `json:"subnetId"`
		AvailabilityDomain string   `json:"availabilityDomain"`
		BootVolumeSizeGB   *int64   `json:"bootVolumeSizeGB"`
		OCPUs              *float32 `json:"ocpus"`
		Region             string   `json:"region"`
		MemoryGB           *float32 `json:"memoryGB"`
		SSHKeyID           int64    `json:"sshKeyId"`
		RootPassword       string   `json:"rootPassword"`
		IntervalSeconds    int      `json:"intervalSeconds"`
		CreateNumbers      int      `json:"createNumbers"`
		Architecture       string   `json:"architecture"`
		OperationSystem    string   `json:"operationSystem"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}

	// Java createInstance parity: when an interval is supplied, persist a
	// recurring create task instead of launching a one-shot instance.
	if req.IntervalSeconds > 0 {
		if req.RootPassword == "" {
			jsonErr(w, "rootPassword required for recurring create task")
			return
		}
		if req.CreateNumbers <= 0 {
			req.CreateNumbers = 1
		}
		if req.Architecture == "" {
			req.Architecture = "AMD"
		}
		if req.OperationSystem == "" {
			req.OperationSystem = "Ubuntu"
		}
		task := &db.CreateTask{
			TenantID:        req.TenantID,
			Region:          req.Region,
			OCPUs:           float64(ptrFloat(req.OCPUs, 1)),
			MemoryGB:        float64(ptrFloat(req.MemoryGB, 1)),
			Disk:            ptrInt64(req.BootVolumeSizeGB, 50),
			Architecture:    req.Architecture,
			IntervalSeconds: req.IntervalSeconds,
			CreateNumbers:   req.CreateNumbers,
			OperationSystem: req.OperationSystem,
			RootPassword:    req.RootPassword,
		}
		if err := s.store.CreateCreateTask(task); err != nil {
			jsonErr(w, "create recurring task: "+err.Error())
			return
		}
		s.audit(req.TenantID, "instance:create-task", fmt.Sprintf("schedule %d instances every %ds", task.CreateNumbers, task.IntervalSeconds), r)
		jsonOK(w, task)
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
		Action              string `json:"action"`
		TenantID            int64  `json:"tenantId"`
		PreserveBootVolume  bool   `json:"preserveBootVolume"`
		PreserveDataVolumes bool   `json:"preserveDataVolumes"`
		CaptchaCode         string `json:"captchaCode"`
		CaptchaTarget       string `json:"captchaTarget"`
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
		// Java parity: termination is protected by a single-use captcha sent
		// over the configured notification channel. When a channel exists we
		// require it; without any channel the captcha cannot be delivered, so
		// the check is skipped (legacy/airgapped deployments).
		hasChannel := false
		if t, _ := s.store.GetConfig("telegram_token"); t != "" {
			hasChannel = true
		}
		if d, _ := s.store.GetConfig("dingtalk_webhook"); d != "" {
			hasChannel = true
		}
		if hasChannel {
			if req.CaptchaCode == "" {
				jsonErr(w, "captcha code required: request one via /api/captcha/send first")
				return
			}
			if !verifyCaptcha(req.CaptchaTarget, req.CaptchaCode) {
				jsonErr(w, "invalid or expired captcha code")
				return
			}
		}
		if err := client.TerminateInstance(ctx, bareOCID(instanceID), req.PreserveBootVolume, req.PreserveDataVolumes); err != nil {
			jsonErr(w, "terminate: "+err.Error())
			return
		}
		if err := s.store.UpdateInstanceState(instanceID, "TERMINATING"); err != nil {
			log.Printf("[instances] UpdateInstanceState %s: %v", instanceID, err)
		}
	case "start":
		_, err := client.InstanceAction(ctx, bareOCID(instanceID), core.InstanceActionActionStart)
		if err != nil {
			jsonErr(w, "start: "+err.Error())
			return
		}
		if err := s.store.UpdateInstanceState(instanceID, "STARTING"); err != nil {
			log.Printf("[instances] UpdateInstanceState %s (start): %v", instanceID, err)
		}
	case "stop":
		_, err := client.InstanceAction(ctx, bareOCID(instanceID), core.InstanceActionActionStop)
		if err != nil {
			jsonErr(w, "stop: "+err.Error())
			return
		}
		if err := s.store.UpdateInstanceState(instanceID, "STOPPING"); err != nil {
			log.Printf("[instances] UpdateInstanceState %s (stop): %v", instanceID, err)
		}
	case "reboot":
		_, err := client.InstanceAction(ctx, bareOCID(instanceID), core.InstanceActionActionReset)
		if err != nil {
			jsonErr(w, "reboot: "+err.Error())
			return
		}
		if err := s.store.UpdateInstanceState(instanceID, "STARTING"); err != nil {
			log.Printf("[instances] UpdateInstanceState %s (reboot): %v", instanceID, err)
		}
	case "softstop":
		_, err := client.InstanceAction(ctx, bareOCID(instanceID), core.InstanceActionActionSoftstop)
		if err != nil {
			jsonErr(w, "softstop: "+err.Error())
			return
		}
		if err := s.store.UpdateInstanceState(instanceID, "STOPPING"); err != nil {
			log.Printf("[instances] UpdateInstanceState %s (softstop): %v", instanceID, err)
		}
	case "softreset":
		_, err := client.InstanceAction(ctx, bareOCID(instanceID), core.InstanceActionActionSoftreset)
		if err != nil {
			jsonErr(w, "softreset: "+err.Error())
			return
		}
		if err := s.store.UpdateInstanceState(instanceID, "STARTING"); err != nil {
			log.Printf("[instances] UpdateInstanceState %s (softreset): %v", instanceID, err)
		}
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
	if req.Action != "stopChangeIp" {
		s.notify(req.TenantID, fmt.Sprintf("【实例操作】实例 %s %s 命令已下发", instanceID, req.Action))
	}
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
	payload, err := json.Marshal(req)
	if err != nil {
		log.Printf("[batch:start] marshal: %v", err)
		jsonErr(w, "marshal payload: "+err.Error())
		return
	}
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
		ID:                 fmt.Sprintf("%d:%s", tenantID, strOr(i.Id, "")),
		TenantID:           tenantID,
		Name:               strOr(i.DisplayName, ""),
		OCID:               strOr(i.Id, ""),
		Shape:              strOr(i.Shape, ""),
		State:              string(i.LifecycleState),
		OCPU:               ocpu,
		MemoryGB:           memGB,
		BootVolumeGB:       bootVolGB,
		ImageID:            imageID,
		AvailabilityDomain: ad,
		FaultDomain:        fd,
		Region:             region,
	}
}

func (s *Server) syncVNICs(ctx context.Context, tenantID int64, regions []string) {
	instances, err := s.store.ListInstances(tenantID)
	if err != nil {
		log.Printf("[syncVnics] list instances: %v", err)
		return
	}
	if len(instances) == 0 {
		return
	}
	// Fetch tenant once, create per-region clients in goroutines.
	tenant, err := s.store.GetTenant(tenantID)
	if err != nil || tenant == nil {
		log.Printf("[syncVnics] tenant %d not found", tenantID)
		return
	}
	// Group instances by region
	byRegion := make(map[string][]db.Instance)
	for _, inst := range instances {
		region := inst.Region
		if region == "" {
			region = tenant.Region
		}
		byRegion[region] = append(byRegion[region], inst)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	sem := make(chan struct{}, 5)
	for region, insts := range byRegion {
		wg.Add(1)
		sem <- struct{}{}
		go func(region string, insts []db.Instance) {
			defer wg.Done()
			defer func() { <-sem }()
			client, err := s.clientFor(tenant)
			if err != nil {
				mu.Lock()
				log.Printf("[syncVnics] region %s oci client: %v", region, err)
				mu.Unlock()
				return
			}
			client.SetRegion(region)
			for _, inst := range insts {
				parts := strings.SplitN(inst.OCID, ":", 2)
				ocid := parts[len(parts)-1]
				if ocid == "" {
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
				if err := s.store.UpsertInstance(&inst); err != nil {
					mu.Lock()
					log.Printf("[sync] upsert vnic %s: %v", inst.OCID, err)
					mu.Unlock()
				}
			}
		}(region, insts)
	}
	wg.Wait()
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
		TenantID            int64    `json:"tenant_id"`
		InstanceID          string   `json:"instance_id"`
		CidrList            []string `json:"cidr_list"`
		ChangeCfDNS         bool     `json:"change_cf_dns"`
		SelectedDomainCfgID int64    `json:"selected_domain_cfg_id"`
		DomainPrefix        string   `json:"domain_prefix"`
		EnableProxy         *bool    `json:"enable_proxy"`
		TTL                 int      `json:"ttl"`
		Remark              string   `json:"remark"`
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
	dnsErr := ""
	if req.ChangeCfDNS {
		if err := s.updateCfDNSAfterChangeIP(req.TenantID, req.SelectedDomainCfgID, req.DomainPrefix, newIP, req.EnableProxy, req.TTL, req.Remark); err != nil {
			dnsErr = err.Error()
			log.Printf("[change-ip] cloudflare dns update: %v", err)
		}
	}
	// Keep the DB in sync so the UI shows the new IP right away (the next
	// sync would also fix it, but that may take a while).
	if err := s.store.UpdateInstancePublicIP(req.InstanceID, newIP); err != nil {
		log.Printf("[change-ip] update instance public_ip: %v", err)
	}
	s.audit(req.TenantID, "instance:change-ip", req.InstanceID+" → "+maskIP(newIP), r)
	s.notify(req.TenantID, fmt.Sprintf("【更换公共IP】实例 %s 新公网IP: %s", req.InstanceID, newIP))
	jsonOK(w, map[string]string{"new_ip": newIP, "status": "ok", "dns_error": dnsErr})
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
		TenantID    int64  `json:"tenant_id"`
		InstanceID  string `json:"instance_id"`
		RetainBL    bool   `json:"retain_bl"`
		RetainNatGW bool   `json:"retain_nat_gw"`
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
		SSHPort    int    `json:"ssh_port"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}
	client, _, ok := s.clientForInstance(req.TenantID, req.InstanceID, w)
	if !ok {
		return
	}
	if req.SSHPort <= 0 {
		req.SSHPort = 22
	}
	inst, err := client.GetInstance(r.Context(), bareOCID(req.InstanceID))
	if err != nil {
		jsonErr(w, "get instance: "+err.Error())
		return
	}
	if inst.Shape == nil || !strings.Contains(strings.ToLower(*inst.Shape), "vm.standard.e") {
		jsonErr(w, "该实例不支持一键开启下行500Mbps")
		return
	}
	nlbIP, err := client.Enable500Mbps(r.Context(), bareOCID(req.InstanceID), req.SSHPort)
	if err != nil {
		jsonErr(w, "enable 500M: "+err.Error())
		return
	}
	s.audit(req.TenantID, "instance:500m-enable", req.InstanceID, r)
	s.notify(req.TenantID, fmt.Sprintf("【一键开启下行500Mbps】实例 %s 已开启，NLB IP: %s", req.InstanceID, nlbIP))
	jsonOK(w, map[string]string{"status": "ok", "nlb_ip": nlbIP})
}

func (s *Server) handleOneClickClose500M(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		TenantID    int64  `json:"tenant_id"`
		InstanceID  string `json:"instance_id"`
		RetainBL    bool   `json:"retain_bl"`
		RetainNatGW bool   `json:"retain_nat_gw"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}
	client, _, ok := s.clientForInstance(req.TenantID, req.InstanceID, w)
	if !ok {
		return
	}
	if err := client.Disable500Mbps(r.Context(), bareOCID(req.InstanceID), req.RetainBL, req.RetainNatGW); err != nil {
		jsonErr(w, "disable 500M: "+err.Error())
		return
	}
	s.audit(req.TenantID, "instance:500m-disable", req.InstanceID, r)
	s.notify(req.TenantID, fmt.Sprintf("【关闭下行500Mbps】实例 %s 已关闭", req.InstanceID))
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
		TenantID         int64  `json:"tenant_id"`
		InstanceID       string `json:"instance_id"`
		KeepBackupVolume bool   `json:"keep_backup_volume"`
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
	// Run the full rescue sequence in a background goroutine so the HTTP call
	// returns immediately, matching the Java original.
	ocid := bareOCID(req.InstanceID)
	instanceID := req.InstanceID
	keepBackup := req.KeepBackupVolume

	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
		defer cancel()
		s.runAutoRescue(ctx, req.TenantID, instanceID, ocid, client, keepBackup)
	}()

	s.audit(req.TenantID, "instance:auto-rescue:started", req.InstanceID, r)
	jsonOK(w, map[string]interface{}{"status": "running", "instance_id": req.InstanceID})
}

func (s *Server) runAutoRescue(ctx context.Context, tenantID int64, instanceID, ocid string, client *ociclient.Client, keepBackup bool) {
	tenant, err := s.store.GetTenant(tenantID)
	if err != nil || tenant == nil {
		log.Printf("[autoRescue] tenant %d not found", tenantID)
		return
	}
	fail := func(step string, err error) {
		log.Printf("[autoRescue] %s: %v", step, err)
		s.notify(tenantID, fmt.Sprintf("【自动救援/缩小硬盘任务】用户:[%s] 步骤[%s]失败：%v", tenant.Name, step, err))
	}
	inst, err := client.GetInstance(ctx, ocid)
	if err != nil {
		fail("获取实例", err)
		return
	}
	compartmentID := strOr(inst.CompartmentId, tenant.TenancyOCID)
	ad := strOr(inst.AvailabilityDomain, "")
	if ad == "" {
		fail("获取可用域", fmt.Errorf("instance has no availability domain"))
		return
	}

	// (1) Stop the instance.
	initialState := string(inst.LifecycleState)
	if initialState == "RUNNING" || initialState == "STARTING" {
		if _, err := client.InstanceAction(ctx, ocid, core.InstanceActionActionStop); err != nil {
			fail("关机", err)
			return
		}
		if !client.WaitForState(ctx, ocid, "STOPPED", 5*time.Minute) {
			fail("等待关机", fmt.Errorf("timeout waiting for STOPPED"))
			return
		}
	}
	attachments, err := client.ListBootVolumeAttachments(ctx, compartmentID, ocid)
	if err != nil || len(attachments) == 0 {
		fail("获取引导卷", fmt.Errorf("no boot volume attachment: %w", err))
		return
	}
	oldBV := strOr(attachments[0].BootVolumeId, "")
	attachmentID := strOr(attachments[0].Id, "")
	if oldBV == "" || attachmentID == "" {
		fail("获取引导卷", fmt.Errorf("missing boot volume or attachment id"))
		return
	}

	// (2) Backup the old boot volume.
	backup, err := client.CreateBootVolumeBackup(ctx, oldBV, "Old-BootVolume-Backup")
	if err != nil {
		fail("备份引导卷", err)
		return
	}
	backupID := strOr(backup.Id, "")
	for i := 0; i < 60; i++ {
		b, err := client.GetBootVolumeBackup(ctx, backupID)
		if err == nil && b.LifecycleState == core.BootVolumeBackupLifecycleStateAvailable {
			break
		}
		select {
		case <-ctx.Done():
			fail("等待备份完成", ctx.Err())
			return
		case <-time.After(5 * time.Second):
		}
	}

	// (3) Detach and (4) delete the old boot volume.
	if err := client.DetachBootVolume(ctx, attachmentID); err != nil {
		fail("分离引导卷", err)
		return
	}
	if err := client.DeleteBootVolume(ctx, oldBV); err != nil {
		fail("删除引导卷", err)
		return
	}

	// (5) Create a temporary AMD 47GB instance, then clone its boot volume.
	subnet, err := client.EnsurePublicSubnet(ctx, compartmentID)
	if err != nil || subnet.Id == nil {
		fail("准备公网子网", fmt.Errorf("no public subnet: %w", err))
		return
	}
	image, err := client.FindImageForOS(ctx, compartmentID, "Ubuntu", "VM.Standard.E2.1.Micro")
	if err != nil {
		fail("选择镜像", err)
		return
	}
	tmpName := fmt.Sprintf("oci-helper-rescue-%d", time.Now().UnixNano()/1e6)
	tmpInst, err := client.LaunchTaskInstance(ctx, ad, "VM.Standard.E2.1.Micro", *image.Id, *subnet.Id, tmpName, 1, 1, 47, "ocihelper2024")
	if err != nil {
		fail("创建AMD实例", err)
		return
	}
	tmpID := strOr(tmpInst.Id, "")
	if !client.WaitForState(ctx, tmpID, "RUNNING", 5*time.Minute) {
		fail("等待AMD实例启动", fmt.Errorf("timeout"))
		return
	}
	tmpAttachments, err := client.ListBootVolumeAttachments(ctx, compartmentID, tmpID)
	if err != nil || len(tmpAttachments) == 0 || tmpAttachments[0].BootVolumeId == nil {
		fail("获取AMD引导卷", fmt.Errorf("no attachment"))
		return
	}

	// (6) Clone the new boot volume onto the original instance's AD.
	clone, err := client.CreateBootVolume(ctx, compartmentID, ad, *tmpAttachments[0].BootVolumeId, "Cloned-Boot-Volume", 47)
	if err != nil {
		fail("克隆引导卷", err)
		return
	}
	cloneID := strOr(clone.Id, "")
	for i := 0; i < 60; i++ {
		v, err := client.GetBootVolume(ctx, cloneID)
		if err == nil && v.LifecycleState == core.BootVolumeLifecycleStateAvailable {
			break
		}
		select {
		case <-ctx.Done():
			fail("等待克隆完成", ctx.Err())
			return
		case <-time.After(5 * time.Second):
		}
	}

	// (7) Attach the clone to the rescued instance.
	if _, err := client.AttachBootVolume(ctx, cloneID, ocid); err != nil {
		fail("附加引导卷", err)
		return
	}

	// (8) Delete the temporary AMD instance.
	if err := client.TerminateInstance(ctx, tmpID, false, false); err != nil {
		log.Printf("[autoRescue] terminate temp instance: %v", err)
	}
	if !keepBackup {
		if err := client.DeleteBootVolumeBackup(ctx, backupID); err != nil {
			log.Printf("[autoRescue] delete backup: %v", err)
		}
	}

	// (9) Start the instance and notify.
	if _, err := client.InstanceAction(ctx, ocid, core.InstanceActionActionStart); err != nil {
		fail("启动实例", err)
		return
	}
	client.WaitForState(ctx, ocid, "RUNNING", 5*time.Minute)
	publicIP := ""
	if vnics, err := client.GetInstanceVNICs(ctx, compartmentID, ocid); err == nil && len(vnics) > 0 && vnics[0].PublicIp != nil {
		publicIP = *vnics[0].PublicIp
	}
	s.notify(tenantID, fmt.Sprintf("【自动救援/缩小硬盘任务】\n用户：%s\n区域：%s\n实例：%s\n公网IP：%s\nSSH端口：22\nSSH账号：root\nSSH密码：ocihelper2024",
		tenant.Name, tenant.Region, instanceID, publicIP))
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
		// Cancel context early on server shutdown so goroutine exits immediately.
		go func() {
			select {
			case <-s.stopping:
				log.Printf("[vnc] shutdown, stopping poll for conn=%s", strOr(conn.Id, ""))
				cancel()
			case <-ctx.Done():
			}
		}()
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
		"status":                "creating",
		"connection_id":         strOr(conn.Id, ""),
		"connection_string":     strOr(conn.ConnectionString, ""),
		"vnc_connection_string": strOr(conn.VncConnectionString, ""),
		"fingerprint":           strOr(conn.Fingerprint, ""),
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
			"id":         strOr(v.Id, ""),
			"public_ip":  strOr(v.PublicIp, ""),
			"private_ip": strOr(v.PrivateIp, ""),
			"subnet_id":  strOr(v.SubnetId, ""),
			"mac":        strOr(v.MacAddress, ""),
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
					"id": strOr(bv.Id, ""),
					"size_gb": func() int64 {
						if bv.SizeInGBs != nil {
							return *bv.SizeInGBs
						}
						return 0
					}(),
					"vpus_per_gb": func() int64 {
						if bv.VpusPerGB != nil {
							return *bv.VpusPerGB
						}
						return 0
					}(),
					"state": string(bv.LifecycleState),
				}
			}
		}
	}
	// Get shape config
	shapeCfg := map[string]interface{}{}
	if inst.ShapeConfig != nil {
		shapeCfg["ocpus"] = func() float32 {
			if inst.ShapeConfig.Ocpus != nil {
				return *inst.ShapeConfig.Ocpus
			}
			return 0
		}()
		shapeCfg["memory_gb"] = func() float32 {
			if inst.ShapeConfig.MemoryInGBs != nil {
				return *inst.ShapeConfig.MemoryInGBs
			}
			return 0
		}()
	}
	jsonOK(w, map[string]interface{}{
		"id":                  strOr(inst.Id, ""),
		"display_name":        strOr(inst.DisplayName, ""),
		"shape":               strOr(inst.Shape, ""),
		"state":               string(inst.LifecycleState),
		"region":              strOr(inst.Region, ""),
		"availability_domain": strOr(inst.AvailabilityDomain, ""),
		"fault_domain":        strOr(inst.FaultDomain, ""),
		"time_created": func() string {
			if inst.TimeCreated != nil {
				return inst.TimeCreated.Format(time.RFC3339)
			}
			return ""
		}(),
		"shape_config": shapeCfg,
		"vnic":         vnicInfo,
		"boot_volume":  bootVolumeInfo,
	})
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
	// Sanitize the password: strip newlines and other control characters
	// that could break the YAML block scalar and inject shell commands.
	// Then wrap in single quotes in the YAML chpasswd list to avoid
	// injection when password contains ':' or other YAML-significant chars.
	// Escape any single quotes in the password: ' → '\''.
	sanitized := strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == '\t' || r == 0 {
			return -1 // drop
		}
		if r < 32 {
			return -1 // drop other control chars
		}
		return r
	}, password)
	quotedPwd := "'" + strings.ReplaceAll(sanitized, "'", `'\''`) + "'"
	return "#cloud-config\n" +
		"ssh_pwauth: yes\n" +
		"chpasswd:\n" +
		"  list: |\n" +
		"    root:" + quotedPwd + "\n" +
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
