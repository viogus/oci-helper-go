package handler

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/oracle/oci-go-sdk/v65/core"
	"github.com/viogus/oci-helper-go/internal/telegram"
)

// ── Keyboard builders ────────────────────────────────────────────────

func tgMainKeyboard() telegram.InlineKeyboardMarkup {
	return telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{{Text: "🖥 Instances", CallbackData: "instances:0"}, {Text: "📋 Tasks", CallbackData: "tasks:0"}},
			{{Text: "📊 Status", CallbackData: "status"}, {Text: "❓ Help", CallbackData: "help"}},
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
	navRow = append(navRow, telegram.InlineKeyboardButton{Text: "🔙 Main", CallbackData: "main"})
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
				{Text: "🔄 Reboot", CallbackData: fmt.Sprintf("instances:action:%s:reboot", instanceID)},
				{Text: "🔄 Soft", CallbackData: fmt.Sprintf("instances:action:%s:softreset", instanceID)},
			},
			{{Text: "🔙 Back", CallbackData: "instances:0"}},
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
	navRow = append(navRow, telegram.InlineKeyboardButton{Text: "🔙 Main", CallbackData: "main"})
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

// ── Callback router ──────────────────────────────────────────────────

func (s *Server) handleTGCallback(bot *telegram.Bot, chatID int64, messageID int, callbackID, data string) {
	_ = bot.AnswerCallbackQuery(callbackID, "")
	parts := strings.SplitN(data, ":", 4)
	action := parts[0]

	switch {
	case action == "main":
		kb := tgMainKeyboard()
		tgSend(bot, chatID, messageID, "oci-helper Bot — Main Menu\nSelect an option:", &kb)

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

	case action == "status":
		s.tgSendStatus(bot, chatID, messageID)

	case action == "help":
		helpText := `oci-helper Bot Commands:
/start - Main menu
/instances - List instances
/tasks - List tasks
/status - System status`
		kb := tgMainKeyboard()
		tgSend(bot, chatID, messageID, helpText, &kb)

	case parts[0] == "instances" && len(parts) >= 3 && parts[1] == "detail":
		s.tgSendInstanceDetail(bot, chatID, messageID, parts[2])

	case parts[0] == "instances" && len(parts) >= 4 && parts[1] == "action":
		instanceID, actionType := parts[2], parts[3]
		s.tgPerformAction(bot, chatID, messageID, callbackID, instanceID, actionType)
		s.tgSendInstanceDetail(bot, chatID, messageID, instanceID)

	case parts[0] == "tasks" && len(parts) >= 3 && parts[1] == "detail":
		taskID, _ := strconv.ParseInt(parts[2], 10, 64)
		s.tgSendTaskDetail(bot, chatID, messageID, taskID)
	}
}

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
	text := fmt.Sprintf("📌 %s\nState: %s\nShape: %s\nOCPU: %.1f | Mem: %.1f GB\nPublic IP: %s\nPrivate IP: %s\nBoot Volume: %d GB",
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
			text := fmt.Sprintf("📋 Task #%d\nType: %s\nStatus: %s\nProgress: %d%%\nMessage: %s\nCreated: %s",
				t.ID, t.Type, t.Status, t.Progress, t.Message, t.CreatedAt.Format("2006-01-02 15:04"))
			bk := telegram.InlineKeyboardMarkup{
				InlineKeyboard: [][]telegram.InlineKeyboardButton{
					{{Text: "🔙 Back", CallbackData: "tasks:0"}},
				},
			}
			tgSend(bot, chatID, messageID, text, &bk)
			return
		}
	}
	kb := tgMainKeyboard()
	tgSend(bot, chatID, messageID, "Task not found", &kb)
}

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
	text := fmt.Sprintf("📊 System Status\n\n🔹 Tenants: %d\n🖥 Instances: %d (running: %d)\n📋 Active Tasks: %d",
		len(tenants), len(instances), runningInstances, running)
	kb := tgMainKeyboard()
	tgSend(bot, chatID, messageID, text, &kb)
}
