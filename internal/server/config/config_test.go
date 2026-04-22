package config

import (
	"strings"
	"testing"
)

func TestLoad_RejectsInvalidRuntimeSettings(t *testing.T) {
	tests := []struct {
		name    string
		envKey  string
		envVal  string
		wantErr string
	}{
		{
			name:    "zero shutdown timeout",
			envKey:  "SHUTDOWN_TIMEOUT",
			envVal:  "0s",
			wantErr: "SHUTDOWN_TIMEOUT: must be a positive duration",
		},
		{
			name:    "negative shutdown timeout",
			envKey:  "SHUTDOWN_TIMEOUT",
			envVal:  "-1s",
			wantErr: "SHUTDOWN_TIMEOUT: must be a positive duration",
		},
		{
			name:    "zero request buffer size",
			envKey:  "REQUEST_BUFFER_SIZE",
			envVal:  "0",
			wantErr: "REQUEST_BUFFER_SIZE: must be a positive integer",
		},
		{
			name:    "negative request buffer size",
			envKey:  "REQUEST_BUFFER_SIZE",
			envVal:  "-1",
			wantErr: "REQUEST_BUFFER_SIZE: must be a positive integer",
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

	t.Setenv("SHUTDOWN_TIMEOUT", "10s")
	t.Setenv("REQUEST_BUFFER_SIZE", "128")
	t.Setenv("LOG_LEVEL", "info")
	t.Setenv("OTEL_EXPORTER_OTLP_INSECURE", "true")
}
