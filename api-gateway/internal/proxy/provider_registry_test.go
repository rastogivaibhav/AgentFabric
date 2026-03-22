package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalizeProvider_GeminiAlias(t *testing.T) {
	assert.Equal(t, ProviderGoogle, NormalizeProvider("gemini"))
	assert.Equal(t, ProviderGoogle, NormalizeProvider(" google "))
}

func TestProviderForHost_Google(t *testing.T) {
	provider, ok := ProviderForHost("generativelanguage.googleapis.com")
	require.True(t, ok)
	assert.Equal(t, ProviderGoogle, provider)
}

func TestProviderForHost_VertexAISuffix(t *testing.T) {
	provider, ok := ProviderForHost("europe-west4-aiplatform.googleapis.com")
	require.True(t, ok)
	assert.Equal(t, ProviderVertexAI, provider)
}

func TestProviderForHost_BedrockSuffix(t *testing.T) {
	provider, ok := ProviderForHost("bedrock-runtime.eu-west-1.amazonaws.com")
	require.True(t, ok)
	assert.Equal(t, ProviderBedrock, provider)
}

func TestSupportedProviders_IncludesGoogle(t *testing.T) {
	assert.Contains(t, SupportedProviders(), ProviderGoogle)
	assert.Contains(t, SupportedProviders(), ProviderVertexAI)
	assert.Contains(t, SupportedProviders(), ProviderBedrock)
}

func TestGeminiParser_ParseRequest_FromPath(t *testing.T) {
	parser, ok := ParserFor(ProviderGoogle)
	require.True(t, ok)

	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-1.5-pro:generateContent", nil)
	body := []byte(`{"contents":[{"parts":[{"text":"hello from gemini"}]}]}`)
	model, streaming, estimatedTokens, err := parser.ParseRequest(req, body)
	require.NoError(t, err)
	assert.Equal(t, "gemini-1.5-pro", model)
	assert.False(t, streaming)
	assert.Greater(t, estimatedTokens, int64(0))
}

func TestGeminiParser_ParseStreamingRequest(t *testing.T) {
	parser, ok := ParserFor("gemini")
	require.True(t, ok)

	req := httptest.NewRequest(http.MethodPost, "/v1beta/models/gemini-1.5-flash:streamGenerateContent?alt=sse", nil)
	body := []byte(`{"contents":[{"parts":[{"text":"stream please"}]}]}`)
	model, streaming, _, err := parser.ParseRequest(req, body)
	require.NoError(t, err)
	assert.Equal(t, "gemini-1.5-flash", model)
	assert.True(t, streaming)
}

func TestGeminiParser_ParseUsage(t *testing.T) {
	parser, ok := ParserFor(ProviderGoogle)
	require.True(t, ok)

	in, out, err := parser.ParseUsage([]byte(`{"usageMetadata":{"promptTokenCount":12,"candidatesTokenCount":34,"totalTokenCount":46}}`))
	require.NoError(t, err)
	assert.Equal(t, int64(12), in)
	assert.Equal(t, int64(34), out)
}

func TestProviderRouteHint(t *testing.T) {
	assert.Equal(t, "/proxy/google/v1beta/models/gemini-1.5-pro:generateContent", ProviderRouteHint(ProviderGoogle))
	assert.Equal(t, "/proxy/vertexai/v1/projects/demo/locations/us-central1/publishers/google/models/gemini-1.5-pro:generateContent", ProviderRouteHint(ProviderVertexAI))
	assert.Equal(t, "/proxy/bedrock/model/anthropic.claude-3-5-sonnet-20240620-v1:0/invoke", ProviderRouteHint(ProviderBedrock))
}

func TestVertexAIParser_ParseRequest_FromPath(t *testing.T) {
	parser, ok := ParserFor(ProviderVertexAI)
	require.True(t, ok)

	req := httptest.NewRequest(http.MethodPost, "/v1/projects/demo/locations/us-central1/publishers/google/models/gemini-1.5-pro:generateContent", nil)
	body := []byte(`{"contents":[{"parts":[{"text":"hello from vertex"}]}]}`)
	model, streaming, estimatedTokens, err := parser.ParseRequest(req, body)
	require.NoError(t, err)
	assert.Equal(t, "gemini-1.5-pro", model)
	assert.False(t, streaming)
	assert.Greater(t, estimatedTokens, int64(0))
}

func TestBedrockParser_ParseRequest_FromPath(t *testing.T) {
	parser, ok := ParserFor(ProviderBedrock)
	require.True(t, ok)

	req := httptest.NewRequest(http.MethodPost, "/model/anthropic.claude-3-5-sonnet-20240620-v1:0/invoke", nil)
	body := []byte(`{"messages":[{"role":"user","content":[{"text":"hello from bedrock"}]}]}`)
	model, streaming, estimatedTokens, err := parser.ParseRequest(req, body)
	require.NoError(t, err)
	assert.Equal(t, "anthropic.claude-3-5-sonnet-20240620-v1:0", model)
	assert.False(t, streaming)
	assert.Greater(t, estimatedTokens, int64(0))
}
