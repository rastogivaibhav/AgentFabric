package proxy

import (
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	priced "github.com/govagn/api-gateway/internal/pricing"
)

type cachedProxyResponse struct {
	StatusCode int
	Header     http.Header
	Body       []byte
	Usage      priced.Usage
	ExpiresAt  time.Time
}

type proxyResponseCache struct {
	mu           sync.RWMutex
	ttl          time.Duration
	maxEntrySize int
	entries      map[string]cachedProxyResponse
}

func newProxyResponseCache() *proxyResponseCache {
	return &proxyResponseCache{
		ttl:          90 * time.Second,
		maxEntrySize: 512 * 1024,
		entries:      make(map[string]cachedProxyResponse),
	}
}

func (c *proxyResponseCache) Key(tenantID, provider, model, path, rawQuery string, body []byte) string {
	sum := sha256.Sum256([]byte(tenantID + "|" + provider + "|" + model + "|" + path + "|" + rawQuery + "|" + string(body)))
	return hex.EncodeToString(sum[:])
}

func (c *proxyResponseCache) Get(key string) (cachedProxyResponse, bool) {
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok {
		return cachedProxyResponse{}, false
	}
	if time.Now().After(entry.ExpiresAt) {
		c.mu.Lock()
		delete(c.entries, key)
		c.mu.Unlock()
		return cachedProxyResponse{}, false
	}
	return cloneCachedResponse(entry), true
}

func (c *proxyResponseCache) Put(key string, response cachedProxyResponse) {
	if len(response.Body) == 0 || len(response.Body) > c.maxEntrySize {
		return
	}
	if response.StatusCode != http.StatusOK {
		return
	}
	response.ExpiresAt = time.Now().Add(c.ttl)
	c.mu.Lock()
	c.entries[key] = cloneCachedResponse(response)
	c.mu.Unlock()
}

func cloneCachedResponse(entry cachedProxyResponse) cachedProxyResponse {
	clonedHeaders := make(http.Header, len(entry.Header))
	for key, values := range entry.Header {
		copied := make([]string, len(values))
		copy(copied, values)
		clonedHeaders[key] = copied
	}
	clonedBody := make([]byte, len(entry.Body))
	copy(clonedBody, entry.Body)
	entry.Header = clonedHeaders
	entry.Body = clonedBody
	return entry
}
