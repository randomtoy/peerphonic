package peerphonic

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
)

type cacheStatusStub struct{}

func (cacheStatusStub) Stats(context.Context) (domain.CacheStats, error) {
	return domain.CacheStats{
		Capacity: 100, Entries: 2, Size: 75, PinnedEntries: 1, PinnedSize: 50,
	}, nil
}

func TestHealth(t *testing.T) {
	t.Parallel()

	response := httptest.NewRecorder()
	NewHandler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/health", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"status":"ok"`) {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestRootIsReachableForClientDiscovery(t *testing.T) {
	t.Parallel()

	response := httptest.NewRecorder()
	NewHandler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/", nil))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"service":"peerphonic"`) {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestUnknownRouteIsNotFound(t *testing.T) {
	t.Parallel()

	response := httptest.NewRecorder()
	NewHandler().ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/unknown", nil))
	if response.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusNotFound)
	}
}

func TestCacheStatus(t *testing.T) {
	t.Parallel()

	response := httptest.NewRecorder()
	NewHandler(cacheStatusStub{}).ServeHTTP(response,
		httptest.NewRequest(http.MethodGet, "/api/v1/cache/status", nil))
	if response.Code != http.StatusOK ||
		!strings.Contains(response.Body.String(), `"capacityBytes":100`) ||
		!strings.Contains(response.Body.String(), `"pinnedEntries":1`) {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
}
