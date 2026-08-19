CREATE TABLE audit_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    occurred_at TEXT NOT NULL,
    request_id TEXT NOT NULL,
    actor TEXT NOT NULL,
    remote_address TEXT NOT NULL,
    method TEXT NOT NULL,
    path TEXT NOT NULL,
    status INTEGER NOT NULL
);

CREATE INDEX audit_log_occurred_at_idx ON audit_log(occurred_at DESC, id DESC);

CREATE TRIGGER audit_log_retention
AFTER INSERT ON audit_log
WHEN NEW.id % 100 = 0
BEGIN
    DELETE FROM audit_log
    WHERE id IN (
        SELECT id FROM audit_log
        ORDER BY occurred_at DESC, id DESC
        LIMIT -1 OFFSET 10000
    );
END;
