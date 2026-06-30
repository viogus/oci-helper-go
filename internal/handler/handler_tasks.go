package handler

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/viogus/oci-helper-go/internal/db"
)

func (s *Server) handleBatchCreate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		TenantIDs          []int64 `json:"tenant_ids"`
		InstancesPerTenant int     `json:"instances_per_tenant"`
		Region             string  `json:"region"`
		Shape              string  `json:"shape"`
		ImageID            string  `json:"image_id"`
		SubnetID           string  `json:"subnet_id"`
		AvailabilityDomain string  `json:"availability_domain"`
		BootVolumeSizeGB   int64   `json:"boot_volume_size_gb"`
		DisplayNamePrefix  string  `json:"display_name_prefix"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}
	if len(req.TenantIDs) == 0 {
		jsonErr(w, "at least one tenant required")
		return
	}
	if req.InstancesPerTenant < 1 {
		req.InstancesPerTenant = 1
	}
	if req.BootVolumeSizeGB < 50 {
		req.BootVolumeSizeGB = 50
	}
	if req.DisplayNamePrefix == "" {
		req.DisplayNamePrefix = "oci-helper"
	}

	// Create one task per tenant
	var taskIDs []int64
	for _, tid := range req.TenantIDs {
		payload, _ := json.Marshal(map[string]interface{}{
			"tenant_id":            tid,
			"instances_per_tenant": req.InstancesPerTenant,
			"region":               req.Region,
			"shape":                req.Shape,
			"image_id":             req.ImageID,
			"subnet_id":            req.SubnetID,
			"availability_domain":  req.AvailabilityDomain,
			"boot_volume_size_gb":  req.BootVolumeSizeGB,
			"display_name_prefix":  req.DisplayNamePrefix,
		})
		task := &db.Task{
			TenantID: tid,
			Type:     "batch_create",
			Status:   "pending",
			Payload:  string(payload),
		}
		if err := s.store.CreateTask(task); err != nil {
			jsonErr(w, "create task: "+err.Error())
			return
		}
		taskIDs = append(taskIDs, task.ID)
	}
	s.audit(0, "batch-create:submit", strconv.Itoa(len(req.TenantIDs))+" tenants", r)
	jsonOK(w, map[string]interface{}{"task_ids": taskIDs, "status": "pending"})
}

func (s *Server) handleCreateTasks(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		keyword := r.URL.Query().Get("keyword")
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		size, _ := strconv.Atoi(r.URL.Query().Get("size"))
		if size < 1 {
			size = 20
		}
		// List all tasks (no DB-level pagination) then filter in memory
		// so pagination total reflects actual batch_create count.
		all, err := s.store.ListTasks()
		if err != nil {
			jsonErr(w, "list tasks: "+err.Error())
			return
		}
		var filtered []db.Task
		for _, t := range all {
			if t.Type == "batch_create" {
				if keyword != "" && !strings.Contains(strings.ToLower(t.Payload), strings.ToLower(keyword)) &&
					!strings.Contains(strings.ToLower(t.Status), strings.ToLower(keyword)) {
					continue
				}
				filtered = append(filtered, t)
			}
		}
		if filtered == nil {
			filtered = []db.Task{}
		}
		total := int64(len(filtered))
		// Apply pagination in memory
		start := (page - 1) * size
		if start > len(filtered) {
			start = len(filtered)
		}
		end := start + size
		if end > len(filtered) {
			end = len(filtered)
		}
		pageItems := filtered[start:end]
		jsonOK(w, map[string]interface{}{"data": pageItems, "total": total, "page": page, "size": size})

	case http.MethodPost:
		var req struct {
			Action  string  `json:"action"`
			TaskIDs []int64 `json:"task_ids"`
			TaskID  int64   `json:"task_id"`
			Payload string  `json:"payload"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonErr(w, "invalid body: "+err.Error())
			return
		}
		switch req.Action {
		case "stop":
			for _, id := range req.TaskIDs {
				s.store.UpdateTaskStatus(id, "cancelled", 0, "stopped by user")
			}
			s.audit(0, "create-tasks:stop", strings.Trim(strings.Join(strings.Fields(strconv.FormatInt(req.TaskIDs[0], 10)), ","), "[]"), r)
			jsonOK(w, map[string]string{"status": "ok"})
		case "pause":
			for _, id := range req.TaskIDs {
				s.store.UpdateTaskStatus(id, "paused", 0, "paused by user")
			}
			s.audit(0, "create-tasks:pause", strconv.Itoa(len(req.TaskIDs)), r)
			jsonOK(w, map[string]string{"status": "ok"})
		case "resume":
			for _, id := range req.TaskIDs {
				s.store.UpdateTaskStatus(id, "pending", 0, "resumed by user")
			}
			s.audit(0, "create-tasks:resume", strconv.Itoa(len(req.TaskIDs)), r)
			jsonOK(w, map[string]string{"status": "ok"})
		case "delete":
			for _, id := range req.TaskIDs {
				s.store.UpdateTaskStatus(id, "cancelled", 0, "deleted by user")
			}
			s.audit(0, "create-tasks:delete", strconv.Itoa(len(req.TaskIDs)), r)
			jsonOK(w, map[string]string{"status": "ok"})
		case "update":
			if req.TaskID > 0 && req.Payload != "" {
				if err := s.store.UpdateTaskPayload(req.TaskID, req.Payload); err != nil {
					jsonErr(w, "update task: "+err.Error())
					return
				}
				s.audit(0, "create-tasks:update", strconv.FormatInt(req.TaskID, 10), r)
				jsonOK(w, map[string]string{"status": "ok"})
			} else {
				jsonErr(w, "task_id and payload required for update")
			}
		default:
			jsonErr(w, "unknown action: "+req.Action)
		}

	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}
