package minecraft

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

type VanillaProvider struct {
	fetcher *VersionFetcher
}

func (p *VanillaProvider) Flavor() string {
	return "vanilla"
}

func (p *VanillaProvider) GetVersions(includeSnapshots bool) ([]VersionInfo, error) {
	manifest, err := p.fetcher.getManifest()
	if err != nil {
		return nil, err
	}

	var versions []VersionInfo
	for _, v := range manifest.Versions {
		if includeSnapshots || v.Type == "release" {
			versions = append(versions, VersionInfo{
				ID:   v.ID,
				Type: v.Type,
			})
		}
	}
	return versions, nil
}

func (p *VanillaProvider) DownloadServer(destDir string, version string) error {
	jarPath := filepath.Join(destDir, "server.jar")
	if _, err := os.Stat(jarPath); err == nil {
		return nil // Already downloaded
	}

	downloadURL, err := p.fetcher.GetServerDownloadURL(version)
	if err != nil {
		return fmt.Errorf("failed to get download URL: %w", err)
	}

	return downloadJar(downloadURL, jarPath, fmt.Sprintf("Vanilla %s", version))
}

func (p *VanillaProvider) StartCommand(serverDir string, memoryMB int) (string, []string) {
	return "java", []string{
		fmt.Sprintf("-Xmx%dM", memoryMB),
		fmt.Sprintf("-Xms%dM", memoryMB),
		"-jar", "server.jar", "nogui",
	}
}

// downloadJar is a shared helper for downloading JAR files
func downloadJar(url, destPath, label string) error {
	fmt.Printf("Downloading %s...\n", label)

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status: %s", resp.Status)
	}

	file, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	bytesWritten, err := io.Copy(file, resp.Body)
	if err != nil {
		os.Remove(destPath)
		return fmt.Errorf("failed to write file: %w", err)
	}

	fmt.Printf("Downloaded %s (%.2f MB)\n", label, float64(bytesWritten)/(1024*1024))
	return nil
}
