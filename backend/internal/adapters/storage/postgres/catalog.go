package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/randomtoy/peerphonic/backend/internal/adapters/storage/sqlite"
)

// Catalog exposes the PostgreSQL implementation through the same metadata
// ports as the SQLite catalog. Shared relational behavior lives in the SQL
// catalog implementation; connection setup and schema migrations are selected
// explicitly for PostgreSQL here.
type Catalog struct {
	*sqlite.Catalog
}

func Open(ctx context.Context, connectionURL string) (*Catalog, error) {
	if strings.TrimSpace(connectionURL) == "" {
		return nil, fmt.Errorf("PostgreSQL connection URL is required")
	}
	catalog, err := sqlite.OpenPostgres(ctx, connectionURL)
	if err != nil {
		return nil, err
	}
	return &Catalog{Catalog: catalog}, nil
}
