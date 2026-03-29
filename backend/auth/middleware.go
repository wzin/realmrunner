package auth

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"github.com/wzin/realmrunner/config"
	"golang.org/x/crypto/bcrypt"
)

type User struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	PasswordHash string    `json:"-"`
	Role         string    `json:"role"` // admin, operator, viewer
	CreatedAt    time.Time `json:"created_at"`
}

type Middleware struct {
	config *config.Config
	db     *sql.DB
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password" binding:"required"`
}

type LoginResponse struct {
	Token    string `json:"token"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

type Claims struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

func NewMiddleware(cfg *config.Config, db *sql.DB) *Middleware {
	m := &Middleware{config: cfg, db: db}
	m.initUsersTable()
	m.migrateFromSinglePassword()
	return m
}

func (m *Middleware) initUsersTable() {
	schema := `
	CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		username TEXT NOT NULL UNIQUE,
		password_hash TEXT NOT NULL,
		role TEXT NOT NULL DEFAULT 'viewer',
		created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
	);
	`
	m.db.Exec(schema)
}

func (m *Middleware) migrateFromSinglePassword() {
	// Check if any users exist
	var count int
	m.db.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	if count > 0 {
		return // Already migrated
	}

	// Create default admin user from env password hash
	if m.config.PasswordHash == "" {
		return
	}

	id := uuid.New().String()
	_, err := m.db.Exec(
		"INSERT INTO users (id, username, password_hash, role) VALUES (?, ?, ?, ?)",
		id, "admin", m.config.PasswordHash, "owner",
	)
	if err != nil {
		log.Printf("Failed to create default admin user: %v", err)
		return
	}
	log.Println("Created default owner user (username: admin, password from REALMRUNNER_PASSWORD_HASH)")
}

func (m *Middleware) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	// If username is empty, try legacy single-password mode
	username := req.Username
	if username == "" {
		username = "admin"
	}

	// Look up user
	var user User
	err := m.db.QueryRow(
		"SELECT id, username, password_hash, role FROM users WHERE username = ?",
		username,
	).Scan(&user.ID, &user.Username, &user.PasswordHash, &user.Role)

	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
		return
	}

	// Generate JWT token
	token, err := m.generateToken(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, LoginResponse{Token: token, Username: user.Username, Role: user.Role})
}

func (m *Middleware) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, err := m.extractClaims(c)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
			c.Abort()
			return
		}

		c.Set("user_id", claims.UserID)
		c.Set("username", claims.Username)
		c.Set("role", claims.Role)
		c.Next()
	}
}

func (m *Middleware) RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, _ := c.Get("role")
		roleStr, _ := role.(string)

		// Owner has access to everything
		if roleStr == "owner" {
			c.Next()
			return
		}

		for _, r := range roles {
			if roleStr == r {
				c.Next()
				return
			}
		}

		c.JSON(http.StatusForbidden, gin.H{"error": "insufficient permissions"})
		c.Abort()
	}
}

func (m *Middleware) extractClaims(c *gin.Context) (*Claims, error) {
	var tokenString string

	authHeader := c.GetHeader("Authorization")
	if authHeader != "" {
		parts := strings.Split(authHeader, " ")
		if len(parts) == 2 && parts[0] == "Bearer" {
			tokenString = parts[1]
		}
	}

	if tokenString == "" {
		tokenString = c.Query("token")
	}

	if tokenString == "" {
		return nil, fmt.Errorf("authorization required")
	}

	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return []byte(m.config.JWTSecret), nil
	})

	if err != nil || !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	claims, ok := token.Claims.(*Claims)
	if !ok {
		return nil, fmt.Errorf("invalid token claims")
	}

	return claims, nil
}

func (m *Middleware) generateToken(user User) (string, error) {
	claims := Claims{
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(24 * time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(m.config.JWTSecret))
}

// User management functions

func (m *Middleware) ListUsers() ([]User, error) {
	rows, err := m.db.Query("SELECT id, username, role, created_at FROM users ORDER BY created_at")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.Role, &u.CreatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, nil
}

func (m *Middleware) CreateUser(username, password, role string) (*User, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	user := &User{
		ID:       uuid.New().String(),
		Username: username,
		Role:     role,
	}

	_, err = m.db.Exec(
		"INSERT INTO users (id, username, password_hash, role) VALUES (?, ?, ?, ?)",
		user.ID, user.Username, string(hash), user.Role,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create user: %w", err)
	}
	return user, nil
}

func (m *Middleware) UpdateUser(id, role string) error {
	_, err := m.db.Exec("UPDATE users SET role = ? WHERE id = ?", role, id)
	return err
}

func (m *Middleware) DeleteUser(id string) error {
	// Don't allow deleting the last owner
	var ownerCount int
	m.db.QueryRow("SELECT COUNT(*) FROM users WHERE role = 'owner'").Scan(&ownerCount)

	var userRole string
	m.db.QueryRow("SELECT role FROM users WHERE id = ?", id).Scan(&userRole)

	if userRole == "owner" && ownerCount <= 1 {
		return fmt.Errorf("cannot delete the last owner user")
	}

	_, err := m.db.Exec("DELETE FROM users WHERE id = ?", id)
	return err
}

func (m *Middleware) ChangePassword(id, newPassword string) error {
	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	_, err = m.db.Exec("UPDATE users SET password_hash = ? WHERE id = ?", string(hash), id)
	return err
}
