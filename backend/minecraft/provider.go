package minecraft

import "strings"

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
	// StartCommand returns the command and args to start this server type.
	// version is the Minecraft version, used to pick a compatible Java runtime.
	StartCommand(serverDir string, memoryMB int, version string) (string, []string)
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

// IsPrerelease reports whether a version id is a snapshot, pre-release or
// release candidate rather than a full release (e.g. 26.3-rc-1, 1.21.9-pre2).
func IsPrerelease(id string) bool {
	lower := strings.ToLower(id)
	for _, marker := range []string{"-rc", "-pre", "-snapshot", "snapshot", "experimental"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	// Weekly snapshots such as 24w14a.
	return snapshotRe.MatchString(lower)
}

func filterVersions(versions []VersionInfo, includeSnapshots bool) []VersionInfo {
	if includeSnapshots {
		return versions
	}
	filtered := make([]VersionInfo, 0, len(versions))
	for _, v := range versions {
		if v.Type == "release" {
			filtered = append(filtered, v)
		}
	}
	return filtered
}
