package httpclient

import (
	"context"
	"fmt"
	"strings"

	domainclient "github.com/Kotaro7750/keda-launcher-scaler/pkg/client"
)

// Error represents a non-accepted HTTP API response.
type Error struct {
	StatusCode int
	Status     string
	Message    string
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Message == "" {
		if e.Status != "" {
			return fmt.Sprintf("launch request failed: %s", e.Status)
		}
		return fmt.Sprintf("launch request failed: status %d", e.StatusCode)
	}
	if e.Status != "" {
		return fmt.Sprintf("launch request failed: %s: %s", e.Status, e.Message)
	}
	return fmt.Sprintf("launch request failed: status %d: %s", e.StatusCode, e.Message)
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
			StatusCode: response.StatusCode(),
			Status:     response.Status(),
			Message:    strings.TrimSpace(response.JSON400.Message),
		}
	}
	if response.JSON408 != nil {
		return domainclient.AcceptedRequest{}, &Error{
			StatusCode: response.StatusCode(),
			Status:     response.Status(),
			Message:    strings.TrimSpace(response.JSON408.Message),
		}
	}

	return domainclient.AcceptedRequest{}, &Error{
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
