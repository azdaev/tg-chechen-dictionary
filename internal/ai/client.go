package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/sirupsen/logrus"
)

const openRouterURL = "https://openrouter.ai/api/v1/chat/completions"

type Client struct {
	apiKey string
	model  string
	http   *http.Client
	log    *logrus.Logger
}

func New(apiKey, model string, log *logrus.Logger) *Client {
	return &Client{
		apiKey: apiKey,
		model:  model,
		http:   &http.Client{Timeout: 30 * time.Second},
		log:    log,
	}
}

type message struct {
	Role    string `json:"role"`
	Content any    `json:"content"`
}

type chatRequest struct {
	Model       string    `json:"model"`
	Messages    []message `json:"messages"`
	Temperature float64   `json:"temperature"`
	MaxTokens   int       `json:"max_tokens,omitempty"`
}

type chatResponse struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func (c *Client) complete(ctx context.Context, msgs []message, maxTokens ...int) (string, error) {
	req := chatRequest{
		Model:       c.model,
		Messages:    msgs,
		Temperature: 0.3,
	}
	if len(maxTokens) > 0 && maxTokens[0] > 0 {
		req.MaxTokens = maxTokens[0]
	}
	body, err := json.Marshal(req)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	for _, m := range msgs {
		switch content := m.Content.(type) {
		case string:
			c.log.WithFields(logrus.Fields{
				"role":    m.Role,
				"content": content,
			}).Debug("AI request")
		default:
			c.log.WithFields(logrus.Fields{
				"role":    m.Role,
				"content": "[multipart]",
			}).Debug("AI request")
		}
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, openRouterURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("new request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return "", fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		c.log.WithFields(logrus.Fields{
			"status": resp.StatusCode,
			"body":   string(respBody),
		}).Error("AI response error")
		return "", fmt.Errorf("openrouter status %d: %s", resp.StatusCode, respBody)
	}

	var chatResp chatResponse
	if err := json.Unmarshal(respBody, &chatResp); err != nil {
		return "", fmt.Errorf("unmarshal response: %w", err)
	}

	if chatResp.Error != nil {
		return "", fmt.Errorf("openrouter error: %s", chatResp.Error.Message)
	}

	if len(chatResp.Choices) == 0 {
		return "", fmt.Errorf("openrouter returned no choices")
	}

	result := chatResp.Choices[0].Message.Content
	c.log.WithField("content", result).Debug("AI response")

	return result, nil
}

// extractJSON extracts the first JSON object from a string,
// handling cases where the model wraps JSON in markdown code blocks.
func extractJSON(s string) string {
	start := -1
	for i := 0; i < len(s)-2; i++ {
		if s[i] == '{' {
			start = i
			break
		}
	}
	if start == -1 {
		return s
	}

	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return s[start:]
}
