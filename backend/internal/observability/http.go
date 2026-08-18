package observability

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

var durationBuckets = [...]float64{0.005, 0.025, 0.1, 0.5, 1, 5, 15, 60}

type requestKey struct {
	surface string
	method  string
	status  string
}

type durationMetric struct {
	count   uint64
	sum     float64
	buckets [len(durationBuckets)]uint64
}

type HTTPMetrics struct {
	mu        sync.Mutex
	now       func() time.Time
	requests  map[requestKey]uint64
	inFlight  map[string]int64
	durations map[string]durationMetric
}

func NewHTTPMetrics() *HTTPMetrics {
	return newHTTPMetrics(time.Now)
}

func newHTTPMetrics(now func() time.Time) *HTTPMetrics {
	return &HTTPMetrics{
		now: now, requests: make(map[requestKey]uint64),
		inFlight: make(map[string]int64), durations: make(map[string]durationMetric),
	}
}

func (m *HTTPMetrics) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		surface := requestSurface(request.URL.Path)
		method := requestMethod(request.Method)
		started := m.now()
		m.changeInFlight(surface, 1)
		observed := &responseObserver{ResponseWriter: writer}
		defer func() {
			m.observe(surface, method, statusClass(observed.status), m.now().Sub(started))
			m.changeInFlight(surface, -1)
		}()
		next.ServeHTTP(observed, request)
	})
}

func (m *HTTPMetrics) ServeHTTP(writer http.ResponseWriter, _ *http.Request) {
	requests, inFlight, durations := m.snapshot()
	writer.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

	var output strings.Builder
	output.WriteString("# HELP peerphonic_http_requests_total Completed HTTP requests by API, method, and status class.\n")
	output.WriteString("# TYPE peerphonic_http_requests_total counter\n")
	requestKeys := make([]requestKey, 0, len(requests))
	for key := range requests {
		requestKeys = append(requestKeys, key)
	}
	sort.Slice(requestKeys, func(i, j int) bool {
		left, right := requestKeys[i], requestKeys[j]
		if left.surface != right.surface {
			return left.surface < right.surface
		}
		if left.method != right.method {
			return left.method < right.method
		}
		return left.status < right.status
	})
	for _, key := range requestKeys {
		fmt.Fprintf(&output,
			"peerphonic_http_requests_total{api=%q,method=%q,status=%q} %d\n",
			key.surface, key.method, key.status, requests[key],
		)
	}

	output.WriteString("# HELP peerphonic_http_requests_in_flight Current HTTP requests by API.\n")
	output.WriteString("# TYPE peerphonic_http_requests_in_flight gauge\n")
	for _, surface := range sortedKeys(inFlight) {
		fmt.Fprintf(&output, "peerphonic_http_requests_in_flight{api=%q} %d\n", surface, inFlight[surface])
	}

	output.WriteString("# HELP peerphonic_http_request_duration_seconds HTTP request duration by API.\n")
	output.WriteString("# TYPE peerphonic_http_request_duration_seconds histogram\n")
	for _, surface := range sortedKeys(durations) {
		metric := durations[surface]
		for index, upperBound := range durationBuckets {
			fmt.Fprintf(&output,
				"peerphonic_http_request_duration_seconds_bucket{api=%q,le=%q} %d\n",
				surface, strconv.FormatFloat(upperBound, 'g', -1, 64), metric.buckets[index],
			)
		}
		fmt.Fprintf(&output,
			"peerphonic_http_request_duration_seconds_bucket{api=%q,le=\"+Inf\"} %d\n",
			surface, metric.count,
		)
		fmt.Fprintf(&output, "peerphonic_http_request_duration_seconds_sum{api=%q} %s\n",
			surface, strconv.FormatFloat(metric.sum, 'g', -1, 64))
		fmt.Fprintf(&output, "peerphonic_http_request_duration_seconds_count{api=%q} %d\n",
			surface, metric.count)
	}
	_, _ = writer.Write([]byte(output.String()))
}

func (m *HTTPMetrics) changeInFlight(surface string, delta int64) {
	m.mu.Lock()
	m.inFlight[surface] += delta
	m.mu.Unlock()
}

func (m *HTTPMetrics) observe(surface, method, status string, duration time.Duration) {
	seconds := max(0, duration.Seconds())
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests[requestKey{surface: surface, method: method, status: status}]++
	metric := m.durations[surface]
	metric.count++
	metric.sum += seconds
	for index, upperBound := range durationBuckets {
		if seconds <= upperBound {
			metric.buckets[index]++
		}
	}
	m.durations[surface] = metric
}

func (m *HTTPMetrics) snapshot() (
	map[requestKey]uint64,
	map[string]int64,
	map[string]durationMetric,
) {
	m.mu.Lock()
	defer m.mu.Unlock()
	requests := make(map[requestKey]uint64, len(m.requests))
	for key, value := range m.requests {
		requests[key] = value
	}
	inFlight := make(map[string]int64, len(m.inFlight))
	for key, value := range m.inFlight {
		inFlight[key] = value
	}
	durations := make(map[string]durationMetric, len(m.durations))
	for key, value := range m.durations {
		durations[key] = value
	}
	return requests, inFlight, durations
}

func requestSurface(path string) string {
	switch {
	case path == "/rest" || strings.HasPrefix(path, "/rest/"):
		return "opensubsonic"
	case path == "/api/v1" || strings.HasPrefix(path, "/api/v1/"):
		return "peerphonic"
	default:
		return "discovery"
	}
}

func requestMethod(method string) string {
	method = strings.ToUpper(method)
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return method
	default:
		return "OTHER"
	}
}

func statusClass(status int) string {
	if status == 0 {
		status = http.StatusOK
	}
	if status < 100 || status > 999 {
		return "other"
	}
	return strconv.Itoa(status/100) + "xx"
}

func sortedKeys[T any](values map[string]T) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

type responseObserver struct {
	http.ResponseWriter
	status int
}

func (w *responseObserver) WriteHeader(status int) {
	if status >= 100 && status < 200 {
		w.ResponseWriter.WriteHeader(status)
		return
	}
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *responseObserver) Write(contents []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(contents)
}

func (w *responseObserver) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}
