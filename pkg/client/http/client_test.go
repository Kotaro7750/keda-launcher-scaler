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
	tests := []struct {
		name       string
		response   *PostRequestsResponse
		wantStatus int
		wantMsg    string
	}{
		{
			name: "bad request",
			response: &PostRequestsResponse{
				Body:         []byte(`{"message":"bad request"}`),
				HTTPResponse: (&fakeHTTPResponse{status: "400 Bad Request", statusCode: 400}).httpResponse(),
				JSON400:      &ErrorResponse{Message: "bad request"},
			},
			wantStatus: 400,
			wantMsg:    "bad request",
		},
		{
			name: "unknown scaled object",
			response: &PostRequestsResponse{
				Body:         []byte(`{"message":"target ScaledObject is not known to this receiver"}`),
				HTTPResponse: (&fakeHTTPResponse{status: "404 Not Found", statusCode: 404}).httpResponse(),
				JSON404:      &ErrorResponse{Message: "target ScaledObject is not known to this receiver"},
			},
			wantStatus: 404,
			wantMsg:    "target ScaledObject is not known to this receiver",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeClientWithResponses{response: tt.response}
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
			if apiErr.Operation != "launch request" || apiErr.StatusCode != tt.wantStatus || apiErr.Message != tt.wantMsg {
				t.Fatalf("api error = %+v", apiErr)
			}
		})
	}
}

func TestClientListScaledObjects(t *testing.T) {
	fake := &fakeClientWithResponses{
		listResponse: &ListScaledObjectsResponse{
			HTTPResponse: (&fakeHTTPResponse{status: "200 OK", statusCode: 200}).httpResponse(),
			JSON200: &ScaledObjectList{
				ScaledObjects: []ScaledObject{
					{Namespace: "default", Name: "worker"},
					{Namespace: "ops", Name: "metrics"},
				},
			},
		},
	}
	client := &HTTPClient{client: fake}

	got, err := client.ListScaledObjects(context.Background())
	if err != nil {
		t.Fatalf("ListScaledObjects() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("ListScaledObjects() len = %d, want 2", len(got))
	}
	if got[0].Namespace != "default" || got[0].Name != "worker" {
		t.Fatalf("first scaled object = %+v", got[0])
	}
	if got[1].Namespace != "ops" || got[1].Name != "metrics" {
		t.Fatalf("second scaled object = %+v", got[1])
	}
}

func TestClientDeleteRequest(t *testing.T) {
	deletedAt := time.Date(2026, 4, 25, 11, 0, 0, 0, time.UTC)
	fake := &fakeClientWithResponses{
		deleteResponse: &DeleteScaledObjectRequestResponse{
			HTTPResponse: (&fakeHTTPResponse{status: "200 OK", statusCode: 200}).httpResponse(),
			JSON200: &DeletedRequest{
				RequestId:      "req-1",
				EffectiveStart: deletedAt.Add(-time.Minute),
				EffectiveEnd:   deletedAt,
				ScaledObject:   ScaledObject{Namespace: "default", Name: "worker"},
			},
		},
	}
	client := &HTTPClient{client: fake}

	got, err := client.DeleteRequest(context.Background(), domainclient.DeleteRequest{
		RequestID: " req-1 ",
		ScaledObject: domainclient.ScaledObject{
			Namespace: " default ",
			Name:      " worker ",
		},
	})
	if err != nil {
		t.Fatalf("DeleteRequest() error = %v", err)
	}
	if fake.deleteNamespace != "default" || fake.deleteName != "worker" || fake.deleteRequestID != "req-1" {
		t.Fatalf("delete request params = %q %q %q", fake.deleteNamespace, fake.deleteName, fake.deleteRequestID)
	}
	if got.RequestID != "req-1" {
		t.Fatalf("deleted requestID = %q", got.RequestID)
	}
	if got.ScaledObject.Namespace != "default" || got.ScaledObject.Name != "worker" {
		t.Fatalf("deleted scaledObject = %+v", got.ScaledObject)
	}
	if !got.EffectiveStart.Equal(deletedAt.Add(-time.Minute)) || !got.EffectiveEnd.Equal(deletedAt) {
		t.Fatalf("deleted window = %s to %s", got.EffectiveStart, got.EffectiveEnd)
	}
}

func TestClientDeleteRequest_MapsAPIError(t *testing.T) {
	fake := &fakeClientWithResponses{
		deleteResponse: &DeleteScaledObjectRequestResponse{
			Body:         []byte(`{"message":"not found"}`),
			HTTPResponse: (&fakeHTTPResponse{status: "404 Not Found", statusCode: 404}).httpResponse(),
			JSON404:      &ErrorResponse{Message: "not found"},
		},
	}
	client := &HTTPClient{client: fake}

	_, err := client.DeleteRequest(context.Background(), domainclient.DeleteRequest{
		RequestID: "req-1",
		ScaledObject: domainclient.ScaledObject{
			Namespace: "default",
			Name:      "worker",
		},
	})
	if err == nil {
		t.Fatal("DeleteRequest() succeeded, want error")
	}

	var apiErr *Error
	if !errors.As(err, &apiErr) {
		t.Fatalf("error type = %T, want *Error", err)
	}
	if apiErr.Operation != "delete request" || apiErr.StatusCode != 404 || apiErr.Message != "not found" {
		t.Fatalf("api error = %+v", apiErr)
	}
}

type fakeClientWithResponses struct {
	request         PostRequestsJSONRequestBody
	response        *PostRequestsResponse
	listResponse    *ListScaledObjectsResponse
	deleteResponse  *DeleteScaledObjectRequestResponse
	deleteNamespace string
	deleteName      string
	deleteRequestID string
	err             error
}

func (f *fakeClientWithResponses) PostRequestsWithBodyWithResponse(context.Context, string, io.Reader, ...RequestEditorFn) (*PostRequestsResponse, error) {
	panic("unexpected call")
}

func (f *fakeClientWithResponses) PostRequestsWithResponse(_ context.Context, body PostRequestsJSONRequestBody, _ ...RequestEditorFn) (*PostRequestsResponse, error) {
	f.request = body
	return f.response, f.err
}

func (f *fakeClientWithResponses) ListScaledObjectsWithResponse(context.Context, ...RequestEditorFn) (*ListScaledObjectsResponse, error) {
	return f.listResponse, f.err
}

func (f *fakeClientWithResponses) DeleteScaledObjectRequestWithResponse(_ context.Context, namespace string, name string, requestID string, _ ...RequestEditorFn) (*DeleteScaledObjectRequestResponse, error) {
	f.deleteNamespace = namespace
	f.deleteName = name
	f.deleteRequestID = requestID
	return f.deleteResponse, f.err
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
