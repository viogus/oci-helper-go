package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/viogus/oci-helper-go/internal/db"
	"github.com/viogus/oci-helper-go/internal/telegram"
)

func (s *Server) handleTasks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	keyword := r.URL.Query().Get("keyword")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 { page = 1 }
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if size < 1 { size = 20 }
	list, total, err := s.store.ListTasksPaginated(keyword, page, size)
	if err != nil {
		jsonErr(w, "list tasks: "+err.Error())
		return
	}
	if list == nil { list = []db.Task{} }
	jsonOK(w, map[string]interface{}{"data": list, "total": total, "page": page, "size": size})
}

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	keyword := r.URL.Query().Get("keyword")
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 { page = 1 }
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if size < 1 { size = 20 }
	list, total, err := s.store.ListAuditPaginated(keyword, page, size)
	if err != nil {
		jsonErr(w, "list audit: "+err.Error())
		return
	}
	if list == nil { list = []db.AuditLog{} }
	jsonOK(w, map[string]interface{}{"data": list, "total": total, "page": page, "size": size})
}

func (s *Server) handleTelegramWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	token, _ := s.store.GetConfig("telegram_token")
	if token == "" {
		jsonErr(w, "telegram_token not configured")
		return
	}
	var update telegram.Update
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		jsonErr(w, "invalid body")
		return
	}
	if update.Message.MessageID == 0 {
		jsonOK(w, map[string]string{"status": "ignored"})
		return
	}
	bot := telegram.New(token)
	text := update.Message.Text
	chatID := update.Message.Chat.ID
	var reply string
	switch {
	case text == "/start":
		reply = "oci-helper Telegram Bot\n/instances - List\n/status - General status"
	case text == "/instances":
		instances, _ := s.store.ListInstances(0)
		infos := make([]telegram.InstanceInfo, 0, len(instances))
		for _, i := range instances {
			infos = append(infos, telegram.InstanceInfo{Name: i.Name, State: i.State, Shape: i.Shape, PublicIP: i.PublicIP, OCPU: i.OCPU, MemoryGB: i.MemoryGB})
		}
		reply = telegram.FormatInstances(infos)
	case text == "/status":
		tenants, _ := s.store.ListTenants()
		instances, _ := s.store.ListInstances(0)
		reply = fmt.Sprintf("Tenants: %d\nInstances: %d", len(tenants), len(instances))
	default:
		reply = "Unknown command. /start for help."
	}
	if err := bot.SendMessage(chatID, reply); err != nil {
		log.Printf("[telegram] send: %v", err)
		jsonErr(w, "send failed")
		return
	}
	jsonOK(w, map[string]string{"status": "ok"})
}

func (s *Server) handleLimits(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		TenantID    int64  `json:"tenant_id"`
		ServiceName string `json:"service_name"`
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
	limits, err := client.GetLimits(r.Context(), req.TenantID, req.ServiceName)
	if err != nil {
		jsonErr(w, "get limits: "+err.Error())
		return
	}
	jsonOK(w, limits)
}

func (s *Server) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	tailStr := r.URL.Query().Get("tail")
	tail := 100
	if n, err := strconv.Atoi(tailStr); err == nil && n > 0 && n <= 1000 {
		tail = n
	}
	jsonOK(w, map[string]interface{}{
		"lines": []string{"Logs: docker logs oci-helper"},
		"tail":  tail,
	})
}

func (s *Server) handleIPInfo(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"ip": r.RemoteAddr})
}
