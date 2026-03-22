package proxy

import "net/http"

// azureOpenAIParser reuses the OpenAI-compatible payload shape but allows a
// future endpoint-specific registry entry without duplicating parsing logic.
type azureOpenAIParser struct {
	endpoint string
}

func (p *azureOpenAIParser) ParseRequest(r *http.Request, body []byte) (model string, streaming bool, estimatedTokens int64, err error) {
	return (&openAIParser{}).ParseRequest(r, body)
}

func (p *azureOpenAIParser) ParseUsage(body []byte) (inputTokens, outputTokens int64, err error) {
	return (&openAIParser{}).ParseUsage(body)
}

func (p *azureOpenAIParser) ParseStreamingUsage(chunks [][]byte) (inputTokens, outputTokens int64) {
	return (&openAIParser{}).ParseStreamingUsage(chunks)
}

func (p *azureOpenAIParser) Upstream() string {
	return p.endpoint
}

func (p *azureOpenAIParser) AuthHeader() string {
	return "api-key"
}

func (p *azureOpenAIParser) AuthValue(realKey string) string {
	return realKey
}
