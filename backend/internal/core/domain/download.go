package domain

import "time"

type DownloadState string

const (
	DownloadStateDownloading DownloadState = "downloading"
	DownloadStateCached      DownloadState = "cached"
	DownloadStateFailed      DownloadState = "failed"
	DownloadStateEvicted     DownloadState = "evicted"
)

// TrackDownload describes a provider-owned background job that materializes
// one selected track in the media cache.
type TrackDownload struct {
	ID             string
	Provider       string
	SourceID       string
	TrackID        string
	Name           string
	State          DownloadState
	CompletedBytes int64
	TotalBytes     int64
	Error          string
	StartedAt      time.Time
	UpdatedAt      time.Time
}
