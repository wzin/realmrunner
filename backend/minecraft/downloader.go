package minecraft

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

func DownloadServer(dataDir, serverID, version string) error {
	fetcher := NewVersionFetcher()

	// Get download URL
	downloadURL, err := fetcher.GetServerDownloadURL(version)
	if err != nil {
		return fmt.Errorf("failed to get download URL: %w", err)
	}

	// Prepare destination path
	serverDir := filepath.Join(dataDir, "servers", serverID)
	jarPath := filepath.Join(serverDir, "server.jar")

	// Check if already exists
	if _, err := os.Stat(jarPath); err == nil {
		fmt.Printf("Server JAR for version %s already exists (cached), skipping download\n", version)
		return nil // Already downloaded
	}

	fmt.Printf("Downloading Minecraft server version %s...\n", version)

	// Download server.jar
	resp, err := http.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("failed to download server: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed with status: %s", resp.Status)
	}

	// Create destination file
	file, err := os.Create(jarPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	// Copy content
	bytesWritten, err := io.Copy(file, resp.Body)
	if err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	fmt.Printf("Successfully downloaded Minecraft server version %s (%.2f MB)\n", version, float64(bytesWritten)/(1024*1024))

	return nil
}
