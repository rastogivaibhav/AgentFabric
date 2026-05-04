package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/govagn/collector/internal/auth"
	"github.com/govagn/collector/internal/config"
	"github.com/govagn/collector/internal/exporter"
	"github.com/govagn/collector/internal/processor"
	"github.com/govagn/collector/internal/receiver"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
)

var gatewayReadyProbeClient = &http.Client{
	Timeout: 5 * time.Second,
	CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	},
}

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to load config: %v\n", err)
		os.Exit(1)
	}

	logger, _ := zap.NewProduction()
	if cfg.Debug {
		logger, _ = zap.NewDevelopment()
	}
	defer logger.Sync()
	if err := processor.ConfigurePricingFromEnv(); err != nil {
		logger.Fatal("invalid pricing configuration", zap.Error(err))
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Build processing pipeline
	spanExporter := exporter.NewHTTPExporter(cfg.Gateway.Endpoint, cfg.Gateway.AuthToken, logger)
	spanProcessor := processor.NewAgentProcessor(cfg, logger, spanExporter)
	jwtValidator := auth.NewJWTValidator(cfg.Auth.JWTSecret, cfg.Auth.RequireAuth)

	// --- gRPC OTLP Receiver (mTLS enforced) ---
	grpcOpts := []grpc.ServerOption{
		grpc.KeepaliveParams(keepalive.ServerParameters{
			MaxConnectionIdle: 15 * time.Second,
			Time:              5 * time.Second,
			Timeout:           1 * time.Second,
		}),
		grpc.MaxRecvMsgSize(32 * 1024 * 1024), // 32MB
		grpc.ChainUnaryInterceptor(
			auth.GRPCRateLimiter(cfg.RateLimit.SpansPerSecond),
			auth.GRPCTokenValidator(jwtValidator),
		),
	}

	if cfg.TLS.Enabled {
		tlsCreds, err := newServerTLSCredentials(cfg.TLS, logger)
		if err != nil {
			logger.Fatal("failed to load TLS credentials", zap.Error(err))
		}
		grpcOpts = append(grpcOpts, grpc.Creds(tlsCreds))
		if cfg.TLS.MutualTLS {
			logger.Info("mTLS enabled: requiring client certificate verification")
		}
	}

	grpcServer := grpc.NewServer(grpcOpts...)
	otlpReceiver := receiver.NewOTLPReceiver(spanProcessor, logger, cfg)
	coltracepb.RegisterTraceServiceServer(grpcServer, otlpReceiver)

	grpcLis, err := net.Listen("tcp", cfg.GRPC.Addr)
	if err != nil {
		logger.Fatal("gRPC listen failed", zap.Error(err))
	}

	// --- HTTP OTLP + Metrics ---
	httpMux := http.NewServeMux()
	httpMux.Handle("/v1/traces", receiver.NewHTTPOTLPHandler(spanProcessor, jwtValidator, logger))
	httpMux.Handle("/metrics", promhttp.Handler())

	// Multi-source receivers
	vscodeReceiver := receiver.NewVSCodeExtensionReceiver()
	webhookReceiver := receiver.NewWebhookReceiver()
	directAPIReceiver := receiver.NewDirectAPIReceiver()
	vscodeReceiver.SetSubmitter(spanProcessor)
	webhookReceiver.SetSubmitter(spanProcessor)
	directAPIReceiver.SetSubmitter(spanProcessor)

	httpMux.HandleFunc("/api/v1/telemetry/vscode", receiver.VSCodeExtensionHandler(vscodeReceiver))
	httpMux.HandleFunc("/api/v1/telemetry/webhook", receiver.WebhookHandler(webhookReceiver))
	httpMux.HandleFunc("/api/v1/telemetry/events", receiver.DirectAPIHandler(directAPIReceiver))

	healthHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok","version":"1.0.0","mode":"health","checks":{"receiver":{"status":"ok"},"gateway_export":{"status":"configured"}}}`))
	}
	readyHandler := func(w http.ResponseWriter, r *http.Request) {
		collectorReady(w, r, cfg)
	}
	httpMux.HandleFunc("/healthz", healthHandler)
	httpMux.HandleFunc("/readyz", readyHandler)

	httpServer := newHTTPServer(cfg.HTTP.Addr, httpMux)

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		logger.Info("gRPC receiver listening", zap.String("addr", cfg.GRPC.Addr))
		if err := grpcServer.Serve(grpcLis); err != nil {
			logger.Error("gRPC serve error", zap.Error(err))
		}
	}()

	go func() {
		logger.Info("HTTP receiver listening", zap.String("addr", cfg.HTTP.Addr))
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP serve error", zap.Error(err))
		}
	}()

	// Start background discovery scan (k8s + process discovery)
	go spanProcessor.RunDiscovery(ctx)

	logger.Info("Govagn Collector started",
		zap.String("grpc", cfg.GRPC.Addr),
		zap.String("http", cfg.HTTP.Addr),
		zap.String("node", cfg.NodeName),
	)

	<-sigCh
	logger.Info("Shutting down collector...")

	grpcServer.GracefulStop()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer shutdownCancel()
	httpServer.Shutdown(shutdownCtx)
	spanProcessor.Shutdown()

	logger.Info("Collector stopped cleanly")
}

func newHTTPServer(addr string, handler http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadTimeout:       10 * time.Second,
		ReadHeaderTimeout: 10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
}

func collectorReady(w http.ResponseWriter, r *http.Request, cfg *config.Config) {
	w.Header().Set("Content-Type", "application/json")
	response := map[string]any{
		"status": "ok",
		"mode":   "ready",
		"checks": map[string]map[string]any{
			"receiver":           {"status": "ok"},
			"gateway_export":     {"status": "configured"},
			"pricing_config":     {"status": "loaded"},
			"gateway_auth_token": {"status": "configured", "contract": "Authorization: Bearer <GV_GATEWAY_AUTH_TOKEN>"},
		},
	}
	checks := response["checks"].(map[string]map[string]any)

	if cfg == nil {
		response["status"] = "degraded"
		response["error"] = "collector config unavailable"
		checks["receiver"] = map[string]any{"status": "unavailable"}
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(response)
		return
	}
	if strings.TrimSpace(cfg.Gateway.AuthToken) == "" {
		response["status"] = "degraded"
		response["error"] = "gateway auth token missing"
		checks["gateway_auth_token"] = map[string]any{"status": "unavailable"}
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(response)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	url := strings.TrimRight(cfg.Gateway.Endpoint, "/") + "/readyz"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		response["status"] = "degraded"
		response["error"] = "gateway probe build failed"
		checks["gateway_readyz"] = map[string]any{"status": "unavailable", "error": err.Error()}
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(response)
		return
	}
	resp, err := gatewayReadyProbeClient.Do(req)
	if err != nil {
		response["status"] = "degraded"
		response["error"] = "gateway readyz unreachable"
		checks["gateway_readyz"] = map[string]any{"status": "unavailable", "url": url, "error": err.Error()}
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(response)
		return
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode != http.StatusOK {
		response["status"] = "degraded"
		response["error"] = "gateway readyz returned non-200"
		checks["gateway_readyz"] = map[string]any{"status": "unavailable", "url": url, "status_code": resp.StatusCode}
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(response)
		return
	}
	checks["gateway_readyz"] = map[string]any{"status": "ok", "url": url}
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}

// newServerTLSCredentials creates gRPC server TLS credentials with optional mTLS
func newServerTLSCredentials(tlsCfg config.TLSConfig, logger *zap.Logger) (credentials.TransportCredentials, error) {
	// Load server certificate and key
	serverCert, err := tls.LoadX509KeyPair(tlsCfg.CertFile, tlsCfg.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("load server cert: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{serverCert},
	}

	// If mTLS enabled, require client certificates
	if tlsCfg.MutualTLS {
		if tlsCfg.ClientCA == "" {
			return nil, fmt.Errorf("mTLS enabled but client_ca not specified")
		}

		clientCABytes, err := ioutil.ReadFile(tlsCfg.ClientCA)
		if err != nil {
			return nil, fmt.Errorf("read client CA: %w", err)
		}

		clientCAPool := x509.NewCertPool()
		if !clientCAPool.AppendCertsFromPEM(clientCABytes) {
			return nil, fmt.Errorf("parse client CA certificates")
		}

		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		tlsConfig.ClientCAs = clientCAPool

		logger.Info("mTLS configured",
			zap.String("client_ca", tlsCfg.ClientCA),
			zap.String("server_cert", tlsCfg.CertFile),
		)
	}

	return credentials.NewTLS(tlsConfig), nil
}
