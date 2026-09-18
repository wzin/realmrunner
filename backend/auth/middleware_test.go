package auth

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	_ "github.com/mattn/go-sqlite3"
	"github.com/wzin/realmrunner/config"
	"golang.org/x/crypto/bcrypt"
)

func newTestMiddleware(t *testing.T) *Middleware {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })

	hash, err := bcrypt.GenerateFromPassword([]byte("s3cret"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}

	cfg := &config.Config{PasswordHash: string(hash), JWTSecret: "test-secret"}
	return NewMiddleware(cfg, db)
}

func login(t *testing.T, m *Middleware, username, password string) *httptest.ResponseRecorder {
	t.Helper()

	router := gin.New()
	router.POST("/login", m.Login)

	body, _ := json.Marshal(map[string]string{"username": username, "password": password})
	req := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	return w
}

func TestLoginWithMigratedAdminUser(t *testing.T) {
	m := newTestMiddleware(t)

	w := login(t, m, "admin", "s3cret")
	if w.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", w.Code, w.Body.String())
	}

	var resp LoginResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Token == "" {
		t.Error("no token returned")
	}
	if resp.Role != "owner" {
		t.Errorf("role = %q, want owner", resp.Role)
	}
}

// Logging in without a username keeps the old single-password flow working.
func TestLoginLegacySinglePassword(t *testing.T) {
	m := newTestMiddleware(t)

	if w := login(t, m, "", "s3cret"); w.Code != http.StatusOK {
		t.Errorf("legacy login failed: %d %s", w.Code, w.Body.String())
	}
}

func TestLoginRejectsBadCredentials(t *testing.T) {
	m := newTestMiddleware(t)

	if w := login(t, m, "admin", "wrong"); w.Code != http.StatusUnauthorized {
		t.Errorf("wrong password returned %d, want 401", w.Code)
	}
	if w := login(t, m, "nobody", "s3cret"); w.Code != http.StatusUnauthorized {
		t.Errorf("unknown user returned %d, want 401", w.Code)
	}
}

func protectedRouter(m *Middleware, roles ...string) *gin.Engine {
	router := gin.New()
	group := router.Group("/", m.RequireAuth())
	if len(roles) > 0 {
		group.Use(m.RequireRole(roles...))
	}
	group.GET("/protected", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"username": c.GetString("username"), "role": c.GetString("role")})
	})
	return router
}

func tokenFor(t *testing.T, m *Middleware, username, password string) string {
	t.Helper()

	w := login(t, m, username, password)
	if w.Code != http.StatusOK {
		t.Fatalf("login failed: %d %s", w.Code, w.Body.String())
	}
	var resp LoginResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	return resp.Token
}

func TestRequireAuth(t *testing.T) {
	m := newTestMiddleware(t)
	router := protectedRouter(m)
	token := tokenFor(t, m, "admin", "s3cret")

	tests := []struct {
		name    string
		prepare func(*http.Request)
		want    int
	}{
		{"bearer header", func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+token) }, http.StatusOK},
		{"query parameter", func(r *http.Request) { r.URL.RawQuery = "token=" + token }, http.StatusOK},
		{"no credentials", func(r *http.Request) {}, http.StatusUnauthorized},
		{"malformed header", func(r *http.Request) { r.Header.Set("Authorization", token) }, http.StatusUnauthorized},
		{"garbage token", func(r *http.Request) { r.Header.Set("Authorization", "Bearer not-a-token") }, http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			tt.prepare(req)

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != tt.want {
				t.Errorf("got %d, want %d (%s)", w.Code, tt.want, w.Body.String())
			}
		})
	}
}

// A token signed with a different secret, or with an unexpected algorithm, must
// never be accepted.
func TestRequireAuthRejectsForgedTokens(t *testing.T) {
	m := newTestMiddleware(t)
	router := protectedRouter(m)

	claims := Claims{UserID: "1", Username: "attacker", Role: "owner",
		RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}}

	wrongSecret, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte("other-secret"))
	if err != nil {
		t.Fatal(err)
	}
	unsigned, err := jwt.NewWithClaims(jwt.SigningMethodNone, claims).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatal(err)
	}
	expired := Claims{UserID: "1", Username: "admin", Role: "owner",
		RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour))}}
	expiredToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, expired).SignedString([]byte("test-secret"))
	if err != nil {
		t.Fatal(err)
	}

	for name, token := range map[string]string{
		"wrong secret":    wrongSecret,
		"alg none":        unsigned,
		"expired":         expiredToken,
		"empty signature": strings.Join(strings.Split(wrongSecret, ".")[:2], ".") + ".",
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			req.Header.Set("Authorization", "Bearer "+token)

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			if w.Code != http.StatusUnauthorized {
				t.Errorf("forged token accepted: %d %s", w.Code, w.Body.String())
			}
		})
	}
}

func TestRequireRole(t *testing.T) {
	m := newTestMiddleware(t)

	if _, err := m.CreateUser("viewer1", "pw", "viewer"); err != nil {
		t.Fatal(err)
	}
	if _, err := m.CreateUser("operator1", "pw", "operator"); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		user string
		want int
	}{
		{"admin", http.StatusOK},     // owner bypasses the role check
		{"operator1", http.StatusOK}, // explicitly allowed
		{"viewer1", http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.user, func(t *testing.T) {
			password := "pw"
			if tt.user == "admin" {
				password = "s3cret"
			}
			token := tokenFor(t, m, tt.user, password)

			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			req.Header.Set("Authorization", "Bearer "+token)

			w := httptest.NewRecorder()
			protectedRouter(m, "operator").ServeHTTP(w, req)
			if w.Code != tt.want {
				t.Errorf("got %d, want %d", w.Code, tt.want)
			}
		})
	}
}

func TestUserManagement(t *testing.T) {
	m := newTestMiddleware(t)

	user, err := m.CreateUser("builder", "hunter2", "operator")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	if _, err := m.CreateUser("builder", "hunter2", "operator"); err == nil {
		t.Error("duplicate username should be rejected")
	}

	users, err := m.ListUsers()
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(users) != 2 {
		t.Errorf("ListUsers returned %d users, want 2", len(users))
	}

	if err := m.UpdateUser(user.ID, "viewer"); err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}

	if err := m.ChangePassword(user.ID, "new-password"); err != nil {
		t.Fatalf("ChangePassword: %v", err)
	}
	if w := login(t, m, "builder", "new-password"); w.Code != http.StatusOK {
		t.Errorf("login with the new password failed: %d", w.Code)
	}
	if w := login(t, m, "builder", "hunter2"); w.Code != http.StatusUnauthorized {
		t.Error("the old password still works after a change")
	}

	if err := m.DeleteUser(user.ID); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
}

func TestDeleteLastOwnerIsRejected(t *testing.T) {
	m := newTestMiddleware(t)

	users, err := m.ListUsers()
	if err != nil {
		t.Fatal(err)
	}

	var ownerID string
	for _, u := range users {
		if u.Role == "owner" {
			ownerID = u.ID
		}
	}
	if ownerID == "" {
		t.Fatal("no owner user was created")
	}

	if err := m.DeleteUser(ownerID); err == nil {
		t.Error("deleting the last owner should fail")
	}

	second, err := m.CreateUser("owner2", "pw", "owner")
	if err != nil {
		t.Fatal(err)
	}
	if err := m.DeleteUser(ownerID); err != nil {
		t.Errorf("deleting an owner with another owner present failed: %v", err)
	}
	if err := m.DeleteUser(second.ID); err == nil {
		t.Error("deleting the remaining owner should fail")
	}
}
