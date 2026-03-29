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
	userHandlers := NewUserHandlers(authMiddleware)

	// Self-service
	protected.GET("/me", userHandlers.GetMe)
	protected.PUT("/me/password", userHandlers.ChangePassword)

	// User management (admin only)
	admin := protected.Group("")
	admin.Use(authMiddleware.RequireRole("admin"))
	admin.GET("/users", userHandlers.ListUsers)
	admin.POST("/users", userHandlers.CreateUser)
	admin.PUT("/users/:id", userHandlers.UpdateUser)
	admin.DELETE("/users/:id", userHandlers.DeleteUser)

	// Server endpoints
	protected.GET("/servers", handlers.ListServers)
	protected.POST("/servers", handlers.CreateServer)
	protected.GET("/servers/:id", handlers.GetServer)
	protected.POST("/servers/:id/start", handlers.StartServer)
	protected.POST("/servers/:id/stop", handlers.StopServer)
	protected.POST("/servers/:id/reset", handlers.ResetServer)
	protected.DELETE("/servers/:id/wipeout", handlers.WipeoutServer)
	protected.POST("/servers/:id/command", handlers.SendCommand)

	// Server upgrade, limits, schedule, and files
	protected.POST("/servers/:id/upgrade", handlers.UpgradeServer)
	protected.PUT("/servers/:id/limits", handlers.SetLimits)
	protected.PUT("/servers/:id/schedule", handlers.SetSchedule)
	protected.GET("/servers/:id/files", handlers.ListFiles)
	protected.GET("/servers/:id/files/*path", handlers.ReadFile)
	protected.PUT("/servers/:id/files/*path", handlers.WriteFile)

	// Backups
	protected.GET("/servers/:id/backups", handlers.ListBackups)
	protected.POST("/servers/:id/backups", handlers.CreateBackup)
	protected.POST("/servers/:id/backups/:bid/restore", handlers.RestoreBackup)
	protected.DELETE("/servers/:id/backups/:bid", handlers.DeleteBackup)

	// Whitelist and ops
	protected.GET("/servers/:id/whitelist", handlers.GetWhitelist)
	protected.POST("/servers/:id/whitelist", handlers.AddToWhitelist)
	protected.DELETE("/servers/:id/whitelist/:uuid", handlers.RemoveFromWhitelist)
	protected.GET("/servers/:id/ops", handlers.GetOps)
	protected.POST("/servers/:id/ops", handlers.AddOp)
	protected.DELETE("/servers/:id/ops/:uuid", handlers.RemoveOp)

	// Metrics endpoints
	protected.GET("/servers/:id/metrics", handlers.GetServerMetrics)
	protected.GET("/servers/:id/metrics/history", handlers.GetServerMetricsHistory)

	// Version and flavor endpoints
	protected.GET("/versions", handlers.GetVersions)
	protected.GET("/flavors", handlers.GetFlavors)

	// WebSocket endpoint
	protected.GET("/ws/:id", handlers.HandleWebSocket)
}
