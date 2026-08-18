package peerphonic

import (
	"context"
	"encoding/json"
	"net/http"
)

type readinessChecker interface {
	Ping(ctx context.Context) error
}

func NewReadinessHandler(checker readinessChecker) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet {
			writer.Header().Set("Allow", http.MethodGet)
			http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		status := http.StatusOK
		state := "ready"
		if checker == nil || checker.Ping(request.Context()) != nil {
			status = http.StatusServiceUnavailable
			state = "unavailable"
		}
		writer.Header().Set("Content-Type", "application/json; charset=utf-8")
		writer.WriteHeader(status)
		_ = json.NewEncoder(writer).Encode(map[string]string{
			"status":  state,
			"service": "peerphonic",
		})
	})
}
