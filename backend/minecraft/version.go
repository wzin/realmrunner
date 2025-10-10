package minecraft

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

const versionManifestURL = "https://launchermeta.mojang.com/mc/game/version_manifest.json"

type VersionManifest struct {
	Latest struct {
		Release  string `json:"release"`
		Snapshot string `json:"snapshot"`
	} `json:"latest"`
	Versions []Version `json:"versions"`
}

type Version struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	URL         string `json:"url"`
	Time        string `json:"time"`
	ReleaseTime string `json:"releaseTime"`
}

type VersionDetails struct {
	Downloads struct {
		Server struct {
			URL string `json:"url"`
		} `json:"server"`
	} `json:"downloads"`
}

type VersionFetcher struct {
	mu          sync.RWMutex
	manifest    *VersionManifest
	lastFetched time.Time
	cacheTTL    time.Duration
}

func NewVersionFetcher() *VersionFetcher {
	return &VersionFetcher{
		cacheTTL: 1 * time.Hour,
	}
}

func (vf *VersionFetcher) GetVersions() ([]string, error) {
	manifest, err := vf.getManifest()
	if err != nil {
		return nil, err
	}

	// Filter for release versions only
	versions := []string{}
	for _, v := range manifest.Versions {
		if v.Type == "release" {
			versions = append(versions, v.ID)
		}
	}

	return versions, nil
}

func (vf *VersionFetcher) GetServerDownloadURL(version string) (string, error) {
	manifest, err := vf.getManifest()
	if err != nil {
		return "", err
	}

	// Find version in manifest
	var versionURL string
	for _, v := range manifest.Versions {
		if v.ID == version {
			versionURL = v.URL
			break
		}
	}

	if versionURL == "" {
		return "", fmt.Errorf("version %s not found", version)
	}

	// Fetch version details
	resp, err := http.Get(versionURL)
	if err != nil {
		return "", fmt.Errorf("failed to fetch version details: %w", err)
	}
	defer resp.Body.Close()

	var details VersionDetails
	if err := json.NewDecoder(resp.Body).Decode(&details); err != nil {
		return "", fmt.Errorf("failed to parse version details: %w", err)
	}

	if details.Downloads.Server.URL == "" {
		return "", fmt.Errorf("server download not available for version %s", version)
	}

	return details.Downloads.Server.URL, nil
}

func (vf *VersionFetcher) getManifest() (*VersionManifest, error) {
	vf.mu.RLock()
	if vf.manifest != nil && time.Since(vf.lastFetched) < vf.cacheTTL {
		defer vf.mu.RUnlock()
		return vf.manifest, nil
	}
	vf.mu.RUnlock()

	// Fetch manifest
	vf.mu.Lock()
	defer vf.mu.Unlock()

	// Double-check after acquiring write lock
	if vf.manifest != nil && time.Since(vf.lastFetched) < vf.cacheTTL {
		return vf.manifest, nil
	}

	resp, err := http.Get(versionManifestURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch version manifest: %w", err)
	}
	defer resp.Body.Close()

	var manifest VersionManifest
	if err := json.NewDecoder(resp.Body).Decode(&manifest); err != nil {
		return nil, fmt.Errorf("failed to parse version manifest: %w", err)
	}

	vf.manifest = &manifest
	vf.lastFetched = time.Now()

	return vf.manifest, nil
}
