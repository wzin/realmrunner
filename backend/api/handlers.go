package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/wzin/realmrunner/config"
	"github.com/wzin/realmrunner/minecraft"
	"github.com/wzin/realmrunner/server"
	"github.com/wzin/realmrunner/websocket"
)

type Handlers struct {
	manager        *server.Manager
	hub            *websocket.Hub
	config         *config.Config
	versionFetcher *minecraft.VersionFetcher
}

func NewHandlers(manager *server.Manager, hub *websocket.Hub, cfg *config.Config) *Handlers {
	return &Handlers{
		manager:        manager,
		hub:            hub,
		config:         cfg,
		versionFetcher: minecraft.NewVersionFetcher(),
	}
}

type CreateServerRequest struct {
	Name    string `json:"name" binding:"required"`
	Version string `json:"version" binding:"required"`
	Port    int    `json:"port" binding:"required"`
}

type CommandRequest struct {
	Command string `json:"command" binding:"required"`
}

func (h *Handlers) ListServers(c *gin.Context) {
	servers, err := h.manager.GetAllServers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, servers)
}

func (h *Handlers) CreateServer(c *gin.Context) {
	var req CreateServerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	// Create server record
	srv, err := h.manager.CreateServer(req.Name, req.Version, req.Port)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Download and setup server in background
	go func() {
		if err := minecraft.DownloadServer(h.config.DataDir, srv.ID, req.Version); err != nil {
			// Log error but don't fail the request
			// The user will see the error when they try to start the server
			return
		}
	}()

	c.JSON(http.StatusCreated, srv)
}

func (h *Handlers) GetServer(c *gin.Context) {
	id := c.Param("id")
	srv, err := h.manager.GetServer(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}
	c.JSON(http.StatusOK, srv)
}

func (h *Handlers) StartServer(c *gin.Context) {
	id := c.Param("id")
	if err := h.manager.StartServer(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "server started"})
}

func (h *Handlers) StopServer(c *gin.Context) {
	id := c.Param("id")
	if err := h.manager.StopServer(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "server stopped"})
}

func (h *Handlers) ResetServer(c *gin.Context) {
	id := c.Param("id")
	if err := h.manager.ResetServer(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "server reset"})
}

func (h *Handlers) WipeoutServer(c *gin.Context) {
	id := c.Param("id")
	if err := h.manager.WipeoutServer(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "server wiped out"})
}

func (h *Handlers) SendCommand(c *gin.Context) {
	id := c.Param("id")
	var req CommandRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if err := h.manager.SendCommand(id, req.Command); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "command sent"})
}

func (h *Handlers) GetVersions(c *gin.Context) {
	versions, err := h.versionFetcher.GetVersions()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"versions": versions})
}

func (h *Handlers) HandleWebSocket(c *gin.Context) {
	id := c.Param("id")

	// Verify server exists
	_, err := h.manager.GetServer(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}

	websocket.HandleConnection(c.Writer, c.Request, h.hub, h.manager, id)
}
