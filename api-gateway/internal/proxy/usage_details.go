package proxy

import (
	"encoding/json"
	"strings"

	priced "github.com/govagn/api-gateway/internal/pricing"
)

func ParseDetailedUsage(provider string, body []byte) (priced.Usage, error) {
	switch NormalizeProvider(provider) {
	case ProviderOpenAI, ProviderAzureOpenAI:
		var resp struct {
			Usage struct {
				PromptTokens       int `json:"prompt_tokens"`
				CompletionTokens   int `json:"completion_tokens"`
				PromptTokenDetails struct {
					CachedTokens int `json:"cached_tokens"`
				} `json:"prompt_tokens_details"`
				CompletionTokenDetails struct {
					ReasoningTokens int `json:"reasoning_tokens"`
				} `json:"completion_tokens_details"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return priced.Usage{}, err
		}
		inputTokens := int64(resp.Usage.PromptTokens - resp.Usage.PromptTokenDetails.CachedTokens)
		if inputTokens < 0 {
			inputTokens = int64(resp.Usage.PromptTokens)
		}
		outputTokens := int64(resp.Usage.CompletionTokens - resp.Usage.CompletionTokenDetails.ReasoningTokens)
		if outputTokens < 0 {
			outputTokens = int64(resp.Usage.CompletionTokens)
		}
		return priced.Usage{
			InputTokens:     inputTokens,
			OutputTokens:    outputTokens,
			CacheReadTokens: int64(resp.Usage.PromptTokenDetails.CachedTokens),
			ReasoningTokens: int64(resp.Usage.CompletionTokenDetails.ReasoningTokens),
		}, nil
	case ProviderAnthropic:
		var resp struct {
			Usage struct {
				InputTokens              int `json:"input_tokens"`
				OutputTokens             int `json:"output_tokens"`
				CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
				CacheReadInputTokens     int `json:"cache_read_input_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return priced.Usage{}, err
		}
		return priced.Usage{
			InputTokens:      int64(resp.Usage.InputTokens),
			OutputTokens:     int64(resp.Usage.OutputTokens),
			CacheReadTokens:  int64(resp.Usage.CacheReadInputTokens),
			CacheWriteTokens: int64(resp.Usage.CacheCreationInputTokens),
		}, nil
	case ProviderGoogle, ProviderVertexAI:
		var resp struct {
			UsageMetadata struct {
				PromptTokenCount        int `json:"promptTokenCount"`
				CandidatesTokenCount    int `json:"candidatesTokenCount"`
				CachedContentTokenCount int `json:"cachedContentTokenCount"`
				ThoughtsTokenCount      int `json:"thoughtsTokenCount"`
			} `json:"usageMetadata"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return priced.Usage{}, err
		}
		outputTokens := int64(resp.UsageMetadata.CandidatesTokenCount - resp.UsageMetadata.ThoughtsTokenCount)
		if outputTokens < 0 {
			outputTokens = int64(resp.UsageMetadata.CandidatesTokenCount)
		}
		return priced.Usage{
			InputTokens:     int64(resp.UsageMetadata.PromptTokenCount),
			OutputTokens:    outputTokens,
			CacheReadTokens: int64(resp.UsageMetadata.CachedContentTokenCount),
			ReasoningTokens: int64(resp.UsageMetadata.ThoughtsTokenCount),
		}, nil
	case ProviderBedrock:
		if usage, ok, err := parseBedrockUsage(body); err != nil {
			return priced.Usage{}, err
		} else if ok {
			return priced.Usage{
				InputTokens:  usage.InputTokens,
				OutputTokens: usage.OutputTokens,
			}, nil
		}
		return priced.Usage{}, nil
	default:
		return priced.Usage{}, nil
	}
}

func ParseDetailedStreamingUsage(provider string, chunks [][]byte, parser ProviderParser) priced.Usage {
	inputTokens, outputTokens := parser.ParseStreamingUsage(chunks)
	usage := priced.Usage{
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
	}
	switch NormalizeProvider(provider) {
	case ProviderOpenAI, ProviderAzureOpenAI, ProviderAnthropic, ProviderGoogle, ProviderVertexAI, ProviderBedrock:
		for _, chunk := range chunks {
			line := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(string(chunk)), "data:"))
			if line == "" || line == "[DONE]" {
				continue
			}
			if detailed, err := ParseDetailedUsage(provider, []byte(line)); err == nil {
				if detailed.TotalTokens() > 0 {
					usage = detailed
				}
			}
		}
	}
	return usage
}
