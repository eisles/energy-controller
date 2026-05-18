package config

import (
	"os"
	"strconv"
	"time"

	"github.com/eisles/energy-controller/backend/internal/control"
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
	ControlSettings   control.Settings
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
		ControlSettings: control.Settings{
			StartExportThresholdW: envInt("START_EXPORT_THRESHOLD_W", 700),
			StopExportThresholdW:  envInt("STOP_EXPORT_THRESHOLD_W", 300),
			SafetyMarginW:         envInt("SAFETY_MARGIN_W", 150),
			MinChargeW:            envInt("MIN_CHARGE_W", 400),
			MaxChargeW:            envInt("MAX_CHARGE_W", 1500),
			TargetSoc:             envInt("TARGET_SOC", 90),
			MinCommandInterval:    time.Duration(envInt("MIN_COMMAND_INTERVAL_SEC", 60)) * time.Second,
			MinCommandDiffW:       envInt("MIN_COMMAND_DIFF_W", 100),
		},
		Clock: realClock{},
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

func envInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}
