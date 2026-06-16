package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/viogus/oci-helper-go/internal/db"
	"github.com/viogus/oci-helper-go/internal/dingtalk"
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

	// Try to read from configured log file
	logFile := s.cfg.LogFile
	if logFile == "" {
		logFile = os.Getenv("OCI_LOG_FILE")
	}
	if logFile != "" {
		data, err := os.ReadFile(logFile)
		if err == nil {
			lines := strings.Split(string(data), "\n")
			// Return last N lines
			start := 0
			if len(lines) > tail {
				start = len(lines) - tail
			}
			result := lines[start:]
			jsonOK(w, map[string]interface{}{
				"lines": result,
				"tail":  tail,
				"file":  logFile,
			})
			return
		}
	}

	// Fallback: try Docker-style location
	altPaths := []string{"/proc/1/fd/1", "/var/log/oci-helper.log", "/app/oci-helper/oci-helper.log"}
	for _, p := range altPaths {
		data, err := os.ReadFile(p)
		if err == nil && len(data) > 0 {
			lines := strings.Split(string(data), "\n")
			start := 0
			if len(lines) > tail {
				start = len(lines) - tail
			}
			jsonOK(w, map[string]interface{}{
				"lines": lines[start:],
				"tail":  tail,
				"file":  p,
			})
			return
		}
	}

	// No log file found — return docker hint
	jsonOK(w, map[string]interface{}{
		"lines": []string{"No log file found. Set OCI_LOG_FILE env var, or use: docker logs oci-helper"},
		"tail":  tail,
	})
}

func (s *Server) handleIPInfo(w http.ResponseWriter, r *http.Request) {
	jsonOK(w, map[string]string{"ip": r.RemoteAddr})
}

// --- DingTalk Notifications ---

func (s *Server) handleDingTalkNotify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	webhookURL, _ := s.store.GetConfig("dingtalk_webhook")
	if webhookURL == "" {
		jsonErr(w, "dingtalk_webhook not configured")
		return
	}
	var req struct {
		Content string `json:"content"`
		Title   string `json:"title"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}
	bot := dingtalk.New(webhookURL)
	if req.Title != "" {
		if err := bot.SendMarkdown(req.Title, req.Content); err != nil {
			jsonErr(w, "dingtalk: "+err.Error())
			return
		}
	} else {
		if err := bot.SendText(req.Content); err != nil {
			jsonErr(w, "dingtalk: "+err.Error())
			return
		}
	}
	s.audit(0, "dingtalk:notify", req.Content, r)
	jsonOK(w, map[string]string{"status": "ok"})
}

func (s *Server) handleDingTalkTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	webhookURL, _ := s.store.GetConfig("dingtalk_webhook")
	if webhookURL == "" {
		jsonErr(w, "dingtalk_webhook not configured")
		return
	}
	bot := dingtalk.New(webhookURL)
	if err := bot.SendText("oci-helper DingTalk notification test"); err != nil {
		jsonErr(w, "dingtalk test failed: "+err.Error())
		return
	}
	jsonOK(w, map[string]string{"status": "ok"})
}

// --- One-click self-update ---

func (s *Server) handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	// Query GitHub releases for the local project repo
	type releaseInfo struct {
		TagName     string `json:"tag_name"`
		PublishedAt string `json:"published_at"`
		HTMLURL     string `json:"html_url"`
		Body        string `json:"body"`
	}
	// Try the local repo first, fall back to reference project
	repos := []string{"viogus/oci-helper-go", "Yohann0617/oci-helper"}
	for _, repo := range repos {
		url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
		resp, err := http.Get(url)
		if err != nil {
			continue
		}
		var info releaseInfo
		if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
			resp.Body.Close()
			continue
		}
		resp.Body.Close()
		if info.TagName != "" {
			jsonOK(w, map[string]interface{}{
				"current_repo": repo,
				"latest":       info.TagName,
				"published_at": info.PublishedAt,
				"html_url":     info.HTMLURL,
				"body":         info.Body,
			})
			return
		}
	}
	jsonOK(w, map[string]string{"error": "no releases found"})
}

func (s *Server) handleUpdateNow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	// For a Docker-based deployment, the recommended update method is to pull a new image.
	// For non-Docker: download the release asset and replace the binary.
	// This endpoint provides the update instructions.
	s.audit(0, "update:trigger", "", r)
	jsonOK(w, map[string]interface{}{
		"status":  "update_instructions",
		"message": `To update in Docker: docker pull ghcr.io/viogus/oci-helper-go:latest && docker compose up -d. For binary: download from GitHub releases and replace the binary.`,
	})
}

// --- Settings helpers ---

func (s *Server) handleNotifyTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	// Test all configured notification channels
	results := map[string]string{}

	if webhookURL, _ := s.store.GetConfig("dingtalk_webhook"); webhookURL != "" {
		bot := dingtalk.New(webhookURL)
		if err := bot.SendText("oci-helper notification test"); err != nil {
			results["dingtalk"] = "failed: " + err.Error()
		} else {
			results["dingtalk"] = "ok"
		}
	}
	if token, _ := s.store.GetConfig("telegram_token"); token != "" {
		results["telegram"] = "configured (test via /start in bot)"
	}

	if len(results) == 0 {
		jsonErr(w, "no notification channels configured")
		return
	}
	jsonOK(w, map[string]interface{}{"results": results})
}
