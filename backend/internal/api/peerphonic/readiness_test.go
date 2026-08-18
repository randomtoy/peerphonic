package peerphonic

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type readinessCheckerStub struct {
	err error
}

func (s readinessCheckerStub) Ping(context.Context) error {
	return s.err
}

func TestReadinessHandler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		checker readinessChecker
		status  int
		body    string
	}{
		{name: "ready", checker: readinessCheckerStub{}, status: http.StatusOK, body: `"status":"ready"`},
		{
			name: "storage unavailable", checker: readinessCheckerStub{err: errors.New("database closed")},
			status: http.StatusServiceUnavailable, body: `"status":"unavailable"`,
		},
		{name: "missing checker", status: http.StatusServiceUnavailable, body: `"status":"unavailable"`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			response := httptest.NewRecorder()
			NewReadinessHandler(test.checker).ServeHTTP(
				response, httptest.NewRequest(http.MethodGet, "/api/v1/ready", nil),
			)
			if response.Code != test.status || !strings.Contains(response.Body.String(), test.body) {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
			if strings.Contains(response.Body.String(), "database closed") {
				t.Fatalf("readiness response leaked storage error: %s", response.Body.String())
			}
		})
	}
}

func TestReadinessHandlerRejectsMutations(t *testing.T) {
	t.Parallel()

	response := httptest.NewRecorder()
	NewReadinessHandler(readinessCheckerStub{}).ServeHTTP(
		response, httptest.NewRequest(http.MethodPost, "/api/v1/ready", nil),
	)
	if response.Code != http.StatusMethodNotAllowed || response.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("status = %d, allow = %q", response.Code, response.Header().Get("Allow"))
	}
}
