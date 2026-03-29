package api

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var allowedExtensions = map[string]bool{
	".properties": true,
	".yml":        true,
	".yaml":       true,
	".json":       true,
	".toml":       true,
	".txt":        true,
	".cfg":        true,
	".conf":       true,
}

var blockedFiles = map[string]bool{
	"server.jar": true,
}

const maxFileSize = 1024 * 1024 // 1MB

func validateFilePath(serverDir, requestedPath string) (string, error) {
	cleaned := filepath.Clean(requestedPath)

	if filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("absolute paths not allowed")
	}

	if strings.Contains(cleaned, "..") {
		return "", fmt.Errorf("path traversal not allowed")
	}

	fullPath := filepath.Join(serverDir, cleaned)
	absPath, err := filepath.Abs(fullPath)
	if err != nil {
		return "", fmt.Errorf("invalid path")
	}

	absServerDir, err := filepath.Abs(serverDir)
	if err != nil {
		return "", fmt.Errorf("invalid server directory")
	}

	if !strings.HasPrefix(absPath, absServerDir+string(os.PathSeparator)) && absPath != absServerDir {
		return "", fmt.Errorf("path outside server directory")
	}

	// Check symlinks
	realPath, err := filepath.EvalSymlinks(filepath.Dir(fullPath))
	if err == nil {
		realServerDir, _ := filepath.EvalSymlinks(absServerDir)
		if !strings.HasPrefix(realPath, realServerDir) {
			return "", fmt.Errorf("symlink escape detected")
		}
	}

	base := filepath.Base(cleaned)
	if blockedFiles[base] {
		return "", fmt.Errorf("file %s cannot be edited", base)
	}

	ext := filepath.Ext(base)
	if !allowedExtensions[ext] {
		return "", fmt.Errorf("file type %s not allowed", ext)
	}

	return absPath, nil
}

type FileInfo struct {
	Name     string `json:"name"`
	Path     string `json:"path"`
	Size     int64  `json:"size"`
	Editable bool   `json:"editable"`
}

func listEditableFiles(serverDir string) ([]FileInfo, error) {
	var files []FileInfo

	err := filepath.Walk(serverDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip errors
		}

		// Skip directories but still walk into them (max 2 levels deep)
		rel, _ := filepath.Rel(serverDir, path)
		depth := strings.Count(rel, string(os.PathSeparator))
		if info.IsDir() {
			if depth > 2 {
				return filepath.SkipDir
			}
			return nil
		}

		// Skip large files
		if info.Size() > maxFileSize {
			return nil
		}

		ext := filepath.Ext(info.Name())
		editable := allowedExtensions[ext] && !blockedFiles[info.Name()]

		if editable {
			files = append(files, FileInfo{
				Name:     info.Name(),
				Path:     rel,
				Size:     info.Size(),
				Editable: true,
			})
		}

		return nil
	})

	return files, err
}
