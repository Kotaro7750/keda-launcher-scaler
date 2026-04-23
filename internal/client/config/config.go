package config

import (
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
)

const (
	defaultServiceName     = "keda-launcher-client"
	defaultReceiverURL     = "http://localhost:8080"
	defaultShutdownTimeout = 10 * time.Second
	defaultLogLevel        = "info"
	defaultOTLPTLSInsecure = true
)

// Config holds runtime settings loaded from environment variables.
type Config struct {
	ServiceName string `mapstructure:"SERVICE_NAME"`
	ReceiverURL string `mapstructure:"RECEIVER_URL"`
	RequestID   string `mapstructure:"REQUEST_ID"`

	ScaledObjectNamespace string        `mapstructure:"SCALED_OBJECT_NAMESPACE"`
	ScaledObjectName      string        `mapstructure:"SCALED_OBJECT_NAME"`
	RequestInterval       time.Duration `mapstructure:"REQUEST_INTERVAL"`
	RequestDuration       time.Duration `mapstructure:"REQUEST_DURATION"`

	ShutdownTimeout time.Duration `mapstructure:"SHUTDOWN_TIMEOUT"`
	LogLevel        string        `mapstructure:"LOG_LEVEL"`

	OTLPEndpoint string `mapstructure:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	OTLPInsecure bool   `mapstructure:"OTEL_EXPORTER_OTLP_INSECURE"`
}

func Load() (Config, error) {
	v := viper.New()
	for key, value := range map[string]any{
		"SERVICE_NAME":                defaultServiceName,
		"RECEIVER_URL":                defaultReceiverURL,
		"REQUEST_ID":                  "",
		"SCALED_OBJECT_NAMESPACE":     "",
		"SCALED_OBJECT_NAME":          "",
		"REQUEST_INTERVAL":            "",
		"REQUEST_DURATION":            "",
		"SHUTDOWN_TIMEOUT":            defaultShutdownTimeout.String(),
		"LOG_LEVEL":                   defaultLogLevel,
		"OTEL_EXPORTER_OTLP_ENDPOINT": "",
		"OTEL_EXPORTER_OTLP_INSECURE": defaultOTLPTLSInsecure,
	} {
		v.SetDefault(key, value)
	}
	v.AutomaticEnv()

	var cfg Config
	if err := v.Unmarshal(&cfg, viper.DecodeHook(
		mapstructure.ComposeDecodeHookFunc(
			mapstructure.StringToTimeDurationHookFunc(),
		),
	)); err != nil {
		return Config{}, fmt.Errorf("decode config: %w", err)
	}

	cfg.ServiceName = strings.TrimSpace(cfg.ServiceName)
	cfg.ReceiverURL = strings.TrimSpace(cfg.ReceiverURL)
	cfg.RequestID = strings.TrimSpace(cfg.RequestID)
	cfg.ScaledObjectNamespace = strings.TrimSpace(cfg.ScaledObjectNamespace)
	cfg.ScaledObjectName = strings.TrimSpace(cfg.ScaledObjectName)
	cfg.LogLevel = strings.ToLower(strings.TrimSpace(cfg.LogLevel))
	cfg.OTLPEndpoint = strings.TrimSpace(cfg.OTLPEndpoint)

	receiverURL, err := normalizeReceiverURL(cfg.ReceiverURL)
	if err != nil {
		return Config{}, fmt.Errorf("RECEIVER_URL: %w", err)
	}
	cfg.ReceiverURL = receiverURL

	if cfg.ScaledObjectNamespace == "" {
		return Config{}, fmt.Errorf("SCALED_OBJECT_NAMESPACE: must not be empty")
	}
	if cfg.ScaledObjectName == "" {
		return Config{}, fmt.Errorf("SCALED_OBJECT_NAME: must not be empty")
	}
	if cfg.RequestInterval <= 0 {
		return Config{}, fmt.Errorf("REQUEST_INTERVAL: must be a positive duration")
	}
	if cfg.RequestDuration <= 0 {
		return Config{}, fmt.Errorf("REQUEST_DURATION: must be a positive duration")
	}
	if cfg.RequestDuration <= cfg.RequestInterval {
		return Config{}, fmt.Errorf("REQUEST_DURATION: must be greater than REQUEST_INTERVAL")
	}
	if cfg.ShutdownTimeout <= 0 {
		return Config{}, fmt.Errorf("SHUTDOWN_TIMEOUT: must be a positive duration")
	}
	if cfg.LogLevel == "" {
		return Config{}, fmt.Errorf("LOG_LEVEL: must not be empty")
	}

	return cfg, nil
}

func normalizeReceiverURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("invalid URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("scheme must be http or https")
	}
	if parsed.Host == "" {
		return "", fmt.Errorf("host must not be empty")
	}

	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return parsed.String(), nil
}
