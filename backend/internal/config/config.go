package config

import (
	"os"
	"strconv"
	"time"
)

type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time {
	return time.Now()
}

type Config struct {
	AppEnv            string
	HTTPPort          string
	DBPath            string
	FrontendDir       string
	MockMode          bool
	SimulationMode    bool
	EnableRealControl bool
	Clock             Clock
}

func Load() Config {
	return Config{
		AppEnv:            env("APP_ENV", "local"),
		HTTPPort:          env("HTTP_PORT", "8080"),
		DBPath:            env("DB_PATH", "./data/energy.db"),
		FrontendDir:       env("FRONTEND_DIR", "../frontend/out"),
		MockMode:          envBool("MOCK_MODE", true),
		SimulationMode:    envBool("SIMULATION_MODE", true),
		EnableRealControl: envBool("ENABLE_REAL_CONTROL", false),
		Clock:             realClock{},
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}
