package receiver

import (
	"testing"
	"time"

	"github.com/Kotaro7750/keda-launcher-scaler/internal/arbitrator"
	"github.com/Kotaro7750/keda-launcher-scaler/internal/types"
)

func TestNormalizeRequest(t *testing.T) {
	now := mustParseTime(t, "2026-04-22T10:00:00Z")
	startAt := mustParseTime(t, "2026-04-22T10:05:00Z")
	endAt := mustParseTime(t, "2026-04-22T10:10:00Z")
	duration := 5 * time.Minute

	tests := []struct {
		name    string
		input   RequestInput
		want    arbitrator.RequestWindow
		wantErr string
	}{
		{
			name: "duration uses explicit startAt",
			input: RequestInput{
				RequestID: "req-duration",
				ScaledObject: types.ScaledObjectKey{
					Namespace: "default",
					Name:      "worker",
				},
				StartAt:  &startAt,
				Duration: &duration,
			},
			want: arbitrator.RequestWindow{
				RequestID: arbitrator.RequestId("req-duration"),
				ScaledObject: types.ScaledObjectKey{
					Namespace: "default",
					Name:      "worker",
				},
				StartAt: startAt.UTC(),
				EndAt:   startAt.UTC().Add(duration),
			},
		},
		{
			name: "endAt uses current time when startAt is absent",
			input: RequestInput{
				RequestID: "req-end-at",
				ScaledObject: types.ScaledObjectKey{
					Namespace: "default",
					Name:      "worker",
				},
				EndAt: &endAt,
			},
			want: arbitrator.RequestWindow{
				RequestID: arbitrator.RequestId("req-end-at"),
				ScaledObject: types.ScaledObjectKey{
					Namespace: "default",
					Name:      "worker",
				},
				StartAt: now,
				EndAt:   endAt.UTC(),
			},
		},
		{
			name: "requestId is required",
			input: RequestInput{
				ScaledObject: types.ScaledObjectKey{
					Namespace: "default",
					Name:      "worker",
				},
				Duration: &duration,
			},
			wantErr: "requestId is required",
		},
		{
			name: "scaledObject namespace is required",
			input: RequestInput{
				RequestID: "req",
				ScaledObject: types.ScaledObjectKey{
					Name: "worker",
				},
				Duration: &duration,
			},
			wantErr: "scaledObject namespace and name are required",
		},
		{
			name: "duration and endAt are mutually exclusive",
			input: RequestInput{
				RequestID: "req",
				ScaledObject: types.ScaledObjectKey{
					Namespace: "default",
					Name:      "worker",
				},
				EndAt:    &endAt,
				Duration: &duration,
			},
			wantErr: "endAt and duration are mutually exclusive",
		},
		{
			name: "duration or endAt is required",
			input: RequestInput{
				RequestID: "req",
				ScaledObject: types.ScaledObjectKey{
					Namespace: "default",
					Name:      "worker",
				},
			},
			wantErr: "either endAt or duration must be provided",
		},
		{
			name: "zero duration is rejected",
			input: RequestInput{
				RequestID: "req",
				ScaledObject: types.ScaledObjectKey{
					Namespace: "default",
					Name:      "worker",
				},
				Duration: durationPtr(0),
			},
			wantErr: "duration must be positive",
		},
		{
			name: "negative duration is rejected",
			input: RequestInput{
				RequestID: "req",
				ScaledObject: types.ScaledObjectKey{
					Namespace: "default",
					Name:      "worker",
				},
				Duration: durationPtr(-time.Second),
			},
			wantErr: "duration must be positive",
		},
		{
			name: "endAt must not be before startAt",
			input: RequestInput{
				RequestID: "req",
				ScaledObject: types.ScaledObjectKey{
					Namespace: "default",
					Name:      "worker",
				},
				StartAt: &endAt,
				EndAt:   &startAt,
			},
			wantErr: "endAt must be after or equal to startAt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeRequest(tt.input, now)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("NormalizeRequest() error = nil, want %q", tt.wantErr)
				}
				if err.Error() != tt.wantErr {
					t.Fatalf("NormalizeRequest() error = %q, want %q", err.Error(), tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizeRequest() error = %v", err)
			}
			if got != tt.want {
				t.Fatalf("NormalizeRequest() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func mustParseTime(t *testing.T, value string) time.Time {
	t.Helper()

	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("time.Parse(%q) error = %v", value, err)
	}
	return parsed
}

func durationPtr(value time.Duration) *time.Duration {
	return &value
}
