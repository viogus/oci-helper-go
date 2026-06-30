package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/viogus/oci-helper-go/internal/db"
	"github.com/viogus/oci-helper-go/internal/dingtalk"
	"github.com/viogus/oci-helper-go/internal/telegram"
)

func (s *Server) handleGlance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	db := s.store.DB()

	var usersCount int64
	db.QueryRow("SELECT COUNT(*) FROM users").Scan(&usersCount)

	var tasksCount int64
	db.QueryRow("SELECT COUNT(*) FROM tasks").Scan(&tasksCount)

	var regionsCount int64
	db.QueryRow("SELECT COUNT(DISTINCT region) FROM tenants").Scan(&regionsCount)

	var tenantsCount int64
	db.QueryRow("SELECT COUNT(*) FROM tenants").Scan(&tenantsCount)

	var instancesCount int64
	db.QueryRow("SELECT COUNT(*) FROM instances").Scan(&instancesCount)

	var runningCount int64
	db.QueryRow("SELECT COUNT(*) FROM instances WHERE state = 'RUNNING'").Scan(&runningCount)

	// Add in-memory task count
	memTasksMu.Lock()
	memTasksCount := len(memTasks)
	memTasksMu.Unlock()

	totalTasks := tasksCount + int64(memTasksCount)
	days := int(time.Since(s.startTime).Hours() / 24)

	// Aggregate cities/map data from ip_data geolocation fields.
	cities := s.glanceCities()

	jsonOK(w, map[string]interface{}{
		"users":           usersCount,
		"tasks":           totalTasks,
		"regions":         regionsCount,
		"days":            days,
		"currentVersion":  version,
		"tenants":         tenantsCount,
		"instances":       instancesCount,
		"runningInstances": runningCount,
		"cities":          cities,
	})
}

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
	// Verify secret token header to prevent unauthorized webhook calls.
	// Set via: https://api.telegram.org/bot<TOKEN>/setWebhook?url=...&secret_token=<value>
	webhookSecret, _ := s.store.GetConfig("telegram_webhook_secret")
	if webhookSecret != "" {
		if r.Header.Get("X-Telegram-Bot-Api-Secret-Token") != webhookSecret {
			log.Printf("[telegram] webhook: invalid secret token from %s", maskIP(extractIP(r)))
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
	}
	var update telegram.Update
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		jsonErr(w, "invalid body")
		return
	}
	bot := telegram.New(token)

	// Handle callback queries (button clicks)
	if update.CallbackQuery != nil {
		s.handleTGCallback(bot, update.CallbackQuery.Message.Chat.ID,
			update.CallbackQuery.Message.MessageID,
			update.CallbackQuery.ID, update.CallbackQuery.Data)
		jsonOK(w, map[string]string{"status": "ok"})
		return
	}

	// Handle regular messages
	if update.Message != nil && update.Message.MessageID > 0 {
		chatID := update.Message.Chat.ID
		text := update.Message.Text

		switch {
		case text == "/start":
			kb := tgMainKeyboard()
			bot.SendKeyboard(chatID, "oci-helper Bot — Main Menu\nSelect an option:", kb)
		case text == "/instances":
			s.tgSendInstanceList(bot, chatID, 0, 0)
		case text == "/tasks":
			s.tgSendTaskList(bot, chatID, 0, 0)
		case text == "/status":
			tenants, _ := s.store.ListTenants()
			instances, _ := s.store.ListInstances(0)
			text := fmt.Sprintf("📊 Statistics\n\nTenants: %d\nInstances: %d", len(tenants), len(instances))
			bot.SendMessage(chatID, text)
		default:
			bot.SendMessage(chatID, "Unknown command. Use /start for main menu.")
		}
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
		Region      string `json:"region"`
		ServiceName string `json:"service_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}
	if req.TenantID == 0 {
		jsonErr(w, "tenant_id required")
		return
	}
	tenant, err := s.store.GetTenant(req.TenantID)
	if err != nil || tenant == nil {
		jsonErr(w, "tenant not found")
		return
	}
	// Use specified region or tenant default.
	if req.Region != "" {
		tenant.Region = req.Region
	}
	client, err := s.clientFor(tenant)
	if err != nil {
		jsonErr(w, "oci client: "+err.Error())
		return
	}
	limits, err := client.GetLimits(r.Context(), tenant.Region, req.ServiceName)
	if err != nil {
		jsonErr(w, "get limits: "+err.Error())
		return
	}
	jsonOK(w, map[string]interface{}{
		"total": len(limits),
		"items": limits,
	})
}

// GET /api/limits/services?tenant_id=X&region=Y
func (s *Server) handleLimitsServices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	tenantID, _ := strconv.ParseInt(r.URL.Query().Get("tenant_id"), 10, 64)
	region := r.URL.Query().Get("region")
	if tenantID == 0 || region == "" {
		jsonErr(w, "tenant_id and region required")
		return
	}
	tenant, err := s.store.GetTenant(tenantID)
	if err != nil || tenant == nil {
		jsonErr(w, "tenant not found")
		return
	}
	tenant.Region = region
	client, err := s.clientFor(tenant)
	if err != nil {
		jsonErr(w, "oci client: "+err.Error())
		return
	}
	services, err := client.ListServices(r.Context())
	if err != nil {
		jsonErr(w, "list services: "+err.Error())
		return
	}
	jsonOK(w, services)
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
		httpClient := &http.Client{Timeout: 15 * time.Second}
		resp, err := httpClient.Get(url)
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

// ── G11: Send Captcha ───────────────────────────────────────────────────

var (
	captchaStore   = make(map[string]captchaEntry)
	captchaStoreMu sync.Mutex
)

type captchaEntry struct {
	Code      string
	ExpiresAt time.Time
}

func (s *Server) handleCaptchaSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Recipient string `json:"recipient"` // "telegram" or "dingtalk"
		Target    string `json:"target"`    // chat_id or webhook override
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, "invalid body: "+err.Error())
		return
	}
	if req.Recipient == "" || req.Target == "" {
		jsonErr(w, "recipient and target required")
		return
	}

	// Generate 6-digit code
	code := fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)

	// Store in memory with 5-minute TTL
	captchaStoreMu.Lock()
	captchaStore[req.Target] = captchaEntry{
		Code:      code,
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}
	captchaStoreMu.Unlock()

	message := fmt.Sprintf("Your verification code is: %s (valid for 5 minutes)", code)

	switch req.Recipient {
	case "telegram":
		token, _ := s.store.GetConfig("telegram_token")
		if token == "" {
			jsonErr(w, "no notification channel configured")
			return
		}
		bot := telegram.New(token)
		// Parse target as chat ID (int64)
		chatID, err := strconv.ParseInt(req.Target, 10, 64)
		if err != nil {
			jsonErr(w, "invalid telegram chat_id: "+err.Error())
			return
		}
		if err := bot.SendMessage(chatID, message); err != nil {
			jsonErr(w, "telegram send: "+err.Error())
			return
		}
	case "dingtalk":
		webhookURL, _ := s.store.GetConfig("dingtalk_webhook")
		if webhookURL == "" {
			jsonErr(w, "no notification channel configured")
			return
		}
		bot := dingtalk.New(webhookURL)
		if err := bot.SendText(message); err != nil {
			jsonErr(w, "dingtalk send: "+err.Error())
			return
		}
	default:
		jsonErr(w, "unknown recipient type: "+req.Recipient+". Use telegram or dingtalk")
		return
	}

	s.audit(0, "captcha:send", req.Recipient, r)
	jsonOK(w, map[string]string{"status": "ok", "message": "captcha sent"})
}

// ── Glance Cities (IP geolocation map data) ───────────────────────────────

// glanceCity is a single map marker for the dashboard world map.
type glanceCity struct {
	Lat     float64 `json:"lat"`
	Lng     float64 `json:"lng"`
	Country string  `json:"country"`
	Area    string  `json:"area"`
	City    string  `json:"city"`
	Org     string  `json:"org"`
	Asn     string  `json:"asn"`
	Count   int     `json:"count"`
}

func (s *Server) glanceCities() []glanceCity {
	rows, err := s.store.DB().Query(`SELECT lat, lng, country, area, city, org, asn, COUNT(*) as cnt
		FROM ip_data
		WHERE lat != 0 AND lng != 0
		GROUP BY lat, lng
		ORDER BY cnt DESC`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var cities []glanceCity
	for rows.Next() {
		var c glanceCity
		if err := rows.Scan(&c.Lat, &c.Lng, &c.Country, &c.Area, &c.City, &c.Org, &c.Asn, &c.Count); err != nil {
			continue
		}
		cities = append(cities, c)
	}
	if cities == nil {
		cities = []glanceCity{}
	}
	return cities
}
