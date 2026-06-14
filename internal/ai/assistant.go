package ai

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Client struct {
	apiKey string
	model  string
	client *http.Client
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

type ChatResponse struct {
	Choices []struct {
		Message ChatMessage `json:"message"`
	} `json:"choices"`
}

func New(apiKey, model string) *Client {
	if model == "" {
		model = "Qwen/Qwen2.5-7B-Instruct"
	}
	return &Client{
		apiKey: apiKey,
		model:  model,
		client: &http.Client{Timeout: 60 * time.Second},
	}
}

func (c *Client) Chat(messages []ChatMessage) (string, error) {
	req := ChatRequest{
		Model:    c.model,
		Messages: messages,
		Stream:   false,
	}
	data, _ := json.Marshal(req)

	httpReq, _ := http.NewRequest("POST", "https://api.siliconflow.cn/v1/chat/completions", bytes.NewReader(data))
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("request: %w", err)
	}
	defer resp.Body.Close()

	var chatResp ChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&chatResp); err != nil {
		return "", fmt.Errorf("decode: %w", err)
	}
	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("no response from model")
	}
	return chatResp.Choices[0].Message.Content, nil
}
