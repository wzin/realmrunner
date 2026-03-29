package api

import (
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/wzin/realmrunner/mods"
)

func getModDir(flavor string) string {
	switch flavor {
	case "paper", "purpur":
		return "plugins"
	case "fabric":
		return "mods"
	default:
		return ""
	}
}

func getLoader(flavor string) string {
	switch flavor {
	case "paper", "purpur":
		return "paper"
	default:
		return flavor
	}
}

func (h *Handlers) ListMods(c *gin.Context) {
	id := c.Param("id")
	srv, err := h.manager.GetServer(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}

	if getModDir(srv.Flavor) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mods not supported for vanilla servers"})
		return
	}

	installed, err := mods.ListInstalledMods(h.manager.GetDB(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if installed == nil {
		installed = []mods.InstalledMod{}
	}
	c.JSON(http.StatusOK, gin.H{"mods": installed})
}

func (h *Handlers) SearchMods(c *gin.Context) {
	id := c.Param("id")
	srv, err := h.manager.GetServer(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}

	if getModDir(srv.Flavor) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mods not supported for vanilla servers"})
		return
	}

	var req struct {
		Query string `json:"query" binding:"required"`
		Limit int    `json:"limit"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query is required"})
		return
	}

	loader := getLoader(srv.Flavor)
	results, err := mods.SearchMods(req.Query, loader, srv.Version, req.Limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, results)
}

func (h *Handlers) GetModVersions(c *gin.Context) {
	id := c.Param("id")
	projectID := c.Param("projectId")

	srv, err := h.manager.GetServer(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}

	loader := getLoader(srv.Flavor)
	versions, err := mods.GetModVersions(projectID, loader, srv.Version)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if versions == nil {
		versions = []mods.ModVersion{}
	}
	c.JSON(http.StatusOK, gin.H{"versions": versions})
}

func (h *Handlers) InstallMod(c *gin.Context) {
	id := c.Param("id")
	srv, err := h.manager.GetServer(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}

	modDir := getModDir(srv.Flavor)
	if modDir == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mods not supported for vanilla servers"})
		return
	}

	var req struct {
		ModrinthID string `json:"modrinth_id" binding:"required"`
		VersionID  string `json:"version_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "modrinth_id and version_id required"})
		return
	}

	// Get version details
	loader := getLoader(srv.Flavor)
	versions, err := mods.GetModVersions(req.ModrinthID, loader, srv.Version)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get mod versions"})
		return
	}

	// Find the requested version
	var targetVersion *mods.ModVersion
	for i := range versions {
		if versions[i].ID == req.VersionID {
			targetVersion = &versions[i]
			break
		}
	}
	if targetVersion == nil || len(targetVersion.Files) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "version not found"})
		return
	}

	// Get primary file
	var downloadURL, filename string
	for _, f := range targetVersion.Files {
		if f.Primary || downloadURL == "" {
			downloadURL = f.URL
			filename = f.Filename
		}
	}

	// Ensure mod directory exists
	serverDir := h.manager.GetServerDir(srv.ID)
	destDir := filepath.Join(serverDir, modDir)
	os.MkdirAll(destDir, 0755)

	// Download
	if err := mods.DownloadMod(downloadURL, destDir, filename); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to download mod: " + err.Error()})
		return
	}

	// Record in DB
	installed := &mods.InstalledMod{
		ID:          uuid.New().String(),
		ServerID:    id,
		ModrinthID:  req.ModrinthID,
		Name:        targetVersion.Name,
		Version:     targetVersion.VersionNumber,
		Filename:    filename,
		Loader:      loader,
		InstalledAt: time.Now(),
	}
	if err := mods.InsertInstalledMod(h.manager.GetDB(), installed); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record mod"})
		return
	}

	c.JSON(http.StatusCreated, installed)
}

func (h *Handlers) RemoveMod(c *gin.Context) {
	id := c.Param("id")
	modID := c.Param("modId")

	srv, err := h.manager.GetServer(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}

	modDir := getModDir(srv.Flavor)
	if modDir == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mods not supported"})
		return
	}

	installed, err := mods.GetInstalledMod(h.manager.GetDB(), modID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "mod not found"})
		return
	}

	// Delete file
	serverDir := h.manager.GetServerDir(id)
	filePath := filepath.Join(serverDir, modDir, installed.Filename)
	os.Remove(filePath)

	// Delete DB record
	mods.DeleteInstalledMod(h.manager.GetDB(), modID)

	c.JSON(http.StatusOK, gin.H{"message": "mod removed"})
}
