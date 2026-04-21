package httpreceiver

import (
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Kotaro7750/keda-launcher-scaler/internal/arbitrator"
	"github.com/Kotaro7750/keda-launcher-scaler/internal/types"
	"github.com/labstack/echo/v4"
	echomiddleware "github.com/oapi-codegen/echo-middleware"
)

func TestHTTPReceiverPostRequests_AcceptsDurationRequest(t *testing.T) {
	out := make(chan arbitrator.RequestWindow, 1)
	e := newTestHTTPReceiver(t, out)

	rec := servePostRequests(t, e, `{
		"requestId": "req-duration",
		"scaledObject": {
			"namespace": "default",
			"name": "worker"
		},
		"startAt": "2026-04-22T10:00:00Z",
		"duration": "5m"
	}`)

	if rec.Code != stdhttp.StatusAccepted {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, stdhttp.StatusAccepted, rec.Body.String())
	}

	got := <-out
	wantStart := mustParseTime(t, "2026-04-22T10:00:00Z")
	wantEnd := mustParseTime(t, "2026-04-22T10:05:00Z")
	want := arbitrator.RequestWindow{
		RequestID: arbitrator.RequestId("req-duration"),
		ScaledObject: types.ScaledObjectKey{
			Namespace: "default",
			Name:      "worker",
		},
		StartAt: wantStart,
		EndAt:   wantEnd,
	}
	if got != want {
		t.Fatalf("request window = %+v, want %+v", got, want)
	}

	var response AcceptedRequest
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.RequestId != "req-duration" || response.EffectiveStart != wantStart || response.EffectiveEnd != wantEnd {
		t.Fatalf("response = %+v", response)
	}
}

func TestHTTPReceiverPostRequests_AcceptsEndAtRequest(t *testing.T) {
	out := make(chan arbitrator.RequestWindow, 1)
	e := newTestHTTPReceiver(t, out)

	rec := servePostRequests(t, e, `{
		"requestId": "req-end-at",
		"scaledObject": {
			"namespace": "default",
			"name": "worker"
		},
		"startAt": "2026-04-22T10:00:00Z",
		"endAt": "2026-04-22T10:05:00Z"
	}`)

	if rec.Code != stdhttp.StatusAccepted {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, stdhttp.StatusAccepted, rec.Body.String())
	}

	got := <-out
	if got.RequestID != arbitrator.RequestId("req-end-at") {
		t.Fatalf("requestId = %q", got.RequestID)
	}
	if got.StartAt != mustParseTime(t, "2026-04-22T10:00:00Z") {
		t.Fatalf("startAt = %s", got.StartAt)
	}
	if got.EndAt != mustParseTime(t, "2026-04-22T10:05:00Z") {
		t.Fatalf("endAt = %s", got.EndAt)
	}
}

func TestHTTPReceiverPostRequests_RejectsDomainValidationErrors(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "invalid Go duration",
			body: `{
				"requestId": "req",
				"scaledObject": {
					"namespace": "default",
					"name": "worker"
				},
				"duration": "five minutes"
			}`,
		},
		{
			name: "zero duration",
			body: `{
				"requestId": "req",
				"scaledObject": {
					"namespace": "default",
					"name": "worker"
				},
				"duration": "0s"
			}`,
		},
		{
			name: "endAt before startAt",
			body: `{
				"requestId": "req",
				"scaledObject": {
					"namespace": "default",
					"name": "worker"
				},
				"startAt": "2026-04-22T10:10:00Z",
				"endAt": "2026-04-22T10:05:00Z"
			}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := make(chan arbitrator.RequestWindow, 1)
			e := newTestHTTPReceiver(t, out)

			rec := servePostRequests(t, e, tt.body)

			if rec.Code != stdhttp.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, stdhttp.StatusBadRequest, rec.Body.String())
			}
			select {
			case got := <-out:
				t.Fatalf("unexpected request sent: %+v", got)
			default:
			}
		})
	}
}

func newTestHTTPReceiver(t *testing.T, out chan<- arbitrator.RequestWindow) *echo.Echo {
	t.Helper()

	e := echo.New()
	swagger, err := GetSwagger()
	if err != nil {
		t.Fatalf("GetSwagger() error = %v", err)
	}
	swagger.Servers = nil
	e.Use(echomiddleware.OapiRequestValidator(swagger))
	RegisterHandlers(e, NewStrictHandler(&httpReceiverServer{out: out}, nil))
	return e
}

func servePostRequests(t *testing.T, e *echo.Echo, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(stdhttp.MethodPost, "/requests", strings.NewReader(body))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func mustParseTime(t *testing.T, value string) time.Time {
	t.Helper()

	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("time.Parse(%q) error = %v", value, err)
	}
	return parsed
}
