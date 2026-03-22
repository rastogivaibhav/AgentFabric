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

func TestSupportedProviders_IncludesGoogle(t *testing.T) {
	assert.Contains(t, SupportedProviders(), ProviderGoogle)
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
}
