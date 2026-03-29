package minecraft

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const purpurAPIBase = "https://api.purpurmc.org/v2/purpur"

type PurpurProvider struct {
	mu          sync.RWMutex
	versions    []VersionInfo
	lastFetched time.Time
}

type purpurVersionsResponse struct {
	Versions []string `json:"versions"`
}

func (p *PurpurProvider) Flavor() string {
	return "purpur"
}

func (p *PurpurProvider) GetVersions(includeSnapshots bool) ([]VersionInfo, error) {
	p.mu.RLock()
	if p.versions != nil && time.Since(p.lastFetched) < time.Hour {
		defer p.mu.RUnlock()
		return p.versions, nil
	}
	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.versions != nil && time.Since(p.lastFetched) < time.Hour {
		return p.versions, nil
	}

	resp, err := http.Get(purpurAPIBase)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch Purpur versions: %w", err)
	}
	defer resp.Body.Close()

	var data purpurVersionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to parse Purpur versions: %w", err)
	}

	versions := make([]VersionInfo, 0, len(data.Versions))
	// Reverse so newest is first
	for i := len(data.Versions) - 1; i >= 0; i-- {
		versions = append(versions, VersionInfo{
			ID:   data.Versions[i],
			Type: "release",
		})
	}

	p.versions = versions
	p.lastFetched = time.Now()
	return versions, nil
}

func (p *PurpurProvider) DownloadServer(destDir string, version string) error {
	jarPath := filepath.Join(destDir, "server.jar")
	if _, err := os.Stat(jarPath); err == nil {
		return nil
	}

	downloadURL := fmt.Sprintf("%s/%s/latest/download", purpurAPIBase, version)
	return downloadJar(downloadURL, jarPath, fmt.Sprintf("Purpur %s", version))
}

func (p *PurpurProvider) StartCommand(serverDir string, memoryMB int) (string, []string) {
	return "java", []string{
		fmt.Sprintf("-Xmx%dM", memoryMB),
		fmt.Sprintf("-Xms%dM", memoryMB),
		"-jar", "server.jar", "nogui",
	}
}
