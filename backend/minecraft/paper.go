package minecraft

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const paperAPIBase = "https://api.papermc.io/v2/projects/paper"

type PaperProvider struct {
	mu          sync.RWMutex
	versions    []VersionInfo
	lastFetched time.Time
}

type paperVersionsResponse struct {
	Versions []string `json:"versions"`
}

type paperBuildsResponse struct {
	Builds []struct {
		Build    int    `json:"build"`
		Channel  string `json:"channel"`
		Downloads struct {
			Application struct {
				Name string `json:"name"`
			} `json:"application"`
		} `json:"downloads"`
	} `json:"builds"`
}

func (p *PaperProvider) Flavor() string {
	return "paper"
}

func (p *PaperProvider) GetVersions(includeSnapshots bool) ([]VersionInfo, error) {
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

	resp, err := http.Get(paperAPIBase)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch Paper versions: %w", err)
	}
	defer resp.Body.Close()

	var data paperVersionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to parse Paper versions: %w", err)
	}

	versions := make([]VersionInfo, len(data.Versions))
	for i, v := range data.Versions {
		versions[i] = VersionInfo{ID: v, Type: "release"}
	}

	// Reverse so newest is first
	sort.Slice(versions, func(i, j int) bool {
		return i > j // reverse the original order (PaperMC returns oldest first)
	})

	p.versions = versions
	p.lastFetched = time.Now()
	return versions, nil
}

func (p *PaperProvider) DownloadServer(destDir string, version string) error {
	jarPath := filepath.Join(destDir, "server.jar")
	if _, err := os.Stat(jarPath); err == nil {
		return nil
	}

	// Get latest build for this version
	buildsURL := fmt.Sprintf("%s/versions/%s/builds", paperAPIBase, version)
	resp, err := http.Get(buildsURL)
	if err != nil {
		return fmt.Errorf("failed to fetch Paper builds: %w", err)
	}
	defer resp.Body.Close()

	var builds paperBuildsResponse
	if err := json.NewDecoder(resp.Body).Decode(&builds); err != nil {
		return fmt.Errorf("failed to parse Paper builds: %w", err)
	}

	if len(builds.Builds) == 0 {
		return fmt.Errorf("no builds available for Paper %s", version)
	}

	// Use the latest build
	latest := builds.Builds[len(builds.Builds)-1]
	fileName := latest.Downloads.Application.Name
	if fileName == "" {
		fileName = fmt.Sprintf("paper-%s-%d.jar", version, latest.Build)
	}

	downloadURL := fmt.Sprintf("%s/versions/%s/builds/%d/downloads/%s", paperAPIBase, version, latest.Build, fileName)
	return downloadJar(downloadURL, jarPath, fmt.Sprintf("Paper %s (build %d)", version, latest.Build))
}

func (p *PaperProvider) StartCommand(serverDir string, memoryMB int) (string, []string) {
	return "java", []string{
		fmt.Sprintf("-Xmx%dM", memoryMB),
		fmt.Sprintf("-Xms%dM", memoryMB),
		"-jar", "server.jar", "nogui",
	}
}
