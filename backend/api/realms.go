package api

import (
	"database/sql"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type Realm struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	MaxCPU      float64   `json:"max_cpu_cores"`
	MaxMemoryMB int       `json:"max_memory_mb"`
	MaxServers  int       `json:"max_servers"`
	CreatedAt   time.Time `json:"created_at"`
}

func InitRealmTables(db *sql.DB) {
	db.Exec(`CREATE TABLE IF NOT EXISTS realms (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL UNIQUE,
		max_cpu_cores REAL DEFAULT 0,
		max_memory_mb INTEGER DEFAULT 0,
		max_servers INTEGER DEFAULT 10,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	db.Exec(`CREATE TABLE IF NOT EXISTS realm_admins (
		realm_id TEXT NOT NULL,
		user_id TEXT NOT NULL,
		PRIMARY KEY (realm_id, user_id)
	)`)
	db.Exec(`CREATE TABLE IF NOT EXISTS server_viewers (
		server_id TEXT NOT NULL,
		user_id TEXT NOT NULL,
		PRIMARY KEY (server_id, user_id)
	)`)
	// Add realm_id to servers if not exists
	db.Exec("ALTER TABLE servers ADD COLUMN realm_id TEXT DEFAULT ''")
}

func MigrateToRealms(db *sql.DB) {
	// Check if default realm exists
	var count int
	db.QueryRow("SELECT COUNT(*) FROM realms").Scan(&count)
	if count > 0 {
		return
	}

	// Create default realm
	realmID := uuid.New().String()
	db.Exec("INSERT INTO realms (id, name, max_cpu_cores, max_memory_mb, max_servers) VALUES (?, 'Default', 0, 0, 100)", realmID)

	// Assign all existing servers to default realm
	db.Exec("UPDATE servers SET realm_id = ? WHERE realm_id = ''", realmID)

	// Promote first admin to owner
	db.Exec("UPDATE users SET role = 'owner' WHERE role = 'admin' AND id = (SELECT id FROM users WHERE role = 'admin' ORDER BY created_at LIMIT 1)")

	// Make remaining admins realm admins
	rows, _ := db.Query("SELECT id FROM users WHERE role = 'admin'")
	if rows != nil {
		defer rows.Close()
		for rows.Next() {
			var userID string
			rows.Scan(&userID)
			db.Exec("INSERT OR IGNORE INTO realm_admins (realm_id, user_id) VALUES (?, ?)", realmID, userID)
		}
	}
}

type RealmHandlers struct {
	db *sql.DB
}

func NewRealmHandlers(db *sql.DB) *RealmHandlers {
	return &RealmHandlers{db: db}
}

func (h *RealmHandlers) ListRealms(c *gin.Context) {
	role, _ := c.Get("role")
	userID, _ := c.Get("user_id")

	var rows *sql.Rows
	var err error

	if role == "owner" {
		rows, err = h.db.Query("SELECT id, name, max_cpu_cores, max_memory_mb, max_servers, created_at FROM realms ORDER BY name")
	} else if role == "admin" {
		rows, err = h.db.Query(`SELECT r.id, r.name, r.max_cpu_cores, r.max_memory_mb, r.max_servers, r.created_at
			FROM realms r JOIN realm_admins ra ON r.id = ra.realm_id WHERE ra.user_id = ? ORDER BY r.name`, userID)
	} else {
		c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var realms []Realm
	for rows.Next() {
		var r Realm
		rows.Scan(&r.ID, &r.Name, &r.MaxCPU, &r.MaxMemoryMB, &r.MaxServers, &r.CreatedAt)
		realms = append(realms, r)
	}
	if realms == nil {
		realms = []Realm{}
	}
	c.JSON(http.StatusOK, gin.H{"realms": realms})
}

func (h *RealmHandlers) CreateRealm(c *gin.Context) {
	var req struct {
		Name        string  `json:"name" binding:"required"`
		MaxCPU      float64 `json:"max_cpu_cores"`
		MaxMemoryMB int     `json:"max_memory_mb"`
		MaxServers  int     `json:"max_servers"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	if req.MaxServers == 0 {
		req.MaxServers = 10
	}

	id := uuid.New().String()
	_, err := h.db.Exec("INSERT INTO realms (id, name, max_cpu_cores, max_memory_mb, max_servers) VALUES (?, ?, ?, ?, ?)",
		id, req.Name, req.MaxCPU, req.MaxMemoryMB, req.MaxServers)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, Realm{ID: id, Name: req.Name, MaxCPU: req.MaxCPU, MaxMemoryMB: req.MaxMemoryMB, MaxServers: req.MaxServers})
}

func (h *RealmHandlers) UpdateRealm(c *gin.Context) {
	id := c.Param("id")
	var req struct {
		Name        string  `json:"name"`
		MaxCPU      float64 `json:"max_cpu_cores"`
		MaxMemoryMB int     `json:"max_memory_mb"`
		MaxServers  int     `json:"max_servers"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	_, err := h.db.Exec("UPDATE realms SET name=?, max_cpu_cores=?, max_memory_mb=?, max_servers=? WHERE id=?",
		req.Name, req.MaxCPU, req.MaxMemoryMB, req.MaxServers, id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "realm updated"})
}

func (h *RealmHandlers) DeleteRealm(c *gin.Context) {
	id := c.Param("id")
	// Check no servers in realm
	var count int
	h.db.QueryRow("SELECT COUNT(*) FROM servers WHERE realm_id = ?", id).Scan(&count)
	if count > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "realm has servers, move or delete them first"})
		return
	}

	h.db.Exec("DELETE FROM realm_admins WHERE realm_id = ?", id)
	h.db.Exec("DELETE FROM realms WHERE id = ?", id)
	c.JSON(http.StatusOK, gin.H{"message": "realm deleted"})
}

func (h *RealmHandlers) ListRealmAdmins(c *gin.Context) {
	realmID := c.Param("id")
	rows, err := h.db.Query(`SELECT u.id, u.username, u.role FROM users u
		JOIN realm_admins ra ON u.id = ra.user_id WHERE ra.realm_id = ?`, realmID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var admins []map[string]string
	for rows.Next() {
		var id, username, role string
		rows.Scan(&id, &username, &role)
		admins = append(admins, map[string]string{"id": id, "username": username, "role": role})
	}
	if admins == nil {
		admins = []map[string]string{}
	}
	c.JSON(http.StatusOK, gin.H{"admins": admins})
}

func (h *RealmHandlers) AddRealmAdmin(c *gin.Context) {
	realmID := c.Param("id")
	var req struct {
		UserID string `json:"user_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id required"})
		return
	}

	_, err := h.db.Exec("INSERT OR IGNORE INTO realm_admins (realm_id, user_id) VALUES (?, ?)", realmID, req.UserID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "admin added"})
}

func (h *RealmHandlers) RemoveRealmAdmin(c *gin.Context) {
	realmID := c.Param("id")
	userID := c.Param("uid")
	h.db.Exec("DELETE FROM realm_admins WHERE realm_id = ? AND user_id = ?", realmID, userID)
	c.JSON(http.StatusOK, gin.H{"message": "admin removed"})
}

func (h *RealmHandlers) ListServerViewers(c *gin.Context) {
	serverID := c.Param("id")
	rows, err := h.db.Query(`SELECT u.id, u.username FROM users u
		JOIN server_viewers sv ON u.id = sv.user_id WHERE sv.server_id = ?`, serverID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var viewers []map[string]string
	for rows.Next() {
		var id, username string
		rows.Scan(&id, &username)
		viewers = append(viewers, map[string]string{"id": id, "username": username})
	}
	if viewers == nil {
		viewers = []map[string]string{}
	}
	c.JSON(http.StatusOK, gin.H{"viewers": viewers})
}

func (h *RealmHandlers) AddServerViewer(c *gin.Context) {
	serverID := c.Param("id")
	var req struct {
		UserID string `json:"user_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id required"})
		return
	}

	h.db.Exec("INSERT OR IGNORE INTO server_viewers (server_id, user_id) VALUES (?, ?)", serverID, req.UserID)
	c.JSON(http.StatusOK, gin.H{"message": "viewer added"})
}

func (h *RealmHandlers) RemoveServerViewer(c *gin.Context) {
	serverID := c.Param("id")
	userID := c.Param("uid")
	h.db.Exec("DELETE FROM server_viewers WHERE server_id = ? AND user_id = ?", serverID, userID)
	c.JSON(http.StatusOK, gin.H{"message": "viewer removed"})
}
