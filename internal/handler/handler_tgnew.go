package handler

// Telegram bot features added for Java parity:
//   - System metrics (host CPU/mem/disk/net) menu
//   - AI chat model selection (DeepSeek-R1 / DeepSeek-V3 / Qwen)
//   - Instance terminate with preserve-boot-volume confirmation
//   - Backup restore flow
//   - SSH management (/ssh_config + /ssh + ssh menu)
//   - Defense enable/disable full flow (tenant -> VCN -> CIDRs -> confirm)

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/viogus/oci-helper-go/internal/system"
	"github.com/viogus/oci-helper-go/internal/telegram"
	gossh "golang.org/x/crypto/ssh"
)

// ── State storage ────────────────────────────────────────────────────

var (
	tgAIModelMu sync.Mutex
	tgAIModels  = map[int64]string{} // chatID -> model key (deepseek_r1/v3, qwen)

	tgRestoreMu     sync.Mutex
	tgRestoreStates = map[int64]*tgRestoreState{}

	sshConnsMu       sync.Mutex
	tgSSHConnections = map[int64]*tgSSHConn{}
	tgSSHExpiry      = 30 * time.Minute

	tgDefenseMu     sync.Mutex
	tgDefenseStates = map[int64]*tgDefenseState{}
)

type tgRestoreState struct {
	Step     string // "password" | "data"
	Password string
}

type tgSSHConn struct {
	Host     string
	Port     int
	Username string
	Password string
	LastUsed time.Time
}

type tgDefenseState struct {
	Step     string // "tenant" | "vcn" | "cidr" | "confirm"
	TenantID int64
	VcnID    string
	CIDRs    []string
}

// ── System metrics ───────────────────────────────────────────────────

func (s *Server) tgSystemMetrics(bot *telegram.Bot, chatID int64, messageID int) {
	snap, err := system.Collect()
	if err != nil {
		kb := tgMainKeyboard()
		tgSend(bot, chatID, messageID, "❌ 获取系统资源信息失败: "+err.Error(), &kb)
		return
	}

	var sb strings.Builder
	sb.WriteString("📊 系统资源监控\n\n")
	sb.WriteString(fmt.Sprintf("🖥 主机: %s\n", strOr(&snap.Hostname, "-")))
	sb.WriteString(fmt.Sprintf("💻 系统: %s / %s (%s)\n", strOr(&snap.OS, "-"), strOr(&snap.Platform, "-"), strOr(&snap.Arch, "-")))
	sb.WriteString(fmt.Sprintf("⏱ 运行时间: %s\n", tgFormatUptime(snap.Uptime)))
	sb.WriteString(fmt.Sprintf("🧮 CPU: %s (%d 核)\n", strOr(&snap.CPUModel, "-"), snap.CPUCount))
	sb.WriteString(fmt.Sprintf("   ⚡ 使用率: %.2f%% (用户 %.2f%% / 系统 %.2f%% / 空闲 %.2f%%)\n",
		snap.CPU.UsedPercent, snap.CPU.User, snap.CPU.System, snap.CPU.Idle))
	sb.WriteString(fmt.Sprintf("💾 内存: %.2f%% (已用 %s / 可用 %s / 总计 %s)\n",
		snap.Memory.UsedPercent, tgFormatBytes(snap.Memory.Used), tgFormatBytes(snap.Memory.Available), tgFormatBytes(snap.Memory.Total)))
	if len(snap.Disks) > 0 {
		for _, d := range snap.Disks {
			sb.WriteString(fmt.Sprintf("📀 %s: %.2f%% (%s / %s)\n",
				d.Mount, d.UsedPercent, tgFormatBytes(d.Used), tgFormatBytes(d.Total)))
		}
	}
	sb.WriteString(fmt.Sprintf("🌐 网络: ↑ %.2f KB/s ↓ %.2f KB/s (总 ↑ %s / ↓ %s)",
		snap.Network.TxRateKBps, snap.Network.RxRateKBps,
		tgFormatBytes(snap.Network.BytesSent), tgFormatBytes(snap.Network.BytesRecv)))

	kb := telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "🔄 刷新", CallbackData: "sysmetrics"}},
		{{Text: "\U0001f519 主菜单", CallbackData: "main"}},
	}}
	tgSend(bot, chatID, messageID, sb.String(), &kb)
}

func tgFormatBytes(b uint64) string {
	if b < 1024 {
		return fmt.Sprintf("%d B", b)
	}
	units := []string{"KB", "MB", "GB", "TB", "PB"}
	i := -1
	v := float64(b)
	for {
		v /= 1024
		i++
		if v < 1024 || i == len(units)-1 {
			break
		}
	}
	return fmt.Sprintf("%.1f %s", v, units[i])
}

func tgFormatUptime(secs uint64) string {
	d := secs / 86400
	h := (secs % 86400) / 3600
	m := (secs % 3600) / 60
	if d > 0 {
		return fmt.Sprintf("%dd %dh %dm", d, h, m)
	}
	if h > 0 {
		return fmt.Sprintf("%dh %dm", h, m)
	}
	return fmt.Sprintf("%dm", m)
}

// ── AI model selection (Java parity) ─────────────────────────────────

var tgModelIDs = map[string]string{
	"deepseek_r1": "deepseek-ai/DeepSeek-R1-0528-Qwen3-8B",
	"deepseek_v3": "deepseek-ai/DeepSeek-V3",
	"qwen":        "Qwen/Qwen2.5-7B-Instruct",
}

func tgModelDisplayName(key string) string {
	switch key {
	case "deepseek_r1":
		return "🧠 DeepSeek-R1"
	case "deepseek_v3":
		return "⚡ DeepSeek-V3"
	case "qwen":
		return "🌟 Qwen-2.5"
	default:
		return key
	}
}

func (s *Server) tgAIModelMenu(bot *telegram.Bot, chatID int64, messageID int) {
	tgAIModelMu.Lock()
	current := tgAIModels[chatID]
	tgAIModelMu.Unlock()
	text := "🤖 AI 模型选择"
	if current != "" {
		text += "\n当前: " + tgModelDisplayName(current)
	}
	kb := telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "🧠 DeepSeek-R1", CallbackData: "ai:model:set:deepseek_r1"}},
		{{Text: "⚡ DeepSeek-V3", CallbackData: "ai:model:set:deepseek_v3"}},
		{{Text: "🌟 Qwen-2.5", CallbackData: "ai:model:set:qwen"}},
		{{Text: "\U0001f519 主菜单", CallbackData: "main"}},
	}}
	tgSend(bot, chatID, messageID, text, &kb)
}

// ── Instance terminate (Java parity: preserve boot volume choice) ────

func (s *Server) tgTerminateConfirm(bot *telegram.Bot, chatID int64, messageID int, instanceID string) {
	inst, err := s.store.GetInstanceByID(instanceID)
	if err != nil || inst == nil {
		tgSend(bot, chatID, messageID, "实例不存在。", nil)
		return
	}
	kb := telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{
			{Text: "🗑 删除（保留引导卷）", CallbackData: fmt.Sprintf("instances:terminate:confirm:%s:keep", instanceID)},
		},
		{
			{Text: "🗑 删除（连同引导卷）", CallbackData: fmt.Sprintf("instances:terminate:confirm:%s:del", instanceID)},
		},
		{{Text: "\U0001f519 返回", CallbackData: "instances:0"}},
	}}
	tgSend(bot, chatID, messageID, fmt.Sprintf("⚠️ 确认终止实例 %s？此操作不可恢复。", inst.Name), &kb)
}

func (s *Server) tgTerminateDo(bot *telegram.Bot, chatID int64, messageID int, callbackID, instanceID string, preserveBootVolume bool) {
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
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	if err := client.TerminateInstance(ctx, bareOCID(instanceID), preserveBootVolume, false); err != nil {
		_ = bot.AnswerCallbackQuery(callbackID, "Terminate failed: "+err.Error())
		tgSend(bot, chatID, messageID, "❌ 终止失败: "+err.Error(), nil)
		return
	}
	s.store.UpdateInstanceState(instanceID, "TERMINATING")
	s.audit(inst.TenantID, "instance:terminate", instanceID+" preserve_boot="+strconv.FormatBool(preserveBootVolume), nil)
	_ = bot.AnswerCallbackQuery(callbackID, "Terminate initiated")
	tgSend(bot, chatID, messageID, fmt.Sprintf("✅ 实例 %s 终止指令已提交。", inst.Name), nil)
}

// ── Backup restore ───────────────────────────────────────────────────

func (s *Server) tgRestoreRespond(bot *telegram.Bot, chatID int64, text string) {
	tgRestoreMu.Lock()
	st := tgRestoreStates[chatID]
	if st == nil {
		tgRestoreMu.Unlock()
		return
	}
	text = strings.TrimSpace(text)
	switch st.Step {
	case "password":
		if text == "" {
			tgRestoreMu.Unlock()
			bot.SendMessage(chatID, "密码不能为空，请重新输入。")
			return
		}
		st.Password = text
		st.Step = "data"
		tgRestoreMu.Unlock()
		bot.SendMessage(chatID, "请粘贴备份数据（备份时返回的 base64 字符串）。")
	case "data":
		password := st.Password
		delete(tgRestoreStates, chatID)
		tgRestoreMu.Unlock()
		bot.SendMessage(chatID, "⏳ 正在恢复，请稍候...")
		nT, nI, err := s.restoreData(password, text)
		if err != nil {
			bot.SendMessage(chatID, "❌ 恢复失败: "+err.Error())
			return
		}
		s.audit(0, "backup:restore", fmt.Sprintf("%d tenants, %d instances", nT, nI), nil)
		bot.SendMessage(chatID, fmt.Sprintf("✅ 恢复成功: %d 租户, %d 实例。", nT, nI))
	}
}

// ── SSH management (Java parity: /ssh_config + /ssh) ─────────────────

func (s *Server) tgSSHManagement(bot *telegram.Bot, chatID int64, messageID int) {
	sshConnsMu.Lock()
	conn := tgSSHConnections[chatID]
	sshConnsMu.Unlock()

	var sb strings.Builder
	sb.WriteString("🔌 SSH 管理\n\n")
	sshConnsMu.Lock()
	lastUsed := ""
	if conn != nil {
		lastUsed = conn.LastUsed.Format("15:04:05")
	}
	sshConnsMu.Unlock()
	if conn == nil {
		sb.WriteString("未配置 SSH 连接。\n\n使用 /ssh_config host port user pwd 配置连接。")
	} else {
		sb.WriteString(fmt.Sprintf("已配置: %s:%d (%s@%s)\n最后使用: %s\n\n", conn.Host, conn.Port, conn.Username, conn.Host, lastUsed))
		sb.WriteString("使用 /ssh <命令> 执行远程命令。")
	}
	var kb *telegram.InlineKeyboardMarkup
	if conn != nil {
		kb = &telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{{Text: "🗑 移除连接", CallbackData: "ssh:remove"}, {Text: "\U0001f519 主菜单", CallbackData: "main"}},
		}}
	} else {
		kb = &telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{{Text: "\U0001f519 主菜单", CallbackData: "main"}},
		}}
	}
	tgSend(bot, chatID, messageID, sb.String(), kb)
}

func (s *Server) tgSSHConfig(bot *telegram.Bot, chatID int64, text string) {
	// /ssh_config host port user pwd
	fields := strings.Fields(text)
	if len(fields) < 5 {
		bot.SendMessage(chatID, "格式: /ssh_config host port user pwd")
		return
	}
	port, err := strconv.Atoi(fields[2])
	if err != nil || port <= 0 || port > 65535 {
		bot.SendMessage(chatID, "端口无效，请使用数字 (1-65535)。")
		return
	}
	conn := &tgSSHConn{
		Host:     fields[1],
		Port:     port,
		Username: fields[3],
		Password: fields[4],
		LastUsed: time.Now(),
	}
	sshConnsMu.Lock()
	tgSSHConnections[chatID] = conn
	sshConnsMu.Unlock()
	bot.SendMessage(chatID, fmt.Sprintf("✅ SSH 连接已保存: %s:%d (%s)\n现在可以使用 /ssh <命令> 执行远程命令。", conn.Host, conn.Port, conn.Username))
}

var tgInteractiveCommands = []string{
	"vi", "vim", "nano", "emacs", "top", "htop", "less", "more",
	"tail -f", "watch", "ssh", "telnet", "ftp", "mysql", "psql",
	"python", "node", "irb", "php -a",
}

func (s *Server) tgSSHExec(bot *telegram.Bot, chatID int64, text string) {
	sshConnsMu.Lock()
	conn := tgSSHConnections[chatID]
	sshConnsMu.Unlock()
	if conn == nil {
		bot.SendMessage(chatID, "未配置 SSH 连接，请先使用 /ssh_config host port user pwd 配置。")
		return
	}
	command := strings.TrimSpace(strings.TrimPrefix(text, "/ssh"))
	if command == "" {
		bot.SendMessage(chatID, "用法: /ssh <命令>，例如 /ssh ls -la")
		return
	}
	for _, ic := range tgInteractiveCommands {
		if strings.HasPrefix(command, ic) {
			bot.SendMessage(chatID, "❌ 不支持交互式命令（如 vi, top, tail -f 等），请使用非交互式命令。")
			return
		}
	}

	bot.SendMessage(chatID, "⏳ 正在执行: "+command)

	// Password auth over SSH.
	cfg := &gossh.ClientConfig{
		User: conn.Username,
		Auth: []gossh.AuthMethod{gossh.Password(conn.Password)},
		HostKeyCallback: func(hostname string, remote net.Addr, key gossh.PublicKey) error {
			return nil // TG SSH is convenience tooling; same trust model as Java's jsch
		},
		Timeout: 10 * time.Second,
	}
	addr := net.JoinHostPort(conn.Host, strconv.Itoa(conn.Port))
	client, err := gossh.Dial("tcp", addr, cfg)
	if err != nil {
		bot.SendMessage(chatID, "❌ SSH 连接失败: "+err.Error())
		return
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		bot.SendMessage(chatID, "❌ 创建会话失败: "+err.Error())
		return
	}
	defer session.Close()

	// 30s timeout like the Java original.
	type result struct {
		out string
		err error
	}
	ch := make(chan result, 1)
	go func() {
		out, err := session.CombinedOutput(command)
		ch <- result{string(out), err}
	}()

	select {
	case <-time.After(30 * time.Second):
		_ = session.Close()
		bot.SendMessage(chatID, "⏱️ 命令执行超时（超过 30 秒）。")
	case res := <-ch:
		sshConnsMu.Lock()
		if c, ok := tgSSHConnections[chatID]; ok {
			c.LastUsed = time.Now()
		}
		sshConnsMu.Unlock()
		out := strings.TrimSpace(res.out)
		if out == "" && res.err != nil {
			out = res.err.Error()
		}
		if len(out) > 3500 {
			out = out[:3500] + "\n… (输出过长已截断)"
		}
		if res.err != nil && out == "" {
			bot.SendMessage(chatID, "❌ 命令执行失败: "+res.err.Error())
			return
		}
		bot.SendMessage(chatID, "```\n"+out+"\n```")
	}
}

// ── Defense full flow (Java parity) ──────────────────────────────────

// parseCIDRList validates and normalizes one-CIDR-per-line text input.
func parseCIDRList(text string) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		cidr := strings.TrimSpace(line)
		if cidr == "" {
			continue
		}
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			continue
		}
		out = append(out, cidr)
	}
	return out
}

func (s *Server) tgDefenseTenantList(bot *telegram.Bot, chatID int64, messageID int) {
	tenants, _ := s.store.ListTenants()
	if len(tenants) == 0 {
		tgSend(bot, chatID, messageID, "没有可用的租户配置。", nil)
		return
	}
	var rows [][]telegram.InlineKeyboardButton
	for _, t := range tenants {
		label := t.Name
		if len(label) > 30 {
			label = label[:30] + "…"
		}
		rows = append(rows, []telegram.InlineKeyboardButton{{
			Text: label, CallbackData: fmt.Sprintf("defense:tenant:%d", t.ID),
		}})
	}
	rows = append(rows, []telegram.InlineKeyboardButton{{Text: "\U0001f519 返回", CallbackData: "defense"}})
	tgSend(bot, chatID, messageID, "🛡 防御模式 — 选择租户:", &telegram.InlineKeyboardMarkup{InlineKeyboard: rows})
}

func (s *Server) tgDefenseVcnList(bot *telegram.Bot, chatID int64, messageID int, tenantID int64, disable bool) {
	tenant, err := s.store.GetTenant(tenantID)
	if err != nil || tenant == nil {
		tgSend(bot, chatID, messageID, "租户不存在。", nil)
		return
	}
	client, err := s.clientFor(tenant)
	if err != nil {
		tgSend(bot, chatID, messageID, "OCI 客户端错误: "+err.Error(), nil)
		return
	}
	vcns, err := client.ListVCNs(context.Background(), tenant.TenancyOCID)
	if err != nil || len(vcns) == 0 {
		tgSend(bot, chatID, messageID, "未找到 VCN，请先在面板同步。", nil)
		return
	}
	prefix := "defense:vcn"
	if disable {
		prefix = "defense:disable:vcn"
	}
	var rows [][]telegram.InlineKeyboardButton
	for i, v := range vcns {
		if i >= 15 {
			break
		}
		name := strOr(v.DisplayName, *v.Id)
		if len(name) > 30 {
			name = name[:30] + "…"
		}
		rows = append(rows, []telegram.InlineKeyboardButton{{
			Text: name, CallbackData: fmt.Sprintf("%s:%d:%s", prefix, tenantID, *v.Id),
		}})
	}
	rows = append(rows, []telegram.InlineKeyboardButton{{Text: "\U0001f519 返回", CallbackData: "defense"}})
	tgSend(bot, chatID, messageID, "🛡 防御模式 — 选择 VCN:", &telegram.InlineKeyboardMarkup{InlineKeyboard: rows})
}

func (s *Server) tgDefenseConfirm(bot *telegram.Bot, chatID int64, messageID int, st *tgDefenseState, disable bool) {
	if disable {
		kb := telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{{Text: "✅ 确认关闭", CallbackData: fmt.Sprintf("defense:disable:confirm:%d:%s", st.TenantID, st.VcnID)}},
			{{Text: "\U0001f519 返回", CallbackData: "defense"}},
		}}
		tgSend(bot, chatID, messageID, "🛡 确认关闭防御模式？将恢复原始入站规则。", &kb)
		return
	}
	kb := telegram.InlineKeyboardMarkup{InlineKeyboard: [][]telegram.InlineKeyboardButton{
		{{Text: "✅ 确认启用", CallbackData: fmt.Sprintf("defense:confirm:%d:%s", st.TenantID, st.VcnID)}},
		{{Text: "\U0001f519 返回", CallbackData: "defense"}},
	}}
	tgSend(bot, chatID, messageID, fmt.Sprintf("🛡 确认启用防御模式？\n\n封禁 %d 个 CIDR:\n%s", len(st.CIDRs), strings.Join(st.CIDRs, "\n")), &kb)
}

func (s *Server) tgDefenseEnableDo(bot *telegram.Bot, chatID int64, messageID int, callbackID string, tenantID int64, vcnID string) {
	tgDefenseMu.Lock()
	st := tgDefenseStates[chatID]
	if st == nil {
		tgDefenseMu.Unlock()
		return
	}
	cidrs := st.CIDRs
	delete(tgDefenseStates, chatID)
	tgDefenseMu.Unlock()

	_ = bot.AnswerCallbackQuery(callbackID, "Enabling defense...")
	n, err := s.enableDefense(context.Background(), tenantID, vcnID, cidrs)
	if err != nil {
		tgSend(bot, chatID, messageID, "❌ 启用防御失败: "+err.Error(), nil)
		return
	}
	tgSend(bot, chatID, messageID, fmt.Sprintf("✅ 防御模式已启用，已封禁 %d 个 IP。", n), nil)
}

func (s *Server) tgDefenseDisableDo(bot *telegram.Bot, chatID int64, messageID int, callbackID string, tenantID int64, vcnID string) {
	_ = bot.AnswerCallbackQuery(callbackID, "Disabling defense...")
	if err := s.disableDefense(context.Background(), tenantID, vcnID); err != nil {
		tgSend(bot, chatID, messageID, "❌ 关闭防御失败: "+err.Error(), nil)
		return
	}
	tgSend(bot, chatID, messageID, "✅ 防御模式已关闭，规则已恢复。", nil)
}
