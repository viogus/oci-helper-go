package handler

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

// startupNotify reports a successful panel start to the configured channels.
func (s *Server) startupNotify() {
	time.Sleep(2 * time.Second)
	s.notifyGlobal(fmt.Sprintf("【oci-helper】服务启动成功，当前版本：%s", version))
}

// startDailyBroadcast sends the daily tenant/task summary when enabled. The
// cron value is a simple "HH:MM" 24h string for portability.
func (s *Server) startDailyBroadcast() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	var lastSent string
	for {
		select {
		case <-s.stopping:
			return
		case <-ticker.C:
			enabled, _ := s.store.GetConfig("daily_broadcast_enabled")
			if enabled != "true" {
				continue
			}
			cronVal, _ := s.store.GetConfig("daily_broadcast_cron")
			if cronVal == "" {
				cronVal = "08:00"
			}
			now := time.Now()
			today := now.Format("2006-01-02")
			if today+"|"+cronVal == lastSent {
				continue
			}
			parts := strings.SplitN(cronVal, ":", 2)
			if len(parts) != 2 {
				continue
			}
			hh, err1 := strconv.Atoi(parts[0])
			mm, err2 := strconv.Atoi(parts[1])
			if err1 != nil || err2 != nil || now.Hour() != hh || now.Minute() != mm {
				continue
			}
			lastSent = today + "|" + cronVal
			s.sendDailyBroadcast()
		}
	}
}

func (s *Server) sendDailyBroadcast() {
	tenants, err := s.store.ListTenants()
	if err != nil {
		log.Printf("[daily-broadcast] list tenants: %v", err)
		return
	}
	inactive := make([]string, 0)
	for _, t := range tenants {
		if t.AccountStatus == "INACTIVE" {
			inactive = append(inactive, t.Name)
		}
	}
	tasks, err := s.store.ListActiveCreateTasks()
	if err != nil {
		log.Printf("[daily-broadcast] list tasks: %v", err)
	}
	taskText := "无"
	if len(tasks) > 0 {
		var b strings.Builder
		for _, t := range tasks {
			fmt.Fprintf(&b, "[%d] %d核/%dGB/%dGB %d台 每%d秒\n",
				t.ID, int(t.OCPUs), int(t.MemoryGB), t.Disk, t.CreateNumbers, t.IntervalSeconds)
		}
		taskText = b.String()
	}
	inactiveText := "无"
	if len(inactive) > 0 {
		inactiveText = strings.Join(inactive, "、")
	}
	s.notifyGlobal(fmt.Sprintf("【每日播报】\n时间：%s\n总API配置数：%d\n失效API配置数：%d\n失效配置：%s\n正在执行的开机任务：\n%s",
		time.Now().Format("2006-01-02 15:04"), len(tenants), len(inactive), inactiveText, taskText))
}

// startVersionUpdateNotify checks GitHub once per day and notifies when a new
// release is available and notifications are enabled.
func (s *Server) startVersionUpdateNotify() {
	ticker := time.NewTicker(12 * time.Hour)
	defer ticker.Stop()
	notified := ""
	for {
		select {
		case <-s.stopping:
			return
		case <-ticker.C:
			enabled, _ := s.store.GetConfig("version_update_notifications_enabled")
			if enabled != "true" {
				continue
			}
			repo, _ := s.store.GetConfig("update_repo")
			if repo == "" {
				continue
			}
			client := &http.Client{Timeout: 15 * time.Second}
			resp, err := client.Get("https://api.github.com/repos/" + repo + "/releases/latest")
			if err != nil {
				continue
			}
			var info struct {
				TagName string `json:"tag_name"`
				Body    string `json:"body"`
			}
			if json.NewDecoder(resp.Body).Decode(&info) != nil || info.TagName == "" {
				resp.Body.Close()
				continue
			}
			resp.Body.Close()
			if info.TagName == notified || info.TagName == version {
				continue
			}
			notified = info.TagName
			s.notifyGlobal(fmt.Sprintf("🔔【oci-helper】版本更新啦！当前：%s 最新：%s\n%s", version, info.TagName, info.Body))
		}
	}
}

// cleanLogTask clears the log file every 8 hours like the Java original.
func (s *Server) cleanLogTask() {
	ticker := time.NewTicker(8 * time.Hour)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopping:
			return
		case <-ticker.C:
			logFile := s.cfg.LogFile
			if logFile == "" {
				logFile = os.Getenv("OCI_LOG_FILE")
			}
			if logFile != "" {
				if err := os.Truncate(logFile, 0); err != nil {
					log.Printf("[clean-log] truncate %s: %v", logFile, err)
				}
			}
		}
	}
}
