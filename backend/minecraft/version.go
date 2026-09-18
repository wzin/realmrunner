package minecraft

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

var versionManifestURL = "https://piston-meta.mojang.com/mc/game/version_manifest_v2.json"

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
	JavaVersion struct {
		MajorVersion int `json:"majorVersion"`
	} `json:"javaVersion"`
}

type VersionFetcher struct {
	mu          sync.RWMutex
	manifest    *VersionManifest
	lastFetched time.Time
	cacheTTL    time.Duration

	detailsMu sync.RWMutex
	details   map[string]*VersionDetails
}

func NewVersionFetcher() *VersionFetcher {
	return &VersionFetcher{
		cacheTTL: 1 * time.Hour,
		details:  make(map[string]*VersionDetails),
	}
}

// detailsClient has a timeout so a slow Mojang API can never block a server start.
var detailsClient = &http.Client{Timeout: 10 * time.Second}

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
	details, err := vf.getVersionDetails(version)
	if err != nil {
		return "", err
	}

	if details.Downloads.Server.URL == "" {
		return "", fmt.Errorf("server download not available for version %s", version)
	}

	return details.Downloads.Server.URL, nil
}

// GetJavaMajor returns the Java major version Mojang declares for a release.
func (vf *VersionFetcher) GetJavaMajor(version string) (int, error) {
	details, err := vf.getVersionDetails(version)
	if err != nil {
		return 0, err
	}
	if details.JavaVersion.MajorVersion == 0 {
		return 0, fmt.Errorf("no java version declared for %s", version)
	}
	return details.JavaVersion.MajorVersion, nil
}

func (vf *VersionFetcher) getVersionDetails(version string) (*VersionDetails, error) {
	vf.detailsMu.RLock()
	cached, ok := vf.details[version]
	vf.detailsMu.RUnlock()
	if ok {
		return cached, nil
	}

	manifest, err := vf.getManifest()
	if err != nil {
		return nil, err
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
		return nil, fmt.Errorf("version %s not found", version)
	}

	resp, err := detailsClient.Get(versionURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch version details: %w", err)
	}
	defer resp.Body.Close()

	var details VersionDetails
	if err := json.NewDecoder(resp.Body).Decode(&details); err != nil {
		return nil, fmt.Errorf("failed to parse version details: %w", err)
	}

	vf.detailsMu.Lock()
	if vf.details == nil {
		vf.details = make(map[string]*VersionDetails)
	}
	vf.details[version] = &details
	vf.detailsMu.Unlock()

	return &details, nil
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
