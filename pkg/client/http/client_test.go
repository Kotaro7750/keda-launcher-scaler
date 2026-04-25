package httpclient

import (
	"context"
	"errors"
	"io"
	"net/http"
	"testing"
	"time"

	domainclient "github.com/Kotaro7750/keda-launcher-scaler/pkg/client"
)

func TestClientLaunch(t *testing.T) {
	startAt := time.Date(2026, 4, 25, 10, 0, 0, 0, time.UTC)
	fake := &fakeClientWithResponses{
		response: &PostRequestsResponse{
			HTTPResponse: (&fakeHTTPResponse{status: "202 Accepted", statusCode: 202}).httpResponse(),
			JSON202: &AcceptedRequest{
				RequestId:      "req-1",
				EffectiveStart: startAt,
				EffectiveEnd:   startAt.Add(time.Minute),
				ScaledObject: ScaledObject{
					Namespace: "default",
					Name:      "worker",
				},
			},
		},
	}
	client := &HTTPClient{client: fake}

	got, err := client.Launch(context.Background(), domainclient.LaunchRequest{
		RequestID: "  req-1  ",
		ScaledObject: domainclient.ScaledObject{
			Namespace: " default ",
			Name:      " worker ",
		},
		StartAt:  &startAt,
		Duration: time.Minute,
	})
	if err != nil {
		t.Fatalf("Launch() error = %v", err)
	}

	if fake.request.RequestId != "req-1" {
		t.Fatalf("requestID = %q", fake.request.RequestId)
	}
	if fake.request.ScaledObject.Namespace != "default" || fake.request.ScaledObject.Name != "worker" {
		t.Fatalf("scaledObject = %+v", fake.request.ScaledObject)
	}
	if fake.request.Duration == nil || *fake.request.Duration != "1m0s" {
		t.Fatalf("duration = %v", fake.request.Duration)
	}
	if fake.request.StartAt == nil || !fake.request.StartAt.Equal(startAt) {
		t.Fatalf("startAt = %v", fake.request.StartAt)
	}

	if got.RequestID != "req-1" {
		t.Fatalf("accepted requestID = %q", got.RequestID)
	}
	if got.ScaledObject.Namespace != "default" || got.ScaledObject.Name != "worker" {
		t.Fatalf("accepted scaledObject = %+v", got.ScaledObject)
	}
	if !got.EffectiveStart.Equal(startAt) || !got.EffectiveEnd.Equal(startAt.Add(time.Minute)) {
		t.Fatalf("accepted window = %s to %s", got.EffectiveStart, got.EffectiveEnd)
	}
}

func TestClientLaunch_MapsAPIError(t *testing.T) {
	fake := &fakeClientWithResponses{
		response: &PostRequestsResponse{
			Body:         []byte(`{"message":"bad request"}`),
			HTTPResponse: (&fakeHTTPResponse{status: "400 Bad Request", statusCode: 400}).httpResponse(),
			JSON400:      &ErrorResponse{Message: "bad request"},
		},
	}
	client := &HTTPClient{client: fake}

	_, err := client.Launch(context.Background(), domainclient.LaunchRequest{
		RequestID: "req-1",
		ScaledObject: domainclient.ScaledObject{
			Namespace: "default",
			Name:      "worker",
		},
		Duration: time.Minute,
	})
	if err == nil {
		t.Fatal("Launch() succeeded, want error")
	}

	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T, want *Error", err)
	}
	if apiErr.StatusCode != 400 || apiErr.Message != "bad request" {
		t.Fatalf("api error = %+v", apiErr)
	}
}

type fakeClientWithResponses struct {
	request  PostRequestsJSONRequestBody
	response *PostRequestsResponse
	err      error
}

func (f *fakeClientWithResponses) PostRequestsWithBodyWithResponse(context.Context, string, io.Reader, ...RequestEditorFn) (*PostRequestsResponse, error) {
	panic("unexpected call")
}

func (f *fakeClientWithResponses) PostRequestsWithResponse(_ context.Context, body PostRequestsJSONRequestBody, _ ...RequestEditorFn) (*PostRequestsResponse, error) {
	f.request = body
	return f.response, f.err
}

type fakeHTTPResponse struct {
	status     string
	statusCode int
}

func (r *fakeHTTPResponse) httpResponse() *http.Response {
	return &http.Response{
		Status:     r.status,
		StatusCode: r.statusCode,
	}
}
