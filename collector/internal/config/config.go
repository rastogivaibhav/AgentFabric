package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/viper"
)

type Config struct {
	Debug    bool   `mapstructure:"debug"`
	NodeName string `mapstructure:"node_name"`

	GRPC struct {
		Addr string `mapstructure:"addr"`
	} `mapstructure:"grpc"`

	HTTP struct {
		Addr string `mapstructure:"addr"`
	} `mapstructure:"http"`

	TLS TLSConfig `mapstructure:"tls"`

	Auth struct {
		JWTSecret   string `mapstructure:"jwt_secret"`
		RequireAuth bool   `mapstructure:"require_auth"`
	} `mapstructure:"auth"`

	RateLimit struct {
		SpansPerSecond int `mapstructure:"spans_per_second"`
		BurstSize      int `mapstructure:"burst_size"`
	} `mapstructure:"rate_limit"`

	Gateway struct {
		Endpoint  string `mapstructure:"endpoint"`
		AuthToken string `mapstructure:"auth_token"`
		Timeout   int    `mapstructure:"timeout_seconds"`
	} `mapstructure:"gateway"`

	Processor struct {
		BatchSize       int `mapstructure:"batch_size"`
		BatchTimeoutMS  int `mapstructure:"batch_timeout_ms"`
		MaxQueueSize    int `mapstructure:"max_queue_size"`
		Workers         int `mapstructure:"workers"`
		MaxAttributeLen int `mapstructure:"max_attribute_len"`
	} `mapstructure:"processor"`

	Discovery struct {
		Enabled         bool   `mapstructure:"enabled"`
		KubernetesMode  bool   `mapstructure:"kubernetes_mode"`
		ProcessScanSecs int    `mapstructure:"process_scan_seconds"`
		Namespace       string `mapstructure:"namespace"`
	} `mapstructure:"discovery"`

	PII struct {
		Enabled  bool     `mapstructure:"enabled"`
		Patterns []string `mapstructure:"patterns"`
		Redact   bool     `mapstructure:"redact"`
	} `mapstructure:"pii"`
}

type TLSConfig struct {
	Enabled   bool   `mapstructure:"enabled"`
	CertFile  string `mapstructure:"cert_file"`
	KeyFile   string `mapstructure:"key_file"`
	CAFile    string `mapstructure:"ca_file"`
	ClientCA  string `mapstructure:"client_ca"`  // CA to verify client certificates (mTLS)
	MutualTLS bool   `mapstructure:"mutual_tls"` // Require client certificate verification
}

func Load() (*Config, error) {
	v := viper.New()

	// Defaults
	v.SetDefault("debug", false)
	v.SetDefault("node_name", hostname())
	v.SetDefault("grpc.addr", ":4317")
	v.SetDefault("http.addr", ":4318")
	v.SetDefault("tls.enabled", false)
	v.SetDefault("auth.require_auth", true)
	v.SetDefault("rate_limit.spans_per_second", 50000)
	v.SetDefault("rate_limit.burst_size", 10000)
	v.SetDefault("gateway.endpoint", "http://localhost:8080")
	v.SetDefault("gateway.timeout_seconds", 10)
	v.SetDefault("processor.batch_size", 512)
	v.SetDefault("processor.batch_timeout_ms", 200)
	v.SetDefault("processor.max_queue_size", 100000)
	v.SetDefault("processor.workers", 4)
	v.SetDefault("processor.max_attribute_len", 4096)
	v.SetDefault("discovery.enabled", true)
	v.SetDefault("discovery.process_scan_seconds", 30)
	v.SetDefault("pii.enabled", true)
	v.SetDefault("pii.redact", true)

	// Config file
	v.SetConfigName("collector")
	v.SetConfigType("yaml")
	v.AddConfigPath("/etc/govagn")
	v.AddConfigPath(".")

	// Env overrides: GV_GRPC_ADDR, GV_GATEWAY_ENDPOINT etc
	v.SetEnvPrefix("GV")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("reading config: %w", err)
		}
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshaling config: %w", err)
	}

	if cfg.Auth.JWTSecret == "" {
		cfg.Auth.JWTSecret = os.Getenv("GV_JWT_SECRET")
	}
	if cfg.Gateway.AuthToken == "" {
		cfg.Gateway.AuthToken = os.Getenv("GV_GATEWAY_AUTH_TOKEN")
	}

	if err := validateConfig(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func hostname() string {
	h, _ := os.Hostname()
	return h
}

func validateConfig(cfg *Config) error {
	if cfg == nil {
		return fmt.Errorf("config is nil")
	}
	if !strictConfigEnabled() {
		return nil
	}
	if strings.TrimSpace(cfg.Gateway.Endpoint) == "" {
		return fmt.Errorf("gateway.endpoint is required in production")
	}
	if strings.EqualFold(strings.TrimSpace(cfg.Gateway.Endpoint), "http://localhost:8080") {
		return fmt.Errorf("gateway.endpoint must be set explicitly in production; refusing the built-in localhost default")
	}
	if !cfg.Auth.RequireAuth {
		return fmt.Errorf("auth.require_auth=false is not supported in production; OTLP auth must remain enabled")
	}
	if strings.TrimSpace(cfg.Gateway.AuthToken) == "" {
		return fmt.Errorf("GV_GATEWAY_AUTH_TOKEN is required in production so the collector can authenticate to the gateway /internal/ingest endpoint")
	}
	if cfg.Auth.RequireAuth && strings.TrimSpace(cfg.Auth.JWTSecret) == "" {
		return fmt.Errorf("GV_JWT_SECRET is required when auth.require_auth=true in production")
	}
	if cfg.TLS.Enabled {
		if strings.TrimSpace(cfg.TLS.CertFile) == "" || strings.TrimSpace(cfg.TLS.KeyFile) == "" {
			return fmt.Errorf("tls.cert_file and tls.key_file are required when tls.enabled=true in production")
		}
	}
	switch strings.TrimSpace(cfg.Auth.JWTSecret) {
	case "dev-secret-change-in-production", "dev-secret-change-in-prod":
		return fmt.Errorf("GV_JWT_SECRET must not use the development sentinel in production")
	}
	return nil
}

func strictConfigEnabled() bool {
	if os.Getenv("GV_STRICT_CONFIG") == "true" {
		return true
	}
	return strings.EqualFold(os.Getenv("GV_ENV"), "production")
}
