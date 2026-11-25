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
	Addr     string
	DBPath   string
	LogLevel string
}

// Load reads PORT, DB_PATH, and LOG_LEVEL.
// PORT defaults to 8080, DB_PATH to todos.db, and LOG_LEVEL to INFO.
// Addr is a listen address on all interfaces, for example ":8080".
func Load() (Config, error) {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	} else {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return Config{}, fmt.Errorf("PORT must be a number from 1 to 65535, got %q", port)
		}
		port = strconv.Itoa(n)
	}

	dbPath := os.Getenv("DB_PATH")
	if dbPath == "" {
		dbPath = "todos.db"
	}

	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = "INFO"
	} else {
		switch strings.ToUpper(strings.TrimSpace(logLevel)) {
		case "DEBUG", "INFO", "ERROR":
			logLevel = strings.ToUpper(strings.TrimSpace(logLevel))
		default:
			return Config{}, fmt.Errorf("LOG_LEVEL must be DEBUG, INFO, or ERROR, got %q", logLevel)
		}
	}
	return Config{Addr: ":" + port, DBPath: dbPath, LogLevel: logLevel}, nil
}
