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

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		e.endpoint+"/internal/ingest", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+e.authToken)
	req.Header.Set("X-AF-Source", "collector")

	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("http post: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("gateway returned %d", resp.StatusCode)
	}
	return nil
}
