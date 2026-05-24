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
	t.Setenv("ECOFLOW_DELTA3_READ_ENABLED", "true")
	t.Setenv("ECOFLOW_PRIVATE_API_HOST", "api.delta3.test")
	t.Setenv("ECOFLOW_PRIVATE_EMAIL", "delta3@example.com")
	t.Setenv("ECOFLOW_PRIVATE_PASSWORD", "delta3-password")
	t.Setenv("ECOFLOW_DELTA3_DEVICE_SN", "delta3-device")
	t.Setenv("ECOFLOW_DELTA3_DEVICE_TYPE", "DELTA_3_PLUS")
	t.Setenv("ECOFLOW_DELTA3_MQTT_CLIENT_ID", "delta3-client")
	t.Setenv("ECOFLOW_DELTA3_TIMEOUT_SEC", "12")
	t.Setenv("ECOFLOW_DELTA3_ALLOW_WRITE_WITH_AUTO_CONTROL", "true")
	t.Setenv("ECOFLOW_DELTA3_EXECUTE", "true")
	t.Setenv("ECOFLOW_DELTA3_ALLOW_PRIVATE_API_WRITE", "true")
	t.Setenv("DELTA3_AUX_ENABLED", "true")
	t.Setenv("DELTA3_AUX_MIN_CHARGE_W", "120")
	t.Setenv("DELTA3_AUX_MAX_CHARGE_W", "1300")
	t.Setenv("DELTA3_AUX_SAFETY_MARGIN_W", "60")
	t.Setenv("DELTA3_AUX_MIN_COMMAND_DIFF_W", "120")
	t.Setenv("DELTA3_AUX_MAX_INCREASE_STEP_W", "250")
	t.Setenv("DELTA3_AUX_MAX_DECREASE_STEP_W", "450")
	t.Setenv("DELTA3_AUX_MIN_COMMAND_INTERVAL_SEC", "180")
	t.Setenv("DELTA3_AUX_STOP_IMPORT_THRESHOLD_W", "70")
	t.Setenv("DELTA3_AUX_TARGET_MAX_SOC_BUFFER_PERCENT", "4")
	t.Setenv("WEATHER_LATITUDE", "35.1")
	t.Setenv("WEATHER_LONGITUDE", "139.2")
	t.Setenv("WEATHER_TIMEZONE", "Asia/Tokyo")
	t.Setenv("WEATHER_BASE_URL", "https://api.open-meteo.test")
	t.Setenv("AUTO_CONTROL_ENABLED", "true")
	t.Setenv("CONFIRM_ECOFLOW_WRITE", "I_UNDERSTAND")
	t.Setenv("REAL_CONTROL_TRIAL_UNTIL", "2026-05-20T13:30:00+09:00")
	t.Setenv("POLL_INTERVAL_SEC", "45")
	t.Setenv("START_EXPORT_THRESHOLD_W", "900")
	t.Setenv("STOP_EXPORT_THRESHOLD_W", "450")
	t.Setenv("SAFETY_MARGIN_W", "200")
	t.Setenv("MIN_CHARGE_W", "500")
	t.Setenv("MAX_CHARGE_W", "1600")
	t.Setenv("TARGET_SOC", "85")
	t.Setenv("MIN_COMMAND_INTERVAL_SEC", "120")
	t.Setenv("MIN_COMMAND_DIFF_W", "200")
	t.Setenv("NIGHT_SAFETY_MARGIN_KWH", "0.8")
	t.Setenv("EFFECTIVE_CHARGE_THRESHOLD_W", "120")
	t.Setenv("TARGET_EXPORT_BUFFER_W", "180")
	t.Setenv("MAX_INCREASE_STEP_W", "300")
	t.Setenv("MAX_DECREASE_STEP_W", "500")
	t.Setenv("RESERVE_RAISE_STEP_PERCENT", "3")
	t.Setenv("DEFAULT_RESERVE_SOC", "28")
	t.Setenv("PASS_THROUGH_ENABLED", "true")
	t.Setenv("PASS_THROUGH_COOLDOWN_SEC", "420")
	t.Setenv("NOTIFICATION_ENABLED", "true")
	t.Setenv("NOTIFICATION_PROVIDER", "slack")
	t.Setenv("SLACK_WEBHOOK_URL", "https://hooks.slack.test/sample")
	t.Setenv("MANUAL_CHARGE_ALERT_EXPORT_W", "800")
	t.Setenv("MANUAL_CHARGE_ALERT_SOC", "96")
	t.Setenv("MANUAL_CHARGE_ALERT_CONSECUTIVE", "4")
	t.Setenv("MANUAL_CHARGE_ALERT_COOLDOWN_MINUTES", "45")

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
	if !cfg.Delta3ReadEnabled {
		t.Fatal("Delta3ReadEnabled = false, want true")
	}
	if cfg.Delta3PrivateAPIHost != "api.delta3.test" || cfg.Delta3PrivateEmail != "delta3@example.com" || cfg.Delta3PrivatePassword != "delta3-password" {
		t.Fatalf("unexpected delta3 private config: host=%q email=%q", cfg.Delta3PrivateAPIHost, cfg.Delta3PrivateEmail)
	}
	if cfg.Delta3DeviceSN != "delta3-device" || cfg.Delta3DeviceType != "DELTA_3_PLUS" || cfg.Delta3MQTTClientID != "delta3-client" {
		t.Fatalf("unexpected delta3 device config: sn=%q type=%q client=%q", cfg.Delta3DeviceSN, cfg.Delta3DeviceType, cfg.Delta3MQTTClientID)
	}
	if cfg.Delta3Timeout != 12*time.Second {
		t.Fatalf("Delta3Timeout = %s, want 12s", cfg.Delta3Timeout)
	}
	if !cfg.Delta3AllowAutoWrite || !cfg.Delta3ExecuteWrite || !cfg.Delta3AllowPrivateWrite {
		t.Fatalf("unexpected delta3 write gates: auto=%v execute=%v private=%v", cfg.Delta3AllowAutoWrite, cfg.Delta3ExecuteWrite, cfg.Delta3AllowPrivateWrite)
	}
	if !cfg.Delta3Aux.Enabled || cfg.Delta3Aux.MinChargeW != 120 || cfg.Delta3Aux.MaxChargeW != 1300 || cfg.Delta3Aux.SafetyMarginW != 60 {
		t.Fatalf("unexpected delta3 aux config: %+v", cfg.Delta3Aux)
	}
	if cfg.Delta3Aux.MinCommandDiffW != 120 || cfg.Delta3Aux.MaxIncreaseStepW != 250 || cfg.Delta3Aux.MaxDecreaseStepW != 450 {
		t.Fatalf("unexpected delta3 aux command steps: %+v", cfg.Delta3Aux)
	}
	if cfg.Delta3Aux.MinCommandInterval != 180*time.Second || cfg.Delta3Aux.StopImportThresholdW != 70 || cfg.Delta3Aux.TargetMaxSocBufferPercent != 4 {
		t.Fatalf("unexpected delta3 aux guard config: %+v", cfg.Delta3Aux)
	}
	if !cfg.WeatherEnabled {
		t.Fatal("WeatherEnabled = false, want true")
	}
	if cfg.WeatherLatitude != 35.1 || cfg.WeatherLongitude != 139.2 {
		t.Fatalf("Weather location = %f,%f, want 35.1,139.2", cfg.WeatherLatitude, cfg.WeatherLongitude)
	}
	if cfg.WeatherTimezone != "Asia/Tokyo" || cfg.WeatherBaseURL != "https://api.open-meteo.test" {
		t.Fatalf("unexpected weather config: timezone=%q baseURL=%q", cfg.WeatherTimezone, cfg.WeatherBaseURL)
	}
	if !cfg.AutoControlEnabled {
		t.Fatal("AutoControlEnabled = false, want true")
	}
	if cfg.ConfirmEcoFlowWrite != "I_UNDERSTAND" {
		t.Fatalf("ConfirmEcoFlowWrite = %q, want I_UNDERSTAND", cfg.ConfirmEcoFlowWrite)
	}
	if cfg.RealControlTrialUntil.IsZero() || cfg.RealControlTrialUntil.Format(time.RFC3339) != "2026-05-20T13:30:00+09:00" {
		t.Fatalf("RealControlTrialUntil = %s, want 2026-05-20T13:30:00+09:00", cfg.RealControlTrialUntil.Format(time.RFC3339))
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
	if cfg.ControlSettings.NightSafetyMarginKWh != 0.8 {
		t.Fatalf("NightSafetyMarginKWh = %f, want 0.8", cfg.ControlSettings.NightSafetyMarginKWh)
	}
	if cfg.ControlSettings.EffectiveChargeThresholdW != 120 {
		t.Fatalf("EffectiveChargeThresholdW = %d, want 120", cfg.ControlSettings.EffectiveChargeThresholdW)
	}
	if cfg.ControlSettings.TargetExportBufferW != 180 {
		t.Fatalf("TargetExportBufferW = %d, want 180", cfg.ControlSettings.TargetExportBufferW)
	}
	if cfg.ControlSettings.MaxIncreaseStepW != 300 {
		t.Fatalf("MaxIncreaseStepW = %d, want 300", cfg.ControlSettings.MaxIncreaseStepW)
	}
	if cfg.ControlSettings.MaxDecreaseStepW != 500 {
		t.Fatalf("MaxDecreaseStepW = %d, want 500", cfg.ControlSettings.MaxDecreaseStepW)
	}
	if cfg.ControlSettings.ReserveRaiseStepPercent != 3 {
		t.Fatalf("ReserveRaiseStepPercent = %d, want 3", cfg.ControlSettings.ReserveRaiseStepPercent)
	}
	if cfg.ControlSettings.DefaultReserveSoc != 28 {
		t.Fatalf("DefaultReserveSoc = %d, want 28", cfg.ControlSettings.DefaultReserveSoc)
	}
	if !cfg.ControlSettings.PassThroughEnabled {
		t.Fatal("PassThroughEnabled = false, want true")
	}
	if cfg.ControlSettings.PassThroughCooldown != 420*time.Second {
		t.Fatalf("PassThroughCooldown = %s, want 420s", cfg.ControlSettings.PassThroughCooldown)
	}
	if !cfg.NotificationEnabled {
		t.Fatal("NotificationEnabled = false, want true")
	}
	if cfg.NotificationProvider != "slack" || cfg.SlackWebhookURL != "https://hooks.slack.test/sample" {
		t.Fatalf("unexpected notification config: provider=%q url=%q", cfg.NotificationProvider, cfg.SlackWebhookURL)
	}
	if cfg.ManualChargeAlert.ExportThresholdW != 800 || cfg.ManualChargeAlert.SocThreshold != 96 || cfg.ManualChargeAlert.ConsecutiveCount != 4 {
		t.Fatalf("unexpected manual charge alert config: %+v", cfg.ManualChargeAlert)
	}
	if cfg.ManualChargeAlert.Cooldown != 45*time.Minute {
		t.Fatalf("ManualChargeAlert.Cooldown = %s, want 45m", cfg.ManualChargeAlert.Cooldown)
	}
}
