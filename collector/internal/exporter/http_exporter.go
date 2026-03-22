package exporter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"
)

type Exporter interface {
	Export(ctx context.Context, spans interface{}) error
}

type HTTPExporter struct {
	endpoint  string
	authToken string
	client    *http.Client
	logger    *zap.Logger
}

func NewHTTPExporter(endpoint, authToken string, logger *zap.Logger) *HTTPExporter {
	return &HTTPExporter{
		endpoint:  endpoint,
		authToken: authToken,
		logger:    logger,
		client: &http.Client{
			Timeout: 15 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 20,
				IdleConnTimeout:     90 * time.Second,
			},
		},
	}
}

func (e *HTTPExporter) Export(ctx context.Context, spans interface{}) error {
	body, err := json.Marshal(map[string]interface{}{"spans": spans})
	if err != nil {
		return fmt.Errorf("marshal spans: %w", err)
	}

	backoffs := []time.Duration{0, 250 * time.Millisecond, 750 * time.Millisecond}
	var lastErr error
	for attempt, wait := range backoffs {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return fmt.Errorf("export canceled after retry wait: %w", ctx.Err())
			case <-time.After(wait):
			}
		}

		req, reqErr := http.NewRequestWithContext(ctx, http.MethodPost,
			e.endpoint+"/internal/ingest", bytes.NewReader(body))
		if reqErr != nil {
			return fmt.Errorf("build request: %w", reqErr)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+e.authToken)
		req.Header.Set("X-AF-Source", "collector")

		resp, doErr := e.client.Do(req)
		if doErr != nil {
			lastErr = fmt.Errorf("http post attempt %d failed: %w", attempt+1, doErr)
			e.logger.Warn("collector export attempt failed", zap.Int("attempt", attempt+1), zap.Error(doErr))
			continue
		}

		if resp.StatusCode < 300 {
			resp.Body.Close()
			return nil
		}

		resp.Body.Close()
		lastErr = fmt.Errorf("gateway returned %d", resp.StatusCode)
		if resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
			return lastErr
		}
		e.logger.Warn("collector export got retryable response",
			zap.Int("attempt", attempt+1),
			zap.Int("status_code", resp.StatusCode))
	}
	return lastErr
}
