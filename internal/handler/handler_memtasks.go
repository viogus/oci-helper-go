package handler

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"sync"
	"time"
)

type memTask struct {
	ID           string   `json:"id"`
	TenantID     int64    `json:"tenant_id"`
	InstanceID   string   `json:"instance_id"`
	InstanceName string   `json:"instance_name"`
	Username     string   `json:"username"`
	Region       string   `json:"region"`
	CidrList     []string `json:"cidr_list"`
	Ocpus        string   `json:"ocpus"`
	Memory       string   `json:"memory"`
	Shape        string   `json:"shape"`
	TaskType     string   `json:"task_type"` // "change_ip" or "update_cfg"
	Paused       bool     `json:"paused"`
	Attempts     int64    `json:"attempts"`
	CreatedAt    string   `json:"created_at"`
	Cancel       chan struct{} `json:"-"`
}

var (
	memTasks   = make(map[string]*memTask)
	memTasksMu sync.Mutex
)

func (s *Server) handleMemTasksChangeIP(w http.ResponseWriter, r *http.Request) {
	s.handleMemTasks(w, r, "change_ip")
}

func (s *Server) handleMemTasksUpdateCfg(w http.ResponseWriter, r *http.Request) {
	s.handleMemTasks(w, r, "update_cfg")
}

func (s *Server) handleMemTasks(w http.ResponseWriter, r *http.Request, taskType string) {
	switch r.Method {
	case http.MethodGet:
		memTasksMu.Lock()
		var all []*memTask
		for _, t := range memTasks {
			if t.TaskType == taskType {
				all = append(all, t)
			}
		}
		memTasksMu.Unlock()

		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		if page < 1 {
			page = 1
		}
		size, _ := strconv.Atoi(r.URL.Query().Get("size"))
		if size < 1 {
			size = 20
		}
		total := len(all)
		start := (page - 1) * size
		if start > total {
			start = total
		}
		end := start + size
		if end > total {
			end = total
		}
		list := all[start:end]
		if list == nil {
			list = []*memTask{}
		}

		jsonOK(w, map[string]interface{}{
			"data":  list,
			"total": total,
			"page":  page,
			"size":  size,
		})

	case http.MethodPost:
		var req struct {
			Action     string   `json:"action"`
			TenantID   int64    `json:"tenant_id"`
			InstanceID string   `json:"instance_id"`
			TaskIDs    []string `json:"task_ids"`
			CidrList   []string `json:"cidr_list"`
			Ocpus      string   `json:"ocpus"`
			Memory     string   `json:"memory"`
			Shape      string   `json:"shape"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonErr(w, "invalid body: "+err.Error())
			return
		}

		switch req.Action {
		case "add":
			id := generateID()
			task := &memTask{
				ID:         id,
				TenantID:   req.TenantID,
				InstanceID: req.InstanceID,
				CidrList:   req.CidrList,
				Ocpus:      req.Ocpus,
				Memory:     req.Memory,
				Shape:      req.Shape,
				TaskType:   taskType,
				CreatedAt:  time.Now().Format("2006-01-02 15:04:05"),
				Cancel:     make(chan struct{}),
			}
			// Get tenant info for display
			if tenant, err := s.store.GetTenant(req.TenantID); err == nil && tenant != nil {
				task.Username = tenant.Name
				task.Region = tenant.Region
			}
			memTasksMu.Lock()
			memTasks[id] = task
			memTasksMu.Unlock()

			// Start background retry loop
			if taskType == "change_ip" {
				go s.runChangeIPLoop(task)
			} else {
				go s.runUpdateCfgLoop(task)
			}
			s.audit(req.TenantID, "mem-task:add:"+taskType, req.InstanceID, r)
			jsonOK(w, map[string]string{"task_id": id, "status": "started"})

		case "pause":
			memTasksMu.Lock()
			for _, id := range req.TaskIDs {
				if t, ok := memTasks[id]; ok {
					t.Paused = true
				}
			}
			memTasksMu.Unlock()
			jsonOK(w, map[string]string{"status": "ok"})

		case "resume":
			memTasksMu.Lock()
			for _, id := range req.TaskIDs {
				if t, ok := memTasks[id]; ok {
					t.Paused = false
				}
			}
			memTasksMu.Unlock()
			jsonOK(w, map[string]string{"status": "ok"})

		case "delete":
			memTasksMu.Lock()
			for _, id := range req.TaskIDs {
				if t, ok := memTasks[id]; ok {
					close(t.Cancel)
					delete(memTasks, id)
				}
			}
			memTasksMu.Unlock()
			jsonOK(w, map[string]string{"status": "ok"})

		default:
			jsonErr(w, "unknown action: "+req.Action)
		}
	default:
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) runChangeIPLoop(task *memTask) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-task.Cancel:
			return
		case <-s.stopping:
			return
		case <-ticker.C:
			memTasksMu.Lock()
			if task.Paused {
				memTasksMu.Unlock()
				continue
			}
			task.Attempts++
			memTasksMu.Unlock()

			tenant, err := s.store.GetTenant(task.TenantID)
			if err != nil || tenant == nil {
				log.Printf("[mem-task] change-ip: tenant %d not found, removing task %s", task.TenantID, task.ID)
				memTasksMu.Lock()
				delete(memTasks, task.ID)
				memTasksMu.Unlock()
				return
			}
			client, err := s.clientFor(tenant)
			if err != nil {
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			newIP, err := client.ChangeInstanceIP(ctx, task.InstanceID, task.CidrList)
			cancel()
			if err == nil {
				log.Printf("[mem-task] change-ip done: %s -> %s", task.InstanceID, newIP)
				memTasksMu.Lock()
				delete(memTasks, task.ID)
				memTasksMu.Unlock()
				return // success, exit loop
			}
		}
	}
}

func (s *Server) runUpdateCfgLoop(task *memTask) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-task.Cancel:
			return
		case <-s.stopping:
			return
		case <-ticker.C:
			memTasksMu.Lock()
			if task.Paused {
				memTasksMu.Unlock()
				continue
			}
			task.Attempts++
			memTasksMu.Unlock()

			tenant, err := s.store.GetTenant(task.TenantID)
			if err != nil || tenant == nil {
				log.Printf("[mem-task] update-cfg: tenant %d not found, removing task %s", task.TenantID, task.ID)
				memTasksMu.Lock()
				delete(memTasks, task.ID)
				memTasksMu.Unlock()
				return
			}
			client, err := s.clientFor(tenant)
			if err != nil {
				continue
			}
			var ocpu, mem float32
			var parseFail bool
			if task.Ocpus != "" {
				o, err := strconv.ParseFloat(task.Ocpus, 32)
				if err != nil || o <= 0 {
					parseFail = true
				} else {
					ocpu = float32(o)
				}
			}
			if task.Memory != "" {
				m, err := strconv.ParseFloat(task.Memory, 32)
				if err != nil || m <= 0 {
					parseFail = true
				} else {
					mem = float32(m)
				}
			}
			if parseFail {
				continue
			}
			ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
			err = client.UpdateInstance(ctx, task.InstanceID, task.Shape, ocpu, mem)
			cancel()
			if err == nil {
				log.Printf("[mem-task] update-cfg done: %s", task.InstanceID)
				memTasksMu.Lock()
				delete(memTasks, task.ID)
				memTasksMu.Unlock()
				return
			}
		}
	}
}

func generateID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		// fallback to timestamp if rand fails (should never happen)
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return fmt.Sprintf("%x", b)
}
