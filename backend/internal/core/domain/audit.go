package domain

import "time"

type AuditEntry struct {
	ID            int64     `json:"id"`
	OccurredAt    time.Time `json:"occurredAt"`
	RequestID     string    `json:"requestId"`
	Actor         string    `json:"actor"`
	RemoteAddress string    `json:"remoteAddress"`
	Method        string    `json:"method"`
	Path          string    `json:"path"`
	Status        int       `json:"status"`
}
