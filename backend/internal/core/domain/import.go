package domain

import "time"

type SourceImportState string

const (
	SourceImportStateFetching SourceImportState = "fetching_metadata"
	SourceImportStateScanning SourceImportState = "scanning"
	SourceImportStateReady    SourceImportState = "ready"
	SourceImportStateFailed   SourceImportState = "failed"
)

// SourceImport describes an asynchronous source import without exposing the
// provider-specific locator used to retrieve it.
type SourceImport struct {
	ID        string
	Provider  string
	Name      string
	SourceID  string
	Tracks    int
	State     SourceImportState
	Error     string
	CreatedAt time.Time
	UpdatedAt time.Time
}
