package config

import (
	"testing"
	"time"
)

func TestLoadControlSettingsFromEnvironment(t *testing.T) {
	t.Setenv("NATURE_MODE", "cloud")
	t.Setenv("NATURE_ACCESS_TOKEN", "nature-token")
	t.Setenv("NATURE_APPLIANCE_ID", "nature-appliance")
	t.Setenv("NATURE_LOCAL_BASE_URL", "http://remo-e.test")
	t.Setenv("ECOFLOW_ACCESS_KEY", "ecoflow-access")
	t.Setenv("ECOFLOW_SECRET_KEY", "ecoflow-secret")
	t.Setenv("ECOFLOW_DEVICE_SN", "ecoflow-device")
	t.Setenv("ECOFLOW_BASE_URL", "https://api-e.ecoflow.test")
	t.Setenv("AUTO_CONTROL_ENABLED", "true")
	t.Setenv("POLL_INTERVAL_SEC", "45")
	t.Setenv("START_EXPORT_THRESHOLD_W", "900")
	t.Setenv("STOP_EXPORT_THRESHOLD_W", "450")
	t.Setenv("SAFETY_MARGIN_W", "200")
	t.Setenv("MIN_CHARGE_W", "500")
	t.Setenv("MAX_CHARGE_W", "1600")
	t.Setenv("TARGET_SOC", "85")
	t.Setenv("MIN_COMMAND_INTERVAL_SEC", "120")
	t.Setenv("MIN_COMMAND_DIFF_W", "200")

	cfg := Load()

	if cfg.NatureMode != "cloud" {
		t.Fatalf("NatureMode = %q, want cloud", cfg.NatureMode)
	}
	if cfg.NatureAccessToken != "nature-token" {
		t.Fatalf("NatureAccessToken was not loaded")
	}
	if cfg.NatureApplianceID != "nature-appliance" {
		t.Fatalf("NatureApplianceID = %q, want nature-appliance", cfg.NatureApplianceID)
	}
	if cfg.NatureLocalBaseURL != "http://remo-e.test" {
		t.Fatalf("NatureLocalBaseURL = %q, want http://remo-e.test", cfg.NatureLocalBaseURL)
	}
	if cfg.EcoFlowAccessKey != "ecoflow-access" {
		t.Fatalf("EcoFlowAccessKey was not loaded")
	}
	if cfg.EcoFlowSecretKey != "ecoflow-secret" {
		t.Fatalf("EcoFlowSecretKey was not loaded")
	}
	if cfg.EcoFlowDeviceSN != "ecoflow-device" {
		t.Fatalf("EcoFlowDeviceSN = %q, want ecoflow-device", cfg.EcoFlowDeviceSN)
	}
	if cfg.EcoFlowBaseURL != "https://api-e.ecoflow.test" {
		t.Fatalf("EcoFlowBaseURL = %q, want https://api-e.ecoflow.test", cfg.EcoFlowBaseURL)
	}
	if !cfg.AutoControlEnabled {
		t.Fatal("AutoControlEnabled = false, want true")
	}
	if cfg.PollInterval != 45*time.Second {
		t.Fatalf("PollInterval = %s, want 45s", cfg.PollInterval)
	}
	if cfg.ControlSettings.StartExportThresholdW != 900 {
		t.Fatalf("StartExportThresholdW = %d, want 900", cfg.ControlSettings.StartExportThresholdW)
	}
	if cfg.ControlSettings.StopExportThresholdW != 450 {
		t.Fatalf("StopExportThresholdW = %d, want 450", cfg.ControlSettings.StopExportThresholdW)
	}
	if cfg.ControlSettings.SafetyMarginW != 200 {
		t.Fatalf("SafetyMarginW = %d, want 200", cfg.ControlSettings.SafetyMarginW)
	}
	if cfg.ControlSettings.MinChargeW != 500 {
		t.Fatalf("MinChargeW = %d, want 500", cfg.ControlSettings.MinChargeW)
	}
	if cfg.ControlSettings.MaxChargeW != 1600 {
		t.Fatalf("MaxChargeW = %d, want 1600", cfg.ControlSettings.MaxChargeW)
	}
	if cfg.ControlSettings.TargetSoc != 85 {
		t.Fatalf("TargetSoc = %d, want 85", cfg.ControlSettings.TargetSoc)
	}
	if cfg.ControlSettings.MinCommandInterval != 120*time.Second {
		t.Fatalf("MinCommandInterval = %s, want 120s", cfg.ControlSettings.MinCommandInterval)
	}
	if cfg.ControlSettings.MinCommandDiffW != 200 {
		t.Fatalf("MinCommandDiffW = %d, want 200", cfg.ControlSettings.MinCommandDiffW)
	}
}
