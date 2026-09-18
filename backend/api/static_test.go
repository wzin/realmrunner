package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/wzin/realmrunner/version"
)

func newStaticRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	dist := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dist, "assets"), 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dist, "index.html"), []byte("<!DOCTYPE html><div id=app></div>"), 0644)
	os.WriteFile(filepath.Join(dist, "assets", "index-abc123.js"), []byte("console.log(1)"), 0644)

	router := gin.New()
	router.GET("/api/version", GetVersion)
	RegisterStatic(router, dist)
	return router
}

func get(t *testing.T, router *gin.Engine, path string) *httptest.ResponseRecorder {
	t.Helper()

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
	return w
}

// index.html names the hashed asset files. If a browser caches it, the next
// deployment leaves that visitor asking for asset files that no longer exist,
// and they get a blank page.
func TestIndexIsNeverCached(t *testing.T) {
	router := newStaticRouter(t)

	for _, path := range []string{"/", "/dashboard", "/share/e8b307f0c4eb49140b4b90e08b583791"} {
		w := get(t, router, path)
		if w.Code != http.StatusOK {
			t.Fatalf("%s returned %d", path, w.Code)
		}
		cacheControl := w.Header().Get("Cache-Control")
		if !strings.Contains(cacheControl, "no-cache") || !strings.Contains(cacheControl, "no-store") {
			t.Errorf("%s served with Cache-Control %q", path, cacheControl)
		}
		if !strings.Contains(w.Body.String(), "id=app") {
			t.Errorf("%s did not return the app shell", path)
		}
	}
}

// Hashed assets are safe to cache forever, and doing so is what makes the
// no-cache rule on index.html cheap.
func TestHashedAssetsAreCachedForever(t *testing.T) {
	router := newStaticRouter(t)

	w := get(t, router, "/assets/index-abc123.js")
	if w.Code != http.StatusOK {
		t.Fatalf("asset returned %d", w.Code)
	}

	cacheControl := w.Header().Get("Cache-Control")
	if !strings.Contains(cacheControl, "immutable") || !strings.Contains(cacheControl, "max-age=31536000") {
		t.Errorf("asset served with Cache-Control %q", cacheControl)
	}
}

// An unknown API path must not be answered with the app shell: an API caller
// would see HTML and a 200 instead of the error.
func TestUnknownAPIPathReturns404(t *testing.T) {
	router := newStaticRouter(t)

	w := get(t, router, "/api/does-not-exist")
	if w.Code != http.StatusNotFound {
		t.Errorf("got %d, want 404", w.Code)
	}
	if strings.Contains(w.Body.String(), "id=app") {
		t.Error("an API path was answered with the app shell")
	}
}

func TestVersionEndpoint(t *testing.T) {
	router := newStaticRouter(t)

	original := version.Version
	version.Version = "v2.3.0"
	t.Cleanup(func() { version.Version = original })

	w := get(t, router, "/api/version")
	if w.Code != http.StatusOK {
		t.Fatalf("got %d", w.Code)
	}

	var info version.Info
	if err := json.Unmarshal(w.Body.Bytes(), &info); err != nil {
		t.Fatal(err)
	}
	if info.Version != "v2.3.0" {
		t.Errorf("version = %q", info.Version)
	}
}
