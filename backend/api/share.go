package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/wzin/realmrunner/metrics"
	"github.com/wzin/realmrunner/server"
	"github.com/wzin/realmrunner/websocket"
)

func generateShareToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// GenerateShareLink creates a share token for a server (authenticated)
func (h *Handlers) GenerateShareLink(c *gin.Context) {
	id := c.Param("id")
	srv, err := h.manager.GetServer(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}

	token := generateShareToken()
	if err := server.SetShareToken(h.manager.GetDB(), id, token); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	shareURL := "/share/" + token
	c.JSON(http.StatusOK, gin.H{
		"token":     token,
		"share_url": shareURL,
		"server":    srv.Name,
	})
}

// RevokeShareLink removes the share token
func (h *Handlers) RevokeShareLink(c *gin.Context) {
	id := c.Param("id")
	if err := server.SetShareToken(h.manager.GetDB(), id, ""); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "share link revoked"})
}

// Public share endpoints (no auth required)

func (h *Handlers) GetSharedServer(c *gin.Context) {
	token := c.Param("token")
	srv, err := server.GetServerByShareToken(h.manager.GetDB(), token)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "invalid share link"})
		return
	}

	// Return limited info — no share token, no internal details
	resp := gin.H{
		"name":           srv.Name,
		"version":        srv.Version,
		"flavor":         srv.Flavor,
		"status":         srv.Status,
		"connection_url": srv.Port,
	}

	// Include metrics if running
	collector := h.manager.GetCollector()
	if collector != nil {
		if latest := collector.GetLatest(srv.ID); latest != nil {
			resp["metrics"] = latest
		}
	}

	c.JSON(http.StatusOK, resp)
}

func (h *Handlers) GetSharedMetricsHistory(c *gin.Context) {
	token := c.Param("token")
	rangeStr := c.DefaultQuery("range", "24h")

	srv, err := server.GetServerByShareToken(h.manager.GetDB(), token)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "invalid share link"})
		return
	}

	points, err := metrics.GetMetricsHistory(h.manager.GetDB(), srv.ID, rangeStr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if points == nil {
		points = []metrics.MetricPoint{}
	}
	c.JSON(http.StatusOK, gin.H{"points": points})
}

func (h *Handlers) HandleSharedWebSocket(c *gin.Context) {
	token := c.Param("token")
	srv, err := server.GetServerByShareToken(h.manager.GetDB(), token)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "invalid share link"})
		return
	}

	websocket.HandleConnection(c.Writer, c.Request, h.hub, h.manager, srv.ID)
}
