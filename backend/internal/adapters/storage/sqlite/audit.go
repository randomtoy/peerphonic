package sqlite

import (
	"context"
	"fmt"
	"time"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
)

func (c *Catalog) AppendAuditEntry(ctx context.Context, entry domain.AuditEntry) error {
	_, err := c.db.ExecContext(ctx, `INSERT INTO audit_log(
		occurred_at, request_id, actor, remote_address, method, path, status
	) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		entry.OccurredAt.UTC().Format(time.RFC3339Nano), entry.RequestID, entry.Actor,
		entry.RemoteAddress, entry.Method, entry.Path, entry.Status,
	)
	if err != nil {
		return fmt.Errorf("append audit entry: %w", err)
	}
	return nil
}

func (c *Catalog) AuditEntries(ctx context.Context, limit int) ([]domain.AuditEntry, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	rows, err := c.db.QueryContext(ctx, `SELECT id, occurred_at, request_id, actor,
		remote_address, method, path, status
		FROM audit_log ORDER BY occurred_at DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list audit entries: %w", err)
	}
	defer rows.Close()
	entries := make([]domain.AuditEntry, 0, limit)
	for rows.Next() {
		var entry domain.AuditEntry
		var occurredAt string
		if err := rows.Scan(&entry.ID, &occurredAt, &entry.RequestID, &entry.Actor,
			&entry.RemoteAddress, &entry.Method, &entry.Path, &entry.Status); err != nil {
			return nil, fmt.Errorf("scan audit entry: %w", err)
		}
		entry.OccurredAt, err = time.Parse(time.RFC3339Nano, occurredAt)
		if err != nil {
			return nil, fmt.Errorf("parse audit timestamp: %w", err)
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate audit entries: %w", err)
	}
	return entries, nil
}
