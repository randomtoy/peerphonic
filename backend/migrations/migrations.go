package migrations

import "embed"

// Files contains the ordered SQLite schema migrations.
//
//go:embed *.sql
var Files embed.FS
