package domain

// ManagedSource describes one imported source and its persistent operational
// state without exposing provider-specific transport objects.
type ManagedSource struct {
	Provider string
	ID       string
	Name     string
	Tracks   int
	Attached bool
	Paused   bool
	Pinned   bool
}
