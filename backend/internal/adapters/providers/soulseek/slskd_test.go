package soulseek

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSlskdProviderStatusAuthenticatesWithAPIKey(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/base/api/v0/session" {
			t.Errorf("path = %q", request.URL.Path)
		}
		if request.Header.Get("X-API-Key") != "0123456789abcdef" {
			t.Errorf("API key = %q", request.Header.Get("X-API-Key"))
		}
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"authenticated":true}`))
	}))
	defer server.Close()
	client, err := NewSlskd(server.URL+"/base", "0123456789abcdef", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	status := client.ProviderStatus(context.Background())
	if !status.Configured || !status.Reachable || !status.Authenticated || status.Provider != Name {
		t.Fatalf("ProviderStatus() = %#v", status)
	}
}

func TestSlskdProviderStatusReportsRejectedAndUnavailableConnections(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusUnauthorized)
	}))
	client, err := NewSlskd(server.URL, "wrong-api-key", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	status := client.ProviderStatus(context.Background())
	if !status.Reachable || status.Authenticated || status.Message == "" {
		t.Fatalf("rejected ProviderStatus() = %#v", status)
	}
	server.Close()
	status = client.ProviderStatus(context.Background())
	if status.Reachable || status.Authenticated || status.Message == "" {
		t.Fatalf("unavailable ProviderStatus() = %#v", status)
	}
}

func TestNewSlskdValidatesEndpoint(t *testing.T) {
	t.Parallel()

	for _, endpoint := range []string{"", "slskd:5030", "ftp://slskd:5030"} {
		if _, err := NewSlskd(endpoint, "", time.Second); err == nil {
			t.Fatalf("NewSlskd(%q) error = nil", endpoint)
		}
	}
}
