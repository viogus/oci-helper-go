package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const apiURL = "https://api.telegram.org/bot"

type Bot struct {
	token  string
	client *http.Client
}

// ── API types ────────────────────────────────────────────────────────

type Update struct {
	UpdateID      int            `json:"update_id"`
	Message       *Message       `json:"message,omitempty"`
	CallbackQuery *CallbackQuery `json:"callback_query,omitempty"`
}

type Message struct {
	MessageID int  `json:"message_id"`
	Chat      Chat `json:"chat"`
	Text      string `json:"text"`
}

type Chat struct {
	ID int64 `json:"id"`
}

type CallbackQuery struct {
	ID      string   `json:"id"`
	From    User     `json:"from"`
	Message *Message `json:"message"`
	Data    string   `json:"data"`
}

type User struct {
	ID           int64  `json:"id"`
	FirstName    string `json:"first_name"`
	LastName     string `json:"last_name,omitempty"`
	Username     string `json:"username,omitempty"`
	LanguageCode string `json:"language_code,omitempty"`
}

type InlineKeyboardButton struct {
	Text         string `json:"text"`
	CallbackData string `json:"callback_data"`
}

type InlineKeyboardMarkup struct {
	InlineKeyboard [][]InlineKeyboardButton `json:"inline_keyboard"`
}

// ── Constructor ──────────────────────────────────────────────────────

func New(token string) *Bot {
	return &Bot{
		token:  token,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

// ── Telegram API methods ─────────────────────────────────────────────

func (b *Bot) SendMessage(chatID int64, text string) error {
	body := map[string]interface{}{
		"chat_id": chatID,
		"text":    text,
	}
	return b.call("sendMessage", body, nil)
}

func (b *Bot) SendKeyboard(chatID int64, text string, keyboard InlineKeyboardMarkup) error {
	body := map[string]interface{}{
		"chat_id":      chatID,
		"text":         text,
		"reply_markup": keyboard,
	}
	return b.call("sendMessage", body, nil)
}

func (b *Bot) EditMessageText(chatID int64, messageID int, text string, keyboard *InlineKeyboardMarkup) error {
	body := map[string]interface{}{
		"chat_id":    chatID,
		"message_id": messageID,
		"text":       text,
	}
	if keyboard != nil {
		body["reply_markup"] = *keyboard
	}
	return b.call("editMessageText", body, nil)
}

func (b *Bot) AnswerCallbackQuery(callbackID, text string) error {
	body := map[string]interface{}{
		"callback_query_id": callbackID,
		"text":              text,
		"show_alert":        false,
	}
	return b.call("answerCallbackQuery", body, nil)
}

func (b *Bot) GetUpdates(offset int) ([]Update, error) {
	var result struct {
		OK     bool     `json:"ok"`
		Result []Update `json:"result"`
	}
	url := fmt.Sprintf("%s%s/getUpdates?offset=%d&timeout=25", apiURL, b.token, offset)
	if err := b.doGet(url, &result); err != nil {
		return nil, err
	}
	if !result.OK {
		return nil, fmt.Errorf("telegram: getUpdates not ok")
	}
	return result.Result, nil
}

// ── Formatting helpers ───────────────────────────────────────────────

func FormatInstances(instances []InstanceInfo) string {
	if len(instances) == 0 {
		return "No instances found."
	}
	msg := fmt.Sprintf("Instances (%d):\n", len(instances))
	for _, i := range instances {
		ip := i.PublicIP
		if ip == "" {
			ip = "-"
		}
		msg += fmt.Sprintf("\n• %s\n  State: %s | Shape: %s | IP: %s", i.Name, i.State, i.Shape, ip)
	}
	return msg
}

type InstanceInfo struct {
	Name, State, Shape, PublicIP string
	OCPU, MemoryGB               float64
}

// ── Internal ─────────────────────────────────────────────────────────

func (b *Bot) call(method string, body map[string]interface{}, result interface{}) error {
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	url := fmt.Sprintf("%s%s/%s", apiURL, b.token, method)
	resp, err := b.client.Post(url, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("post: %w", err)
	}
	defer resp.Body.Close()

	if result != nil {
		return json.NewDecoder(resp.Body).Decode(result)
	}

	// Parse response to check for errors
	raw, _ := io.ReadAll(resp.Body)
	var check struct {
		OK bool `json:"ok"`
	}
	if err := json.Unmarshal(raw, &check); err != nil || !check.OK {
		// Try to extract description
		var errResp struct {
			Description string `json:"description"`
		}
		json.Unmarshal(raw, &errResp)
		if errResp.Description != "" {
			return fmt.Errorf("telegram: %s", errResp.Description)
		}
		if !check.OK {
			return fmt.Errorf("telegram: API error")
		}
	}
	return nil
}

func (b *Bot) doGet(url string, result interface{}) error {
	resp, err := b.client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(result)
}
