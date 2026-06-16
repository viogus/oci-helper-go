package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
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
		client: &http.Client{Timeout: 120 * time.Second},
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

// ChatStream sends a streaming chat request and returns a channel of tokens.
func (c *Client) ChatStream(ctx context.Context, messages []ChatMessage) (<-chan string, error) {
	req := ChatRequest{
		Model:    c.model,
		Messages: messages,
		Stream:   true,
	}
	data, _ := json.Marshal(req)

	httpReq, err := http.NewRequestWithContext(ctx, "POST", "https://api.siliconflow.cn/v1/chat/completions", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := c.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("request: %w", err)
	}

	ch := make(chan string, 16)
	go func() {
		defer resp.Body.Close()
		defer close(ch)

		reader := bufio.NewReader(resp.Body)
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err != io.EOF {
					ch <- fmt.Sprintf("[error: %v]", err)
				}
				return
			}
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			dataStr := strings.TrimPrefix(line, "data: ")
			if dataStr == "[DONE]" {
				return
			}
			var event struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
			}
			if err := json.Unmarshal([]byte(dataStr), &event); err != nil {
				continue
			}
			if len(event.Choices) > 0 && event.Choices[0].Delta.Content != "" {
				select {
				case ch <- event.Choices[0].Delta.Content:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return ch, nil
}

// Search performs a DuckDuckGo Instant Answer search and returns text results.
func Search(query string) (string, error) {
	searchURL := fmt.Sprintf("https://api.duckduckgo.com/?q=%s&format=json&no_html=1&skip_disambig=1", url.QueryEscape(query))
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(searchURL)
	if err != nil {
		return "", fmt.Errorf("search request: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		AbstractText string `json:"AbstractText"`
		RelatedTopics []struct {
			Text string `json:"Text"`
		} `json:"RelatedTopics"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("search decode: %w", err)
	}

	var out strings.Builder
	if result.AbstractText != "" {
		out.WriteString("Abstract: " + result.AbstractText + "\n")
	}
	count := 0
	for _, topic := range result.RelatedTopics {
		if topic.Text == "" {
			continue
		}
		out.WriteString("- " + topic.Text + "\n")
		count++
		if count >= 5 {
			break
		}
	}
	return out.String(), nil
}
