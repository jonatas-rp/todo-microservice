// Package config loads process settings from the environment.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config is the runtime configuration of the API process.
type Config struct {
	Addr       string
	DBPath     string
	LogLevel   string
	StoreAddr  string
	PathPrefix string
}

// StoreConfig is the runtime configuration of the store process.
type StoreConfig struct {
	Addr     string
	DBPath   string
	LogLevel string
}

// Load reads PORT, DB_PATH, LOG_LEVEL, STORE_ADDR, and PATH_PREFIX.
// PORT defaults to 8080, DB_PATH to todos.db, and LOG_LEVEL to INFO.
// Addr is a listen address on all interfaces, for example ":8080".
// When STORE_ADDR is set, the API uses the remote store and DB_PATH is ignored.
// PATH_PREFIX is the external ingress path, such as /dev, and is empty when the
// process is reached directly.
func Load() (Config, error) {
	port, err := portAddr("PORT", os.Getenv("PORT"), "8080")
	if err != nil {
		return Config{}, err
	}
	logLevel, err := parseLogLevel(os.Getenv("LOG_LEVEL"))
	if err != nil {
		return Config{}, err
	}
	storeAddr := strings.TrimSpace(os.Getenv("STORE_ADDR"))
	dbPath := ""
	if storeAddr == "" {
		dbPath = os.Getenv("DB_PATH")
		if dbPath == "" {
			dbPath = "todos.db"
		}
	}
	pathPrefix, err := parsePathPrefix(os.Getenv("PATH_PREFIX"))
	if err != nil {
		return Config{}, err
	}
	return Config{
		Addr:       port,
		DBPath:     dbPath,
		LogLevel:   logLevel,
		StoreAddr:  storeAddr,
		PathPrefix: pathPrefix,
	}, nil
}

// LoadStore reads GRPC_ADDR, DB_PATH, and LOG_LEVEL for the store process.
// GRPC_ADDR defaults to :50051, DB_PATH to todos.db, and LOG_LEVEL to INFO.
func LoadStore() (StoreConfig, error) {
	addr, err := listenAddr("GRPC_ADDR", os.Getenv("GRPC_ADDR"), ":50051")
	if err != nil {
		return StoreConfig{}, err
	}
	logLevel, err := parseLogLevel(os.Getenv("LOG_LEVEL"))
	if err != nil {
		return StoreConfig{}, err
	}
	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "todos.db"
	}
	return StoreConfig{Addr: addr, DBPath: dbPath, LogLevel: logLevel}, nil
}

func portAddr(name, raw, fallback string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > 65535 {
		return "", fmt.Errorf("%s must be a number from 1 to 65535, got %q", name, raw)
	}
	return ":" + strconv.Itoa(n), nil
}

func listenAddr(name, raw, fallback string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback, nil
	}
	if strings.Contains(raw, ":") {
		return raw, nil
	}
	return portAddr(name, raw, "")
}

func parsePathPrefix(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	if len(raw) < 2 || raw[0] != '/' || strings.HasSuffix(raw, "/") || strings.Contains(raw, "//") {
		return "", fmt.Errorf("PATH_PREFIX must be an absolute path without a trailing slash, got %q", raw)
	}
	for _, r := range raw[1:] {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' {
			return "", fmt.Errorf("PATH_PREFIX must be an absolute path without a trailing slash, got %q", raw)
		}
	}
	return raw, nil
}

func parseLogLevel(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "INFO", nil
	}
	switch strings.ToUpper(raw) {
	case "DEBUG", "INFO", "ERROR":
		return strings.ToUpper(raw), nil
	default:
		return "", fmt.Errorf("LOG_LEVEL must be DEBUG, INFO, or ERROR, got %q", raw)
	}
}
