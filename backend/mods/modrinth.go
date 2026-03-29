package mods

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
)

const modrinthAPI = "https://api.modrinth.com/v2"
const userAgent = "wzin/realmrunner/1.0 (wojtek@ziniewicz.eu)"

type SearchResult struct {
	Slug        string   `json:"slug"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	ProjectType string   `json:"project_type"`
	Downloads   int      `json:"downloads"`
	IconURL     string   `json:"icon_url"`
	Author      string   `json:"author"`
	Categories  []string `json:"display_categories"`
}

type SearchResponse struct {
	Hits      []SearchResult `json:"hits"`
	TotalHits int            `json:"total_hits"`
}

type ModVersion struct {
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	VersionNumber string   `json:"version_number"`
	GameVersions  []string `json:"game_versions"`
	Loaders       []string `json:"loaders"`
	Files         []struct {
		URL      string `json:"url"`
		Filename string `json:"filename"`
		Primary  bool   `json:"primary"`
		Size     int64  `json:"size"`
	} `json:"files"`
}

func modrinthGet(path string) (*http.Response, error) {
	req, err := http.NewRequest("GET", modrinthAPI+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	return http.DefaultClient.Do(req)
}

func SearchMods(query, loader, gameVersion string, limit int) (*SearchResponse, error) {
	if limit <= 0 {
		limit = 20
	}

	// Build facets
	facets := []string{}
	facets = append(facets, `["project_type:mod","project_type:plugin"]`)
	if loader != "" {
		facets = append(facets, fmt.Sprintf(`["categories:%s"]`, loader))
	}
	if gameVersion != "" {
		facets = append(facets, fmt.Sprintf(`["versions:%s"]`, gameVersion))
	}

	facetStr := "[" + joinStrings(facets, ",") + "]"
	path := fmt.Sprintf("/search?query=%s&facets=%s&limit=%d",
		url.QueryEscape(query), url.QueryEscape(facetStr), limit)

	resp, err := modrinthGet(path)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}
	defer resp.Body.Close()

	var result SearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to parse search results: %w", err)
	}
	return &result, nil
}

func GetModVersions(projectID, loader, gameVersion string) ([]ModVersion, error) {
	params := url.Values{}
	if loader != "" {
		params.Set("loaders", fmt.Sprintf(`["%s"]`, loader))
	}
	if gameVersion != "" {
		params.Set("game_versions", fmt.Sprintf(`["%s"]`, gameVersion))
	}

	path := fmt.Sprintf("/project/%s/version?%s", url.PathEscape(projectID), params.Encode())
	resp, err := modrinthGet(path)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var versions []ModVersion
	if err := json.NewDecoder(resp.Body).Decode(&versions); err != nil {
		return nil, err
	}
	return versions, nil
}

func DownloadMod(downloadURL, destDir, filename string) error {
	destPath := filepath.Join(destDir, filename)

	req, err := http.NewRequest("GET", downloadURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("download failed: %s", resp.Status)
	}

	file, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(file, resp.Body)
	return err
}

func joinStrings(strs []string, sep string) string {
	result := ""
	for i, s := range strs {
		if i > 0 {
			result += sep
		}
		result += s
	}
	return result
}
