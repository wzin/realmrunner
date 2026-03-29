package main

import (
	"log"
	"os"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/wzin/realmrunner/api"
	"github.com/wzin/realmrunner/auth"
	"github.com/wzin/realmrunner/config"
	"github.com/wzin/realmrunner/backup"
	"github.com/wzin/realmrunner/cgroup"
	"github.com/wzin/realmrunner/metrics"
	"github.com/wzin/realmrunner/mods"
	"github.com/wzin/realmrunner/minecraft"
	"github.com/wzin/realmrunner/scheduler"
	"github.com/wzin/realmrunner/server"
	"github.com/wzin/realmrunner/websocket"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Initialize database
	db, err := server.InitDB(cfg.DataDir)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Initialize metrics table
	if err := metrics.InitMetricsTable(db); err != nil {
		log.Fatalf("Failed to initialize metrics table: %v", err)
	}

	// Initialize backup table
	if err := backup.InitBackupTable(db); err != nil {
		log.Fatalf("Failed to initialize backup table: %v", err)
	}

	// Initialize WebSocket hub (needed by collector)
	hub := websocket.NewHub()
	go hub.Run()

	// Initialize mods table
	if err := mods.InitModsTable(db); err != nil {
		log.Fatalf("Failed to initialize mods table: %v", err)
	}

	// Initialize metrics collector (with hub for live broadcasting)
	collector := metrics.NewCollector(db, hub)

	// Initialize provider registry
	registry := minecraft.NewRegistry()

	// Initialize cgroup manager (graceful fallback if unavailable)
	cgroupMgr := cgroup.NewManager()

	// Initialize server manager
	manager := server.NewManager(db, cfg, collector, registry, cgroupMgr)

	// Initialize scheduler
	sched := scheduler.NewScheduler(db, manager)
	sched.Start()

	// Set up Gin router
	if os.Getenv("GIN_MODE") != "debug" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.Default()

	// CORS middleware
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
	}))

	// Auth middleware
	authMiddleware := auth.NewMiddleware(cfg, db)

	// Register API routes
	api.RegisterRoutes(router, authMiddleware, manager, hub, cfg)

	// Serve static files (frontend)
	router.Static("/assets", "./dist/assets")
	router.StaticFile("/", "./dist/index.html")
	router.NoRoute(func(c *gin.Context) {
		c.File("./dist/index.html")
	})

	// Start server
	log.Println("RealmRunner starting on :8080")
	if err := router.Run(":8080"); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
