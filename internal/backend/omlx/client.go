package omlx

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const DefaultBaseURL = "http://127.0.0.1:8000"

// Client talks to oMLX's OpenAI-compatible API.
type Client struct {
	BaseURL     string
	APIKey      string
	HTTP        *http.Client
	MaxTokens   int
	Temperature float64
	// Extra is merged into the chat completions JSON body (e.g. enable_thinking).
	Extra map[string]any
}

// NewClient constructs a client; API key from OMLX_API_KEY or ~/.omlx/settings.json.
func NewClient() (*Client, error) {
	key := os.Getenv("OMLX_API_KEY")
	if key == "" {
		var err error
		key, err = readKeyFromSettings()
		if err != nil {
			return nil, err
		}
	}
	return &Client{
		BaseURL:     envOr("OMLX_BASE_URL", DefaultBaseURL),
		APIKey:      key,
		HTTP:        &http.Client{Timeout: 10 * time.Minute},
		MaxTokens:   256,
		Temperature: 0.2,
	}, nil
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func readKeyFromSettings() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	path := filepath.Join(home, ".omlx", "settings.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("OMLX_API_KEY unset and cannot read %s: %w", path, err)
	}
	var s struct {
		Auth struct {
			APIKey string `json:"api_key"`
		} `json:"auth"`
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return "", err
	}
	if s.Auth.APIKey == "" {
		return "", fmt.Errorf("empty api_key in %s", path)
	}
	return s.Auth.APIKey, nil
}

// Message is a chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Usage token counts.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ChatResponse is a chat completions response.
type ChatResponse struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Choices []struct {
		Message Message `json:"message"`
		Finish  string  `json:"finish_reason"`
	} `json:"choices"`
	Usage Usage `json:"usage"`
}

// ChatCompletionResult includes wall-clock.
type ChatCompletionResult struct {
	Response  ChatResponse
	Elapsed   time.Duration
	Text      string
}

// Chat sends a chat completion request.
func (c *Client) Chat(model string, messages []Message) (ChatCompletionResult, error) {
	body := map[string]any{
		"model":       model,
		"messages":    messages,
		"max_tokens":  c.MaxTokens,
		"temperature": c.Temperature,
	}
	for k, v := range c.Extra {
		body[k] = v
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return ChatCompletionResult{}, err
	}
	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/v1/chat/completions", bytes.NewReader(raw))
	if err != nil {
		return ChatCompletionResult{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.APIKey)

	start := time.Now()
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return ChatCompletionResult{}, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	elapsed := time.Since(start)
	if err != nil {
		return ChatCompletionResult{}, err
	}
	if resp.StatusCode >= 300 {
		return ChatCompletionResult{}, fmt.Errorf("omlx %s: %s", resp.Status, truncate(string(data), 400))
	}
	var cr ChatResponse
	if err := json.Unmarshal(data, &cr); err != nil {
		return ChatCompletionResult{}, err
	}
	text := ""
	if len(cr.Choices) > 0 {
		text = cr.Choices[0].Message.Content
	}
	return ChatCompletionResult{Response: cr, Elapsed: elapsed, Text: text}, nil
}

// ListModels returns served model IDs.
func (c *Client) ListModels() ([]string, error) {
	req, err := http.NewRequest(http.MethodGet, c.BaseURL+"/v1/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("omlx %s: %s", resp.Status, truncate(string(data), 200))
	}
	var out struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(out.Data))
	for _, m := range out.Data {
		ids = append(ids, m.ID)
	}
	return ids, nil
}

// ProbeCacheAPIs checks for any cache-control endpoints (SCR readiness).
func (c *Client) ProbeCacheAPIs() map[string]string {
	paths := []string{
		"/v1/cache",
		"/cache",
		"/v1/kv",
		"/health",
		"/v1/health",
	}
	out := map[string]string{}
	for _, p := range paths {
		req, err := http.NewRequest(http.MethodGet, c.BaseURL+p, nil)
		if err != nil {
			out[p] = "error"
			continue
		}
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
		resp, err := c.HTTP.Do(req)
		if err != nil {
			out[p] = err.Error()
			continue
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		out[p] = resp.Status
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
