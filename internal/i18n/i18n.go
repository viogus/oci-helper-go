package i18n

import (
	"fmt"
	"strings"
)

type Messages map[string]string

var catalogs = map[string]Messages{
	"zh_CN": zhCN,
	"zh":    zhCN,
	"en":    enUS,
}

var enUS = Messages{
	"login.success":       "Login successful",
	"login.failed":        "Login failed",
	"login.mfa_required":  "MFA code required",
	"tenant.created":      "Tenant created",
	"tenant.deleted":      "Tenant deleted",
	"tenant.not_found":    "Tenant not found",
	"instance.action":     "Instance %s: %s",
	"instance.created":    "Instance created",
	"instance.not_found":  "Instance not found",
	"sync.complete":       "Synced %d instances",
	"backup.exported":     "Backup exported",
	"backup.restored":     "Backup restored",
	"mfa.enabled":         "MFA enabled",
	"mfa.disabled":        "MFA disabled",
	"mfa.setup":           "MFA secret generated",
	"batch.started":       "Batch task started: %d instances",
	"publicip.created":    "Public IP created",
	"publicip.deleted":    "Public IP deleted",
	"bootvolume.resized":  "Boot volume resized",
	"bootvolume.attached": "Boot volume attached",
	"bootvolume.detached": "Boot volume detached",
}

var zhCN = Messages{
	"login.success":       "登录成功",
	"login.failed":        "登录失败",
	"login.mfa_required":  "需要 MFA 验证码",
	"tenant.created":      "租户已创建",
	"tenant.deleted":      "租户已删除",
	"tenant.not_found":    "租户不存在",
	"instance.action":     "实例 %s: %s",
	"instance.created":    "实例已创建",
	"instance.not_found":  "实例不存在",
	"sync.complete":       "已同步 %d 个实例",
	"backup.exported":     "备份已导出",
	"backup.restored":     "备份已恢复",
	"mfa.enabled":         "MFA 已启用",
	"mfa.disabled":        "MFA 已禁用",
	"mfa.setup":           "MFA 密钥已生成",
	"batch.started":       "批量任务已启动: %d 个实例",
	"publicip.created":    "公网 IP 已创建",
	"publicip.deleted":    "公网 IP 已删除",
	"bootvolume.resized":  "启动卷已调整",
	"bootvolume.attached": "启动卷已挂载",
	"bootvolume.detached": "启动卷已卸载",
}

func T(acceptLanguage, key string, args ...interface{}) string {
	lang := parseAcceptLanguage(acceptLanguage)
	msg, ok := catalogs[lang]
	if !ok {
		msg = enUS
	}
	tmpl, ok := msg[key]
	if !ok {
		tmpl = enUS[key]
	}
	if tmpl == "" {
		return key
	}
	if len(args) > 0 {
		return fmt.Sprintf(tmpl, args...)
	}
	return tmpl
}

func parseAcceptLanguage(header string) string {
	if header == "" {
		return "en"
	}
	parts := strings.Split(header, ",")
	if len(parts) == 0 {
		return "en"
	}
	tag := strings.SplitN(strings.TrimSpace(parts[0]), ";", 2)[0]
	tag = strings.ReplaceAll(tag, "-", "_")
	if strings.HasPrefix(tag, "zh") {
		return "zh_CN"
	}
	return "en"
}
