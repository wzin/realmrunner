package api

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateFilePath(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "fileutil-test-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create test files
	os.WriteFile(filepath.Join(tmpDir, "server.properties"), []byte("test"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "server.jar"), []byte("jar"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("yaml"), 0644)

	tests := []struct {
		name      string
		path      string
		expectErr bool
	}{
		{"valid properties", "server.properties", false},
		{"valid yaml", "config.yml", false},
		{"blocked jar", "server.jar", true},
		{"path traversal", "../../../etc/passwd", true},
		{"absolute path", "/etc/passwd", true},
		{"dotdot in middle", "foo/../../etc/passwd", true},
		{"disallowed extension", "script.sh", true},
		{"disallowed extension exe", "malware.exe", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validateFilePath(tmpDir, tt.path)
			if tt.expectErr && err == nil {
				t.Errorf("Expected error for path %q, got nil", tt.path)
			}
			if !tt.expectErr && err != nil {
				t.Errorf("Unexpected error for path %q: %v", tt.path, err)
			}
		})
	}
}
