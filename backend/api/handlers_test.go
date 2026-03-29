package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"github.com/wzin/realmrunner/auth"
	"github.com/wzin/realmrunner/backup"
	"github.com/wzin/realmrunner/cgroup"
	"github.com/wzin/realmrunner/config"
	"github.com/wzin/realmrunner/metrics"
	"github.com/wzin/realmrunner/minecraft"
	"github.com/wzin/realmrunner/server"
	"github.com/wzin/realmrunner/websocket"
)

func setupTestEnv(t *testing.T) (*gin.Engine, string, func()) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	tmpDir, err := os.MkdirTemp("", "realmrunner-test-*")
	if err != nil {
		t.Fatal(err)
	}

	hash, err2 := bcrypt.GenerateFromPassword([]byte("test"), bcrypt.MinCost)
	if err2 != nil {
		t.Fatal(err2)
	}

	cfg := &config.Config{
		PasswordHash: string(hash),
		JWTSecret:    "test-secret",
		MaxRunning:   3,
		PortRange:    config.PortRange{Min: 25565, Max: 25600},
		MemoryMB:     1024,
		DataDir:      tmpDir,
		BaseURL:      "localhost",
	}

	db, err := server.InitDB(cfg.DataDir)
	if err != nil {
		t.Fatal(err)
	}

	metrics.InitMetricsTable(db)
	backup.InitBackupTable(db)

	collector := metrics.NewCollector(db, nil)
	registry := minecraft.NewRegistry()
	cgroupMgr := cgroup.NewManager()
	manager := server.NewManager(db, cfg, collector, registry, cgroupMgr)
	hub := websocket.NewHub()
	go hub.Run()

	authMiddleware := auth.NewMiddleware(cfg, db)

	router := gin.New()
	RegisterRoutes(router, authMiddleware, manager, hub, cfg)

	cleanup := func() {
		db.Close()
		os.RemoveAll(tmpDir)
	}

	return router, tmpDir, cleanup
}

func getToken(t *testing.T, router *gin.Engine) string {
	t.Helper()
	body := `{"username":"admin","password":"test"}`
	req := httptest.NewRequest("POST", "/api/auth/login", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Login failed: %d %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	return resp["token"].(string)
}

func authReq(method, path, token string, body interface{}) *http.Request {
	var buf *bytes.Buffer
	if body != nil {
		data, _ := json.Marshal(body)
		buf = bytes.NewBuffer(data)
	} else {
		buf = bytes.NewBuffer(nil)
	}
	req := httptest.NewRequest(method, path, buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	return req
}

func TestLoginFlow(t *testing.T) {
	router, _, cleanup := setupTestEnv(t)
	defer cleanup()

	// Test login with wrong password
	body := `{"username":"admin","password":"wrong"}`
	req := httptest.NewRequest("POST", "/api/auth/login", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401, got %d", w.Code)
	}

	// Test login with correct password
	token := getToken(t, router)
	if token == "" {
		t.Fatal("Token is empty")
	}

	// Test authenticated request
	req = authReq("GET", "/api/servers", token, nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}

	// Test unauthenticated request
	req = httptest.NewRequest("GET", "/api/servers", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("Expected 401, got %d", w.Code)
	}
}

func TestServerCRUD(t *testing.T) {
	router, tmpDir, cleanup := setupTestEnv(t)
	defer cleanup()

	token := getToken(t, router)

	// Create server
	createReq := map[string]interface{}{
		"name":    "Test Server",
		"version": "1.21.1",
		"port":    25565,
		"flavor":  "vanilla",
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, authReq("POST", "/api/servers", token, createReq))

	if w.Code != http.StatusCreated {
		t.Fatalf("Create server failed: %d %s", w.Code, w.Body.String())
	}

	var created map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &created)
	serverID := created["id"].(string)

	if created["name"] != "Test Server" {
		t.Errorf("Expected name 'Test Server', got %v", created["name"])
	}
	if created["flavor"] != "vanilla" {
		t.Errorf("Expected flavor 'vanilla', got %v", created["flavor"])
	}

	// Verify server directory was created
	serverDir := filepath.Join(tmpDir, "servers", serverID)
	if _, err := os.Stat(serverDir); os.IsNotExist(err) {
		t.Error("Server directory was not created")
	}

	// List servers
	w = httptest.NewRecorder()
	router.ServeHTTP(w, authReq("GET", "/api/servers", token, nil))

	if w.Code != http.StatusOK {
		t.Fatalf("List servers failed: %d", w.Code)
	}

	var servers []map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &servers)
	if len(servers) != 1 {
		t.Errorf("Expected 1 server, got %d", len(servers))
	}

	// Get server
	w = httptest.NewRecorder()
	router.ServeHTTP(w, authReq("GET", "/api/servers/"+serverID, token, nil))

	if w.Code != http.StatusOK {
		t.Errorf("Get server failed: %d", w.Code)
	}

	// Port conflict
	createReq["name"] = "Duplicate Port"
	w = httptest.NewRecorder()
	router.ServeHTTP(w, authReq("POST", "/api/servers", token, createReq))

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 for duplicate port, got %d", w.Code)
	}

	// Delete server
	w = httptest.NewRecorder()
	router.ServeHTTP(w, authReq("DELETE", "/api/servers/"+serverID+"/wipeout", token, nil))

	if w.Code != http.StatusOK {
		t.Fatalf("Delete server failed: %d %s", w.Code, w.Body.String())
	}

	// Verify deleted
	w = httptest.NewRecorder()
	router.ServeHTTP(w, authReq("GET", "/api/servers", token, nil))
	json.Unmarshal(w.Body.Bytes(), &servers)
	if len(servers) != 0 {
		t.Errorf("Expected 0 servers after delete, got %d", len(servers))
	}
}

func TestResourceLimits(t *testing.T) {
	router, _, cleanup := setupTestEnv(t)
	defer cleanup()

	token := getToken(t, router)

	// Create server
	createReq := map[string]interface{}{
		"name": "Limits Test", "version": "1.21.1", "port": 25565, "flavor": "vanilla",
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, authReq("POST", "/api/servers", token, createReq))
	var created map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &created)
	id := created["id"].(string)

	// Set limits
	limitsReq := map[string]interface{}{
		"cpu_limit":       1.5,
		"memory_limit_mb": 4096,
	}
	w = httptest.NewRecorder()
	router.ServeHTTP(w, authReq("PUT", "/api/servers/"+id+"/limits", token, limitsReq))

	if w.Code != http.StatusOK {
		t.Fatalf("Set limits failed: %d %s", w.Code, w.Body.String())
	}

	// Verify limits
	w = httptest.NewRecorder()
	router.ServeHTTP(w, authReq("GET", "/api/servers/"+id, token, nil))
	var srv map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &srv)

	if srv["cpu_limit"].(float64) != 1.5 {
		t.Errorf("Expected cpu_limit 1.5, got %v", srv["cpu_limit"])
	}
	if srv["memory_limit_mb"].(float64) != 4096 {
		t.Errorf("Expected memory_limit_mb 4096, got %v", srv["memory_limit_mb"])
	}
}

func TestVersionEndpoints(t *testing.T) {
	router, _, cleanup := setupTestEnv(t)
	defer cleanup()

	token := getToken(t, router)

	// Get flavors
	w := httptest.NewRecorder()
	router.ServeHTTP(w, authReq("GET", "/api/flavors", token, nil))

	if w.Code != http.StatusOK {
		t.Fatalf("Get flavors failed: %d", w.Code)
	}

	var flavorsResp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &flavorsResp)
	flavors := flavorsResp["flavors"].([]interface{})
	if len(flavors) < 3 {
		t.Errorf("Expected at least 3 flavors (vanilla, paper, purpur), got %d", len(flavors))
	}
}

func TestUserManagement(t *testing.T) {
	router, _, cleanup := setupTestEnv(t)
	defer cleanup()

	token := getToken(t, router)

	// Get current user
	w := httptest.NewRecorder()
	router.ServeHTTP(w, authReq("GET", "/api/me", token, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("Get me failed: %d", w.Code)
	}

	var me map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &me)
	if me["username"] != "admin" {
		t.Errorf("Expected username 'admin', got %v", me["username"])
	}
	if me["role"] != "owner" {
		t.Errorf("Expected role 'owner', got %v", me["role"])
	}

	// Create a new user
	createUserReq := map[string]interface{}{
		"username": "viewer1",
		"password": "viewerpass",
		"role":     "viewer",
	}
	w = httptest.NewRecorder()
	router.ServeHTTP(w, authReq("POST", "/api/users", token, createUserReq))
	if w.Code != http.StatusCreated {
		t.Fatalf("Create user failed: %d %s", w.Code, w.Body.String())
	}

	var newUser map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &newUser)
	newUserID := newUser["id"].(string)

	// List users
	w = httptest.NewRecorder()
	router.ServeHTTP(w, authReq("GET", "/api/users", token, nil))
	var usersResp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &usersResp)
	users := usersResp["users"].([]interface{})
	if len(users) != 2 {
		t.Errorf("Expected 2 users, got %d", len(users))
	}

	// Login as new user
	loginBody := `{"username":"viewer1","password":"viewerpass"}`
	req := httptest.NewRequest("POST", "/api/auth/login", bytes.NewBufferString(loginBody))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("Login as viewer failed: %d", w.Code)
	}

	var loginResp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &loginResp)
	viewerToken := loginResp["token"].(string)

	// Viewer should not be able to manage users
	w = httptest.NewRecorder()
	router.ServeHTTP(w, authReq("GET", "/api/users", viewerToken, nil))
	if w.Code != http.StatusForbidden {
		t.Errorf("Expected 403 for viewer accessing users, got %d", w.Code)
	}

	// Delete the new user
	w = httptest.NewRecorder()
	router.ServeHTTP(w, authReq("DELETE", "/api/users/"+newUserID, token, nil))
	if w.Code != http.StatusOK {
		t.Errorf("Delete user failed: %d", w.Code)
	}

	// Cannot delete last owner
	// Get owner user ID first
	w = httptest.NewRecorder()
	router.ServeHTTP(w, authReq("GET", "/api/me", token, nil))
	json.Unmarshal(w.Body.Bytes(), &me)
	adminID := me["id"].(string)

	w = httptest.NewRecorder()
	router.ServeHTTP(w, authReq("DELETE", "/api/users/"+adminID, token, nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 when deleting last owner, got %d", w.Code)
	}
}

func TestFileEditorSecurity(t *testing.T) {
	router, tmpDir, cleanup := setupTestEnv(t)
	defer cleanup()

	token := getToken(t, router)

	// Create a server
	createReq := map[string]interface{}{
		"name": "File Test", "version": "1.21.1", "port": 25565, "flavor": "vanilla",
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, authReq("POST", "/api/servers", token, createReq))
	var created map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &created)
	id := created["id"].(string)

	// Create a test properties file
	serverDir := filepath.Join(tmpDir, "servers", id)
	os.WriteFile(filepath.Join(serverDir, "server.properties"), []byte("level-name=world\n"), 0644)

	// Read the file
	w = httptest.NewRecorder()
	router.ServeHTTP(w, authReq("GET", "/api/servers/"+id+"/files/server.properties", token, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("Read file failed: %d %s", w.Code, w.Body.String())
	}

	// Write the file
	writeReq := map[string]interface{}{"content": "level-name=newworld\n"}
	w = httptest.NewRecorder()
	router.ServeHTTP(w, authReq("PUT", "/api/servers/"+id+"/files/server.properties", token, writeReq))
	if w.Code != http.StatusOK {
		t.Errorf("Write file failed: %d %s", w.Code, w.Body.String())
	}

	// Try path traversal
	w = httptest.NewRecorder()
	router.ServeHTTP(w, authReq("GET", "/api/servers/"+id+"/files/../../etc/passwd", token, nil))
	if w.Code == http.StatusOK {
		t.Error("Path traversal should be blocked")
	}

	// Try to read .jar
	w = httptest.NewRecorder()
	router.ServeHTTP(w, authReq("GET", "/api/servers/"+id+"/files/server.jar", token, nil))
	if w.Code == http.StatusOK {
		t.Error("Reading .jar should be blocked")
	}
}

func TestBackups(t *testing.T) {
	router, tmpDir, cleanup := setupTestEnv(t)
	defer cleanup()

	token := getToken(t, router)

	// Create a server
	createReq := map[string]interface{}{
		"name": "Backup Test", "version": "1.21.1", "port": 25565, "flavor": "vanilla",
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, authReq("POST", "/api/servers", token, createReq))
	var created map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &created)
	id := created["id"].(string)

	// Create some files to backup
	serverDir := filepath.Join(tmpDir, "servers", id)
	os.WriteFile(filepath.Join(serverDir, "server.properties"), []byte("level-name=world\n"), 0644)
	os.MkdirAll(filepath.Join(serverDir, "world"), 0755)
	os.WriteFile(filepath.Join(serverDir, "world", "level.dat"), []byte("test-data"), 0644)

	// Create backup
	w = httptest.NewRecorder()
	router.ServeHTTP(w, authReq("POST", "/api/servers/"+id+"/backups", token, nil))
	if w.Code != http.StatusCreated {
		t.Fatalf("Create backup failed: %d %s", w.Code, w.Body.String())
	}

	var backupResp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &backupResp)
	backupID := backupResp["id"].(string)

	// List backups
	w = httptest.NewRecorder()
	router.ServeHTTP(w, authReq("GET", "/api/servers/"+id+"/backups", token, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("List backups failed: %d", w.Code)
	}

	var listResp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &listResp)
	backups := listResp["backups"].([]interface{})
	if len(backups) != 1 {
		t.Errorf("Expected 1 backup, got %d", len(backups))
	}

	// Delete world to test restore
	os.RemoveAll(filepath.Join(serverDir, "world"))

	// Restore backup
	w = httptest.NewRecorder()
	router.ServeHTTP(w, authReq("POST", "/api/servers/"+id+"/backups/"+backupID+"/restore", token, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("Restore backup failed: %d %s", w.Code, w.Body.String())
	}

	// Verify world was restored
	if _, err := os.Stat(filepath.Join(serverDir, "world", "level.dat")); os.IsNotExist(err) {
		t.Error("World was not restored from backup")
	}

	// Delete backup
	w = httptest.NewRecorder()
	router.ServeHTTP(w, authReq("DELETE", "/api/servers/"+id+"/backups/"+backupID, token, nil))
	if w.Code != http.StatusOK {
		t.Errorf("Delete backup failed: %d", w.Code)
	}
}

func TestSchedule(t *testing.T) {
	router, _, cleanup := setupTestEnv(t)
	defer cleanup()

	token := getToken(t, router)

	// Create server
	createReq := map[string]interface{}{
		"name": "Schedule Test", "version": "1.21.1", "port": 25565, "flavor": "vanilla",
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, authReq("POST", "/api/servers", token, createReq))
	var created map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &created)
	id := created["id"].(string)

	// Set schedule
	schedReq := map[string]interface{}{"schedule": "04:00"}
	w = httptest.NewRecorder()
	router.ServeHTTP(w, authReq("PUT", "/api/servers/"+id+"/schedule", token, schedReq))
	if w.Code != http.StatusOK {
		t.Fatalf("Set schedule failed: %d %s", w.Code, w.Body.String())
	}

	// Verify schedule persisted
	w = httptest.NewRecorder()
	router.ServeHTTP(w, authReq("GET", "/api/servers/"+id, token, nil))
	var srv map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &srv)
	if srv["restart_schedule"] != "04:00" {
		t.Errorf("Expected restart_schedule '04:00', got %v", srv["restart_schedule"])
	}
}
