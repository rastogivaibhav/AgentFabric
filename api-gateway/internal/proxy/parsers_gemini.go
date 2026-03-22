package proxy

import (
	"encoding/json"
	"net/http"
	"strings"
)

type geminiParser struct{}

type geminiRequest struct {
	Model             string `json:"model,omitempty"`
	SystemInstruction *struct {
		Parts []struct {
			Text string `json:"text"`
		} `json:"parts"`
	} `json:"systemInstruction,omitempty"`
	Contents []struct {
		Parts []struct {
			Text string `json:"text"`
		} `json:"parts"`
	} `json:"contents"`
}

type geminiUsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
	TotalTokenCount      int `json:"totalTokenCount"`
}

type geminiResponse struct {
	ModelVersion  string              `json:"modelVersion"`
	UsageMetadata geminiUsageMetadata `json:"usageMetadata"`
}

func (p *geminiParser) ParseRequest(r *http.Request, body []byte) (model string, streaming bool, estimatedTokens int64, err error) {
	var req geminiRequest
	if err = json.Unmarshal(body, &req); err != nil {
		return "", false, 0, err
	}
	model = strings.TrimSpace(req.Model)
	if model == "" {
		model = geminiModelFromPath(requestPath(r))
	}
	var totalChars int
	if req.SystemInstruction != nil {
		for _, part := range req.SystemInstruction.Parts {
			totalChars += len(part.Text)
		}
	}
	for _, content := range req.Contents {
		for _, part := range content.Parts {
			totalChars += len(part.Text)
		}
	}
	streaming = strings.Contains(strings.ToLower(requestPath(r)), ":streamgeneratecontent") || strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("alt")), "sse")
	estimatedTokens = int64(totalChars/4) + 1
	return model, streaming, estimatedTokens, nil
}

func (p *geminiParser) ParseUsage(body []byte) (inputTokens, outputTokens int64, err error) {
	var resp geminiResponse
	if err = json.Unmarshal(body, &resp); err != nil {
		return 0, 0, err
	}
	return int64(resp.UsageMetadata.PromptTokenCount), int64(resp.UsageMetadata.CandidatesTokenCount), nil
}

func (p *geminiParser) ParseStreamingUsage(chunks [][]byte) (inputTokens, outputTokens int64) {
	for _, chunk := range chunks {
		line := strings.TrimSpace(string(chunk))
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var resp geminiResponse
		if err := json.Unmarshal([]byte(payload), &resp); err != nil {
			continue
		}
		if resp.UsageMetadata.TotalTokenCount > 0 {
			inputTokens = int64(resp.UsageMetadata.PromptTokenCount)
			outputTokens = int64(resp.UsageMetadata.CandidatesTokenCount)
		}
	}
	return
}

func (p *geminiParser) Upstream() string {
	return "https://generativelanguage.googleapis.com"
}

func (p *geminiParser) AuthHeader() string {
	return "x-goog-api-key"
}

func (p *geminiParser) AuthValue(realKey string) string {
	return realKey
}

func geminiModelFromPath(path string) string {
	normalized := strings.TrimSpace(path)
	marker := "/models/"
	index := strings.Index(strings.ToLower(normalized), marker)
	if index < 0 {
		return ""
	}
	segment := normalized[index+len(marker):]
	if cut := strings.Index(segment, ":"); cut >= 0 {
		segment = segment[:cut]
	}
	if cut := strings.Index(segment, "/"); cut >= 0 {
		segment = segment[:cut]
	}
	return strings.TrimSpace(segment)
}

func requestPath(r *http.Request) string {
	if r == nil || r.URL == nil {
		return ""
	}
	return r.URL.Path
}
