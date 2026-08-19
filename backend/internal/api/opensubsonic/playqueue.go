package opensubsonic

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
	"github.com/randomtoy/peerphonic/backend/internal/core/ports"
	"github.com/randomtoy/peerphonic/backend/internal/core/services"
)

func (h *Handler) getPlayQueue(writer http.ResponseWriter, request *http.Request) {
	if h.playQueue == nil {
		h.writeError(writer, request, http.StatusInternalServerError, 0, "Play queue storage is not configured")
		return
	}
	queue, err := h.playQueue.Get(request.Context(), requestUsername(request))
	if err != nil {
		h.writePlayQueueError(writer, request, err)
		return
	}
	h.write(writer, request, http.StatusOK, response{PlayQueue: makePlayQueue(queue)})
}

func (h *Handler) savePlayQueue(writer http.ResponseWriter, request *http.Request) {
	if h.playQueue == nil {
		h.writeError(writer, request, http.StatusInternalServerError, 0, "Play queue storage is not configured")
		return
	}
	position := int64(0)
	if value := request.Form.Get("position"); value != "" {
		var err error
		position, err = strconv.ParseInt(value, 10, 64)
		if err != nil || position < 0 {
			h.writeError(writer, request, http.StatusBadRequest, 10,
				"Parameter position must be a non-negative integer")
			return
		}
	}
	if _, err := h.playQueue.Save(
		request.Context(), requestUsername(request), request.Form["id"], request.Form.Get("current"),
		position, request.Form.Get("c"),
	); err != nil {
		h.writePlayQueueError(writer, request, err)
		return
	}
	h.write(writer, request, http.StatusOK, response{})
}

func (h *Handler) writePlayQueueError(writer http.ResponseWriter, request *http.Request, err error) {
	switch {
	case errors.Is(err, ports.ErrNotFound):
		h.writeError(writer, request, http.StatusNotFound, 70, "Play queue song not found")
	case errors.Is(err, services.ErrInvalidPlayQueue):
		h.writeError(writer, request, http.StatusBadRequest, 10, err.Error())
	default:
		h.writeError(writer, request, http.StatusInternalServerError, 0, "Failed to update play queue")
	}
}

func makePlayQueue(queue domain.PlayQueue) *playQueue {
	changed := queue.Changed
	if changed.IsZero() {
		changed = time.Unix(0, 0).UTC()
	}
	payload := &playQueue{
		Current: queue.CurrentID, Position: queue.PositionMS, Username: queue.Owner,
		Changed: changed.UTC().Format(time.RFC3339Nano), ChangedBy: queue.ChangedBy,
		Entries: make([]child, 0, len(queue.Tracks)),
	}
	for _, track := range queue.Tracks {
		payload.Entries = append(payload.Entries, trackChild(track))
	}
	return payload
}
