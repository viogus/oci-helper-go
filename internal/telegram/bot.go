package telegram

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

const apiURL = "https://api.telegram.org/bot"

type Bot struct {
	token  string
	client *http.Client
}

type Update struct {
	UpdateID int     `json:"update_id"`
	Message  Message `json:"message"`
}

type Message struct {
	MessageID int    `json:"message_id"`
	Chat      Chat   `json:"chat"`
	Text      string `json:"text"`
}

type Chat struct {
	ID int64 `json:"id"`
}

func New(token string) *Bot {
	return &Bot{
		token:  token,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

func (b *Bot) GetUpdates(offset int) ([]Update, error) {
	var resp struct {
		OK     bool     `json:"ok"`
		Result []Update `json:"result"`
	}
	url := fmt.Sprintf("%s%s/getUpdates?offset=%d&timeout=25", apiURL, b.token, offset)
	if err := b.doGet(url, &resp); err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, fmt.Errorf("telegram: getUpdates not ok")
	}
	return resp.Result, nil
}

func (b *Bot) SendMessage(chatID int64, text string) error {
	var resp struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	body := map[string]interface{}{
		"chat_id": chatID,
		"text":    text,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	url := fmt.Sprintf("%s%s/sendMessage", apiURL, b.token)
	req, err := http.NewRequest("POST", url, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	r, err := b.client.Do(req)
	if err != nil {
		return fmt.Errorf("send: %w", err)
	}
	defer r.Body.Close()
	if err := json.NewDecoder(r.Body).Decode(&resp); err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	if !resp.OK {
		return fmt.Errorf("telegram: %s", resp.Description)
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

func FormatInstances(instances []InstanceInfo) string {
	if len(instances) == 0 {
		return "No instances found."
	}
	msg := fmt.Sprintf("Instances (%d):\n", len(instances))
	for _, i := range instances {
		msg += fmt.Sprintf("\n• %s\n  State: %s | Shape: %s | IP: %s\n  OCPU: %s | Mem: %sGB\n",
			i.Name, i.State, i.Shape, i.PublicIP,
			strconv.FormatFloat(i.OCPU, 'f', 1, 64),
			strconv.FormatFloat(i.MemoryGB, 'f', 1, 64))
	}
	return msg
}

type InstanceInfo struct {
	Name, State, Shape, PublicIP string
	OCPU, MemoryGB               float64
}
