package httpclient

import (
	"context"
	"fmt"
	"strings"

	domainclient "github.com/Kotaro7750/keda-launcher-scaler/pkg/client"
)

// Error represents a non-accepted HTTP API response.
type Error struct {
	Operation  string
	StatusCode int
	Status     string
	Message    string
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	op := e.Operation
	if op == "" {
		op = "request"
	}
	if e.Message == "" {
		if e.Status != "" {
			return fmt.Sprintf("%s failed: %s", op, e.Status)
		}
		return fmt.Sprintf("%s failed: status %d", op, e.StatusCode)
	}
	if e.Status != "" {
		return fmt.Sprintf("%s failed: %s: %s", op, e.Status, e.Message)
	}
	return fmt.Sprintf("%s failed: status %d: %s", op, e.StatusCode, e.Message)
}

// HTTPClient wraps the generated HTTP client with domain-oriented request and response types.
type HTTPClient struct {
	client ClientWithResponsesInterface
}

var _ domainclient.Client = (*HTTPClient)(nil)

// New constructs an HTTP client for the receiver API.
func New(baseURL string, opts ...ClientOption) (*HTTPClient, error) {
	generated, err := NewClientWithResponses(baseURL, opts...)
	if err != nil {
		return nil, err
	}

	return &HTTPClient{client: generated}, nil
}

// Launch sends a launch request and returns the accepted request window on success.
func (c *HTTPClient) Launch(ctx context.Context, req domainclient.LaunchRequest) (domainclient.AcceptedRequest, error) {
	if c == nil || c.client == nil {
		return domainclient.AcceptedRequest{}, fmt.Errorf("client is nil")
	}

	response, err := c.client.PostRequestsWithResponse(ctx, FromDomainLaunchRequest(req))
	if err != nil {
		return domainclient.AcceptedRequest{}, fmt.Errorf("post request: %w", err)
	}
	if response == nil {
		return domainclient.AcceptedRequest{}, fmt.Errorf("post request: nil response")
	}

	if response.JSON202 != nil {
		return ToDomainAcceptedRequest(*response.JSON202), nil
	}

	if response.JSON400 != nil {
		return domainclient.AcceptedRequest{}, &Error{
			Operation:  "launch request",
			StatusCode: response.StatusCode(),
			Status:     response.Status(),
			Message:    strings.TrimSpace(response.JSON400.Message),
		}
	}
	if response.JSON404 != nil {
		return domainclient.AcceptedRequest{}, &Error{
			Operation:  "launch request",
			StatusCode: response.StatusCode(),
			Status:     response.Status(),
			Message:    strings.TrimSpace(response.JSON404.Message),
		}
	}
	if response.JSON408 != nil {
		return domainclient.AcceptedRequest{}, &Error{
			Operation:  "launch request",
			StatusCode: response.StatusCode(),
			Status:     response.Status(),
			Message:    strings.TrimSpace(response.JSON408.Message),
		}
	}

	return domainclient.AcceptedRequest{}, &Error{
		Operation:  "launch request",
		StatusCode: response.StatusCode(),
		Status:     response.Status(),
		Message:    strings.TrimSpace(string(response.Body)),
	}
}

// ListScaledObjects returns the current ScaledObject inventory.
func (c *HTTPClient) ListScaledObjects(ctx context.Context) ([]domainclient.ScaledObject, error) {
	if c == nil || c.client == nil {
		return nil, fmt.Errorf("client is nil")
	}

	response, err := c.client.ListScaledObjectsWithResponse(ctx)
	if err != nil {
		return nil, fmt.Errorf("list scaled objects: %w", err)
	}
	if response == nil {
		return nil, fmt.Errorf("list scaled objects: nil response")
	}
	if response.JSON200 != nil {
		return ToDomainScaledObjects(response.JSON200.ScaledObjects), nil
	}

	return nil, &Error{
		Operation:  "list scaled objects",
		StatusCode: response.StatusCode(),
		Status:     response.Status(),
		Message:    strings.TrimSpace(string(response.Body)),
	}
}

// DeleteRequest deletes a tracked request window.
func (c *HTTPClient) DeleteRequest(ctx context.Context, req domainclient.DeleteRequest) (domainclient.DeletedRequest, error) {
	if c == nil || c.client == nil {
		return domainclient.DeletedRequest{}, fmt.Errorf("client is nil")
	}

	response, err := c.client.DeleteScaledObjectRequestWithResponse(ctx, strings.TrimSpace(req.ScaledObject.Namespace), strings.TrimSpace(req.ScaledObject.Name), strings.TrimSpace(req.RequestID))
	if err != nil {
		return domainclient.DeletedRequest{}, fmt.Errorf("delete request: %w", err)
	}
	if response == nil {
		return domainclient.DeletedRequest{}, fmt.Errorf("delete request: nil response")
	}
	if response.JSON200 != nil {
		return ToDomainDeletedRequest(*response.JSON200), nil
	}

	if response.JSON404 != nil {
		return domainclient.DeletedRequest{}, &Error{
			Operation:  "delete request",
			StatusCode: response.StatusCode(),
			Status:     response.Status(),
			Message:    strings.TrimSpace(response.JSON404.Message),
		}
	}

	return domainclient.DeletedRequest{}, &Error{
		Operation:  "delete request",
		StatusCode: response.StatusCode(),
		Status:     response.Status(),
		Message:    strings.TrimSpace(string(response.Body)),
	}
}

// FromDomainLaunchRequest converts a domain request into the generated HTTP request type.
func FromDomainLaunchRequest(req domainclient.LaunchRequest) LaunchRequest {
	payload := LaunchRequest{
		RequestId: strings.TrimSpace(req.RequestID),
		ScaledObject: ScaledObject{
			Namespace: strings.TrimSpace(req.ScaledObject.Namespace),
			Name:      strings.TrimSpace(req.ScaledObject.Name),
		},
		StartAt: req.StartAt,
		EndAt:   req.EndAt,
	}
	if req.Duration != 0 {
		duration := req.Duration.String()
		payload.Duration = &duration
	}

	return payload
}

// ToDomainAcceptedRequest converts the generated HTTP accepted response into the domain type.
func ToDomainAcceptedRequest(resp AcceptedRequest) domainclient.AcceptedRequest {
	return domainclient.AcceptedRequest{
		RequestID: resp.RequestId,
		ScaledObject: domainclient.ScaledObject{
			Namespace: resp.ScaledObject.Namespace,
			Name:      resp.ScaledObject.Name,
		},
		EffectiveStart: resp.EffectiveStart,
		EffectiveEnd:   resp.EffectiveEnd,
	}
}

// ToDomainScaledObjects converts generated HTTP scaled objects into domain types.
func ToDomainScaledObjects(items []ScaledObject) []domainclient.ScaledObject {
	if len(items) == 0 {
		return nil
	}

	scaledObjects := make([]domainclient.ScaledObject, 0, len(items))
	for _, item := range items {
		scaledObjects = append(scaledObjects, domainclient.ScaledObject{
			Namespace: item.Namespace,
			Name:      item.Name,
		})
	}

	return scaledObjects
}

// ToDomainDeletedRequest converts the generated HTTP deleted response into the domain type.
func ToDomainDeletedRequest(resp DeletedRequest) domainclient.DeletedRequest {
	return domainclient.DeletedRequest{
		RequestID: resp.RequestId,
		ScaledObject: domainclient.ScaledObject{
			Namespace: resp.ScaledObject.Namespace,
			Name:      resp.ScaledObject.Name,
		},
		EffectiveStart: resp.EffectiveStart,
		EffectiveEnd:   resp.EffectiveEnd,
	}
}
