package proxy

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

type RateLimitError struct {
	Scope string
	Limit int
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("%s rate limit exceeded (%d requests per minute)", e.Scope, e.Limit)
}

type rateLimitCounter struct {
	Count   int
	ResetAt time.Time
}

type requestRateLimiter struct {
	mu       sync.Mutex
	window   time.Duration
	limits   map[string]int
	counters map[string]rateLimitCounter
}

func newRequestRateLimiter() *requestRateLimiter {
	return &requestRateLimiter{
		window: time.Minute,
		limits: map[string]int{
			"key":      120,
			"user":     180,
			"team":     300,
			"tenant":   600,
			"provider": 400,
		},
		counters: make(map[string]rateLimitCounter),
	}
}

func (l *requestRateLimiter) Check(scopes map[string]string) error {
	if l == nil {
		return nil
	}
	now := time.Now().UTC()
	l.mu.Lock()
	defer l.mu.Unlock()
	for scope, value := range scopes {
		limit := l.limits[scope]
		trimmed := strings.TrimSpace(value)
		if limit <= 0 || trimmed == "" {
			continue
		}
		key := scope + ":" + trimmed
		counter := l.counters[key]
		if counter.ResetAt.IsZero() || now.After(counter.ResetAt) {
			counter = rateLimitCounter{ResetAt: now.Add(l.window)}
		}
		if counter.Count >= limit {
			return &RateLimitError{Scope: scope, Limit: limit}
		}
		counter.Count++
		l.counters[key] = counter
	}
	return nil
}
