package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
)

const (
	defaultServiceName     = "keda-launcher-scaler"
	defaultHTTPListenAddr  = ":8080"
	defaultGRPCListenAddr  = ":9090"
	defaultShutdownTimeout = 10 * time.Second
	defaultLogLevel        = "info"
	defaultRequestBuffer   = 128
	defaultOTLPTLSInsecure = true
)

// Config holds runtime settings loaded from environment variables.
type Config struct {
	ServiceName       string        `mapstructure:"SERVICE_NAME"`
	HTTPListenAddress string        `mapstructure:"HTTP_LISTEN_ADDRESS"`
	GRPCListenAddress string        `mapstructure:"GRPC_LISTEN_ADDRESS"`
	ShutdownTimeout   time.Duration `mapstructure:"SHUTDOWN_TIMEOUT"`
	LogLevel          string        `mapstructure:"LOG_LEVEL"`
	RequestBufferSize int           `mapstructure:"REQUEST_BUFFER_SIZE"`

	OTLPEndpoint string `mapstructure:"OTEL_EXPORTER_OTLP_ENDPOINT"`
	OTLPInsecure bool   `mapstructure:"OTEL_EXPORTER_OTLP_INSECURE"`
}

func Load() (Config, error) {
	v := viper.New()
	for key, value := range map[string]any{
		"SERVICE_NAME":                defaultServiceName,
		"HTTP_LISTEN_ADDRESS":         defaultHTTPListenAddr,
		"GRPC_LISTEN_ADDRESS":         defaultGRPCListenAddr,
		"SHUTDOWN_TIMEOUT":            defaultShutdownTimeout.String(),
		"LOG_LEVEL":                   defaultLogLevel,
		"REQUEST_BUFFER_SIZE":         defaultRequestBuffer,
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
	cfg.LogLevel = strings.ToLower(cfg.LogLevel)

	if cfg.ShutdownTimeout <= 0 {
		return Config{}, fmt.Errorf("SHUTDOWN_TIMEOUT: must be a positive duration")
	}
	if cfg.RequestBufferSize <= 0 {
		return Config{}, fmt.Errorf("REQUEST_BUFFER_SIZE: must be a positive integer")
	}
	if cfg.LogLevel == "" {
		return Config{}, fmt.Errorf("LOG_LEVEL: must not be empty")
	}

	if v.IsSet("SHUTDOWN_TIMEOUT") && v.GetString("SHUTDOWN_TIMEOUT") != "" && cfg.ShutdownTimeout == 0 {
		return Config{}, fmt.Errorf("SHUTDOWN_TIMEOUT: invalid duration")
	}
	if v.IsSet("REQUEST_BUFFER_SIZE") && v.GetString("REQUEST_BUFFER_SIZE") != "" && cfg.RequestBufferSize == 0 {
		return Config{}, fmt.Errorf("REQUEST_BUFFER_SIZE: invalid integer")
	}
	if v.IsSet("OTEL_EXPORTER_OTLP_INSECURE") && v.GetString("OTEL_EXPORTER_OTLP_INSECURE") != "" {
		if _, err := strconv.ParseBool(v.GetString("OTEL_EXPORTER_OTLP_INSECURE")); err != nil {
			return Config{}, fmt.Errorf("OTEL_EXPORTER_OTLP_INSECURE: invalid boolean")
		}
	}

	return cfg, nil
}
