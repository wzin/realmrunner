package minecraft

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// newMojangTestServer serves a miniature version manifest plus version detail
// documents, mirroring the shape of the real Mojang API.
func newMojangTestServer(t *testing.T) *VersionFetcher {
	t.Helper()

	var base string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/manifest":
			w.Write([]byte(`{"latest":{"release":"26.3","snapshot":"26.3"},"versions":[
			  {"id":"26.3","type":"release","url":"` + base + `/v/26.3"},
			  {"id":"26.3-rc-1","type":"snapshot","url":"` + base + `/v/26.3-rc-1"},
			  {"id":"1.21.11","type":"release","url":"` + base + `/v/1.21.11"}
			]}`))
		case "/v/26.3":
			w.Write([]byte(`{"downloads":{"server":{"url":"` + base + `/jar"}},"javaVersion":{"component":"java-runtime-epsilon","majorVersion":25}}`))
		case "/v/1.21.11":
			w.Write([]byte(`{"downloads":{"server":{"url":"` + base + `/jar"}},"javaVersion":{"component":"java-runtime-delta","majorVersion":21}}`))
		case "/v/26.3-rc-1":
			w.Write([]byte(`{"downloads":{},"javaVersion":{"majorVersion":25}}`))
		case "/jar":
			w.Write([]byte("jar-bytes"))
		default:
			http.Error(w, "nope", http.StatusNotFound)
		}
	}))
	base = srv.URL

	original := versionManifestURL
	versionManifestURL = srv.URL + "/manifest"
	t.Cleanup(func() {
		versionManifestURL = original
		srv.Close()
	})

	return NewVersionFetcher()
}

func TestVanillaGetVersions(t *testing.T) {
	p := &VanillaProvider{fetcher: newMojangTestServer(t)}

	releases, err := p.GetVersions(false)
	if err != nil {
		t.Fatalf("GetVersions: %v", err)
	}
	if len(releases) != 2 {
		t.Fatalf("GetVersions(false) returned %d versions, want 2", len(releases))
	}
	if releases[0].ID != "26.3" {
		t.Errorf("newest release is %q, want 26.3", releases[0].ID)
	}

	all, err := p.GetVersions(true)
	if err != nil {
		t.Fatalf("GetVersions: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("GetVersions(true) returned %d versions, want 3", len(all))
	}
}

func TestVersionFetcherGetJavaMajor(t *testing.T) {
	vf := newMojangTestServer(t)

	if got, err := vf.GetJavaMajor("26.3"); err != nil || got != 25 {
		t.Errorf("GetJavaMajor(26.3) = %d, %v; want 25, nil", got, err)
	}
	if got, err := vf.GetJavaMajor("1.21.11"); err != nil || got != 21 {
		t.Errorf("GetJavaMajor(1.21.11) = %d, %v; want 21, nil", got, err)
	}
	if _, err := vf.GetJavaMajor("9.9.9"); err == nil {
		t.Error("expected an error for an unknown version")
	}
}

func TestVersionFetcherGetServerDownloadURL(t *testing.T) {
	vf := newMojangTestServer(t)

	url, err := vf.GetServerDownloadURL("26.3")
	if err != nil {
		t.Fatalf("GetServerDownloadURL: %v", err)
	}
	if url == "" {
		t.Error("empty download URL")
	}

	if _, err := vf.GetServerDownloadURL("26.3-rc-1"); err == nil {
		t.Error("expected an error when no server download is published")
	}
}

func TestVanillaStartCommandUsesManifestJavaVersion(t *testing.T) {
	p := &VanillaProvider{fetcher: newMojangTestServer(t)}

	root := t.TempDir()
	java21 := fakeJavaHome(t, root, "21")
	java25 := fakeJavaHome(t, root, "25")
	original := javaSearchDirs
	javaSearchDirs = []string{root}
	t.Cleanup(func() { javaSearchDirs = original })

	if cmd, _ := p.StartCommand(t.TempDir(), 1024, "26.3"); cmd != java25 {
		t.Errorf("vanilla 26.3 uses %q, want %q", cmd, java25)
	}
	if cmd, _ := p.StartCommand(t.TempDir(), 1024, "1.21.11"); cmd != java21 {
		t.Errorf("vanilla 1.21.11 uses %q, want %q", cmd, java21)
	}
	// Unknown to the manifest: fall back to the heuristic, not an error.
	if cmd, _ := p.StartCommand(t.TempDir(), 1024, "26.9"); cmd != java25 {
		t.Errorf("vanilla 26.9 uses %q, want %q", cmd, java25)
	}
}

func TestVanillaDownloadServer(t *testing.T) {
	p := &VanillaProvider{fetcher: newMojangTestServer(t)}

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
