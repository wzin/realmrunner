package minecraft

// VersionInfo represents a version available from a provider
type VersionInfo struct {
	ID   string `json:"id"`
	Type string `json:"type"` // release, snapshot, old_beta, old_alpha
}

// Provider defines the interface for different server flavors
type Provider interface {
	Flavor() string
	GetVersions(includeSnapshots bool) ([]VersionInfo, error)
	DownloadServer(destDir string, version string) error
	// StartCommand returns the command and args to start this server type
	StartCommand(serverDir string, memoryMB int) (string, []string)
}

// Registry holds all available providers
type Registry struct {
	providers map[string]Provider
}

func NewRegistry() *Registry {
	r := &Registry{
		providers: make(map[string]Provider),
	}
	r.Register(&VanillaProvider{fetcher: NewVersionFetcher()})
	r.Register(&PaperProvider{})
	r.Register(&PurpurProvider{})
	return r
}

func (r *Registry) Register(p Provider) {
	r.providers[p.Flavor()] = p
}

func (r *Registry) GetProvider(flavor string) (Provider, bool) {
	p, ok := r.providers[flavor]
	return p, ok
}

func (r *Registry) GetAllFlavors() []string {
	flavors := make([]string, 0, len(r.providers))
	for f := range r.providers {
		flavors = append(flavors, f)
	}
	return flavors
}
