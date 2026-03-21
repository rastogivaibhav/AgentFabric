package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/agentfabric/collector/internal/auth"
	"github.com/agentfabric/collector/internal/config"
	"github.com/agentfabric/collector/internal/exporter"
	"github.com/agentfabric/collector/internal/processor"
	"github.com/agentfabric/collector/internal/receiver"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"

	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
)

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
	jwtValidator := auth.NewJWTValidator(cfg.Auth.JWTSecret)

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
		tlsCreds, err := credentials.NewServerTLSFromFile(cfg.TLS.CertFile, cfg.TLS.KeyFile)
		if err != nil {
			logger.Fatal("failed to load TLS creds", zap.Error(err))
		}
		grpcOpts = append(grpcOpts, grpc.Creds(tlsCreds))
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
	httpMux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok","version":"1.0.0"}`))
	})

	httpServer := &http.Server{
		Addr:         cfg.HTTP.Addr,
		Handler:      httpMux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

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

	logger.Info("AgentFabric Collector started",
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
