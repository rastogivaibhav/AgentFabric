package proxy

import (
	"encoding/json"
	"strings"
)

// anthropicParser handles Anthropic Messages API request/response parsing.
type anthropicParser struct{}

type anthropicRequest struct {
	Model    string `json:"model"`
	Stream   bool   `json:"stream"`
	Messages []struct {
		Role    string `json:"role"`
		Content any    `json:"content"`
	} `json:"messages"`
	System    string `json:"system"`
	MaxTokens int    `json:"max_tokens"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type anthropicResponse struct {
	Model string         `json:"model"`
	Usage anthropicUsage `json:"usage"`
}

// anthropicStreamEvent is a single SSE event from the Anthropic streaming API.
// Usage is carried in the message_delta event type.
type anthropicStreamEvent struct {
	Type  string `json:"type"`
	Usage *struct {
		OutputTokens int `json:"output_tokens"`
	} `json:"usage,omitempty"`
	Message *struct {
		Usage anthropicUsage `json:"usage"`
	} `json:"message,omitempty"`
}

func (p *anthropicParser) ParseRequest(body []byte) (model string, streaming bool, estimatedTokens int64, err error) {
	var req anthropicRequest
	if err = json.Unmarshal(body, &req); err != nil {
		return "", false, 0, err
	}
	var totalChars int
	for _, m := range req.Messages {
		switch v := m.Content.(type) {
		case string:
			totalChars += len(v)
		}
	}
	totalChars += len(req.System)
	estimated := int64(totalChars/4) + 1
	return req.Model, req.Stream, estimated, nil
}

func (p *anthropicParser) ParseUsage(body []byte) (inputTokens, outputTokens int64, err error) {
	var resp anthropicResponse
	if err = json.Unmarshal(body, &resp); err != nil {
		return 0, 0, err
	}
	return int64(resp.Usage.InputTokens), int64(resp.Usage.OutputTokens), nil
}

// ParseStreamingUsage reads usage from Anthropic SSE events.
// Input tokens come from the message_start event; output tokens from message_delta.
func (p *anthropicParser) ParseStreamingUsage(chunks [][]byte) (inputTokens, outputTokens int64) {
	for _, chunk := range chunks {
		line := strings.TrimSpace(string(chunk))
		// Anthropic SSE: lines beginning with "data:" carry JSON payloads.
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		var ev anthropicStreamEvent
		if err := json.Unmarshal([]byte(payload), &ev); err != nil {
			continue
		}
		switch ev.Type {
		case "message_start":
			if ev.Message != nil {
				inputTokens = int64(ev.Message.Usage.InputTokens)
			}
		case "message_delta":
			if ev.Usage != nil {
				outputTokens = int64(ev.Usage.OutputTokens)
			}
		}
	}
	return
}

func (p *anthropicParser) Upstream() string {
	return "https://api.anthropic.com"
}

// AuthHeader for Anthropic is x-api-key, not Authorization.
func (p *anthropicParser) AuthHeader() string {
	return "x-api-key"
}

func (p *anthropicParser) AuthValue(realKey string) string {
	return realKey // Anthropic doesn't use "Bearer" prefix
}
