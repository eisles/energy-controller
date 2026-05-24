package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"
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
	AppEnv                  string
	HTTPPort                string
	DBPath                  string
	FrontendDir             string
	MockMode                bool
	SimulationMode          bool
	EnableRealControl       bool
	AutoControlEnabled      bool
	ConfirmEcoFlowWrite     string
	RealControlTrialUntil   time.Time
	NatureMode              string
	NatureAccessToken       string
	NatureApplianceID       string
	NatureLocalBaseURL      string
	EcoFlowAccessKey        string
	EcoFlowSecretKey        string
	EcoFlowDeviceSN         string
	EcoFlowBaseURL          string
	Delta3ReadEnabled       bool
	Delta3PrivateAPIHost    string
	Delta3PrivateEmail      string
	Delta3PrivatePassword   string
	Delta3DeviceSN          string
	Delta3DeviceType        string
	Delta3MQTTClientID      string
	Delta3Timeout           time.Duration
	Delta3AllowAutoWrite    bool
	Delta3ExecuteWrite      bool
	Delta3AllowPrivateWrite bool
	Delta3Aux               Delta3AuxConfig
	WeatherEnabled          bool
	WeatherLatitude         float64
	WeatherLongitude        float64
	WeatherTimezone         string
	WeatherBaseURL          string
	NotificationEnabled     bool
	NotificationProvider    string
	SlackWebhookURL         string
	ManualChargeAlert       ManualChargeAlertConfig
	PollInterval            time.Duration
	ControlSettings         control.Settings
	Clock                   Clock
}

type ManualChargeAlertConfig struct {
	ExportThresholdW int
	SocThreshold     int
	ConsecutiveCount int
	Cooldown         time.Duration
}

type Delta3AuxConfig struct {
	Enabled                   bool
	MinChargeW                int
	MaxChargeW                int
	SafetyMarginW             int
	MinCommandDiffW           int
	MaxIncreaseStepW          int
	MaxDecreaseStepW          int
	MinCommandInterval        time.Duration
	StopImportThresholdW      int
	TargetMaxSocBufferPercent int
}

func Load() Config {
	loadDotEnv()
	weatherLatitudeRaw := env("WEATHER_LATITUDE", "")
	weatherLongitudeRaw := env("WEATHER_LONGITUDE", "")
	realControlTrialUntil := parseTime(env("REAL_CONTROL_TRIAL_UNTIL", ""))
	if realControlTrialUntil.IsZero() {
		if trialMinutes := envInt("REAL_CONTROL_TRIAL_MINUTES", 0); trialMinutes > 0 {
			realControlTrialUntil = time.Now().Add(time.Duration(trialMinutes) * time.Minute)
		}
	}
	return Config{
		AppEnv:                  env("APP_ENV", "local"),
		HTTPPort:                env("HTTP_PORT", "8080"),
		DBPath:                  env("DB_PATH", "./data/energy.db"),
		FrontendDir:             env("FRONTEND_DIR", "../frontend/out"),
		MockMode:                envBool("MOCK_MODE", true),
		SimulationMode:          envBool("SIMULATION_MODE", true),
		EnableRealControl:       envBool("ENABLE_REAL_CONTROL", false),
		AutoControlEnabled:      envBool("AUTO_CONTROL_ENABLED", false),
		ConfirmEcoFlowWrite:     env("CONFIRM_ECOFLOW_WRITE", ""),
		RealControlTrialUntil:   realControlTrialUntil,
		NatureMode:              env("NATURE_MODE", "cloud"),
		NatureAccessToken:       env("NATURE_ACCESS_TOKEN", ""),
		NatureApplianceID:       env("NATURE_APPLIANCE_ID", ""),
		NatureLocalBaseURL:      env("NATURE_LOCAL_BASE_URL", "http://remo-e.local"),
		EcoFlowAccessKey:        env("ECOFLOW_ACCESS_KEY", ""),
		EcoFlowSecretKey:        env("ECOFLOW_SECRET_KEY", ""),
		EcoFlowDeviceSN:         env("ECOFLOW_DEVICE_SN", ""),
		EcoFlowBaseURL:          env("ECOFLOW_BASE_URL", "https://api-e.ecoflow.com"),
		Delta3ReadEnabled:       envBool("ECOFLOW_DELTA3_READ_ENABLED", false),
		Delta3PrivateAPIHost:    env("ECOFLOW_PRIVATE_API_HOST", "api.ecoflow.com"),
		Delta3PrivateEmail:      env("ECOFLOW_PRIVATE_EMAIL", ""),
		Delta3PrivatePassword:   env("ECOFLOW_PRIVATE_PASSWORD", ""),
		Delta3DeviceSN:          env("ECOFLOW_DELTA3_DEVICE_SN", ""),
		Delta3DeviceType:        env("ECOFLOW_DELTA3_DEVICE_TYPE", "DELTA_3"),
		Delta3MQTTClientID:      env("ECOFLOW_DELTA3_MQTT_CLIENT_ID", ""),
		Delta3Timeout:           time.Duration(envInt("ECOFLOW_DELTA3_TIMEOUT_SEC", 20)) * time.Second,
		Delta3AllowAutoWrite:    envBool("ECOFLOW_DELTA3_ALLOW_WRITE_WITH_AUTO_CONTROL", false),
		Delta3ExecuteWrite:      envBool("ECOFLOW_DELTA3_EXECUTE", false),
		Delta3AllowPrivateWrite: envBool("ECOFLOW_DELTA3_ALLOW_PRIVATE_API_WRITE", false),
		Delta3Aux: Delta3AuxConfig{
			Enabled:                   envBool("DELTA3_AUX_ENABLED", false),
			MinChargeW:                envInt("DELTA3_AUX_MIN_CHARGE_W", 100),
			MaxChargeW:                envInt("DELTA3_AUX_MAX_CHARGE_W", 1500),
			SafetyMarginW:             envInt("DELTA3_AUX_SAFETY_MARGIN_W", 50),
			MinCommandDiffW:           envInt("DELTA3_AUX_MIN_COMMAND_DIFF_W", 100),
			MaxIncreaseStepW:          envInt("DELTA3_AUX_MAX_INCREASE_STEP_W", 300),
			MaxDecreaseStepW:          envInt("DELTA3_AUX_MAX_DECREASE_STEP_W", 500),
			MinCommandInterval:        time.Duration(envInt("DELTA3_AUX_MIN_COMMAND_INTERVAL_SEC", 120)) * time.Second,
			StopImportThresholdW:      envInt("DELTA3_AUX_STOP_IMPORT_THRESHOLD_W", 50),
			TargetMaxSocBufferPercent: envInt("DELTA3_AUX_TARGET_MAX_SOC_BUFFER_PERCENT", 2),
		},
		WeatherEnabled:       envBool("WEATHER_FORECAST_ENABLED", weatherLatitudeRaw != "" && weatherLongitudeRaw != ""),
		WeatherLatitude:      parseFloat(weatherLatitudeRaw, 0),
		WeatherLongitude:     parseFloat(weatherLongitudeRaw, 0),
		WeatherTimezone:      env("WEATHER_TIMEZONE", "Asia/Tokyo"),
		WeatherBaseURL:       env("WEATHER_BASE_URL", "https://api.open-meteo.com"),
		NotificationEnabled:  envBool("NOTIFICATION_ENABLED", false),
		NotificationProvider: env("NOTIFICATION_PROVIDER", "slack"),
		SlackWebhookURL:      env("SLACK_WEBHOOK_URL", ""),
		ManualChargeAlert: ManualChargeAlertConfig{
			ExportThresholdW: envInt("MANUAL_CHARGE_ALERT_EXPORT_W", 700),
			SocThreshold:     envInt("MANUAL_CHARGE_ALERT_SOC", 95),
			ConsecutiveCount: envInt("MANUAL_CHARGE_ALERT_CONSECUTIVE", 3),
			Cooldown:         time.Duration(envInt("MANUAL_CHARGE_ALERT_COOLDOWN_MINUTES", 30)) * time.Minute,
		},
		PollInterval: time.Duration(envInt("POLL_INTERVAL_SEC", 30)) * time.Second,
		ControlSettings: control.Settings{
			StartExportThresholdW:     envInt("START_EXPORT_THRESHOLD_W", 700),
			StopExportThresholdW:      envInt("STOP_EXPORT_THRESHOLD_W", 300),
			SafetyMarginW:             envInt("SAFETY_MARGIN_W", 150),
			MinChargeW:                envInt("MIN_CHARGE_W", 400),
			MaxChargeW:                envInt("MAX_CHARGE_W", 1500),
			TargetSoc:                 envInt("TARGET_SOC", 90),
			MinCommandInterval:        time.Duration(envInt("MIN_COMMAND_INTERVAL_SEC", 60)) * time.Second,
			MinCommandDiffW:           envInt("MIN_COMMAND_DIFF_W", 100),
			NightSafetyMarginKWh:      parseFloat(env("NIGHT_SAFETY_MARGIN_KWH", "0.5"), 0.5),
			EffectiveChargeThresholdW: envInt("EFFECTIVE_CHARGE_THRESHOLD_W", 100),
			TargetExportBufferW:       envInt("TARGET_EXPORT_BUFFER_W", 150),
			MaxIncreaseStepW:          envInt("MAX_INCREASE_STEP_W", 400),
			MaxDecreaseStepW:          envInt("MAX_DECREASE_STEP_W", 600),
			ReserveRaiseStepPercent:   envInt("RESERVE_RAISE_STEP_PERCENT", 2),
			DefaultReserveSoc:         envInt("DEFAULT_RESERVE_SOC", 30),
			PassThroughEnabled:        envBool("PASS_THROUGH_ENABLED", false),
			PassThroughCooldown:       time.Duration(envInt("PASS_THROUGH_COOLDOWN_SEC", 300)) * time.Second,
		},
		Clock: realClock{},
	}
}

func loadDotEnv() {
	for _, path := range []string{".env", "../.env"} {
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			key, value, ok := strings.Cut(line, "=")
			if !ok {
				continue
			}
			key = strings.TrimSpace(key)
			value = strings.Trim(strings.TrimSpace(value), `"'`)
			if key == "" {
				continue
			}
			if _, exists := os.LookupEnv(key); !exists {
				_ = os.Setenv(key, value)
			}
		}
		return
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

func parseFloat(value string, fallback float64) float64 {
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func parseTime(value string) time.Time {
	if value == "" {
		return time.Time{}
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}
	}
	return parsed
}
