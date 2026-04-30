package httpreceiver

import (
	"encoding/json"
	stdhttp "net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Kotaro7750/keda-launcher-scaler/internal/server/arbitrator"
	"github.com/Kotaro7750/keda-launcher-scaler/internal/server/types"
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

func TestHTTPReceiverPostRequests_RejectsUnknownScaledObject(t *testing.T) {
	manager := &fakeRequestManager{
		knownScaledObjects: map[types.ScaledObjectKey]struct{}{},
	}
	out := make(chan arbitrator.RequestWindow, 1)
	e := newTestHTTPReceiver(t, out, manager)

	rec := servePostRequests(t, e, `{
		"requestId": "req-unknown",
		"scaledObject": {
			"namespace": "default",
			"name": "missing"
		},
		"duration": "5m"
	}`)

	if rec.Code != stdhttp.StatusNotFound {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, stdhttp.StatusNotFound, rec.Body.String())
	}
	select {
	case got := <-out:
		t.Fatalf("unexpected request sent: %+v", got)
	default:
	}
	if manager.listCalled != 0 {
		t.Fatalf("ListScaledObjects() calls = %d, want 0", manager.listCalled)
	}
}

func TestHTTPReceiverPostRequests_RoutesKnownScaledObjectThroughChannel(t *testing.T) {
	key := types.ScaledObjectKey{Namespace: "default", Name: "worker"}
	manager := &fakeRequestManager{
		knownScaledObjects: map[types.ScaledObjectKey]struct{}{
			key: {},
		},
	}
	out := make(chan arbitrator.RequestWindow, 1)
	e := newTestHTTPReceiver(t, out, manager)

	rec := servePostRequests(t, e, `{
		"requestId": "req-known",
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
	if got.RequestID != arbitrator.RequestId("req-known") {
		t.Fatalf("requestId = %q", got.RequestID)
	}
	if got.ScaledObject != key {
		t.Fatalf("scaledObject = %+v, want %+v", got.ScaledObject, key)
	}
}

func TestHTTPReceiverListScaledObjects_ReturnsEmptySnapshot(t *testing.T) {
	manager := &fakeRequestManager{
		knownScaledObjects: map[types.ScaledObjectKey]struct{}{},
	}
	out := make(chan arbitrator.RequestWindow, 1)
	e := newTestHTTPReceiver(t, out, manager)

	rec := serveGetScaledObjects(t, e)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, stdhttp.StatusOK, rec.Body.String())
	}
	if manager.listCalled != 1 {
		t.Fatalf("ListScaledObjects() calls = %d, want %d", manager.listCalled, 1)
	}
	select {
	case got := <-out:
		t.Fatalf("unexpected request sent: %+v", got)
	default:
	}

	var response ScaledObjectList
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(response.ScaledObjects) != 0 {
		t.Fatalf("response = %+v, want empty list", response)
	}
}

func TestHTTPReceiverListScaledObjects_ReturnsKnownSnapshot(t *testing.T) {
	manager := &fakeRequestManager{
		knownScaledObjects: map[types.ScaledObjectKey]struct{}{
			{Namespace: "default", Name: "alpha"}:  {},
			{Namespace: "default", Name: "worker"}: {},
		},
		listScaledObjects: []types.ScaledObjectKey{
			{Namespace: "default", Name: "worker"},
			{Namespace: "default", Name: "alpha"},
		},
	}
	out := make(chan arbitrator.RequestWindow, 1)
	e := newTestHTTPReceiver(t, out, manager)

	rec := serveGetScaledObjects(t, e)

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, stdhttp.StatusOK, rec.Body.String())
	}
	if manager.listCalled != 1 {
		t.Fatalf("ListScaledObjects() calls = %d, want %d", manager.listCalled, 1)
	}
	select {
	case got := <-out:
		t.Fatalf("unexpected request sent: %+v", got)
	default:
	}

	var response ScaledObjectList
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	want := ScaledObjectList{
		ScaledObjects: []ScaledObject{
			{Namespace: "default", Name: "alpha"},
			{Namespace: "default", Name: "worker"},
		},
	}
	if !reflect.DeepEqual(response, want) {
		t.Fatalf("response = %+v, want %+v", response, want)
	}
}

func TestHTTPReceiverDeleteScaledObjectRequest_DeletesKnownWindow(t *testing.T) {
	key := types.ScaledObjectKey{Namespace: "default", Name: "worker"}
	deleted := arbitrator.RequestWindow{
		RequestID:    "req-delete",
		ScaledObject: key,
		StartAt:      mustParseTime(t, "2026-04-22T10:00:00Z"),
		EndAt:        mustParseTime(t, "2026-04-22T10:05:00Z"),
	}
	manager := &fakeRequestManager{
		knownScaledObjects: map[types.ScaledObjectKey]struct{}{
			key: {},
		},
		deleteWindow: deleted,
	}
	out := make(chan arbitrator.RequestWindow, 1)
	e := newTestHTTPReceiver(t, out, manager)

	rec := serveDeleteScaledObjectRequest(t, e, "/scaledobjects/default/worker/requests/req-delete")

	if rec.Code != stdhttp.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, stdhttp.StatusOK, rec.Body.String())
	}
	if manager.deleteCalled != 1 {
		t.Fatalf("DeleteRequest() calls = %d, want %d", manager.deleteCalled, 1)
	}
	if manager.deletedKey != key {
		t.Fatalf("deleted key = %+v, want %+v", manager.deletedKey, key)
	}
	if manager.deletedRequestID != arbitrator.RequestId("req-delete") {
		t.Fatalf("deleted requestId = %q, want %q", manager.deletedRequestID, "req-delete")
	}
	select {
	case got := <-out:
		t.Fatalf("unexpected request sent: %+v", got)
	default:
	}

	var response DeletedRequest
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if response.RequestId != "req-delete" || response.ScaledObject.Namespace != "default" || response.ScaledObject.Name != "worker" {
		t.Fatalf("response = %+v", response)
	}
	if response.EffectiveStart != deleted.StartAt || response.EffectiveEnd != deleted.EndAt {
		t.Fatalf("response = %+v, want window %+v", response, deleted)
	}
}

func TestHTTPReceiverDeleteScaledObjectRequest_RejectsNotFoundErrors(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{
			name: "unknown request",
			err:  arbitrator.ErrRequestWindowNotFound,
		},
		{
			name: "unknown scaled object",
			err:  arbitrator.ErrScaledObjectNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager := &fakeRequestManager{
				deleteErr: tt.err,
			}
			out := make(chan arbitrator.RequestWindow, 1)
			e := newTestHTTPReceiver(t, out, manager)

			rec := serveDeleteScaledObjectRequest(t, e, "/scaledobjects/default/worker/requests/req-delete")

			if rec.Code != stdhttp.StatusNotFound {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, stdhttp.StatusNotFound, rec.Body.String())
			}
			if manager.deleteCalled != 1 {
				t.Fatalf("DeleteRequest() calls = %d, want %d", manager.deleteCalled, 1)
			}
			select {
			case got := <-out:
				t.Fatalf("unexpected request sent: %+v", got)
			default:
			}

			var response ErrorResponse
			if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if response.Message == "" {
				t.Fatal("expected error response message")
			}
		})
	}
}

func newTestHTTPReceiver(t *testing.T, out chan<- arbitrator.RequestWindow, manager ...requestManager) *echo.Echo {
	t.Helper()

	e := echo.New()
	swagger, err := GetSwagger()
	if err != nil {
		t.Fatalf("GetSwagger() error = %v", err)
	}
	swagger.Servers = nil
	e.Use(echomiddleware.OapiRequestValidator(swagger))
	var requestManager requestManager
	if len(manager) > 0 {
		requestManager = manager[0]
	} else {
		requestManager = &fakeRequestManager{
			knownScaledObjects: map[types.ScaledObjectKey]struct{}{
				{Namespace: "default", Name: "worker"}: {},
			},
		}
	}
	RegisterHandlers(e, NewStrictHandler(&httpReceiverServer{out: out, requestManager: requestManager}, nil))
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

func serveGetScaledObjects(t *testing.T, e *echo.Echo) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(stdhttp.MethodGet, "/scaledobjects", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	return rec
}

func serveDeleteScaledObjectRequest(t *testing.T, e *echo.Echo, path string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(stdhttp.MethodDelete, path, nil)
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

type fakeRequestManager struct {
	knownScaledObjects map[types.ScaledObjectKey]struct{}
	listScaledObjects  []types.ScaledObjectKey
	listCalled         int
	deleteWindow       arbitrator.RequestWindow
	deleteErr          error
	deleteCalled       int
	deletedKey         types.ScaledObjectKey
	deletedRequestID   arbitrator.RequestId
}

func (f *fakeRequestManager) HasScaledObject(key types.ScaledObjectKey) bool {
	_, ok := f.knownScaledObjects[key]
	return ok
}

func (f *fakeRequestManager) ListScaledObjects() []types.ScaledObjectKey {
	f.listCalled++
	if f.listScaledObjects != nil {
		keys := make([]types.ScaledObjectKey, len(f.listScaledObjects))
		copy(keys, f.listScaledObjects)
		return keys
	}
	keys := make([]types.ScaledObjectKey, 0, len(f.knownScaledObjects))
	for key := range f.knownScaledObjects {
		keys = append(keys, key)
	}
	return keys
}

func (f *fakeRequestManager) DeleteRequest(key types.ScaledObjectKey, requestID arbitrator.RequestId) (arbitrator.RequestWindow, error) {
	f.deleteCalled++
	f.deletedKey = key
	f.deletedRequestID = requestID
	if f.deleteErr != nil {
		return arbitrator.RequestWindow{}, f.deleteErr
	}
	return f.deleteWindow, nil
}
