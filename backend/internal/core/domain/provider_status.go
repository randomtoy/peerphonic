package domain

type ProviderStatus struct {
	Provider      string
	Configured    bool
	Reachable     bool
	Authenticated bool
	Message       string
}
