package peerphonic

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetricsRequireMonitoringAccess(t *testing.T) {
	t.Parallel()

	metrics := http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "text/plain")
		_, _ = writer.Write([]byte("peerphonic_http_requests_total 1\n"))
	})
	handler := NewMetricsHandler(metrics, fixedAuthenticator{username: "admin", password: "secret"})

	unauthorized := httptest.NewRecorder()
	handler.ServeHTTP(unauthorized, httptest.NewRequest(http.MethodGet, "/api/v1/metrics", nil))
	if unauthorized.Code != http.StatusUnauthorized {
		t.Fatalf("unauthorized status = %d", unauthorized.Code)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/v1/metrics", nil)
	request.SetBasicAuth("admin", "secret")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "peerphonic_http_requests_total") {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestMetricsRejectMutations(t *testing.T) {
	t.Parallel()

	request := httptest.NewRequest(http.MethodPost, "/api/v1/metrics", nil)
	request.SetBasicAuth("admin", "secret")
	response := httptest.NewRecorder()
	NewMetricsHandler(http.NotFoundHandler(), fixedAuthenticator{
		username: "admin", password: "secret",
	}).ServeHTTP(response, request)
	if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("status = %d, allow = %q", response.Code, response.Header().Get("Allow"))
	}
}
