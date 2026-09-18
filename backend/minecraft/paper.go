package minecraft

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// PaperMC's v2 API was sunset; v3 ("fill") is the current one.
var paperAPIBase = "https://fill.papermc.io/v3/projects/paper"

const paperUserAgent = "realmrunner/1.0 (https://github.com/wzin/realmrunner)"

type PaperProvider struct {
	mu          sync.RWMutex
	versions    []VersionInfo
	javaMajors  map[string]int
	lastFetched time.Time
}

type paperVersionsResponse struct {
	Versions []struct {
		Version struct {
			ID      string `json:"id"`
			Support struct {
				Status string `json:"status"`
			} `json:"support"`
			Java struct {
				Version struct {
					Minimum int `json:"minimum"`
				} `json:"version"`
			} `json:"java"`
		} `json:"version"`
	} `json:"versions"`
}

type paperBuild struct {
	ID        int    `json:"id"`
	Channel   string `json:"channel"`
	Downloads map[string]struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	} `json:"downloads"`
}

func (p *PaperProvider) Flavor() string {
	return "paper"
}

func paperGet(url string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", paperUserAgent)
	return http.DefaultClient.Do(req)
}

func (p *PaperProvider) GetVersions(includeSnapshots bool) ([]VersionInfo, error) {
	p.mu.RLock()
	if p.versions != nil && time.Since(p.lastFetched) < time.Hour {
		cached := p.versions
		p.mu.RUnlock()
		return filterVersions(cached, includeSnapshots), nil
	}
	p.mu.RUnlock()

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.versions != nil && time.Since(p.lastFetched) < time.Hour {
		return filterVersions(p.versions, includeSnapshots), nil
	}

	resp, err := paperGet(paperAPIBase + "/versions")
	if err != nil {
		return nil, fmt.Errorf("failed to fetch Paper versions: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch Paper versions: %s", resp.Status)
	}

	var data paperVersionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return nil, fmt.Errorf("failed to parse Paper versions: %w", err)
	}

	// The API returns newest first and includes pre-releases/release candidates,
	// which we treat as snapshots.
	versions := make([]VersionInfo, 0, len(data.Versions))
	javaMajors := make(map[string]int, len(data.Versions))
	for _, v := range data.Versions {
		id := v.Version.ID
		if id == "" {
			continue
		}
		if v.Version.Java.Version.Minimum > 0 {
			javaMajors[id] = v.Version.Java.Version.Minimum
		}

		versionType := "release"
		if IsPrerelease(id) {
			versionType = "snapshot"
		}
		versions = append(versions, VersionInfo{ID: id, Type: versionType})
	}

	p.versions = versions
	p.javaMajors = javaMajors
	p.lastFetched = time.Now()
	return filterVersions(versions, includeSnapshots), nil
}

func (p *PaperProvider) DownloadServer(destDir string, version string) error {
	jarPath := filepath.Join(destDir, "server.jar")
	if _, err := os.Stat(jarPath); err == nil {
		return nil
	}

	build, err := p.latestBuild(version)
	if err != nil {
		return err
	}

	download, ok := build.Downloads["server:default"]
	if !ok || download.URL == "" {
		return fmt.Errorf("no server download available for Paper %s", version)
	}

	return downloadJar(download.URL, jarPath, fmt.Sprintf("Paper %s (build %d, %s)", version, build.ID, build.Channel))
}

// latestBuild prefers the newest STABLE build and only falls back to
// experimental ones for versions that have no stable build yet (which is the
// case for freshly released Minecraft versions).
func (p *PaperProvider) latestBuild(version string) (*paperBuild, error) {
	resp, err := paperGet(fmt.Sprintf("%s/versions/%s/builds", paperAPIBase, version))
	if err != nil {
		return nil, fmt.Errorf("failed to fetch Paper builds: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("no builds available for Paper %s (%s)", version, resp.Status)
	}

	var builds []paperBuild
	if err := json.NewDecoder(resp.Body).Decode(&builds); err != nil {
		return nil, fmt.Errorf("failed to parse Paper builds: %w", err)
	}
	if len(builds) == 0 {
		return nil, fmt.Errorf("no builds available for Paper %s", version)
	}

	// The API returns builds newest first.
	for i := range builds {
		if strings.EqualFold(builds[i].Channel, "STABLE") {
			return &builds[i], nil
		}
	}
	return &builds[0], nil
}

func (p *PaperProvider) StartCommand(serverDir string, memoryMB int, version string) (string, []string) {
	major := RequiredJavaMajor(version)

	// Paper publishes the minimum Java version per Minecraft version; prefer it.
	p.mu.RLock()
	needFetch := p.javaMajors == nil
	p.mu.RUnlock()
	if needFetch {
		// Populates the version cache, including the Java requirements.
		_, _ = p.GetVersions(true)
	}

	p.mu.RLock()
	if reported, ok := p.javaMajors[version]; ok && reported > 0 {
		major = reported
	}
	p.mu.RUnlock()

	return JavaCommand(major), javaArgs(memoryMB)
}
