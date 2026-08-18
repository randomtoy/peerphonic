package soulseek

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
)

const Name = "soulseek"

type Client struct {
	endpoint   *url.URL
	apiKey     string
	httpClient *http.Client
}

func NewSlskd(endpoint, apiKey string, timeout time.Duration) (*Client, error) {
	endpoint = strings.TrimSpace(endpoint)
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return nil, fmt.Errorf("parse slskd URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("slskd URL must use http or https")
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("slskd URL must include a host")
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &Client{
		endpoint: parsed, apiKey: strings.TrimSpace(apiKey), httpClient: &http.Client{Timeout: timeout},
	}, nil
}

func (c *Client) ProviderStatus(ctx context.Context) domain.ProviderStatus {
	status := domain.ProviderStatus{Provider: Name, Configured: true}
	endpoint := *c.endpoint
	endpoint.Path = strings.TrimRight(endpoint.Path, "/") + "/api/v0/session"
	endpoint.RawPath = ""
	endpoint.RawQuery = ""
	endpoint.Fragment = ""
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		status.Message = "invalid slskd request"
		return status
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("User-Agent", "Peerphonic")
	if c.apiKey != "" {
		request.Header.Set("X-API-Key", c.apiKey)
	}
	response, err := c.httpClient.Do(request)
	if err != nil {
		status.Message = "slskd is unreachable"
		return status
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 64<<10))
	status.Reachable = true
	switch response.StatusCode {
	case http.StatusOK, http.StatusNoContent:
		status.Authenticated = true
		status.Message = "slskd API is ready"
	case http.StatusUnauthorized, http.StatusForbidden:
		status.Message = "slskd rejected the API key"
	default:
		status.Message = fmt.Sprintf("slskd returned HTTP %d", response.StatusCode)
	}
	return status
}
