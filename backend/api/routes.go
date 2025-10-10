package api

import (
	"github.com/gin-gonic/gin"
	"github.com/wzin/realmrunner/auth"
	"github.com/wzin/realmrunner/config"
	"github.com/wzin/realmrunner/server"
	"github.com/wzin/realmrunner/websocket"
)

func RegisterRoutes(
	router *gin.Engine,
	authMiddleware *auth.Middleware,
	manager *server.Manager,
	hub *websocket.Hub,
	cfg *config.Config,
) {
	api := router.Group("/api")

	// Auth endpoints (no auth required)
	api.POST("/auth/login", authMiddleware.Login)

	// Protected endpoints
	protected := api.Group("")
	protected.Use(authMiddleware.RequireAuth())

	handlers := NewHandlers(manager, hub, cfg)

	// Server endpoints
	protected.GET("/servers", handlers.ListServers)
	protected.POST("/servers", handlers.CreateServer)
	protected.GET("/servers/:id", handlers.GetServer)
	protected.POST("/servers/:id/start", handlers.StartServer)
	protected.POST("/servers/:id/stop", handlers.StopServer)
	protected.DELETE("/servers/:id/wipeout", handlers.WipeoutServer)
	protected.POST("/servers/:id/command", handlers.SendCommand)

	// Version endpoints
	protected.GET("/versions", handlers.GetVersions)

	// WebSocket endpoint
	protected.GET("/ws/:id", handlers.HandleWebSocket)
}
