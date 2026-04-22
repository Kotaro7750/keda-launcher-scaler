package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	setValidRuntimeSettings(t)

	got, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got.ServiceName != defaultServiceName {
		t.Fatalf("ServiceName = %q, want %q", got.ServiceName, defaultServiceName)
	}
	if got.ReceiverURL != "http://localhost:8080" {
		t.Fatalf("ReceiverURL = %q", got.ReceiverURL)
	}
	if got.ScaledObjectNamespace != "default" || got.ScaledObjectName != "worker" {
		t.Fatalf("ScaledObject = %s/%s", got.ScaledObjectNamespace, got.ScaledObjectName)
	}
	if got.RequestInterval != 10*time.Second || got.RequestDuration != time.Minute {
		t.Fatalf("request window = interval %s duration %s", got.RequestInterval, got.RequestDuration)
	}
}

func TestLoad_RejectsClientRuntimePolicyViolations(t *testing.T) {
	tests := []struct {
		name    string
		envKey  string
		envVal  string
		wantErr string
	}{
		{
			name:    "missing scaled object namespace",
			envKey:  "SCALED_OBJECT_NAMESPACE",
			envVal:  "",
			wantErr: "SCALED_OBJECT_NAMESPACE: must not be empty",
		},
		{
			name:    "duration equal to interval",
			envKey:  "REQUEST_DURATION",
			envVal:  "10s",
			wantErr: "REQUEST_DURATION: must be greater than REQUEST_INTERVAL",
		},
		{
			name:    "unsupported receiver URL scheme",
			envKey:  "RECEIVER_URL",
			envVal:  "ftp://localhost:8080",
			wantErr: "RECEIVER_URL: scheme must be http or https",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			setValidRuntimeSettings(t)
			t.Setenv(tt.envKey, tt.envVal)

			_, err := Load()
			if err == nil {
				t.Fatal("Load succeeded, want error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Load error = %q, want to contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func setValidRuntimeSettings(t *testing.T) {
	t.Helper()

	t.Setenv("SCALED_OBJECT_NAMESPACE", "default")
	t.Setenv("SCALED_OBJECT_NAME", "worker")
	t.Setenv("REQUEST_INTERVAL", "10s")
	t.Setenv("REQUEST_DURATION", "1m")
	t.Setenv("SHUTDOWN_TIMEOUT", "10s")
	t.Setenv("LOG_LEVEL", "info")
	t.Setenv("OTEL_EXPORTER_OTLP_INSECURE", "true")
}
