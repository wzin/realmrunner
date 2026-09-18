package minecraft

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const paperVersionsJSON = `{"versions":[
  {"version":{"id":"26.3","support":{"status":"SUPPORTED"},"java":{"version":{"minimum":25}}},"builds":[17]},
  {"version":{"id":"26.3-rc-3","support":{"status":"SUPPORTED"},"java":{"version":{"minimum":25}}},"builds":[3]},
  {"version":{"id":"1.21.11","support":{"status":"SUPPORTED"},"java":{"version":{"minimum":21}}},"builds":[9]}
]}`

func newPaperTestServer(t *testing.T) *PaperProvider {
	t.Helper()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/versions":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(paperVersionsJSON))
		case r.URL.Path == "/versions/26.3/builds":
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`[{"id":17,"channel":"ALPHA","downloads":{"server:default":{"name":"paper-26.3-17.jar","url":"` + r.Host + `"}}}]`))
		case strings.HasSuffix(r.URL.Path, "/builds"):
			http.Error(w, "not found", http.StatusNotFound)
		default:
			http.Error(w, "unexpected path "+r.URL.Path, http.StatusNotFound)
		}
	}))

	original := paperAPIBase
	paperAPIBase = srv.URL
	t.Cleanup(func() {
		paperAPIBase = original
		srv.Close()
	})

	return &PaperProvider{}
}

func TestPaperGetVersions(t *testing.T) {
	p := newPaperTestServer(t)

	all, err := p.GetVersions(true)
	if err != nil {
		t.Fatalf("GetVersions: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("GetVersions(true) returned %d versions, want 3", len(all))
	}
	if all[0].ID != "26.3" {
		t.Errorf("newest version is %q, want 26.3", all[0].ID)
	}

	releases, err := p.GetVersions(false)
	if err != nil {
		t.Fatalf("GetVersions: %v", err)
	}
	if len(releases) != 2 {
		t.Fatalf("GetVersions(false) returned %d versions, want 2", len(releases))
	}
	for _, v := range releases {
		if v.ID == "26.3-rc-3" {
			t.Error("release candidate leaked into release list")
		}
	}
}

// A cached snapshot-filtered result must not be served to a later request that
// asked for snapshots (and vice versa).
func TestPaperGetVersionsCacheRespectsSnapshotFlag(t *testing.T) {
	p := newPaperTestServer(t)

	if _, err := p.GetVersions(false); err != nil {
		t.Fatalf("GetVersions: %v", err)
	}
	all, err := p.GetVersions(true)
	if err != nil {
		t.Fatalf("GetVersions: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("cached GetVersions(true) returned %d versions, want 3", len(all))
	}
}

func TestPaperStartCommandUsesReportedJavaVersion(t *testing.T) {
	p := newPaperTestServer(t)

	root := t.TempDir()
	java21 := fakeJavaHome(t, root, "21")
	java25 := fakeJavaHome(t, root, "25")
	originalDirs := javaSearchDirs
	javaSearchDirs = []string{root}
	t.Cleanup(func() { javaSearchDirs = originalDirs })

	// Cold provider: it should fetch the version metadata on demand.
	cmd, args := p.StartCommand(t.TempDir(), 1024, "26.3")
	if cmd != java25 {
		t.Errorf("Paper 26.3 uses %q, want %q", cmd, java25)
	}
	if args[0] != "-Xmx1024M" {
		t.Errorf("unexpected args %v", args)
	}

	if cmd, _ := p.StartCommand(t.TempDir(), 1024, "1.21.11"); cmd != java21 {
		t.Errorf("Paper 1.21.11 uses %q, want %q", cmd, java21)
	}
}

func TestPaperDownloadServer(t *testing.T) {
	var base string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/versions/26.3/builds":
			w.Write([]byte(`[{"id":18,"channel":"ALPHA","downloads":{"server:default":{"name":"paper-26.3-18.jar","url":"` + base + `/jar"}}}]`))
		case "/jar":
			w.Write([]byte("jar-bytes"))
		default:
			http.Error(w, "nope", http.StatusNotFound)
		}
	}))
	defer srv.Close()
	base = srv.URL

	original := paperAPIBase
	paperAPIBase = srv.URL
	t.Cleanup(func() { paperAPIBase = original })

	p := &PaperProvider{}
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
}

func TestPaperDownloadServerSkipsExistingJar(t *testing.T) {
	p := &PaperProvider{}
	destDir := t.TempDir()
	jar := filepath.Join(destDir, "server.jar")
	os.WriteFile(jar, []byte("existing"), 0644)

	original := paperAPIBase
	paperAPIBase = "http://127.0.0.1:1" // would fail if contacted
	t.Cleanup(func() { paperAPIBase = original })

	if err := p.DownloadServer(destDir, "26.3"); err != nil {
		t.Fatalf("DownloadServer: %v", err)
	}
	data, _ := os.ReadFile(jar)
	if string(data) != "existing" {
		t.Error("existing server.jar was overwritten")
	}
}

func TestPaperDownloadServerUnknownVersion(t *testing.T) {
	p := newPaperTestServer(t)
	if err := p.DownloadServer(t.TempDir(), "1.2.3"); err == nil {
		t.Error("expected an error for a version with no builds")
	}
}

// Stable builds must win over newer experimental ones, but a version that only
// has experimental builds (a freshly released Minecraft version) must still work.
func TestPaperLatestBuildPrefersStable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/versions/1.21.11/builds":
			w.Write([]byte(`[{"id":132,"channel":"ALPHA"},{"id":131,"channel":"STABLE"},{"id":130,"channel":"STABLE"}]`))
		case "/versions/26.3/builds":
			w.Write([]byte(`[{"id":17,"channel":"ALPHA"},{"id":16,"channel":"ALPHA"}]`))
		case "/versions/9.9.9/builds":
			w.Write([]byte(`[]`))
		default:
			http.Error(w, "nope", http.StatusNotFound)
		}
	}))
	defer srv.Close()

	original := paperAPIBase
	paperAPIBase = srv.URL
	t.Cleanup(func() { paperAPIBase = original })

	p := &PaperProvider{}

	build, err := p.latestBuild("1.21.11")
	if err != nil {
		t.Fatalf("latestBuild: %v", err)
	}
	if build.ID != 131 {
		t.Errorf("picked build %d (%s), want the newest STABLE build 131", build.ID, build.Channel)
	}

	build, err = p.latestBuild("26.3")
	if err != nil {
		t.Fatalf("latestBuild: %v", err)
	}
	if build.ID != 17 {
		t.Errorf("picked build %d, want 17 (only experimental builds exist)", build.ID)
	}

	if _, err := p.latestBuild("9.9.9"); err == nil {
		t.Error("expected an error when a version has no builds")
	}
}
