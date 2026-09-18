package api

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/wzin/realmrunner/version"
)

// RegisterStatic serves the built frontend.
//
// The caching rules matter for deployments: asset filenames carry a content
// hash, so they can be cached forever, while index.html names those assets and
// must never be cached. A stale index.html points at asset files that no longer
// exist in the new image, which leaves the visitor with a blank page until they
// clear their cache.
func RegisterStatic(router *gin.Engine, distDir string) {
	assets := router.Group("/assets", immutableCache())
	assets.Static("", filepath.Join(distDir, "assets"))

	indexPath := filepath.Join(distDir, "index.html")

	serveIndex := func(c *gin.Context) {
		noStoreHeaders(c)
		c.File(indexPath)
	}

	router.GET("/", serveIndex)
	router.NoRoute(func(c *gin.Context) {
		// Unknown API paths are a 404, not the app shell: returning HTML to an
		// API caller hides the real error.
		if strings.HasPrefix(c.Request.URL.Path, "/api/") {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		serveIndex(c)
	})
}

// immutableCache marks content-hashed files as cacheable for a year.
func immutableCache() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Cache-Control", "public, max-age=31536000, immutable")
		c.Next()
	}
}

// noStoreHeaders stops browsers and proxies from holding on to the app shell.
func noStoreHeaders(c *gin.Context) {
	c.Header("Cache-Control", "no-cache, no-store, must-revalidate")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")
}

// GetVersion reports the running build. It needs no authentication so the login
// page can show which version is deployed.
func GetVersion(c *gin.Context) {
	c.JSON(http.StatusOK, version.Current())
}
