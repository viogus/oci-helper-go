package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/viogus/oci-helper-go/internal/db"
)

func (s *Server) handleInstancePlans(w http.ResponseWriter, r *http.Request) {
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
		list, total, err := s.store.ListInstancePlansPaginated(tenantID, keyword, page, size)
		if err != nil {
			jsonErr(w, "list plans: "+err.Error())
			return
		}
		if list == nil {
			list = []db.InstancePlan{}
		}
		jsonOK(w, map[string]interface{}{"data": list, "total": total, "page": page, "size": size})

	case http.MethodPost:
		var req struct {
			Name              string  `json:"name"`
			TenantID          int64   `json:"tenant_id"`
			Shape             string  `json:"shape"`
			ImageID           string  `json:"image_id"`
			SubnetID          string  `json:"subnet_id"`
			AvailabilityDomain string `json:"availability_domain"`
			BootVolumeSizeGB  int64   `json:"boot_volume_size_gb"`
			OCPUs             float64 `json:"ocpus"`
			MemoryGB          float64 `json:"memory_gb"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonErr(w, "invalid body: "+err.Error())
			return
		}
		if req.Name == "" || req.TenantID == 0 {
			jsonErr(w, "name and tenant_id required")
			return
		}
		p := &db.InstancePlan{
			Name:              req.Name,
			TenantID:          req.TenantID,
			Shape:             req.Shape,
			ImageID:           req.ImageID,
			SubnetID:          req.SubnetID,
			AvailabilityDomain: req.AvailabilityDomain,
			BootVolumeSizeGB:  req.BootVolumeSizeGB,
			OCPUs:             req.OCPUs,
			MemoryGB:          req.MemoryGB,
		}
		if err := s.store.CreateInstancePlan(p); err != nil {
			jsonErr(w, "create plan: "+err.Error())
			return
		}
		s.audit(req.TenantID, "instance-plan:create", req.Name, r)
		jsonOK(w, p)

	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleInstancePlanByID(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/api/instance-plans/")
	idStr = strings.TrimSuffix(idStr, "/")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		jsonErr(w, "invalid plan id")
		return
	}

	switch r.Method {
	case http.MethodPut:
		var req struct {
			Name              string  `json:"name"`
			Shape             string  `json:"shape"`
			ImageID           string  `json:"image_id"`
			SubnetID          string  `json:"subnet_id"`
			AvailabilityDomain string `json:"availability_domain"`
			BootVolumeSizeGB  int64   `json:"boot_volume_size_gb"`
			OCPUs             float64 `json:"ocpus"`
			MemoryGB          float64 `json:"memory_gb"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonErr(w, "invalid body: "+err.Error())
			return
		}
		p := &db.InstancePlan{
			ID:                id,
			Name:              req.Name,
			Shape:             req.Shape,
			ImageID:           req.ImageID,
			SubnetID:          req.SubnetID,
			AvailabilityDomain: req.AvailabilityDomain,
			BootVolumeSizeGB:  req.BootVolumeSizeGB,
			OCPUs:             req.OCPUs,
			MemoryGB:          req.MemoryGB,
		}
		if err := s.store.UpdateInstancePlan(p); err != nil {
			jsonErr(w, "update plan: "+err.Error())
			return
		}
		s.audit(0, "instance-plan:update", fmt.Sprintf("plan:%d", id), r)
		jsonOK(w, p)

	case http.MethodDelete:
		if err := s.store.DeleteInstancePlan(id); err != nil {
			jsonErr(w, "delete plan: "+err.Error())
			return
		}
		s.audit(0, "instance-plan:delete", fmt.Sprintf("plan:%d", id), r)
		jsonOK(w, map[string]string{"status": "ok"})

	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}
