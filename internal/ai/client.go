// Package ai implements non-streaming Chat Completions over HTTP.
package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Planck-code/SupportFlow-AI/internal/config"
)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Client interface {
	Complete(context.Context, []Message) (string, error)
}

var (
	ErrInput    = errors.New("invalid chat messages")
	ErrNetwork  = errors.New("model network failure")
	ErrResponse = errors.New("invalid model response")
	ErrOutput   = errors.New("invalid model output")
)

// UpstreamError contains only the HTTP status. Never retain the upstream body.
type UpstreamError struct{ StatusCode int }

func (e *UpstreamError) Error() string {
	return fmt.Sprintf("model server returned HTTP %d", e.StatusCode)
}

const maxResponseBytes = 1 << 20

type httpClient struct {
	http                 *http.Client
	endpoint, key, model string
	timeout              time.Duration
}

func NewClient(cfg config.AI) (Client, error) {
	endpoint, err := cfg.Endpoint()
	if err != nil {
		return nil, err
	}
	return &httpClient{
		http:     &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		endpoint: endpoint, key: cfg.APIKey, model: cfg.Model, timeout: cfg.Timeout,
	}, nil
}

func (c *httpClient) Complete(ctx context.Context, messages []Message) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if len(messages) == 0 {
		return "", ErrInput
	}
	for _, m := range messages {
		switch m.Role {
		case "user", "system", "assistant":
		default:
			return "", ErrInput
		}
		if strings.TrimSpace(m.Content) == "" || !utf8.ValidString(m.Content) {
			return "", ErrInput
		}
	}
	body, err := json.Marshal(struct {
		Model    string    `json:"model"`
		Messages []Message `json:"messages"`
	}{c.model, messages})
	if err != nil {
		return "", ErrInput
	}
	// Keep this POST non-replayable, including on a reused connection.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, io.NopCloser(bytes.NewReader(body)))
	if err != nil {
		return "", config.ErrAIConfig
	}
	req.ContentLength = int64(len(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.key)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", transportError(ctx, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", &UpstreamError{StatusCode: resp.StatusCode}
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return "", transportError(ctx, err)
	}
	if len(raw) > maxResponseBytes || !utf8.Valid(raw) {
		return "", ErrResponse
	}

	var result struct {
		Choices []struct {
			Message *struct {
				Role    string          `json:"role"`
				Content json.RawMessage `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if json.Unmarshal(raw, &result) != nil || len(result.Choices) == 0 || result.Choices[0].Message == nil {
		return "", ErrResponse
	}
	message := result.Choices[0].Message
	if message.Role != "" && message.Role != "assistant" {
		return "", ErrResponse
	}
	var content string
	if json.Unmarshal(message.Content, &content) != nil || strings.TrimSpace(content) == "" {
		return "", ErrOutput
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return content, nil
}

func transportError(ctx context.Context, err error) error {
	if cause := ctx.Err(); cause != nil {
		return cause
	}
	var timeout net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &timeout) && timeout.Timeout()) {
		return context.DeadlineExceeded
	}
	return ErrNetwork
}
