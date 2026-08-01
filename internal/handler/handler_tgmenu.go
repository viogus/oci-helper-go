package handler

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/viogus/oci-helper-go/internal/ai"
	"github.com/viogus/oci-helper-go/internal/auth"
	"github.com/viogus/oci-helper-go/internal/db"
	"github.com/viogus/oci-helper-go/internal/telegram"
	gossh "golang.org/x/crypto/ssh"
)

var (
	tgAIWaiting    = map[int64]bool{}
	tgAIWaitingMu  sync.Mutex
	tgCreateStates = map[int64]*tgCreateState{}
	tgCreateMu     sync.Mutex
	tgSSHStates    = map[int64]bool{}
	tgSSHMu        sync.Mutex
	tgMFAStates    = map[int64]bool{}
	tgMFAMu        sync.Mutex
	tgVolUpdateStates = map[int64]*tgVolUpdateState{}
	tgVolUpdateMu     sync.Mutex
)

type tgCreateState struct {
	TenantID    int64
	Architecture string
	OCPUs       float64
	MemoryGB    float64
	Disk        int64
	Count       int
	Interval    int
	OS          string
	Password    string
	Step        string
}

type tgVolUpdateState struct {
	TenantID int64
	VolumeID string
}

// ── Keyboard builders ────────────────────────────────────────────────

func tgMainKeyboard() telegram.InlineKeyboardMarkup {
	return telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{{Text: "🖥 Instances", CallbackData: "instances:0"}, {Text: "📋 Tasks", CallbackData: "tasks:0"}},
			{{Text: "📊 Status", CallbackData: "status"}, {Text: "❓ Help", CallbackData: "help"}},
			{{Text: "🤖 AI Chat", CallbackData: "ai"}},
			{{Text: "🆕 Create Instance", CallbackData: "create"}},
			{{Text: "🔐 MFA", CallbackData: "mfa"}},
			{{Text: "🛡 Defense", CallbackData: "defense"}, {Text: "🚫 Blacklist", CallbackData: "blacklist"}},
			{{Text: "🔑 SSH Keys", CallbackData: "sshkeys"}, {Text: "📌 Version", CallbackData: "version"}},
			{{Text: "💾 Backup", CallbackData: "backup"}, {Text: "📈 Traffic", CallbackData: "traffic"}},
			{{Text: "💿 Volumes", CallbackData: "volumes"}, {Text: "📋 Plans", CallbackData: "plans"}},
			{{Text: "🖥 VNC", CallbackData: "vnc"}},
			{{Text: "📜 Logs", CallbackData: "logs"}, {Text: "💓 CheckAlive", CallbackData: "checkalive"}},
			{{Text: "⚙️ Configs", CallbackData: "cfg:list"}},
		},
	}
}

type tgInstanceShort struct {
	ID   string
	Name string
}

func tgInstanceKeyboard(items []tgInstanceShort, page, totalPages int) telegram.InlineKeyboardMarkup {
	var kb [][]telegram.InlineKeyboardButton
	start := page * 8
	end := start + 8
	if end > len(items) {
		end = len(items)
	}
	for i := start; i < end; i++ {
		label := items[i].Name
		if len(label) > 30 {
			label = label[:30] + "…"
		}
		kb = append(kb, []telegram.InlineKeyboardButton{
			{Text: label, CallbackData: fmt.Sprintf("instances:detail:%s", items[i].ID)},
		})
	}
	var navRow []telegram.InlineKeyboardButton
	if page > 0 {
		navRow = append(navRow, telegram.InlineKeyboardButton{Text: "⬅️ Prev", CallbackData: fmt.Sprintf("instances:%d", page-1)})
	}
	navRow = append(navRow, telegram.InlineKeyboardButton{Text: "\U0001f519 Main", CallbackData: "main"})
	if page < totalPages-1 {
		navRow = append(navRow, telegram.InlineKeyboardButton{Text: "Next ➡️", CallbackData: fmt.Sprintf("instances:%d", page+1)})
	}
	kb = append(kb, navRow)
	return telegram.InlineKeyboardMarkup{InlineKeyboard: kb}
}

func tgInstanceActionKeyboard(instanceID string) telegram.InlineKeyboardMarkup {
	return telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{
				{Text: "▶️ Start", CallbackData: fmt.Sprintf("instances:action:%s:start", instanceID)},
				{Text: "⏹ Stop", CallbackData: fmt.Sprintf("instances:action:%s:stop", instanceID)},
			},
			{
				{Text: "\U0001f504 Reboot", CallbackData: fmt.Sprintf("instances:action:%s:reboot", instanceID)},
				{Text: "\U0001f504 Soft", CallbackData: fmt.Sprintf("instances:action:%s:softreset", instanceID)},
			},
			{{Text: "\U0001f519 Back", CallbackData: "instances:0"}},
		},
	}
}

type tgTaskShort struct {
	ID     int64
	Type   string
	Status string
}

func tgTaskKeyboard(items []tgTaskShort, page, totalPages int) telegram.InlineKeyboardMarkup {
	var kb [][]telegram.InlineKeyboardButton
	start := page * 8
	end := start + 8
	if end > len(items) {
		end = len(items)
	}
	for i := start; i < end; i++ {
		label := fmt.Sprintf("#%d %s [%s]", items[i].ID, items[i].Type, items[i].Status)
		kb = append(kb, []telegram.InlineKeyboardButton{
			{Text: label, CallbackData: fmt.Sprintf("tasks:detail:%d", items[i].ID)},
		})
	}
	var navRow []telegram.InlineKeyboardButton
	if page > 0 {
		navRow = append(navRow, telegram.InlineKeyboardButton{Text: "⬅️ Prev", CallbackData: fmt.Sprintf("tasks:%d", page-1)})
	}
	navRow = append(navRow, telegram.InlineKeyboardButton{Text: "\U0001f519 Main", CallbackData: "main"})
	if page < totalPages-1 {
		navRow = append(navRow, telegram.InlineKeyboardButton{Text: "Next ➡️", CallbackData: fmt.Sprintf("tasks:%d", page+1)})
	}
	kb = append(kb, navRow)
	return telegram.InlineKeyboardMarkup{InlineKeyboard: kb}
}

// ── Send helpers ─────────────────────────────────────────────────────

func tgSend(bot *telegram.Bot, chatID int64, msgID int, text string, kb *telegram.InlineKeyboardMarkup) error {
	if msgID == 0 {
		if kb != nil {
			return bot.SendKeyboard(chatID, text, *kb)
		}
		return bot.SendMessage(chatID, text)
	}
	return bot.EditMessageText(chatID, msgID, text, kb)
}

// ── Utility ──────────────────────────────────────────────────────────


// ── Callback router ──────────────────────────────────────────────────

func (s *Server) handleTGCallback(bot *telegram.Bot, chatID int64, messageID int, callbackID, data string) {
	_ = bot.AnswerCallbackQuery(callbackID, "")
	parts := strings.SplitN(data, ":", 4)
	action := parts[0]

	switch {
	// ── Exact actions (no sub-actions) ──
	case action == "main":
		kb := tgMainKeyboard()
		tgSend(bot, chatID, messageID, "oci-helper Bot — Main Menu\nSelect an option:", &kb)

	case action == "status":
		s.tgSendStatus(bot, chatID, messageID)

	case action == "help":
		helpText := `oci-helper Bot Commands:
/start - Main menu
/instances - List instances
/tasks - List tasks
/status - System status
/defense - Defense mode
/blacklist - IP blacklist
/sshkeys - SSH keys
/traffic - Instance traffic
/volumes - Boot volumes
/plans - Instance plans
/logs - Recent logs
/checkalive - Instance liveness
/cfg - Tenant configs`
		kb := tgMainKeyboard()
		tgSend(bot, chatID, messageID, helpText, &kb)

	case action == "ai":
		tgAIWaitingMu.Lock()
		tgAIWaiting[chatID] = true
		tgAIWaitingMu.Unlock()
		tgSend(bot, chatID, messageID, "Send me your question. Reply /cancel to stop.", nil)

	case action == "mfa":
		s.tgMFAMenu(bot, chatID, messageID)
	case action == "mfa" && len(parts) >= 2 && parts[1] == "enable":
		secret := auth.GenerateMFA()
		s.setConfig("mfa_secret", secret)
		s.setConfig("mfa_enabled", "false")
		uri := auth.TOTPURI(secret, s.cfg.Username, "oci-helper")
		tgSend(bot, chatID, messageID, fmt.Sprintf("MFA 密钥:\n%s\n\nURI:\n%s\n\n启用后需要输入一次验证码。", secret, uri), nil)
	case action == "mfa" && len(parts) >= 2 && parts[1] == "disable":
		tgMFAMu.Lock()
		tgMFAStates[chatID] = true
		tgMFAMu.Unlock()
		tgSend(bot, chatID, messageID, "回复当前 TOTP 验证码以关闭 MFA。", nil)

	case action == "create" && len(parts) == 1:
		s.tgCreateTenantList(bot, chatID, messageID)
	case action == "create" && len(parts) >= 2 && parts[1] == "tenant":
		tenantID, _ := strconv.ParseInt(parts[2], 10, 64)
		tgCreateMu.Lock()
		tgCreateStates[chatID] = &tgCreateState{TenantID: tenantID, Step: "arch"}
		tgCreateMu.Unlock()
		tgSend(bot, chatID, messageID, "回复架构: AMD / ARM / AMD_E5 / ARM_A2", nil)

	// ── Specific sub-action cases (MUST come before generic page cases) ──

	case action == "instances" && len(parts) >= 3 && parts[1] == "detail":
		s.tgSendInstanceDetail(bot, chatID, messageID, parts[2])

	case action == "instances" && len(parts) >= 4 && parts[1] == "action":
		instanceID, actionType := parts[2], parts[3]
		s.tgPerformAction(bot, chatID, messageID, callbackID, instanceID, actionType)
		s.tgSendInstanceDetail(bot, chatID, messageID, instanceID)

	case action == "tasks" && len(parts) >= 3 && parts[1] == "detail":
		taskID, _ := strconv.ParseInt(parts[2], 10, 64)
		s.tgSendTaskDetail(bot, chatID, messageID, taskID)

	// ── Defense ──
	case action == "defense" && len(parts) == 1:
		s.tgDefenseMenu(bot, chatID, messageID)
	case action == "defense" && len(parts) >= 2 && parts[1] == "enable":
		s.tgDefenseEnablePrompt(bot, chatID, messageID)
	case action == "defense" && len(parts) >= 2 && parts[1] == "disable":
		s.tgDefenseDisableConfirm(bot, chatID, messageID)

	// ── Blacklist ──
	case action == "blacklist" && len(parts) == 1:
		s.tgBlacklistMenu(bot, chatID, messageID)
	case action == "blacklist" && len(parts) >= 2 && parts[1] == "add":
		s.tgBlacklistAddPrompt(bot, chatID, messageID)
	case action == "blacklist" && len(parts) >= 3 && parts[1] == "remove":
		id, err := strconv.ParseInt(parts[2], 10, 64)
		if err != nil {
			kb := tgMainKeyboard()
			tgSend(bot, chatID, messageID, "Invalid blacklist entry ID", &kb)
			return
		}
		s.tgBlacklistRemoveID(bot, chatID, messageID, id)
	case action == "blacklist" && len(parts) >= 2 && parts[1] == "clear":
		if len(parts) >= 3 && parts[2] == "confirm" {
			s.tgBlacklistClear(bot, chatID, messageID)
		} else {
			s.tgBlacklistClearConfirm(bot, chatID, messageID)
		}

	// ── SSH Keys ──
	case action == "sshkeys" && len(parts) == 1:
		s.tgSSHKeysList(bot, chatID, messageID)
	case action == "sshkeys" && len(parts) >= 2 && parts[1] == "generate":
		s.tgSSHKeyGenerate(bot, chatID, messageID)

	// ── Backup ──
	case action == "backup":
		s.tgBackupTrigger(bot, chatID, messageID)

	// ── VNC ──
	case action == "vnc":
		instances, _ := s.store.ListInstances(0)
		var rows [][]telegram.InlineKeyboardButton
		for i, inst := range instances {
			if i >= 10 {
				break
			}
			name := inst.Name
			if name == "" {
				name = inst.ID
			}
			rows = append(rows, []telegram.InlineKeyboardButton{{
				Text: name, CallbackData: fmt.Sprintf("vnc:open:%s", inst.ID),
			}})
		}
		rows = append(rows, []telegram.InlineKeyboardButton{{Text: "Back", CallbackData: "main"}})
		tgSend(bot, chatID, messageID, "选择实例查看 VNC 信息（请在面板 /vnc 打开控制台）", &telegram.InlineKeyboardMarkup{InlineKeyboard: rows})
	case action == "vnc" && len(parts) >= 3 && parts[1] == "open":
		inst, _ := s.store.GetInstanceByID(parts[2])
		if inst == nil {
			tgSend(bot, chatID, messageID, "实例不存在，请先同步。", nil)
			return
		}
		text := fmt.Sprintf("实例 %s\n公网IP: %s", inst.Name, inst.PublicIP)
		if panelURL, _ := s.store.GetConfig("panel_url"); panelURL != "" {
			text += "\nWeb VNC: " + strings.TrimRight(panelURL, "/") + "/vnc?instance_id=" + url.QueryEscape(inst.ID)
		} else {
			text += "\n请在面板打开 /vnc 使用 Web 控制台（可配置 panel_url 生成直达链接）。"
		}
		tgSend(bot, chatID, messageID, text, nil)

	// ── Traffic ──
	case action == "traffic" && len(parts) == 1:
		s.tgTrafficChooseInstance(bot, chatID, messageID)
	case action == "traffic" && len(parts) >= 3 && parts[1] == "query":
		s.tgTrafficQuery(bot, chatID, messageID, parts[2])

	// ── Volumes ──
	case action == "volumes" && len(parts) == 1:
		s.tgVolumeList(bot, chatID, messageID)
	case action == "volumes" && len(parts) == 4 && parts[1] == "terminate":
		tenantID, _ := strconv.ParseInt(parts[2], 10, 64)
		volID := parts[3]
		kb := telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{{
			{Text: "Yes, Delete", CallbackData: fmt.Sprintf("volumes:terminate:confirm:%d:%s", tenantID, volID)},
			{Text: "Back", CallbackData: "volumes"},
		}}}
		tgSend(bot, chatID, messageID, "确认删除该引导卷？此操作不可恢复。", &kb)
	case action == "volumes" && len(parts) == 5 && parts[1] == "terminate" && parts[2] == "confirm":
		tenantID, _ := strconv.ParseInt(parts[3], 10, 64)
		volID := parts[4]
		s.tgVolumeTerminate(bot, chatID, messageID, tenantID, volID)
	case action == "volumes" && len(parts) == 4 && parts[1] == "update":
		tenantID, _ := strconv.ParseInt(parts[2], 10, 64)
		volID := parts[3]
		tgVolUpdateMu.Lock()
		tgVolUpdateStates[chatID] = &tgVolUpdateState{TenantID: tenantID, VolumeID: volID}
		tgVolUpdateMu.Unlock()
		tgSend(bot, chatID, messageID, "回复引导卷配置，空格分隔: size_gb vpus_per_gb", nil)

	// ── Plans ──
	case action == "plans":
		s.tgPlansList(bot, chatID, messageID)

	// ── Logs ──
	case action == "logs":
		s.tgLogTail(bot, chatID, messageID)

	// ── Version ──
	case action == "version":
		s.tgVersionInfo(bot, chatID, messageID)

	// ── CheckAlive ──
	case action == "checkalive" && len(parts) == 1:
		s.tgCheckAlivePrompt(bot, chatID, messageID)
	case action == "checkalive" && len(parts) >= 3 && parts[1] == "do":
		s.tgCheckAliveDo(bot, chatID, messageID, parts[2])

	// ── Configs ──
	case action == "cfg" && len(parts) >= 2 && parts[1] == "list":
		s.tgConfigList(bot, chatID, messageID)

	// ── Generic list/page cases (match LAST) ──

	case action == "instances" && len(parts) >= 1:
		page := 0
		if len(parts) > 1 {
			page, _ = strconv.Atoi(parts[1])
		}
		s.tgSendInstanceList(bot, chatID, messageID, page)

	case action == "tasks" && len(parts) >= 1:
		page := 0
		if len(parts) > 1 {
			page, _ = strconv.Atoi(parts[1])
		}
		s.tgSendTaskList(bot, chatID, messageID, page)
	}
}

func (s *Server) tgCreateTenantList(bot *telegram.Bot, chatID int64, messageID int) {
	tenants, err := s.store.ListTenants()
	if err != nil || len(tenants) == 0 {
		tgSend(bot, chatID, messageID, "没有可用的租户配置。", nil)
		return
	}
	var rows [][]telegram.InlineKeyboardButton
	for _, t := range tenants {
		rows = append(rows, []telegram.InlineKeyboardButton{{
			Text: t.Name, CallbackData: fmt.Sprintf("create:tenant:%d", t.ID),
		}})
	}
	rows = append(rows, []telegram.InlineKeyboardButton{{Text: "Back", CallbackData: "main"}})
	tgSend(bot, chatID, messageID, "选择租户：", &telegram.InlineKeyboardMarkup{InlineKeyboard: rows})
}

func (s *Server) tgCreateRespond(bot *telegram.Bot, chatID int64, text string) {
	tgCreateMu.Lock()
	st := tgCreateStates[chatID]
	if st == nil {
		tgCreateMu.Unlock()
		return
	}
	text = strings.TrimSpace(text)
	switch st.Step {
	case "arch":
		st.Architecture = strings.ToUpper(text)
		st.Step = "config"
		tgCreateMu.Unlock()
		bot.SendMessage(chatID, "回复配置，空格分隔: ocpus memory disk count interval")
	case "config":
		parts := strings.Fields(text)
		if len(parts) != 5 {
			tgCreateMu.Unlock()
			bot.SendMessage(chatID, "格式错误，请回复: ocpus memory disk count interval")
			return
		}
		ocpu, _ := strconv.ParseFloat(parts[0], 64)
		mem, _ := strconv.ParseFloat(parts[1], 64)
		disk, _ := strconv.ParseInt(parts[2], 10, 64)
		count, _ := strconv.Atoi(parts[3])
		interval, _ := strconv.Atoi(parts[4])
		if ocpu <= 0 || mem <= 0 || disk <= 0 || count <= 0 || interval < 30 {
			tgCreateMu.Unlock()
			bot.SendMessage(chatID, "配置数值不合法，请重新输入")
			return
		}
		st.OCPUs, st.MemoryGB, st.Disk, st.Count, st.Interval = ocpu, mem, disk, count, interval
		st.Step = "os"
		tgCreateMu.Unlock()
		bot.SendMessage(chatID, "回复系统: Ubuntu / Oracle Linux")
	case "os":
		st.OS = text
		st.Step = "password"
		tgCreateMu.Unlock()
		bot.SendMessage(chatID, "回复 root 密码")
	case "password":
		st.Password = text
		tenantID := st.TenantID
		task := &db.CreateTask{
			TenantID: tenantID, Region: "", OCPUs: st.OCPUs, MemoryGB: st.MemoryGB,
			Disk: st.Disk, Architecture: st.Architecture, IntervalSeconds: st.Interval,
			CreateNumbers: st.Count, OperationSystem: st.OS, RootPassword: st.Password,
		}
		tgCreateMu.Unlock()
		if err := s.store.CreateCreateTask(task); err != nil {
			bot.SendMessage(chatID, "创建定时任务失败: "+err.Error())
			return
		}
		tgCreateMu.Lock()
		delete(tgCreateStates, chatID)
		tgCreateMu.Unlock()
		bot.SendMessage(chatID, fmt.Sprintf("定时开机任务已创建 (ID=%d)", task.ID))
	}
}

func (s *Server) tgAIRespond(bot *telegram.Bot, chatID int64, text string) {
	if strings.TrimSpace(text) == "/cancel" {
		tgAIWaitingMu.Lock()
		delete(tgAIWaiting, chatID)
		tgAIWaitingMu.Unlock()
		bot.SendMessage(chatID, "AI chat cancelled.")
		return
	}
	apiKey, _ := s.store.GetConfig("siliconflow_key")
	if apiKey == "" {
		bot.SendMessage(chatID, "AI 未配置，请先在设置页填写 SiliconFlow API Key。")
		return
	}
	model, _ := s.store.GetConfig("siliconflow_model")
	client := ai.New(apiKey, model)
	reply, err := client.Chat([]ai.ChatMessage{{Role: "user", Content: text}})
	if err != nil {
		bot.SendMessage(chatID, "AI 请求失败: "+err.Error())
		return
	}
	bot.SendMessage(chatID, reply)
}

func (s *Server) tgMFAMenu(bot *telegram.Bot, chatID int64, messageID int) {
	enabled, _ := s.store.GetConfig("mfa_enabled")
	status := "未启用"
	if enabled == "true" {
		status = "已启用"
	}
	kb := telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{{
		{Text: "Enable", CallbackData: "mfa:enable"},
		{Text: "Disable", CallbackData: "mfa:disable"},
	}, {
		{Text: "Back", CallbackData: "main"},
	}}}
	tgSend(bot, chatID, messageID, fmt.Sprintf("MFA 状态: %s", status), &kb)
}

func (s *Server) tgMFADisable(bot *telegram.Bot, chatID int64, code string) {
	tgMFAMu.Lock()
	delete(tgMFAStates, chatID)
	tgMFAMu.Unlock()
	s.mfaCacheMu.RLock()
	secret := s.mfaCache.secret
	s.mfaCacheMu.RUnlock()
	if secret == "" || !auth.ValidateTOTP(secret, code) {
		bot.SendMessage(chatID, "验证码无效。")
		return
	}
	s.setConfig("mfa_enabled", "false")
	bot.SendMessage(chatID, "MFA 已关闭。")
}

// ── Instances ────────────────────────────────────────────────────────

func (s *Server) tgSendInstanceList(bot *telegram.Bot, chatID int64, messageID int, page int) {
	instances, err := s.store.ListInstances(0)
	if err != nil {
		tgSend(bot, chatID, messageID, "Error loading instances", nil)
		return
	}
	if len(instances) == 0 {
		kb := tgMainKeyboard()
		tgSend(bot, chatID, messageID, "No instances found.", &kb)
		return
	}
	var items []tgInstanceShort
	for _, inst := range instances {
		items = append(items, tgInstanceShort{ID: inst.ID, Name: inst.Name})
	}
	perPage := 8
	totalPages := (len(items) + perPage - 1) / perPage
	if page >= totalPages {
		page = totalPages - 1
	}
	text := fmt.Sprintf("Instances (%d) — Page %d/%d:", len(items), page+1, totalPages)
	kb := tgInstanceKeyboard(items, page, totalPages)
	tgSend(bot, chatID, messageID, text, &kb)
}

func (s *Server) tgSendInstanceDetail(bot *telegram.Bot, chatID int64, messageID int, instanceID string) {
	inst, err := s.store.GetInstanceByID(instanceID)
	if err != nil || inst == nil {
		kb := tgMainKeyboard()
		tgSend(bot, chatID, messageID, "Instance not found. Sync tenants first.", &kb)
		return
	}
	text := fmt.Sprintf("\U0001f4cc %s\nState: %s\nShape: %s\nOCPU: %.1f | Mem: %.1f GB\nPublic IP: %s\nPrivate IP: %s\nBoot Volume: %d GB",
		inst.Name, inst.State, inst.Shape, inst.OCPU, inst.MemoryGB,
		strOr(&inst.PublicIP, "-"), strOr(&inst.PrivateIP, "-"), inst.BootVolumeGB)
	kb := tgInstanceActionKeyboard(instanceID)
	tgSend(bot, chatID, messageID, text, &kb)
}

func (s *Server) tgPerformAction(bot *telegram.Bot, chatID int64, messageID int, callbackID, instanceID, action string) {
	inst, err := s.store.GetInstanceByID(instanceID)
	if err != nil || inst == nil {
		_ = bot.AnswerCallbackQuery(callbackID, "Instance not found")
		return
	}
	tenant, err := s.store.GetTenant(inst.TenantID)
	if err != nil || tenant == nil {
		_ = bot.AnswerCallbackQuery(callbackID, "Tenant not found")
		return
	}
	client, err := s.clientFor(tenant)
	if err != nil {
		_ = bot.AnswerCallbackQuery(callbackID, "OCI client error")
		return
	}
	parts := strings.SplitN(inst.OCID, ":", 2)
	ocid := parts[len(parts)-1]

	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	var actionEnum core.InstanceActionActionEnum
	switch action {
	case "start":
		actionEnum = core.InstanceActionActionStart
	case "stop":
		actionEnum = core.InstanceActionActionStop
	case "reboot":
		actionEnum = core.InstanceActionActionReset
	case "softreset":
		actionEnum = core.InstanceActionActionSoftreset
	default:
		_ = bot.AnswerCallbackQuery(callbackID, "Unknown action")
		return
	}
	_, err = client.InstanceAction(ctx, ocid, actionEnum)
	if err != nil {
		_ = bot.AnswerCallbackQuery(callbackID, fmt.Sprintf("Action failed: %v", err))
		return
	}
	_ = bot.AnswerCallbackQuery(callbackID, fmt.Sprintf("Instance %s: %s initiated", inst.Name, action))
	log.Printf("[tg] %s %s (tenant=%d, chat=%d)", action, instanceID, inst.TenantID, chatID)
}

// ── Tasks ────────────────────────────────────────────────────────────

func (s *Server) tgSendTaskList(bot *telegram.Bot, chatID int64, messageID int, page int) {
	tasks, err := s.store.ListTasks()
	if err != nil {
		tgSend(bot, chatID, messageID, "Error loading tasks", nil)
		return
	}
	if len(tasks) == 0 {
		kb := tgMainKeyboard()
		tgSend(bot, chatID, messageID, "No tasks found.", &kb)
		return
	}
	var items []tgTaskShort
	for _, t := range tasks {
		items = append(items, tgTaskShort{ID: t.ID, Type: t.Type, Status: t.Status})
	}
	perPage := 8
	totalPages := (len(items) + perPage - 1) / perPage
	if page >= totalPages {
		page = totalPages - 1
	}
	text := fmt.Sprintf("Tasks (%d) — Page %d/%d:", len(items), page+1, totalPages)
	kb := tgTaskKeyboard(items, page, totalPages)
	tgSend(bot, chatID, messageID, text, &kb)
}

func (s *Server) tgSendTaskDetail(bot *telegram.Bot, chatID int64, messageID int, taskID int64) {
	tasks, err := s.store.ListTasks()
	if err != nil {
		return
	}
	for _, t := range tasks {
		if t.ID == taskID {
			text := fmt.Sprintf("\U0001f4cb Task #%d\nType: %s\nStatus: %s\nProgress: %d%%\nMessage: %s\nCreated: %s",
				t.ID, t.Type, t.Status, t.Progress, t.Message, t.CreatedAt.Format("2006-01-02 15:04"))
			bk := telegram.InlineKeyboardMarkup{
				InlineKeyboard: [][]telegram.InlineKeyboardButton{
					{{Text: "\U0001f519 Back", CallbackData: "tasks:0"}},
				},
			}
			tgSend(bot, chatID, messageID, text, &bk)
			return
		}
	}
	kb := tgMainKeyboard()
	tgSend(bot, chatID, messageID, "Task not found", &kb)
}

// ── Status ───────────────────────────────────────────────────────────

func (s *Server) tgSendStatus(bot *telegram.Bot, chatID int64, messageID int) {
	tenants, _ := s.store.ListTenants()
	instances, _ := s.store.ListInstances(0)
	tasks, _ := s.store.ListTasks()

	running := 0
	for _, t := range tasks {
		if t.Status == "running" {
			running++
		}
	}
	runningInstances := 0
	for _, inst := range instances {
		if inst.State == "RUNNING" {
			runningInstances++
		}
	}
	text := fmt.Sprintf("\U0001f4ca System Status\n\n\U0001f539 Tenants: %d\n\U0001f5a5 Instances: %d (running: %d)\n\U0001f4cb Active Tasks: %d",
		len(tenants), len(instances), runningInstances, running)
	kb := tgMainKeyboard()
	tgSend(bot, chatID, messageID, text, &kb)
}

// ── Defense handlers ─────────────────────────────────────────────────

func (s *Server) tgDefenseMenu(bot *telegram.Bot, chatID int64, messageID int) {
	kb := telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{{Text: "\U0001f6e1 Enable", CallbackData: "defense:enable"}, {Text: "\U0001f6ab Disable", CallbackData: "defense:disable"}},
			{{Text: "\U0001f519 Back", CallbackData: "main"}},
		},
	}
	tgSend(bot, chatID, messageID, "\U0001f6e1 Defense Mode\n\nEnable: Block specified CIDRs via security list rules.\nDisable: Restore allow-all rule.\n\nUse web UI for setup: /defense", &kb)
}

func (s *Server) tgDefenseEnablePrompt(bot *telegram.Bot, chatID int64, messageID int) {
	text := "To enable defense, visit the web UI:\n→ /defense\n\nProvide: tenant, VCN, and CIDR blacklist (one per line)."
	kb := tgMainKeyboard()
	tgSend(bot, chatID, messageID, text, &kb)
}

func (s *Server) tgDefenseDisableConfirm(bot *telegram.Bot, chatID int64, messageID int) {
	text := "To disable defense, visit the web UI:\n→ /defense\n\nThis restores the allow-all ingress rule."
	kb := tgMainKeyboard()
	tgSend(bot, chatID, messageID, text, &kb)
}

// ── Blacklist handlers ───────────────────────────────────────────────

func (s *Server) tgBlacklistMenu(bot *telegram.Bot, chatID int64, messageID int) {
	allData, _ := s.store.ListIpData(0, "deny")
	kb := telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{{Text: "➕ Add", CallbackData: "blacklist:add"}, {Text: "\U0001f5d1 Clear All", CallbackData: "blacklist:clear"}},
			{{Text: "\U0001f519 Back", CallbackData: "main"}},
		},
	}
	if len(allData) == 0 {
		tgSend(bot, chatID, messageID, "\U0001f6ab Blacklist\n\nNo blocked IPs. Use web UI (/ip-pool) to manage.", &kb)
		return
	}
	text := fmt.Sprintf("\U0001f6ab Blacklist (%d entries)\n\n", len(allData))
	for i, d := range allData {
		if i >= 20 {
			text += fmt.Sprintf("\n... and %d more", len(allData)-20)
			break
		}
		text += fmt.Sprintf("• %s", d.CIDR)
		if d.Label != "" {
			text += fmt.Sprintf(" (%s)", d.Label)
		}
		text += "\n"
	}
	tgSend(bot, chatID, messageID, text, &kb)
}

func (s *Server) tgBlacklistAddPrompt(bot *telegram.Bot, chatID int64, messageID int) {
	text := "To add IPs to the blacklist, use the web UI:\n→ /ip-pool\n\nSelect type: Blacklist, then add CIDR entries."
	kb := tgMainKeyboard()
	tgSend(bot, chatID, messageID, text, &kb)
}

func (s *Server) tgBlacklistRemoveID(bot *telegram.Bot, chatID int64, messageID int, id int64) {
	if err := s.store.DeleteIpData(id); err != nil {
		tgSend(bot, chatID, messageID, fmt.Sprintf("Failed to remove: %v", err), nil)
		return
	}
	s.tgBlacklistMenu(bot, chatID, messageID)
}

func (s *Server) tgBlacklistClearConfirm(bot *telegram.Bot, chatID int64, messageID int) {
	kb := telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "Yes, Clear All", CallbackData: "blacklist:clear:confirm"}},
		{{Text: "Cancel", CallbackData: "blacklist"}},
	}}
	tgSend(bot, chatID, messageID, "Are you sure you want to clear ALL blacklist entries? This cannot be undone.", &kb)
}

func (s *Server) tgBlacklistClear(bot *telegram.Bot, chatID int64, messageID int) {
	allData, _ := s.store.ListIpData(0, "deny")
	count := 0
	for _, d := range allData {
		if err := s.store.DeleteIpData(d.ID); err == nil {
			count++
		}
	}
	kb := tgMainKeyboard()
	tgSend(bot, chatID, messageID, fmt.Sprintf("Cleared %d blacklist entries.", count), &kb)
}

// ── SSH Keys handlers ────────────────────────────────────────────────

func (s *Server) tgSSHKeysList(bot *telegram.Bot, chatID int64, messageID int) {
	keys, _ := s.store.ListSSHKeys(0)
	kb := telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{{Text: "\U0001f511 Generate", CallbackData: "sshkeys:generate"}},
			{{Text: "\U0001f519 Back", CallbackData: "main"}},
		},
	}
	if len(keys) == 0 {
		tgSend(bot, chatID, messageID, "\U0001f511 SSH Keys\n\nNo keys found. Generate one or upload a PEM file via web UI (/ssh-keys).", &kb)
		return
	}
	text := fmt.Sprintf("\U0001f511 SSH Keys (%d)\n\n", len(keys))
	for _, k := range keys {
		fp := k.Fingerprint
		if len(fp) > 16 {
			fp = fp[:16] + "..."
		}
		keyType := "ED25519"
		if strings.Contains(k.PublicKey, "RSA") {
			keyType = "RSA"
		}
		text += fmt.Sprintf("• %s (%s)\n  FP: %s\n", k.Name, keyType, fp)
	}
	tgSend(bot, chatID, messageID, text, &kb)
}

func (s *Server) tgSSHKeyGenerate(bot *telegram.Bot, chatID int64, messageID int) {
	tgSSHMu.Lock()
	tgSSHStates[chatID] = true
	tgSSHMu.Unlock()
	tgSend(bot, chatID, messageID, "回复 SSH Key 名称（生成 RSA 4096 密钥对）", nil)
}

func (s *Server) tgSSHGenerate(bot *telegram.Bot, chatID int64, name string) {
	tgSSHMu.Lock()
	delete(tgSSHStates, chatID)
	tgSSHMu.Unlock()
	pub, priv, err := rsaGenerate()
	if err != nil {
		bot.SendMessage(chatID, "生成失败: "+err.Error())
		return
	}
	encKey, err := s.getSSHEncryptionKey()
	if err != nil {
		bot.SendMessage(chatID, "获取加密密钥失败: "+err.Error())
		return
	}
	encrypted, err := encryptSSHPrivateKey(encKey, priv)
	if err != nil {
		bot.SendMessage(chatID, "加密私钥失败: "+err.Error())
		return
	}
	fingerprint := ""
	if pk, _, _, _, err := gossh.ParseAuthorizedKey(pub); err == nil {
		fingerprint = gossh.FingerprintSHA256(pk)
	}
	key := &db.SSHKey{Name: name, PublicKey: strings.TrimSpace(string(pub)), PrivateKey: encrypted, Fingerprint: fingerprint}
	if err := s.store.CreateSSHKey(key); err != nil {
		bot.SendMessage(chatID, "保存失败: "+err.Error())
		return
	}
	bot.SendMessage(chatID, fmt.Sprintf("✅ SSH Key 已生成\n名称: %s\n指纹: %s\n公钥:\n%s", key.Name, key.Fingerprint, string(pub)))
}

// ── Backup handler ───────────────────────────────────────────────────

func (s *Server) tgBackupTrigger(bot *telegram.Bot, chatID int64, messageID int) {
	text := "To create or restore backups, use the web UI:\n→ /backup\n\nBackups are AES-256-GCM encrypted."
	kb := tgMainKeyboard()
	tgSend(bot, chatID, messageID, text, &kb)
}

// ── Traffic handlers ─────────────────────────────────────────────────

func (s *Server) tgTrafficChooseInstance(bot *telegram.Bot, chatID int64, messageID int) {
	instances, _ := s.store.ListInstances(0)
	if len(instances) == 0 {
		kb := tgMainKeyboard()
		tgSend(bot, chatID, messageID, "No instances found. Sync tenants first.", &kb)
		return
	}
	var items []tgInstanceShort
	for _, inst := range instances {
		items = append(items, tgInstanceShort{ID: inst.ID, Name: inst.Name})
	}
	var trafficKB [][]telegram.InlineKeyboardButton
	start := 0
	end := 8
	if end > len(items) {
		end = len(items)
	}
	for i := start; i < end; i++ {
		label := items[i].Name
		if len(label) > 30 {
			label = label[:30] + "…"
		}
		trafficKB = append(trafficKB, []telegram.InlineKeyboardButton{
			{Text: label, CallbackData: fmt.Sprintf("traffic:query:%s", items[i].ID)},
		})
	}
	trafficKB = append(trafficKB, []telegram.InlineKeyboardButton{
		{Text: "\U0001f519 Back", CallbackData: "main"},
	})
	tgSend(bot, chatID, messageID, fmt.Sprintf("\U0001f4c8 Traffic — Select an instance (%d total):", len(items)), &telegram.InlineKeyboardMarkup{InlineKeyboard: trafficKB})
}

func (s *Server) tgTrafficQuery(bot *telegram.Bot, chatID int64, messageID int, instanceID string) {
	inst, err := s.store.GetInstanceByID(instanceID)
	if err != nil || inst == nil {
		kb := tgMainKeyboard()
		tgSend(bot, chatID, messageID, "Instance not found. Try syncing first.", &kb)
		return
	}
	tenant, _ := s.store.GetTenant(inst.TenantID)
	if tenant == nil {
		kb := tgMainKeyboard()
		tgSend(bot, chatID, messageID, "Tenant not found for this instance.", &kb)
		return
	}
	client, err := s.clientFor(tenant)
	if err != nil {
		kb := tgMainKeyboard()
		tgSend(bot, chatID, messageID, fmt.Sprintf("OCI client error: %v", err), &kb)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Strip composite ID prefix to get OCID
	ocid := instanceID
	if i := strings.IndexByte(instanceID, ':'); i >= 0 {
		ocid = instanceID[i+1:]
	}

	// Get VNICs for the instance
	vnics, err := client.GetInstanceVNICs(ctx, tenant.TenancyOCID, ocid)
	if err != nil || len(vnics) == 0 {
		kb := tgMainKeyboard()
		tgSend(bot, chatID, messageID, fmt.Sprintf("No VNIC found: %v", err), &kb)
		return
	}
	firstVnic := vnics[0]
	if firstVnic.Id == nil {
		kb := tgMainKeyboard()
		tgSend(bot, chatID, messageID, "VNIC has no ID — cannot query traffic", &kb)
		return
	}
	vnicID := *firstVnic.Id
	vnicCompartment := tenant.TenancyOCID
	if firstVnic.CompartmentId != nil {
		vnicCompartment = *firstVnic.CompartmentId
	}

	endTime := time.Now()
	startTime := endTime.Add(-1 * time.Hour)
	vnicTraffic, err := client.GetVNICTtraffic(ctx, vnicCompartment, vnicID, startTime, endTime)
	if err != nil {
		kb := tgMainKeyboard()
		tgSend(bot, chatID, messageID, fmt.Sprintf("Traffic query failed: %v", err), &kb)
		return
	}
	text := fmt.Sprintf("\U0001f4c8 Traffic — %s (last hour)\n\n", inst.Name)
	if len(vnicTraffic) > 0 {
		last := vnicTraffic[len(vnicTraffic)-1]
		text += fmt.Sprintf("• Bytes In:  %.1f KB/s\n• Bytes Out: %.1f KB/s\n• Data points: %d",
			last.BytesInPerSec/1024, last.BytesOutPerSec/1024, len(vnicTraffic))
	} else {
		text += "No traffic data available."
	}
	kb := tgMainKeyboard()
	tgSend(bot, chatID, messageID, text, &kb)
}

// ── Volumes handler ──────────────────────────────────────────────────

func (s *Server) tgVolumeList(bot *telegram.Bot, chatID int64, messageID int) {
	tenants, _ := s.store.ListTenants()
	if len(tenants) == 0 {
		kb := tgMainKeyboard()
		tgSend(bot, chatID, messageID, "\U0001f4bf Boot Volumes\n\nNo tenants configured.", &kb)
		return
	}
	text := "\U0001f4bf Boot Volumes\n\n"
	totalVols := 0
	var rows [][]telegram.InlineKeyboardButton
	for _, t := range tenants {
		client, err := s.clientFor(&t)
		if err != nil {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		vols, err := client.ListBootVolumes(ctx, t.TenancyOCID)
		cancel()
		if err != nil {
			continue
		}
		for _, v := range vols {
			if totalVols >= 20 {
				break
			}
			volID := ""
			if v.Id != nil {
				volID = *v.Id
			}
			name := ""
			if v.DisplayName != nil {
				name = *v.DisplayName
			}
			size := int64(0)
			if v.SizeInGBs != nil {
				size = int64(*v.SizeInGBs)
			}
			state := ""
			if v.LifecycleState != "" {
				state = string(v.LifecycleState)
			}
			text += fmt.Sprintf("• %s — %d GB [%s] (%s)\n", name, size, state, t.Name)
			if volID != "" {
				rows = append(rows, []telegram.InlineKeyboardButton{
					{Text: "Delete " + name, CallbackData: fmt.Sprintf("volumes:terminate:%d:%s", t.ID, volID)},
					{Text: "Update " + name, CallbackData: fmt.Sprintf("volumes:update:%d:%s", t.ID, volID)},
				})
			}
			totalVols++
		}
		if totalVols >= 20 {
			break
		}
	}
	if totalVols == 0 {
		text += "No boot volumes found. Sync tenants first."
	}
	rows = append(rows, []telegram.InlineKeyboardButton{{Text: "\U0001f519 Back", CallbackData: "main"}})
	tgSend(bot, chatID, messageID, text, &telegram.InlineKeyboardMarkup{InlineKeyboard: rows})
}

func (s *Server) tgVolumeTerminate(bot *telegram.Bot, chatID int64, messageID int, tenantID int64, volID string) {
	t, err := s.store.GetTenant(tenantID)
	if err != nil || t == nil {
		tgSend(bot, chatID, messageID, "Tenant not found.", nil)
		return
	}
	client, err := s.clientFor(t)
	if err != nil {
		tgSend(bot, chatID, messageID, fmt.Sprintf("OCI client error: %v", err), nil)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := client.DeleteBootVolume(ctx, volID); err != nil {
		tgSend(bot, chatID, messageID, "删除失败: "+err.Error(), nil)
		return
	}
	tgSend(bot, chatID, messageID, "✅ 引导卷已删除", nil)
}

func (s *Server) tgVolumeUpdate(bot *telegram.Bot, chatID int64, text string) {
	tgVolUpdateMu.Lock()
	st := tgVolUpdateStates[chatID]
	delete(tgVolUpdateStates, chatID)
	tgVolUpdateMu.Unlock()
	if st == nil {
		return
	}
	parts := strings.Fields(text)
	if len(parts) != 2 {
		bot.SendMessage(chatID, "格式错误，请回复: size_gb vpus_per_gb")
		return
	}
	size, err1 := strconv.ParseInt(parts[0], 10, 64)
	vpus, err2 := strconv.ParseInt(parts[1], 10, 64)
	if err1 != nil || err2 != nil || size <= 0 || vpus <= 0 {
		bot.SendMessage(chatID, "数值不合法")
		return
	}
	t, err := s.store.GetTenant(st.TenantID)
	if err != nil || t == nil {
		bot.SendMessage(chatID, "租户不存在")
		return
	}
	client, err := s.clientFor(t)
	if err != nil {
		bot.SendMessage(chatID, "OCI client 错误: "+err.Error())
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if _, err := client.UpdateBootVolumeWithVPU(ctx, st.VolumeID, size, "", vpus); err != nil {
		bot.SendMessage(chatID, "更新失败: "+err.Error())
		return
	}
	bot.SendMessage(chatID, "✅ 引导卷配置已更新")
}

// ── Plans handler ────────────────────────────────────────────────────

func (s *Server) tgPlansList(bot *telegram.Bot, chatID int64, messageID int) {
	plans, _ := s.store.ListInstancePlans(0)
	kb := tgMainKeyboard()
	if len(plans) == 0 {
		tgSend(bot, chatID, messageID, "\U0001f4cb Instance Plans\n\nNo plans found. Create one via web UI (/instance-plans).", &kb)
		return
	}
	text := fmt.Sprintf("\U0001f4cb Instance Plans (%d)\n\n", len(plans))
	for _, p := range plans {
		text += fmt.Sprintf("• %s\n  %s | OCPU:%.0f Mem:%.0fGB Boot:%dGB\n", p.Name, p.Shape, p.OCPUs, p.MemoryGB, p.BootVolumeSizeGB)
	}
	tgSend(bot, chatID, messageID, text, &kb)
}

// ── Logs handler ─────────────────────────────────────────────────────

func (s *Server) tgLogTail(bot *telegram.Bot, chatID int64, messageID int) {
	logFile := s.cfg.LogFile
	if logFile == "" {
		logFile = os.Getenv("OCI_LOG_FILE")
	}
	if logFile == "" {
		kb := tgMainKeyboard()
		tgSend(bot, chatID, messageID, "Log file not configured. Set OCI_LOG_FILE env variable.", &kb)
		return
	}
	f, err := os.Open(logFile)
	if err != nil {
		kb := tgMainKeyboard()
		tgSend(bot, chatID, messageID, fmt.Sprintf("Cannot open log file: %v", err), &kb)
		return
	}
	defer f.Close()
	lines := readLastNLines(f, 20)
	text := "\U0001f4dc Recent Logs (last 20 lines)\n\n"
	if len(lines) == 0 {
		text += "(empty)"
	} else {
		for _, l := range lines {
			if len(l) > 80 {
				l = l[:80] + "..."
			}
			text += l + "\n"
		}
	}
	text = strings.TrimRight(text, "\n")
	kb := tgMainKeyboard()
	tgSend(bot, chatID, messageID, text, &kb)
}

// ── Version handler ──────────────────────────────────────────────────

func (s *Server) tgVersionInfo(bot *telegram.Bot, chatID int64, messageID int) {
	text := "\U0001f4cc oci-helper-go\n\nVersion: latest\nRepo: github.com/viogus/oci-helper-go\nStack: Go 1.26 + Vue 3 + SQLite\nOCI SDK: v65\n\nSingle binary, FROM scratch, 128MB RAM."
	kb := tgMainKeyboard()
	tgSend(bot, chatID, messageID, text, &kb)
}

// ── CheckAlive handlers ─────────────────────────────────────────────

func (s *Server) tgCheckAlivePrompt(bot *telegram.Bot, chatID int64, messageID int) {
	instances, _ := s.store.ListInstances(0)
	if len(instances) == 0 {
		kb := tgMainKeyboard()
		tgSend(bot, chatID, messageID, "No instances to check.", &kb)
		return
	}
	var aliveKB [][]telegram.InlineKeyboardButton
	for i, inst := range instances {
		if i >= 12 {
			break
		}
		label := inst.Name
		if len(label) > 30 {
			label = label[:30] + "…"
		}
		aliveKB = append(aliveKB, []telegram.InlineKeyboardButton{
			{Text: label, CallbackData: fmt.Sprintf("checkalive:do:%s", inst.ID)},
		})
	}
	aliveKB = append(aliveKB, []telegram.InlineKeyboardButton{
		{Text: "\U0001f519 Back", CallbackData: "main"},
	})
	tgSend(bot, chatID, messageID, fmt.Sprintf("\U0001f493 Check Alive — Select instance (%d total):", len(instances)), &telegram.InlineKeyboardMarkup{InlineKeyboard: aliveKB})
}

func (s *Server) tgCheckAliveDo(bot *telegram.Bot, chatID int64, messageID int, instanceID string) {
	inst, err := s.store.GetInstanceByID(instanceID)
	if err != nil || inst == nil {
		kb := tgMainKeyboard()
		tgSend(bot, chatID, messageID, "Instance not found.", &kb)
		return
	}
	status := "❌ DEAD"
	if inst.PublicIP != "" {
		conn, err := net.DialTimeout("tcp", net.JoinHostPort(inst.PublicIP, "22"), 5*time.Second)
		if err == nil {
			conn.Close()
			status = "✅ ALIVE"
		}
	}
	text := fmt.Sprintf("\U0001f493 Check Alive\n\n%s\nShape: %s\nStatus: %s\nPublic IP: %s",
		inst.Name, inst.Shape, status, strOr(&inst.PublicIP, "N/A"))
	kb := tgMainKeyboard()
	tgSend(bot, chatID, messageID, text, &kb)
}

// ── Configs handler ──────────────────────────────────────────────────

func (s *Server) tgConfigList(bot *telegram.Bot, chatID int64, messageID int) {
	tenants, _ := s.store.ListTenants()
	kb := tgMainKeyboard()
	if len(tenants) == 0 {
		tgSend(bot, chatID, messageID, "⚙️ Tenant Configs\n\nNo tenants configured.", &kb)
		return
	}
	text := fmt.Sprintf("⚙️ Tenant Configs (%d)\n\n", len(tenants))
	for _, t := range tenants {
		emoji := "✅"
		if t.Status == "error" {
			emoji = "❌"
		}
		text += fmt.Sprintf("%s %s (ID:%d)\n   Region: %s\n", emoji, t.Name, t.ID, t.Region)
	}
	tgSend(bot, chatID, messageID, text, &kb)
}
