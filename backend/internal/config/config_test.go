package config

import (
	"testing"
	"time"
)

func TestLoadControlSettingsFromEnvironment(t *testing.T) {
	t.Setenv("START_EXPORT_THRESHOLD_W", "900")
	t.Setenv("STOP_EXPORT_THRESHOLD_W", "450")
	t.Setenv("SAFETY_MARGIN_W", "200")
	t.Setenv("MIN_CHARGE_W", "500")
	t.Setenv("MAX_CHARGE_W", "1600")
	t.Setenv("TARGET_SOC", "85")
	t.Setenv("MIN_COMMAND_INTERVAL_SEC", "120")
	t.Setenv("MIN_COMMAND_DIFF_W", "200")

	cfg := Load()

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
