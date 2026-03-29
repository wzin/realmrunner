package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/wzin/realmrunner/backup"
)

func (h *Handlers) ListBackups(c *gin.Context) {
	id := c.Param("id")
	if _, err := h.manager.GetServer(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}

	backups, err := backup.ListBackups(h.manager.GetDB(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if backups == nil {
		backups = []backup.Backup{}
	}
	c.JSON(http.StatusOK, gin.H{"backups": backups})
}

func (h *Handlers) CreateBackup(c *gin.Context) {
	id := c.Param("id")
	if _, err := h.manager.GetServer(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}

	b, err := backup.CreateBackup(h.manager.GetDB(), h.config.DataDir, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, b)
}

func (h *Handlers) RestoreBackup(c *gin.Context) {
	id := c.Param("id")
	bid := c.Param("bid")

	srv, err := h.manager.GetServer(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}
	if srv.Status != "stopped" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "server must be stopped to restore"})
		return
	}

	if err := backup.RestoreBackup(h.manager.GetDB(), h.config.DataDir, id, bid); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "backup restored"})
}

func (h *Handlers) DeleteBackup(c *gin.Context) {
	id := c.Param("id")
	bid := c.Param("bid")

	if err := backup.DeleteBackup(h.manager.GetDB(), h.config.DataDir, id, bid); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "backup deleted"})
}
