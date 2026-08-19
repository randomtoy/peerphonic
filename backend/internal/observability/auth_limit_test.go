package observability

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestAuthFailureLimiterBlocksOnlyAfterFailedAuthentication(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	limiter := NewAuthFailureLimiter(2, time.Minute, 30*time.Second)
	limiter.now = func() time.Time { return now }
	handler := limiter.Wrap(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if password, _, _ := request.BasicAuth(); password == "alice" && request.Header.Get("X-Good") == "yes" {
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		writer.WriteHeader(http.StatusUnauthorized)
	}))
	request := func(good bool) *httptest.ResponseRecorder {
		recorder := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
		req.RemoteAddr = "192.0.2.1:1234"
		req.SetBasicAuth("alice", "wrong")
		if good {
			req.Header.Set("X-Good", "yes")
		}
		handler.ServeHTTP(recorder, req)
		return recorder
	}
	if status := request(true).Code; status != http.StatusNoContent {
		t.Fatalf("successful request status = %d", status)
	}
	for index := 0; index < 2; index++ {
		if status := request(false).Code; status != http.StatusUnauthorized {
			t.Fatalf("failure %d status = %d", index, status)
		}
	}
	blocked := request(true)
	if blocked.Code != http.StatusTooManyRequests || blocked.Header().Get("Retry-After") == "" {
		t.Fatalf("blocked response = %d, headers = %v", blocked.Code, blocked.Header())
	}
	now = now.Add(31 * time.Second)
	if status := request(true).Code; status != http.StatusNoContent {
		t.Fatalf("post-block status = %d", status)
	}
}
