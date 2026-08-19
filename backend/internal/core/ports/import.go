package ports

import (
	"context"
	"io"

	"github.com/randomtoy/peerphonic/backend/internal/core/domain"
)

type SourceImportResult struct {
	SourceID string
	Name     string
	Tracks   int
}

type SourceImporter interface {
	Import(ctx context.Context, source io.Reader) (SourceImportResult, error)
}

type SourceURIImporter interface {
	ImportURI(ctx context.Context, uri string) (domain.SourceImport, error)
	SourceImports(ctx context.Context) ([]domain.SourceImport, error)
}
