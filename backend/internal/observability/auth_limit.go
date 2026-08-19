package observability

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type authFailureState struct {
	firstFailure time.Time
	failures     int
	blockedUntil time.Time
}

// AuthFailureLimiter blocks only clients that repeatedly receive 401
// responses. Successful streaming and other authenticated traffic is never
// counted against the limit.
type AuthFailureLimiter struct {
	mu       sync.Mutex
	limit    int
	window   time.Duration
	blockFor time.Duration
	now      func() time.Time
	states   map[string]authFailureState
	attempts uint64
}

func NewAuthFailureLimiter(limit int, window, blockFor time.Duration) *AuthFailureLimiter {
	if limit <= 0 {
		limit = 10
	}
	if window <= 0 {
		window = time.Minute
	}
	if blockFor <= 0 {
		blockFor = time.Minute
	}
	return &AuthFailureLimiter{
		limit: limit, window: window, blockFor: blockFor, now: time.Now,
		states: make(map[string]authFailureState),
	}
}

func (l *AuthFailureLimiter) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		key := authFailureKey(request)
		if retryAfter := l.blockedFor(key); retryAfter > 0 {
			writer.Header().Set("Retry-After", strconv.Itoa(max(1, int(retryAfter.Round(time.Second)/time.Second))))
			http.Error(writer, "too many failed authentication attempts", http.StatusTooManyRequests)
			return
		}
		observed := &responseObserver{ResponseWriter: writer}
		next.ServeHTTP(observed, request)
		if observed.status == http.StatusUnauthorized {
			l.recordFailure(key)
		}
	})
}

func (l *AuthFailureLimiter) blockedFor(key string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()
	state := l.states[key]
	remaining := state.blockedUntil.Sub(l.now())
	if remaining <= 0 && !state.blockedUntil.IsZero() {
		delete(l.states, key)
		return 0
	}
	return remaining
}

func (l *AuthFailureLimiter) recordFailure(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	state := l.states[key]
	if state.firstFailure.IsZero() || now.Sub(state.firstFailure) >= l.window {
		state = authFailureState{firstFailure: now}
	}
	state.failures++
	if state.failures >= l.limit {
		state.blockedUntil = now.Add(l.blockFor)
	}
	l.states[key] = state
	l.attempts++
	if l.attempts%256 == 0 {
		for candidate, candidateState := range l.states {
			if now.After(candidateState.blockedUntil) && now.Sub(candidateState.firstFailure) >= l.window {
				delete(l.states, candidate)
			}
		}
	}
}

func authFailureKey(request *http.Request) string {
	remote := request.RemoteAddr
	if host, _, err := net.SplitHostPort(remote); err == nil {
		remote = host
	}
	username, _, _ := request.BasicAuth()
	if username == "" {
		username = request.URL.Query().Get("u")
	}
	return strings.ToLower(strings.TrimSpace(remote)) + "\x00" + strings.ToLower(strings.TrimSpace(username))
}
