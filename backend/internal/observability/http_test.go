package observability

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestHTTPMetricsRecordsLowCardinalityRequestData(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.August, 18, 14, 0, 0, 0, time.UTC)
	metrics := newHTTPMetrics(func() time.Time { return now })
	handler := metrics.Wrap(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		now = now.Add(100 * time.Millisecond)
		writer.WriteHeader(http.StatusPartialContent)
	}))
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/rest/stream?id=one", nil))

	response := httptest.NewRecorder()
	metrics.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/metrics", nil))
	body := response.Body.String()
	for _, expected := range []string{
		`peerphonic_http_requests_total{api="opensubsonic",method="GET",status="2xx"} 1`,
		`peerphonic_http_requests_in_flight{api="opensubsonic"} 0`,
		`peerphonic_http_request_duration_seconds_bucket{api="opensubsonic",le="0.1"} 1`,
		`peerphonic_http_request_duration_seconds_bucket{api="opensubsonic",le="+Inf"} 1`,
		`peerphonic_http_request_duration_seconds_sum{api="opensubsonic"} 0.1`,
		`peerphonic_http_request_duration_seconds_count{api="opensubsonic"} 1`,
	} {
		if !strings.Contains(body, expected) {
			t.Fatalf("metrics do not contain %q:\n%s", expected, body)
		}
	}
	if contentType := response.Header().Get("Content-Type"); !strings.HasPrefix(contentType, "text/plain") {
		t.Fatalf("Content-Type = %q", contentType)
	}
}

func TestHTTPMetricsDoNotUseRequestPathAsLabel(t *testing.T) {
	t.Parallel()

	metrics := NewHTTPMetrics()
	handler := metrics.Wrap(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNotFound)
	}))
	for _, path := range []string{"/api/v1/downloads/one", "/api/v1/downloads/two"} {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodDelete, path, nil))
	}
	response := httptest.NewRecorder()
	metrics.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/metrics", nil))
	body := response.Body.String()
	if !strings.Contains(body,
		`peerphonic_http_requests_total{api="peerphonic",method="DELETE",status="4xx"} 2`,
	) || strings.Contains(body, "downloads/one") || strings.Contains(body, "downloads/two") {
		t.Fatalf("unexpected metrics labels:\n%s", body)
	}
}

func TestHTTPMetricsAreSafeForConcurrentRequests(t *testing.T) {
	t.Parallel()

	metrics := NewHTTPMetrics()
	handler := metrics.Wrap(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusNoContent)
	}))
	const requests = 50
	var group sync.WaitGroup
	for range requests {
		group.Add(1)
		go func() {
			defer group.Done()
			handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodPost, "/rest/scrobble", nil))
		}()
	}
	group.Wait()
	response := httptest.NewRecorder()
	metrics.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/metrics", nil))
	if !strings.Contains(response.Body.String(),
		`peerphonic_http_requests_total{api="opensubsonic",method="POST",status="2xx"} 50`,
	) {
		t.Fatalf("unexpected metrics:\n%s", response.Body.String())
	}
}
