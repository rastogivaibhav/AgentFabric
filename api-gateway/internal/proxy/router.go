package proxy

import (
	"encoding/json"
	"strings"
)

type RouteCandidate struct {
	Provider string
	Model    string
	Path     string
	Source   string
}

type ProviderRouter struct{}

func NewProviderRouter() *ProviderRouter {
	return &ProviderRouter{}
}

func (r *ProviderRouter) Resolve(_ string, provider, model, path string) []RouteCandidate {
	canonicalProvider := NormalizeProvider(provider)
	normalizedModel := strings.TrimSpace(model)
	normalizedPath := strings.TrimSpace(path)
	candidates := []RouteCandidate{{
		Provider: canonicalProvider,
		Model:    normalizedModel,
		Path:     normalizedPath,
		Source:   "primary",
	}}
	if fallbackModel := defaultFallbackModel(canonicalProvider, normalizedModel); fallbackModel != "" && fallbackModel != normalizedModel {
		candidates = append(candidates, RouteCandidate{
			Provider: canonicalProvider,
			Model:    fallbackModel,
			Path:     normalizedPath,
			Source:   "fallback",
		})
	}
	return candidates
}

func (r *ProviderRouter) Apply(provider string, body []byte, candidate RouteCandidate, path string) ([]byte, string, error) {
	targetModel := strings.TrimSpace(candidate.Model)
	if targetModel == "" {
		return body, path, nil
	}
	switch NormalizeProvider(provider) {
	case ProviderOpenAI, ProviderAzureOpenAI, ProviderAnthropic:
		updatedBody, err := rewriteJSONModel(body, targetModel)
		return updatedBody, path, err
	case ProviderGoogle, ProviderVertexAI:
		updatedBody, err := rewriteJSONModel(body, targetModel)
		if err != nil {
			return nil, "", err
		}
		return updatedBody, replacePathModel(path, targetModel), nil
	case ProviderBedrock:
		updatedBody, err := rewriteJSONModel(body, targetModel)
		if err != nil {
			return nil, "", err
		}
		return updatedBody, replaceBedrockPathModel(path, targetModel), nil
	default:
		return body, path, nil
	}
}

func rewriteJSONModel(body []byte, model string) ([]byte, error) {
	if len(body) == 0 {
		return body, nil
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return body, nil
	}
	payload["model"] = model
	if _, ok := payload["modelId"]; ok {
		payload["modelId"] = model
	}
	updatedBody, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return updatedBody, nil
}

func replacePathModel(path, targetModel string) string {
	replacers := []string{"/models/", "/publishers/google/models/"}
	for _, marker := range replacers {
		index := strings.Index(strings.ToLower(path), strings.ToLower(marker))
		if index < 0 {
			continue
		}
		start := index + len(marker)
		end := len(path)
		if cut := strings.Index(path[start:], ":"); cut >= 0 {
			end = start + cut
		} else if cut := strings.Index(path[start:], "/"); cut >= 0 {
			end = start + cut
		}
		return path[:start] + targetModel + path[end:]
	}
	return path
}

func replaceBedrockPathModel(path, targetModel string) string {
	markers := []string{"/model/", "/foundation-model/"}
	for _, marker := range markers {
		index := strings.Index(strings.ToLower(path), strings.ToLower(marker))
		if index < 0 {
			continue
		}
		start := index + len(marker)
		end := len(path)
		if cut := strings.Index(path[start:], "/"); cut >= 0 {
			end = start + cut
		}
		return path[:start] + targetModel + path[end:]
	}
	return path
}

func defaultFallbackModel(provider, model string) string {
	normalizedModel := strings.ToLower(strings.TrimSpace(model))
	switch NormalizeProvider(provider) {
	case ProviderOpenAI:
		switch {
		case strings.HasPrefix(normalizedModel, "gpt-4o"):
			return "gpt-4o-mini"
		case strings.HasPrefix(normalizedModel, "gpt-4.1"):
			return "gpt-4o-mini"
		}
	case ProviderAnthropic, ProviderBedrock:
		switch {
		case strings.Contains(normalizedModel, "sonnet"):
			return strings.Replace(model, "sonnet", "haiku", 1)
		case strings.Contains(normalizedModel, "opus"):
			return strings.Replace(model, "opus", "sonnet", 1)
		}
	case ProviderGoogle, ProviderVertexAI:
		switch {
		case strings.Contains(normalizedModel, "gemini-1.5-pro"):
			return strings.Replace(model, "gemini-1.5-pro", "gemini-1.5-flash", 1)
		case strings.Contains(normalizedModel, "gemini-2.0-pro"):
			return strings.Replace(model, "gemini-2.0-pro", "gemini-2.0-flash", 1)
		}
	}
	return ""
}
