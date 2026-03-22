package proxy

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
)

type vertexAIParser struct {
	endpoint string
}

type vertexAIRequest struct {
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

type vertexAIResponse struct {
	Model         string `json:"model,omitempty"`
	ModelVersion  string `json:"modelVersion,omitempty"`
	UsageMetadata struct {
		PromptTokenCount        int `json:"promptTokenCount"`
		CandidatesTokenCount    int `json:"candidatesTokenCount"`
		TotalTokenCount         int `json:"totalTokenCount"`
		CachedContentTokenCount int `json:"cachedContentTokenCount"`
		ThoughtsTokenCount      int `json:"thoughtsTokenCount"`
	} `json:"usageMetadata"`
}

func (p *vertexAIParser) ParseRequest(r *http.Request, body []byte) (model string, streaming bool, estimatedTokens int64, err error) {
	var req vertexAIRequest
	if err = json.Unmarshal(body, &req); err != nil {
		return "", false, 0, err
	}
	model = strings.TrimSpace(req.Model)
	if model == "" {
		model = vertexAIModelFromPath(requestPath(r))
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

func (p *vertexAIParser) ParseUsage(body []byte) (inputTokens, outputTokens int64, err error) {
	var resp vertexAIResponse
	if err = json.Unmarshal(body, &resp); err != nil {
		return 0, 0, err
	}
	return int64(resp.UsageMetadata.PromptTokenCount), int64(resp.UsageMetadata.CandidatesTokenCount), nil
}

func (p *vertexAIParser) ParseStreamingUsage(chunks [][]byte) (inputTokens, outputTokens int64) {
	for _, chunk := range chunks {
		line := strings.TrimSpace(string(chunk))
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var resp vertexAIResponse
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

func (p *vertexAIParser) Upstream() string {
	if strings.TrimSpace(p.endpoint) != "" {
		return p.endpoint
	}
	if endpoint := strings.TrimSpace(os.Getenv("AF_VERTEXAI_ENDPOINT")); endpoint != "" {
		return endpoint
	}
	return "https://us-central1-aiplatform.googleapis.com"
}

func (p *vertexAIParser) AuthHeader() string {
	return "Authorization"
}

func (p *vertexAIParser) AuthValue(realKey string) string {
	return "Bearer " + realKey
}

func vertexAIModelFromPath(path string) string {
	normalized := strings.TrimSpace(path)
	markers := []string{"/publishers/google/models/", "/models/"}
	for _, marker := range markers {
		index := strings.Index(strings.ToLower(normalized), strings.ToLower(marker))
		if index < 0 {
			continue
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
	return ""
}
