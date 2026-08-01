package handler

import (
	"fmt"
	"log"
	"strconv"

	"github.com/viogus/oci-helper-go/internal/dingtalk"
	"github.com/viogus/oci-helper-go/internal/telegram"
)

// notify delivers an operational message to the tenant's configured
// notification recipients, falling back to the global Telegram/DingTalk
// settings when no per-tenant override exists.
func (s *Server) notify(tenantID int64, message string) {
	if tenantID > 0 {
		sent := false
		tgChat, _ := s.store.GetConfig(fmt.Sprintf("tenant_ntg_%d", tenantID))
		if tgChat != "" {
			if token, _ := s.store.GetConfig("telegram_token"); token != "" {
				if chatID, err := strconv.ParseInt(tgChat, 10, 64); err == nil {
					if err := telegram.New(token).SendMessage(chatID, message); err != nil {
						log.Printf("[notify] tenant %d telegram: %v", tenantID, err)
					} else {
						sent = true
					}
				}
			}
		}
		if webhook, _ := s.store.GetConfig(fmt.Sprintf("tenant_ndtalk_%d", tenantID)); webhook != "" {
			if err := dingtalk.New(webhook).SendText(message); err != nil {
				log.Printf("[notify] tenant %d dingtalk: %v", tenantID, err)
			} else {
				sent = true
			}
		}
		if sent {
			return
		}
	}
	s.notifyGlobal(message)
}

// notifyGlobal sends to the globally configured Telegram chat and/or
// DingTalk webhook.
func (s *Server) notifyGlobal(message string) {
	if chatIDStr, _ := s.store.GetConfig("telegram_chat_id"); chatIDStr != "" {
		if token, _ := s.store.GetConfig("telegram_token"); token != "" {
			if chatID, err := strconv.ParseInt(chatIDStr, 10, 64); err == nil {
				if err := telegram.New(token).SendMessage(chatID, message); err != nil {
					log.Printf("[notify] global telegram: %v", err)
				}
			}
		}
	}
	if webhook, _ := s.store.GetConfig("dingtalk_webhook"); webhook != "" {
		if err := dingtalk.New(webhook).SendText(message); err != nil {
			log.Printf("[notify] global dingtalk: %v", err)
		}
	}
}
