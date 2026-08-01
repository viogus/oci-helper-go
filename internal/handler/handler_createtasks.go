package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/oracle/oci-go-sdk/v65/common"
	"github.com/viogus/oci-helper-go/internal/db"
	ociclient "github.com/viogus/oci-helper-go/internal/oci"
)

// handleRecurringTasks lists or creates persisted recurring instance-creation
// schedules.
func (s *Server) handleRecurringTasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		tenantID, _ := strconv.ParseInt(r.URL.Query().Get("tenant_id"), 10, 64)
		list, err := s.store.ListCreateTasks(tenantID)
		if err != nil {
			jsonErr(w, "list recurring tasks: "+err.Error())
			return
		}
		if list == nil {
			list = []db.CreateTask{}
		}
		jsonOK(w, map[string]interface{}{"data": list, "total": len(list)})
	case http.MethodPost:
		var req struct {
			TenantID        int64   `json:"tenant_id"`
			Region          string  `json:"region"`
			OCPUs           float64 `json:"ocpus"`
			MemoryGB        float64 `json:"memory_gb"`
			Disk            int64   `json:"disk"`
			Architecture    string  `json:"architecture"`
			IntervalSeconds int     `json:"interval_seconds"`
			CreateNumbers   int     `json:"create_numbers"`
			OperationSystem string  `json:"operation_system"`
			RootPassword    string  `json:"root_password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonErr(w, "invalid body: "+err.Error())
			return
		}
		if req.TenantID == 0 {
			jsonErr(w, "tenant_id required")
			return
		}
		if req.IntervalSeconds < 30 {
			req.IntervalSeconds = 60
		}
		if req.CreateNumbers < 1 {
			req.CreateNumbers = 1
		}
		if req.OCPUs <= 0 {
			req.OCPUs = 1
		}
		if req.MemoryGB <= 0 {
			req.MemoryGB = 1
		}
		if req.Disk <= 0 {
			req.Disk = 50
		}
		if req.Architecture == "" {
			req.Architecture = "AMD"
		}
		if req.OperationSystem == "" {
			req.OperationSystem = "Ubuntu"
		}
		if req.RootPassword == "" {
			jsonErr(w, "root_password required")
			return
		}
		task := &db.CreateTask{
			TenantID:        req.TenantID,
			Region:          req.Region,
			OCPUs:           req.OCPUs,
			MemoryGB:        req.MemoryGB,
			Disk:            req.Disk,
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
		s.audit(req.TenantID, "create-task:create", fmt.Sprintf("schedule %d instances every %ds", req.CreateNumbers, req.IntervalSeconds), r)
		jsonOK(w, task)
	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

// handleRecurringTaskByID updates, pauses, resumes, stops, or deletes a
// recurring schedule.
func (s *Server) handleRecurringTaskByID(w http.ResponseWriter, r *http.Request) {
	subPath := strings.TrimPrefix(r.URL.Path, "/api/create-tasks/recurring/")
	subPath = strings.TrimSuffix(subPath, "/")
	parts := strings.SplitN(subPath, "/", 2)
	id, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || id <= 0 {
		jsonErr(w, "invalid recurring task id")
		return
	}
	if r.Method == http.MethodPut && len(parts) == 1 {
		var req db.CreateTask
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonErr(w, "invalid body: "+err.Error())
			return
		}
		req.ID = id
		if err := s.store.UpdateCreateTask(&req); err != nil {
			jsonErr(w, "update recurring task: "+err.Error())
			return
		}
		s.audit(req.TenantID, "create-task:update", strconv.FormatInt(id, 10), r)
		jsonOK(w, req)
		return
	}
	if r.Method == http.MethodPost && len(parts) == 2 {
		switch parts[1] {
		case "pause":
			if err := s.store.SetCreateTaskPaused(id, true); err != nil {
				jsonErr(w, "pause: "+err.Error())
				return
			}
		case "resume":
			if err := s.store.SetCreateTaskPaused(id, false); err != nil {
				jsonErr(w, "resume: "+err.Error())
				return
			}
		case "stop":
			if err := s.store.DeleteCreateTask(id); err != nil {
				jsonErr(w, "stop: "+err.Error())
				return
			}
		default:
			jsonErr(w, "unknown action: "+parts[1]+". Use pause|resume|stop")
			return
		}
		s.audit(0, "create-task:"+parts[1], strconv.FormatInt(id, 10), r)
		jsonOK(w, map[string]string{"status": "ok"})
		return
	}
	if r.Method == http.MethodDelete && len(parts) == 1 {
		if err := s.store.DeleteCreateTask(id); err != nil {
			jsonErr(w, "delete: "+err.Error())
			return
		}
		s.audit(0, "create-task:delete", strconv.FormatInt(id, 10), r)
		jsonOK(w, map[string]string{"status": "ok"})
		return
	}
	http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
}

// startCreateTaskScheduler drives persisted recurring schedules. It polls the
// DB every 5 seconds, so a 30+ second interval is honored with minimal drift.
func (s *Server) startCreateTaskScheduler() {
	log.Println("[create-task-scheduler] started")
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopping:
			log.Println("[create-task-scheduler] stopped")
			return
		case <-ticker.C:
			s.pumpCreateTasks()
		}
	}
}

func (s *Server) pumpCreateTasks() {
	tasks, err := s.store.ListActiveCreateTasks()
	if err != nil {
		log.Printf("[create-task-scheduler] list active tasks: %v", err)
		return
	}
	now := time.Now()
	for _, t := range tasks {
		if _, running := s.createTaskRunning.Load(t.ID); running {
			continue
		}
		if t.LastRunAt != nil && now.Sub(*t.LastRunAt) < time.Duration(t.IntervalSeconds)*time.Second {
			continue
		}
		s.createTaskRunning.Store(t.ID, true)
		_ = s.store.SetCreateTaskLastRun(t.ID)
		go func(task db.CreateTask) {
			defer s.createTaskRunning.Delete(task.ID)
			s.runCreateTask(task)
		}(t)
	}
}

func (s *Server) runCreateTask(task db.CreateTask) {
	tenant, err := s.store.GetTenant(task.TenantID)
	if err != nil || tenant == nil {
		log.Printf("[create-task] tenant %d not found", task.TenantID)
		_ = s.store.DeleteCreateTask(task.ID)
		return
	}
	client, err := s.clientFor(tenant)
	if err != nil {
		log.Printf("[create-task] client: %v", err)
		return
	}
	if task.Region != "" {
		client.SetRegion(task.Region)
	} else {
		task.Region = tenant.Region
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Minute)
	defer cancel()

	shape := ociclient.TaskShapeForArchitecture(task.Architecture)
	shapes, err := client.ListAllShapes(ctx, tenant.TenancyOCID)
	if err != nil {
		log.Printf("[create-task] list shapes: %v", err)
		return
	}
	shapeOK := false
	for _, s := range shapes {
		if s.Shape != nil && *s.Shape == shape {
			shapeOK = true
			break
		}
	}
	if !shapeOK {
		s.notify(task.TenantID, fmt.Sprintf("【开机任务】用户:[%s],区域:[%s] 不支持 CPU 架构:[%s]，任务已停止", tenant.Name, task.Region, task.Architecture))
		_ = s.store.DeleteCreateTask(task.ID)
		return
	}
	image, err := client.FindImageForOS(ctx, tenant.TenancyOCID, task.OperationSystem, shape)
	if err != nil {
		log.Printf("[create-task] find image: %v", err)
		return
	}
	if image == nil || image.Id == nil {
		log.Printf("[create-task] image has no id")
		return
	}
	subnet, err := client.EnsurePublicSubnet(ctx, tenant.TenancyOCID)
	if err != nil || subnet.Id == nil {
		s.notify(task.TenantID, fmt.Sprintf("【开机任务】用户:[%s],区域:[%s] 无有效公网 VCN，任务已停止", tenant.Name, task.Region))
		_ = s.store.DeleteCreateTask(task.ID)
		return
	}
	ads, err := client.ListAvailabilityDomains(ctx, tenant.TenancyOCID)
	if err != nil || len(ads) == 0 {
		log.Printf("[create-task] list ADs: %v", err)
		return
	}

	remaining := task.CreateNumbers
	success := 0
	authFailure := false
	for _, ad := range ads {
		if ad.Name == nil || remaining <= 0 {
			continue
		}
		for i := 0; i < remaining; i++ {
			displayName := fmt.Sprintf("%s-%s-%d", task.OperationSystem, tenant.Name, time.Now().UnixNano()/1e6)
			inst, err := client.LaunchTaskInstance(ctx, *ad.Name, shape, *image.Id, *subnet.Id,
				displayName, float32(task.OCPUs), float32(task.MemoryGB), task.Disk, task.RootPassword)
			if err != nil {
				var svcErr common.ServiceError
				if ok := errorAs(err, &svcErr); ok && (svcErr.GetHTTPStatusCode() == 401 || svcErr.GetHTTPStatusCode() == 403) {
					authFailure = true
					break
				}
				log.Printf("[create-task] launch %s failed: %v", displayName, err)
				continue
			}
			success++
			remaining--
			ocpu := task.OCPUs
			mem := task.MemoryGB
			if inst.ShapeConfig != nil {
				if inst.ShapeConfig.Ocpus != nil {
					ocpu = float64(*inst.ShapeConfig.Ocpus)
				}
				if inst.ShapeConfig.MemoryInGBs != nil {
					mem = float64(*inst.ShapeConfig.MemoryInGBs)
				}
			}
			_ = s.store.UpsertInstance(&db.Instance{
				ID:       fmt.Sprintf("%d:%s", task.TenantID, strOr(inst.Id, "")),
				TenantID: task.TenantID,
				Name:     strOr(inst.DisplayName, ""),
				OCID:     strOr(inst.Id, ""),
				Shape:    strOr(inst.Shape, ""),
				State:    string(inst.LifecycleState),
				Region:   task.Region,
				OCPU:     ocpu,
				MemoryGB: mem,
				BootVolumeGB: task.Disk,
			})
		}
	}

	if authFailure {
		s.notify(task.TenantID, fmt.Sprintf("【开机任务】用户:[%s],区域:[%s] 凭证失效或无权限，任务已停止", tenant.Name, task.Region))
		_ = s.store.DeleteCreateTask(task.ID)
		return
	}
	if success > 0 {
		s.notify(task.TenantID, fmt.Sprintf("【开机任务】用户:[%s],区域:[%s] 成功创建 %d 台实例", tenant.Name, task.Region, success))
	}
	if remaining <= 0 {
		_ = s.store.DeleteCreateTask(task.ID)
		return
	}
	if err := s.store.SetCreateTaskRemaining(task.ID, remaining); err != nil {
		log.Printf("[create-task] update remaining: %v", err)
	}
}

func errorAs(err error, target *common.ServiceError) bool {
	for err != nil {
		if se, ok := err.(common.ServiceError); ok {
			*target = se
			return true
		}
		u, ok := err.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}
