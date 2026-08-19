package postgresmigrations

import "embed"

// Files contains the PostgreSQL metadata schema migrations.
//
//go:embed *.sql
var Files embed.FS
