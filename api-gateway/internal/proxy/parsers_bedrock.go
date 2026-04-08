package proxy

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
)

type bedrockParser struct {
	endpoint string
}

func (p *bedrockParser) ParseRequest(r *http.Request, body []byte) (model string, streaming bool, estimatedTokens int64, err error) {
	model = bedrockModelFromPath(requestPath(r))
	streaming = strings.Contains(strings.ToLower(requestPath(r)), "response-stream") || strings.Contains(strings.ToLower(requestPath(r)), "converse-stream")
	estimatedTokens = int64(sumStringValues(body)/4) + 1
	return model, streaming, estimatedTokens, nil
}

func (p *bedrockParser) ParseUsage(body []byte) (inputTokens, outputTokens int64, err error) {
	if usage, ok, parseErr := parseBedrockUsage(body); ok || parseErr != nil {
		return usage.InputTokens, usage.OutputTokens, parseErr
	}
	return 0, 0, nil
}

func (p *bedrockParser) ParseStreamingUsage(chunks [][]byte) (inputTokens, outputTokens int64) {
	for _, chunk := range chunks {
		line := strings.TrimSpace(string(chunk))
		if strings.HasPrefix(line, "data:") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		}
		if line == "" || line == "[DONE]" {
			continue
		}
		if usage, ok, err := parseBedrockUsage([]byte(line)); err == nil && ok {
			inputTokens = usage.InputTokens
			outputTokens = usage.OutputTokens
		}
	}
	return
}

func (p *bedrockParser) Upstream() string {
	if strings.TrimSpace(p.endpoint) != "" {
		return p.endpoint
	}
	if endpoint := strings.TrimSpace(os.Getenv("GV_BEDROCK_ENDPOINT")); endpoint != "" {
		return endpoint
	}
	return "https://bedrock-runtime.us-east-1.amazonaws.com"
}

func (p *bedrockParser) AuthHeader() string {
	return "Authorization"
}

func (p *bedrockParser) AuthValue(realKey string) string {
	return "Bearer " + realKey
}

func bedrockModelFromPath(path string) string {
	normalized := strings.TrimSpace(path)
	markers := []string{"/model/", "/foundation-model/"}
	for _, marker := range markers {
		index := strings.Index(strings.ToLower(normalized), strings.ToLower(marker))
		if index < 0 {
			continue
		}
		segment := normalized[index+len(marker):]
		if cut := strings.Index(segment, "/"); cut >= 0 {
			segment = segment[:cut]
		}
		if cut := strings.Index(segment, ":invoke"); cut >= 0 {
			segment = segment[:cut]
		}
		return strings.TrimSpace(segment)
	}
	return ""
}

func parseBedrockUsage(body []byte) (pricedUsage usageParts, ok bool, err error) {
	type anthropicShape struct {
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}
	var anthropic anthropicShape
	if err = json.Unmarshal(body, &anthropic); err == nil {
		if anthropic.Usage.InputTokens > 0 || anthropic.Usage.OutputTokens > 0 {
			return usageParts{InputTokens: int64(anthropic.Usage.InputTokens), OutputTokens: int64(anthropic.Usage.OutputTokens)}, true, nil
		}
	}
	type converseShape struct {
		Usage struct {
			InputTokens  int `json:"inputTokens"`
			OutputTokens int `json:"outputTokens"`
		} `json:"usage"`
		Metrics struct {
			InputTokenCount  int `json:"inputTokenCount"`
			OutputTokenCount int `json:"outputTokenCount"`
		} `json:"metrics"`
	}
	var converse converseShape
	if err = json.Unmarshal(body, &converse); err != nil {
		return usageParts{}, false, err
	}
	if converse.Usage.InputTokens > 0 || converse.Usage.OutputTokens > 0 {
		return usageParts{InputTokens: int64(converse.Usage.InputTokens), OutputTokens: int64(converse.Usage.OutputTokens)}, true, nil
	}
	if converse.Metrics.InputTokenCount > 0 || converse.Metrics.OutputTokenCount > 0 {
		return usageParts{InputTokens: int64(converse.Metrics.InputTokenCount), OutputTokens: int64(converse.Metrics.OutputTokenCount)}, true, nil
	}
	return usageParts{}, false, nil
}

type usageParts struct {
	InputTokens  int64
	OutputTokens int64
}

func sumStringValues(body []byte) int {
	var value any
	if err := json.Unmarshal(body, &value); err != nil {
		return len(body)
	}
	return sumStringValueNode(value)
}

func sumStringValueNode(value any) int {
	switch v := value.(type) {
	case string:
		return len(v)
	case []any:
		total := 0
		for _, item := range v {
			total += sumStringValueNode(item)
		}
		return total
	case map[string]any:
		total := 0
		for _, item := range v {
			total += sumStringValueNode(item)
		}
		return total
	default:
		return 0
	}
}
