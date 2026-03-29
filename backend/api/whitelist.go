package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
)

type PlayerEntry struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
}

type mojangProfile struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func resolvePlayerUUID(name string) (string, error) {
	resp, err := http.Get(fmt.Sprintf("https://api.mojang.com/users/profiles/minecraft/%s", name))
	if err != nil {
		return "", fmt.Errorf("failed to resolve player: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("player '%s' not found", name)
	}

	var profile mojangProfile
	if err := json.NewDecoder(resp.Body).Decode(&profile); err != nil {
		return "", err
	}

	// Format UUID with dashes
	id := profile.ID
	if len(id) == 32 {
		id = id[:8] + "-" + id[8:12] + "-" + id[12:16] + "-" + id[16:20] + "-" + id[20:]
	}
	return id, nil
}

func readPlayerList(serverDir, filename string) ([]PlayerEntry, error) {
	path := filepath.Join(serverDir, filename)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []PlayerEntry{}, nil
		}
		return nil, err
	}

	var entries []PlayerEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		// Try to parse as whitelist format with extra fields
		var raw []map[string]interface{}
		if err2 := json.Unmarshal(data, &raw); err2 != nil {
			return nil, err
		}
		for _, r := range raw {
			entry := PlayerEntry{}
			if v, ok := r["uuid"].(string); ok {
				entry.UUID = v
			}
			if v, ok := r["name"].(string); ok {
				entry.Name = v
			}
			entries = append(entries, entry)
		}
	}
	return entries, nil
}

func writePlayerList(serverDir, filename string, entries []PlayerEntry) error {
	data, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(serverDir, filename), data, 0644)
}

// Whitelist handlers

func (h *Handlers) GetWhitelist(c *gin.Context) {
	id := c.Param("id")
	if _, err := h.manager.GetServer(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}

	entries, err := readPlayerList(h.manager.GetServerDir(id), "whitelist.json")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"players": entries})
}

func (h *Handlers) AddToWhitelist(c *gin.Context) {
	id := c.Param("id")
	srv, err := h.manager.GetServer(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}

	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	uuid, err := resolvePlayerUUID(req.Name)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	serverDir := h.manager.GetServerDir(id)
	entries, _ := readPlayerList(serverDir, "whitelist.json")

	// Check for duplicates
	for _, e := range entries {
		if e.UUID == uuid {
			c.JSON(http.StatusOK, gin.H{"message": "player already whitelisted", "players": entries})
			return
		}
	}

	entries = append(entries, PlayerEntry{UUID: uuid, Name: req.Name})
	if err := writePlayerList(serverDir, "whitelist.json", entries); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// If running, send reload command
	if srv.Status == "running" {
		h.manager.SendCommand(id, "whitelist reload")
	}

	c.JSON(http.StatusOK, gin.H{"players": entries})
}

func (h *Handlers) RemoveFromWhitelist(c *gin.Context) {
	id := c.Param("id")
	uuid := c.Param("uuid")
	srv, err := h.manager.GetServer(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}

	serverDir := h.manager.GetServerDir(id)
	entries, _ := readPlayerList(serverDir, "whitelist.json")

	var playerName string
	filtered := make([]PlayerEntry, 0)
	for _, e := range entries {
		if e.UUID != uuid {
			filtered = append(filtered, e)
		} else {
			playerName = e.Name
		}
	}

	if err := writePlayerList(serverDir, "whitelist.json", filtered); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if srv.Status == "running" && playerName != "" {
		h.manager.SendCommand(id, "whitelist remove "+playerName)
	}

	c.JSON(http.StatusOK, gin.H{"players": filtered})
}

// Ops handlers

func (h *Handlers) GetOps(c *gin.Context) {
	id := c.Param("id")
	if _, err := h.manager.GetServer(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}

	entries, err := readPlayerList(h.manager.GetServerDir(id), "ops.json")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"players": entries})
}

func (h *Handlers) AddOp(c *gin.Context) {
	id := c.Param("id")
	srv, err := h.manager.GetServer(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}

	var req struct {
		Name string `json:"name" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	// If running, just send the op command
	if srv.Status == "running" {
		h.manager.SendCommand(id, "op "+req.Name)
		// Give the server a moment, then read back the ops list
		c.JSON(http.StatusOK, gin.H{"message": "op command sent"})
		return
	}

	uuid, err := resolvePlayerUUID(req.Name)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	serverDir := h.manager.GetServerDir(id)
	entries, _ := readPlayerList(serverDir, "ops.json")

	for _, e := range entries {
		if e.UUID == uuid {
			c.JSON(http.StatusOK, gin.H{"message": "player already op", "players": entries})
			return
		}
	}

	entries = append(entries, PlayerEntry{UUID: uuid, Name: req.Name})
	writePlayerList(serverDir, "ops.json", entries)
	c.JSON(http.StatusOK, gin.H{"players": entries})
}

func (h *Handlers) RemoveOp(c *gin.Context) {
	id := c.Param("id")
	uuid := c.Param("uuid")
	srv, err := h.manager.GetServer(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}

	serverDir := h.manager.GetServerDir(id)
	entries, _ := readPlayerList(serverDir, "ops.json")

	var playerName string
	filtered := make([]PlayerEntry, 0)
	for _, e := range entries {
		if e.UUID != uuid {
			filtered = append(filtered, e)
		} else {
			playerName = e.Name
		}
	}

	writePlayerList(serverDir, "ops.json", filtered)

	if srv.Status == "running" && playerName != "" {
		h.manager.SendCommand(id, "deop "+playerName)
	}

	c.JSON(http.StatusOK, gin.H{"players": filtered})
}

