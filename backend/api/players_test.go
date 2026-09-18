package api

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// dialWithRetry waits briefly for a listener to come up.
func dialWithRetry(addr string) (net.Conn, error) {
	var err error
	for i := 0; i < 20; i++ {
		var conn net.Conn
		conn, err = net.DialTimeout("tcp", addr, 500*time.Millisecond)
		if err == nil {
			return conn, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return nil, err
}

// createTestServer adds a server record and returns its id.
func createTestServer(t *testing.T, router http.Handler, token string, port int) string {
	t.Helper()

	body := map[string]interface{}{
		"name": "sleepy", "version": "26.3", "port": port, "flavor": "vanilla",
	}
	w := httptest.NewRecorder()
	router.ServeHTTP(w, authReq("POST", "/api/servers", token, body))
	if w.Code != http.StatusOK && w.Code != http.StatusCreated {
		t.Fatalf("create server: %d %s", w.Code, w.Body.String())
	}

	var created map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	id, _ := created["id"].(string)
	if id == "" {
		t.Fatalf("no server id in response: %s", w.Body.String())
	}
	return id
}

func getServerJSON(t *testing.T, router http.Handler, token, id string) map[string]interface{} {
	t.Helper()

	w := httptest.NewRecorder()
	router.ServeHTTP(w, authReq("GET", "/api/servers/"+id, token, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("get server: %d %s", w.Code, w.Body.String())
	}

	var srv map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &srv); err != nil {
		t.Fatal(err)
	}
	return srv
}

// Asking a server that is not running who is online must fail clearly rather
// than hang or report an empty list as though nobody were playing.
func TestListPlayersOnStoppedServer(t *testing.T) {
	router, _, cleanup := setupTestEnv(t)
	defer cleanup()

	token := getToken(t, router)
	id := createTestServer(t, router, token, 25565)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, authReq("GET", "/api/servers/"+id+"/players", token, nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("got %d %s, want 503", w.Code, w.Body.String())
	}
}

func TestPlayerActionsRequireAName(t *testing.T) {
	router, _, cleanup := setupTestEnv(t)
	defer cleanup()

	token := getToken(t, router)
	id := createTestServer(t, router, token, 25565)

	for _, action := range []string{"kick", "ban", "pardon", "op", "deop"} {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, authReq("POST", "/api/servers/"+id+"/players/"+action, token, map[string]string{}))
		if w.Code != http.StatusBadRequest {
			t.Errorf("%s without a player name returned %d, want 400", action, w.Code)
		}
	}
}

func TestSetAutoSleep(t *testing.T) {
	router, _, cleanup := setupTestEnv(t)
	defer cleanup()

	token := getToken(t, router)
	id := createTestServer(t, router, token, 25565)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, authReq("PUT", "/api/servers/"+id+"/autosleep", token,
		map[string]interface{}{"enabled": true, "idle_timeout_min": 30}))
	if w.Code != http.StatusOK {
		t.Fatalf("enable auto-sleep: %d %s", w.Code, w.Body.String())
	}

	srv := getServerJSON(t, router, token, id)
	if srv["auto_sleep"] != true {
		t.Errorf("auto_sleep = %v, want true", srv["auto_sleep"])
	}
	if srv["idle_timeout_min"] != float64(30) {
		t.Errorf("idle_timeout_min = %v, want 30", srv["idle_timeout_min"])
	}
	// A server with auto-sleep on is asleep rather than merely stopped.
	if srv["status"] != "sleeping" {
		t.Errorf("status = %v, want sleeping", srv["status"])
	}

	// Turning it back off releases the port and returns it to stopped.
	w = httptest.NewRecorder()
	router.ServeHTTP(w, authReq("PUT", "/api/servers/"+id+"/autosleep", token,
		map[string]interface{}{"enabled": false}))
	if w.Code != http.StatusOK {
		t.Fatalf("disable auto-sleep: %d %s", w.Code, w.Body.String())
	}

	srv = getServerJSON(t, router, token, id)
	if srv["auto_sleep"] != false {
		t.Errorf("auto_sleep = %v, want false", srv["auto_sleep"])
	}
	if srv["status"] != "stopped" {
		t.Errorf("status = %v, want stopped", srv["status"])
	}
}

// Enabling auto-sleep must claim the public port so players can reach the
// sleeping realm at the usual address.
func TestAutoSleepHoldsThePublicPort(t *testing.T) {
	router, _, cleanup := setupTestEnv(t)
	defer cleanup()

	token := getToken(t, router)
	id := createTestServer(t, router, token, 25571)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, authReq("PUT", "/api/servers/"+id+"/autosleep", token,
		map[string]interface{}{"enabled": true, "idle_timeout_min": 5}))
	if w.Code != http.StatusOK {
		t.Fatalf("enable auto-sleep: %d %s", w.Code, w.Body.String())
	}

	conn, err := dialWithRetry("127.0.0.1:25571")
	if err != nil {
		t.Fatalf("the sleeping realm's port is not reachable: %v", err)
	}
	conn.Close()

	// Releasing it must free the port again.
	w = httptest.NewRecorder()
	router.ServeHTTP(w, authReq("PUT", "/api/servers/"+id+"/autosleep", token,
		map[string]interface{}{"enabled": false}))
	if w.Code != http.StatusOK {
		t.Fatalf("disable auto-sleep: %d %s", w.Code, w.Body.String())
	}
}

func TestSetAutoRestart(t *testing.T) {
	router, _, cleanup := setupTestEnv(t)
	defer cleanup()

	token := getToken(t, router)
	id := createTestServer(t, router, token, 25565)

	// It is on by default.
	srv := getServerJSON(t, router, token, id)
	if srv["auto_restart"] != true {
		t.Errorf("auto_restart = %v, want true by default", srv["auto_restart"])
	}

	w := httptest.NewRecorder()
	router.ServeHTTP(w, authReq("PUT", "/api/servers/"+id+"/autorestart", token,
		map[string]interface{}{"enabled": false}))
	if w.Code != http.StatusOK {
		t.Fatalf("disable auto-restart: %d %s", w.Code, w.Body.String())
	}

	srv = getServerJSON(t, router, token, id)
	if srv["auto_restart"] != false {
		t.Errorf("auto_restart = %v, want false", srv["auto_restart"])
	}
}

func TestSleepRequiresAutoSleep(t *testing.T) {
	router, _, cleanup := setupTestEnv(t)
	defer cleanup()

	token := getToken(t, router)
	id := createTestServer(t, router, token, 25565)

	w := httptest.NewRecorder()
	router.ServeHTTP(w, authReq("POST", "/api/servers/"+id+"/sleep", token, nil))
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d %s, want 400 for a server without auto-sleep", w.Code, w.Body.String())
	}
}

func TestPlayerEndpointsRequireAuth(t *testing.T) {
	router, _, cleanup := setupTestEnv(t)
	defer cleanup()

	token := getToken(t, router)
	id := createTestServer(t, router, token, 25565)

	for _, path := range []string{
		"/api/servers/" + id + "/players",
	} {
		req := httptest.NewRequest("GET", path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Errorf("%s without a token returned %d, want 401", path, w.Code)
		}
	}
}
