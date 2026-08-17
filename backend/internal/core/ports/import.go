package ports

import (
	"context"
	"io"
)

type SourceImportResult struct {
	SourceID string
	Name     string
	Tracks   int
}

type SourceImporter interface {
	Import(ctx context.Context, source io.Reader) (SourceImportResult, error)
}
