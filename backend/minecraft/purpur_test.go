package minecraft

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func newPurpurTestServer(t *testing.T) *PurpurProvider {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/":
			w.Write([]byte(`{"project":"purpur","metadata":{"current":"26.3"},"versions":["1.21.11","26.1.2","26.2","26.3"]}`))
		case "/26.3/latest/download":
			w.Write([]byte("jar-bytes"))
		default:
			http.Error(w, "nope", http.StatusNotFound)
		}
	}))

	original := purpurAPIBase
	purpurAPIBase = srv.URL
	t.Cleanup(func() {
		purpurAPIBase = original
		srv.Close()
	})

	return &PurpurProvider{}
}

func TestPurpurGetVersionsNewestFirst(t *testing.T) {
	p := newPurpurTestServer(t)

	versions, err := p.GetVersions(false)
	if err != nil {
		t.Fatalf("GetVersions: %v", err)
	}
	if len(versions) != 4 {
		t.Fatalf("GetVersions returned %d versions, want 4", len(versions))
	}
	if versions[0].ID != "26.3" {
		t.Errorf("newest version is %q, want 26.3", versions[0].ID)
	}
}

func TestPurpurStartCommandPicksJavaByVersion(t *testing.T) {
	p := &PurpurProvider{}

	root := t.TempDir()
	java21 := fakeJavaHome(t, root, "21")
	java25 := fakeJavaHome(t, root, "25")
	original := javaSearchDirs
	javaSearchDirs = []string{root}
	t.Cleanup(func() { javaSearchDirs = original })

	if cmd, _ := p.StartCommand(t.TempDir(), 512, "26.2"); cmd != java25 {
		t.Errorf("purpur 26.2 uses %q, want %q", cmd, java25)
	}
	if cmd, _ := p.StartCommand(t.TempDir(), 512, "1.21.8"); cmd != java21 {
		t.Errorf("purpur 1.21.8 uses %q, want %q", cmd, java21)
	}
}

func TestPurpurDownloadServer(t *testing.T) {
	p := newPurpurTestServer(t)

	destDir := t.TempDir()
	if err := p.DownloadServer(destDir, "26.3"); err != nil {
		t.Fatalf("DownloadServer: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(destDir, "server.jar"))
	if err != nil {
		t.Fatalf("server.jar not written: %v", err)
	}
	if string(data) != "jar-bytes" {
		t.Errorf("server.jar contains %q, want jar-bytes", data)
	}

	// A download failure must not leave a truncated jar behind.
	other := t.TempDir()
	if err := p.DownloadServer(other, "9.9.9"); err == nil {
		t.Error("expected an error for an unknown version")
	}
}
