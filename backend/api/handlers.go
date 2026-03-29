package api

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/wzin/realmrunner/config"
	"github.com/wzin/realmrunner/metrics"
	"github.com/wzin/realmrunner/minecraft"
	"github.com/wzin/realmrunner/server"
	"github.com/wzin/realmrunner/websocket"
)

type Handlers struct {
	manager  *server.Manager
	hub      *websocket.Hub
	config   *config.Config
	registry *minecraft.Registry
}

func NewHandlers(manager *server.Manager, hub *websocket.Hub, cfg *config.Config) *Handlers {
	return &Handlers{
		manager:  manager,
		hub:      hub,
		config:   cfg,
		registry: manager.GetRegistry(),
	}
}

type CreateServerRequest struct {
	Name    string `json:"name" binding:"required"`
	Version string `json:"version" binding:"required"`
	Flavor  string `json:"flavor"`
	Port    int    `json:"port" binding:"required"`
}

type UpgradeServerRequest struct {
	Version string `json:"version" binding:"required"`
	Flavor  string `json:"flavor"`
}

type SetLimitsRequest struct {
	CPULimit      float64 `json:"cpu_limit"`
	MemoryLimitMB int     `json:"memory_limit_mb"`
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

	flavor := req.Flavor
	if flavor == "" {
		flavor = "vanilla"
	}

	// Create server record
	srv, err := h.manager.CreateServer(req.Name, req.Version, flavor, req.Port)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Download and setup server in background
	go func() {
		provider, ok := h.registry.GetProvider(flavor)
		if !ok {
			return
		}
		serverDir := h.manager.GetServerDir(srv.ID)
		if err := provider.DownloadServer(serverDir, req.Version); err != nil {
			log.Printf("Failed to download server %s: %v", srv.ID, err)
			return
		}
		// Mark server as ready
		server.SetServerReady(h.manager.GetDB(), srv.ID, true)
		log.Printf("Server %s is ready (download complete)", srv.ID)
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

func (h *Handlers) ForceStopServer(c *gin.Context) {
	id := c.Param("id")
	if err := h.manager.ForceStopServer(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "server force stopped"})
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
	flavor := c.DefaultQuery("flavor", "vanilla")
	includeSnapshots := c.Query("include_snapshots") == "true"

	provider, ok := h.registry.GetProvider(flavor)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown flavor: " + flavor})
		return
	}

	versions, err := provider.GetVersions(includeSnapshots)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// For backward compatibility, also return flat string list
	ids := make([]string, len(versions))
	for i, v := range versions {
		ids[i] = v.ID
	}

	c.JSON(http.StatusOK, gin.H{"versions": ids, "version_details": versions})
}

func (h *Handlers) GetFlavors(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"flavors": h.registry.GetAllFlavors()})
}

func (h *Handlers) UpgradeServer(c *gin.Context) {
	id := c.Param("id")
	var req UpgradeServerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if err := h.manager.UpgradeServer(id, req.Version, req.Flavor); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	srv, _ := h.manager.GetServer(id)
	c.JSON(http.StatusOK, h.makeServerResponse(srv))
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

// File editor endpoints

func (h *Handlers) ListFiles(c *gin.Context) {
	id := c.Param("id")
	if _, err := h.manager.GetServer(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}

	serverDir := h.manager.GetServerDir(id)
	files, err := listEditableFiles(serverDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if files == nil {
		files = []FileInfo{}
	}
	c.JSON(http.StatusOK, gin.H{"files": files})
}

func (h *Handlers) ReadFile(c *gin.Context) {
	id := c.Param("id")
	filePath := c.Param("path")
	if filePath != "" && filePath[0] == '/' {
		filePath = filePath[1:]
	}

	if _, err := h.manager.GetServer(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}

	serverDir := h.manager.GetServerDir(id)
	absPath, err := validateFilePath(serverDir, filePath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	data, err := os.ReadFile(absPath)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		return
	}

	if len(data) > maxFileSize {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file too large"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"content": string(data), "path": filePath})
}

func (h *Handlers) WriteFile(c *gin.Context) {
	id := c.Param("id")
	filePath := c.Param("path")
	if filePath != "" && filePath[0] == '/' {
		filePath = filePath[1:]
	}

	if _, err := h.manager.GetServer(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}

	var req struct {
		Content string `json:"content" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if len(req.Content) > maxFileSize {
		c.JSON(http.StatusBadRequest, gin.H{"error": "content too large"})
		return
	}

	serverDir := h.manager.GetServerDir(id)
	absPath, err := validateFilePath(serverDir, filePath)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := os.WriteFile(absPath, []byte(req.Content), 0644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to write file"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "file saved"})
}

// Schedule endpoint

func (h *Handlers) SetSchedule(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Schedule string `json:"schedule"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if _, err := h.manager.GetServer(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "server not found"})
		return
	}

	if err := h.manager.SetRestartSchedule(id, req.Schedule); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	srv, _ := h.manager.GetServer(id)
	c.JSON(http.StatusOK, h.makeServerResponse(srv))
}

// Limits endpoint

func (h *Handlers) SetLimits(c *gin.Context) {
	id := c.Param("id")
	var req SetLimitsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if err := h.manager.SetLimits(id, req.CPULimit, req.MemoryLimitMB); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	srv, _ := h.manager.GetServer(id)
	c.JSON(http.StatusOK, h.makeServerResponse(srv))
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
