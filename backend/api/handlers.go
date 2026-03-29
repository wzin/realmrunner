package api

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/wzin/realmrunner/config"
	"github.com/wzin/realmrunner/metrics"
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

type LatestMetricsResponse struct {
	CPUPercent  float64  `json:"cpu_percent"`
	MemoryMB    float64  `json:"memory_mb"`
	PlayerCount int      `json:"player_count"`
	PlayerNames []string `json:"player_names"`
}

type ServerResponse struct {
	*server.Server
	ConnectionURL string                 `json:"connection_url"`
	Metrics       *LatestMetricsResponse `json:"metrics,omitempty"`
}

func (h *Handlers) makeServerResponse(srv *server.Server) *ServerResponse {
	resp := &ServerResponse{
		Server:        srv,
		ConnectionURL: fmt.Sprintf("%s:%d", h.config.BaseURL, srv.Port),
	}

	// Include latest metrics if server is running
	if srv.Status == server.StatusRunning {
		collector := h.manager.GetCollector()
		if collector != nil {
			if latest := collector.GetLatest(srv.ID); latest != nil {
				resp.Metrics = &LatestMetricsResponse{
					CPUPercent:  latest.CPUPercent,
					MemoryMB:    latest.MemoryMB,
					PlayerCount: latest.PlayerCount,
					PlayerNames: latest.PlayerNames,
				}
			}
		}
	}

	return resp
}

func (h *Handlers) makeServerResponses(servers []*server.Server) []*ServerResponse {
	responses := make([]*ServerResponse, len(servers))
	for i, srv := range servers {
		responses[i] = h.makeServerResponse(srv)
	}
	return responses
}

func (h *Handlers) ListServers(c *gin.Context) {
	servers, err := h.manager.GetAllServers()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, h.makeServerResponses(servers))
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

	c.JSON(http.StatusCreated, h.makeServerResponse(srv))
}

func (h *Handlers) GetServer(c *gin.Context) {
	id := c.Param("id")
	srv, err := h.manager.GetServer(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}
	c.JSON(http.StatusOK, h.makeServerResponse(srv))
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

// Metrics endpoints

func (h *Handlers) GetServerMetrics(c *gin.Context) {
	id := c.Param("id")

	// Verify server exists
	_, err := h.manager.GetServer(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}

	// Try in-memory cache first
	collector := h.manager.GetCollector()
	if collector != nil {
		if latest := collector.GetLatest(id); latest != nil {
			c.JSON(http.StatusOK, latest)
			return
		}
	}

	// Fall back to DB
	db := h.manager.GetDB()
	metric, err := metrics.GetLatestMetric(db, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if metric == nil {
		c.JSON(http.StatusOK, gin.H{"cpu_percent": 0, "memory_mb": 0, "player_count": 0, "player_names": []string{}})
		return
	}
	c.JSON(http.StatusOK, metric)
}

func (h *Handlers) GetServerMetricsHistory(c *gin.Context) {
	id := c.Param("id")
	rangeStr := c.DefaultQuery("range", "24h")

	// Verify server exists
	_, err := h.manager.GetServer(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}

	db := h.manager.GetDB()
	points, err := metrics.GetMetricsHistory(db, id, rangeStr)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if points == nil {
		points = []metrics.MetricPoint{}
	}
	c.JSON(http.StatusOK, gin.H{"points": points})
}
