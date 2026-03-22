package proxy

import (
	"net/http"
	"sort"
	"strings"
)

const (
	ProviderOpenAI      = "openai"
	ProviderAnthropic   = "anthropic"
	ProviderGoogle      = "google"
	ProviderVertexAI    = "vertexai"
	ProviderBedrock     = "bedrock"
	ProviderAzureOpenAI = "azure-openai"
)

type providerDefinition struct {
	Name         string
	DisplayName  string
	Aliases      []string
	Hosts        []string
	HostSuffixes []string
	HostContains []string
	NewParser    func() ProviderParser
}

var providerRegistry = map[string]providerDefinition{
	ProviderOpenAI: {
		Name:        ProviderOpenAI,
		DisplayName: "OpenAI",
		Aliases:     []string{ProviderOpenAI},
		Hosts:       []string{"api.openai.com"},
		NewParser:   func() ProviderParser { return &openAIParser{} },
	},
	ProviderAnthropic: {
		Name:        ProviderAnthropic,
		DisplayName: "Anthropic",
		Aliases:     []string{ProviderAnthropic},
		Hosts:       []string{"api.anthropic.com"},
		NewParser:   func() ProviderParser { return &anthropicParser{} },
	},
	ProviderGoogle: {
		Name:        ProviderGoogle,
		DisplayName: "Google Gemini",
		Aliases:     []string{ProviderGoogle, "gemini"},
		Hosts:       []string{"generativelanguage.googleapis.com"},
		NewParser:   func() ProviderParser { return &geminiParser{} },
	},
	ProviderVertexAI: {
		Name:         ProviderVertexAI,
		DisplayName:  "Google Vertex AI",
		Aliases:      []string{ProviderVertexAI, "vertex-ai", "vertex"},
		Hosts:        []string{"us-central1-aiplatform.googleapis.com", "aiplatform.googleapis.com"},
		HostSuffixes: []string{"-aiplatform.googleapis.com"},
		HostContains: []string{"aiplatform.googleapis.com"},
		NewParser:    func() ProviderParser { return &vertexAIParser{} },
	},
	ProviderBedrock: {
		Name:         ProviderBedrock,
		DisplayName:  "AWS Bedrock",
		Aliases:      []string{ProviderBedrock, "aws-bedrock"},
		Hosts:        []string{"bedrock-runtime.us-east-1.amazonaws.com"},
		HostContains: []string{"bedrock-runtime."},
		NewParser:    func() ProviderParser { return &bedrockParser{} },
	},
}

func NormalizeProvider(provider string) string {
	normalized := strings.ToLower(strings.TrimSpace(provider))
	for canonical, definition := range providerRegistry {
		for _, alias := range definition.Aliases {
			if normalized == alias {
				return canonical
			}
		}
	}
	return normalized
}

func IsSupportedProvider(provider string) bool {
	_, ok := providerRegistry[NormalizeProvider(provider)]
	return ok
}

func SupportedProviders() []string {
	out := make([]string, 0, len(providerRegistry))
	for name := range providerRegistry {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func ProviderDisplayName(provider string) string {
	if definition, ok := providerRegistry[NormalizeProvider(provider)]; ok {
		return definition.DisplayName
	}
	return provider
}

func ProviderForHost(host string) (string, bool) {
	normalizedHost := strings.ToLower(strings.TrimSpace(host))
	for canonical, definition := range providerRegistry {
		for _, knownHost := range definition.Hosts {
			if normalizedHost == knownHost {
				return canonical, true
			}
		}
		for _, suffix := range definition.HostSuffixes {
			if strings.HasSuffix(normalizedHost, suffix) {
				return canonical, true
			}
		}
		for _, fragment := range definition.HostContains {
			if strings.Contains(normalizedHost, fragment) {
				return canonical, true
			}
		}
	}
	return "", false
}

func ProviderRouteHint(provider string) string {
	switch NormalizeProvider(provider) {
	case ProviderOpenAI:
		return "/proxy/openai/v1/chat/completions"
	case ProviderAnthropic:
		return "/proxy/anthropic/v1/messages"
	case ProviderGoogle:
		return "/proxy/google/v1beta/models/gemini-1.5-pro:generateContent"
	case ProviderVertexAI:
		return "/proxy/vertexai/v1/projects/demo/locations/us-central1/publishers/google/models/gemini-1.5-pro:generateContent"
	case ProviderBedrock:
		return "/proxy/bedrock/model/anthropic.claude-3-5-sonnet-20240620-v1:0/invoke"
	default:
		return "/proxy/" + NormalizeProvider(provider)
	}
}

func parserFor(provider string) (ProviderParser, bool) {
	definition, ok := providerRegistry[NormalizeProvider(provider)]
	if !ok || definition.NewParser == nil {
		return nil, false
	}
	return definition.NewParser(), true
}

func canonicalProviderFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	provider, _ := r.Context().Value(ctxKeyProvider{}).(string)
	return NormalizeProvider(provider)
}
