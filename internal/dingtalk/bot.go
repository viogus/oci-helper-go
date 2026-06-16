// Package dingtalk implements a DingTalk (钉钉) bot client for sending text and markdown messages.
//
package dingtalk

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Bot struct {
	webhookURL string
	client     *http.Client
}

type markdownMessage struct {
	MsgType  string `json:"msgtype"`
	Markdown struct {
		Title string `json:"title"`
		Text  string `json:"text"`
	} `json:"markdown"`
}

type textMessage struct {
	MsgType string `json:"msgtype"`
	Text    struct {
		Content string `json:"content"`
	} `json:"text"`
}

func New(webhookURL string) *Bot {
	return &Bot{
		webhookURL: webhookURL,
		client:     &http.Client{Timeout: 15 * time.Second},
	}
}

func (b *Bot) SendText(text string) error {
	msg := textMessage{MsgType: "text"}
	msg.Text.Content = text
	return b.send(msg)
}

func (b *Bot) SendMarkdown(title, text string) error {
	msg := markdownMessage{MsgType: "markdown"}
	msg.Markdown.Title = title
	msg.Markdown.Text = text
	return b.send(msg)
}

func (b *Bot) send(msg interface{}) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	resp, err := b.client.Post(b.webhookURL, "application/json", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("post: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode: %w", err)
	}
	if result.ErrCode != 0 {
		return fmt.Errorf("dingtalk errcode=%d: %s", result.ErrCode, result.ErrMsg)
	}
	return nil
}
