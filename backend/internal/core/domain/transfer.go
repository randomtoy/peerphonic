package domain

// SourceTransfer describes the live transfer state of one attached source.
// Byte counters are scoped to the current provider process; CompletedBytes is
// derived from the provider's persistent media state.
type SourceTransfer struct {
	Provider         string
	ID               string
	Name             string
	CompletedBytes   int64
	TotalBytes       int64
	DownloadedBytes  int64
	UploadedBytes    int64
	DownloadLimit    int64
	UploadLimit      int64
	Peers            int
	ActivePeers      int
	ConnectedSeeders int
	ActiveStreams    int
	Seeding          bool
}
