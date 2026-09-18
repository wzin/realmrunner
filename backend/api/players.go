package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type playerActionRequest struct {
	Player string `json:"player" binding:"required"`
	Reason string `json:"reason"`
}

type autoSleepRequest struct {
	Enabled        bool `json:"enabled"`
	IdleTimeoutMin int  `json:"idle_timeout_min"`
}

type autoRestartRequest struct {
	Enabled bool `json:"enabled"`
}

// ListPlayers reports who is online, asking the server over RCON.
func (h *Handlers) ListPlayers(c *gin.Context) {
	id := c.Param("id")

	players, err := h.manager.Players(id)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, players)
}

// KickPlayer disconnects a player.
func (h *Handlers) KickPlayer(c *gin.Context) {
	h.playerAction(c, func(id string, req playerActionRequest) (string, error) {
		return h.manager.KickPlayer(id, req.Player, req.Reason)
	})
}

// BanPlayer bans a player.
func (h *Handlers) BanPlayer(c *gin.Context) {
	h.playerAction(c, func(id string, req playerActionRequest) (string, error) {
		return h.manager.BanPlayer(id, req.Player, req.Reason)
	})
}

// PardonPlayer lifts a ban.
func (h *Handlers) PardonPlayer(c *gin.Context) {
	h.playerAction(c, func(id string, req playerActionRequest) (string, error) {
		return h.manager.PardonPlayer(id, req.Player)
	})
}

// OpPlayer grants operator status.
func (h *Handlers) OpPlayer(c *gin.Context) {
	h.playerAction(c, func(id string, req playerActionRequest) (string, error) {
		return h.manager.OpPlayer(id, req.Player)
	})
}

// DeopPlayer revokes operator status.
func (h *Handlers) DeopPlayer(c *gin.Context) {
	h.playerAction(c, func(id string, req playerActionRequest) (string, error) {
		return h.manager.DeopPlayer(id, req.Player)
	})
}

func (h *Handlers) playerAction(c *gin.Context, action func(string, playerActionRequest) (string, error)) {
	id := c.Param("id")

	var req playerActionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "a player name is required"})
		return
	}

	response, err := action(id, req)
	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": response})
}

// SetAutoSleep turns idle shutdown on or off for a server.
func (h *Handlers) SetAutoSleep(c *gin.Context) {
	id := c.Param("id")

	var req autoSleepRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if err := h.manager.SetAutoSleep(id, req.Enabled, req.IdleTimeoutMin); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	srv, err := h.manager.GetServer(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}
	c.JSON(http.StatusOK, h.makeServerResponse(srv))
}

// SetAutoRestart controls whether a crashed server restarts by itself.
func (h *Handlers) SetAutoRestart(c *gin.Context) {
	id := c.Param("id")

	var req autoRestartRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if err := h.manager.SetAutoRestart(id, req.Enabled); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	srv, err := h.manager.GetServer(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}
	c.JSON(http.StatusOK, h.makeServerResponse(srv))
}

// WakeServer starts a sleeping server from the UI.
func (h *Handlers) WakeServer(c *gin.Context) {
	id := c.Param("id")

	if err := h.manager.WakeServer(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "server waking up"})
}

// SleepServer stops a server while keeping its port held by the proxy.
func (h *Handlers) SleepServer(c *gin.Context) {
	id := c.Param("id")

	if err := h.manager.Sleep(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "server sleeping"})
}
