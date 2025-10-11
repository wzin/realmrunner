package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	PasswordHash string
	MaxRunning   int
	PortRange    PortRange
	MemoryMB     int
	DataDir      string
	JWTSecret    string
	BaseURL      string
}

type PortRange struct {
	Min int
	Max int
}

func Load() (*Config, error) {
	passwordHash := os.Getenv("REALMRUNNER_PASSWORD_HASH")
	if passwordHash == "" {
		return nil, fmt.Errorf("REALMRUNNER_PASSWORD_HASH is required")
	}

	maxRunning := 3
	if val := os.Getenv("REALMRUNNER_MAX_RUNNING"); val != "" {
		parsed, err := strconv.Atoi(val)
		if err != nil {
			return nil, fmt.Errorf("invalid REALMRUNNER_MAX_RUNNING: %w", err)
		}
		maxRunning = parsed
	}

	portRange := PortRange{Min: 25565, Max: 25600}
	if val := os.Getenv("REALMRUNNER_PORT_RANGE"); val != "" {
		parts := strings.Split(val, "-")
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid REALMRUNNER_PORT_RANGE format, expected: MIN-MAX")
		}
		min, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, fmt.Errorf("invalid port range min: %w", err)
		}
		max, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid port range max: %w", err)
		}
		portRange = PortRange{Min: min, Max: max}
	}

	memoryMB := 2048
	if val := os.Getenv("REALMRUNNER_MEMORY_MB"); val != "" {
		parsed, err := strconv.Atoi(val)
		if err != nil {
			return nil, fmt.Errorf("invalid REALMRUNNER_MEMORY_MB: %w", err)
		}
		memoryMB = parsed
	}

	dataDir := os.Getenv("REALMRUNNER_DATA_DIR")
	if dataDir == "" {
		dataDir = "/data"
	}

	jwtSecret := os.Getenv("REALMRUNNER_JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "change-me-in-production" // Default for development
	}

	baseURL := os.Getenv("REALMRUNNER_BASE_URL")
	if baseURL == "" {
		baseURL = "localhost" // Default for development
	}

	return &Config{
		PasswordHash: passwordHash,
		MaxRunning:   maxRunning,
		PortRange:    portRange,
		MemoryMB:     memoryMB,
		DataDir:      dataDir,
		JWTSecret:    jwtSecret,
		BaseURL:      baseURL,
	}, nil
}

func (pr PortRange) Contains(port int) bool {
	return port >= pr.Min && port <= pr.Max
}
