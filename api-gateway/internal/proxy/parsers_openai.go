package proxy

import (
	"encoding/json"
	"net/http"
	"strings"
)

// openAIParser handles OpenAI chat completions request/response parsing.
type openAIParser struct{}

// openAIRequest is the subset of the OpenAI chat completions request we need.
type openAIRequest struct {
	Model    string `json:"model"`
	Stream   bool   `json:"stream"`
	Messages []struct {
		Role    string `json:"role"`
		Content any    `json:"content"` // string or []part
	} `json:"messages"`
}

// openAIUsage is the usage block in a non-streaming response.
type openAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

type openAIResponse struct {
	Model  string      `json:"model"`
	Usage  openAIUsage `json:"usage"`
	Object string      `json:"object"`
}

// openAIStreamChunk is a single SSE data payload.
type openAIStreamChunk struct {
	Object string      `json:"object"`
	Usage  openAIUsage `json:"usage"` // only in the final chunk with stream_options.include_usage
}

func (p *openAIParser) ParseRequest(_ *http.Request, body []byte) (model string, streaming bool, estimatedTokens int64, err error) {
	var req openAIRequest
	if err = json.Unmarshal(body, &req); err != nil {
		return "", false, 0, err
	}
	// Rough token estimate: 4 chars ≈ 1 token across all messages.
	var totalChars int
	for _, m := range req.Messages {
		switch v := m.Content.(type) {
		case string:
			totalChars += len(v)
		}
	}
	estimated := int64(totalChars/4) + 1
	return req.Model, req.Stream, estimated, nil
}

func (p *openAIParser) ParseUsage(body []byte) (inputTokens, outputTokens int64, err error) {
	var resp openAIResponse
	if err = json.Unmarshal(body, &resp); err != nil {
		return 0, 0, err
	}
	return int64(resp.Usage.PromptTokens), int64(resp.Usage.CompletionTokens), nil
}

// ParseStreamingUsage scans SSE chunks for the usage field.
// OpenAI only emits usage in the final chunk when stream_options.include_usage=true.
func (p *openAIParser) ParseStreamingUsage(chunks [][]byte) (inputTokens, outputTokens int64) {
	for _, chunk := range chunks {
		// SSE lines look like: data: {...}
		line := strings.TrimPrefix(strings.TrimSpace(string(chunk)), "data: ")
		if line == "[DONE]" || line == "" {
			continue
		}
		var c openAIStreamChunk
		if err := json.Unmarshal([]byte(line), &c); err != nil {
			continue
		}
		if c.Usage.TotalTokens > 0 {
			inputTokens = int64(c.Usage.PromptTokens)
			outputTokens = int64(c.Usage.CompletionTokens)
		}
	}
	return
}

// upstream returns the base URL for the OpenAI API.
func (p *openAIParser) Upstream() string {
	return "https://api.openai.com"
}

// AuthHeader returns the header name used to carry the API key.
func (p *openAIParser) AuthHeader() string {
	return "Authorization"
}

// AuthValue formats the real key as an Authorization header value.
func (p *openAIParser) AuthValue(realKey string) string {
	return "Bearer " + realKey
}
