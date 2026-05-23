package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/eisles/energy-controller/backend/internal/ecoflowdelta3"
)

func TestRunDryRunDoesNotRequireCredentials(t *testing.T) {
	var out bytes.Buffer
	err := run(context.Background(), []string{"--sn", "SN123", "--device-type", "DELTA_3", "--set-ac-charge-w", "100"}, emptyEnv, &out)
	if err != nil {
		t.Fatal(err)
	}
	var got output
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Mode != "dry-run" || got.Write["sent"] != false || got.Write["command"] != "set_ac_charge_power" {
		t.Fatalf("output = %#v", got)
	}
	if !strings.Contains(got.Write["hex"].(string), "b00364e80700") {
		t.Fatalf("hex = %s, want AC charge command bytes", got.Write["hex"])
	}
}

func TestRunDryRunGridBypassDisabled(t *testing.T) {
	var out bytes.Buffer
	err := run(context.Background(), []string{"--sn", "SN123", "--device-type", "DELTA_3", "--grid-bypass-disabled=true"}, emptyEnv, &out)
	if err != nil {
		t.Fatal(err)
	}
	var got output
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Mode != "dry-run" || got.Write["sent"] != false || got.Write["command"] != "set_grid_bypass_disabled" {
		t.Fatalf("output = %#v", got)
	}
	if !strings.Contains(got.Write["hex"].(string), "d00101") {
		t.Fatalf("hex = %s, want grid bypass command bytes", got.Write["hex"])
	}
}

func TestRunDryRunRequiresDeviceSN(t *testing.T) {
	err := run(context.Background(), []string{"--device-type", "DELTA_3", "--set-ac-charge-w", "100"}, emptyEnv, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "ECOFLOW_DELTA3_DEVICE_SN") {
		t.Fatalf("error = %v, want device SN guard", err)
	}
}

func TestRunDryRunRejectsUnknownDeviceForBackupReserve(t *testing.T) {
	err := run(context.Background(), []string{"--sn", "SN123", "--device-type", "UNKNOWN", "--backup-reserve-soc", "30"}, emptyEnv, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "unsupported device type") {
		t.Fatalf("error = %v, want unsupported device guard", err)
	}
}

func TestRunExecuteRequiresPrivateWriteFlagBeforeNetwork(t *testing.T) {
	env := map[string]string{
		"MOCK_MODE":                  "false",
		"SIMULATION_MODE":            "false",
		"ENABLE_REAL_CONTROL":        "true",
		"AUTO_CONTROL_ENABLED":       "false",
		"CONFIRM_ECOFLOW_WRITE":      ecoflowdelta3.ConfirmWriteValue,
		"ECOFLOW_PRIVATE_EMAIL":      "user@example.com",
		"ECOFLOW_PRIVATE_PASSWORD":   "secret",
		"ECOFLOW_DELTA3_DEVICE_SN":   "SN123",
		"ECOFLOW_DELTA3_DEVICE_TYPE": "DELTA_3",
	}
	err := run(context.Background(), []string{"--execute", "--set-ac-charge-w", "100"}, mapEnv(env), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "--allow-private-api-write") {
		t.Fatalf("error = %v, want private write flag guard", err)
	}
}

func TestRunExecuteRequiresAutoControlDisabled(t *testing.T) {
	env := map[string]string{
		"MOCK_MODE":                  "false",
		"SIMULATION_MODE":            "false",
		"ENABLE_REAL_CONTROL":        "true",
		"AUTO_CONTROL_ENABLED":       "true",
		"CONFIRM_ECOFLOW_WRITE":      ecoflowdelta3.ConfirmWriteValue,
		"ECOFLOW_PRIVATE_EMAIL":      "user@example.com",
		"ECOFLOW_PRIVATE_PASSWORD":   "secret",
		"ECOFLOW_DELTA3_DEVICE_SN":   "SN123",
		"ECOFLOW_DELTA3_DEVICE_TYPE": "DELTA_3",
	}
	err := run(context.Background(), []string{"--execute", "--allow-private-api-write", "--set-ac-charge-w", "100"}, mapEnv(env), &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "AUTO_CONTROL_ENABLED") {
		t.Fatalf("error = %v, want auto-control guard", err)
	}
}

func TestRunOfflineFixtureDecodesSnapshot(t *testing.T) {
	payload := ecoflowdelta3.BuildGetSnapshotPayload(1)
	file, err := os.CreateTemp(t.TempDir(), "fixture-*.bin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	err = run(context.Background(), []string{"--sn", "SN123", "--device-type", "DELTA_3", "--offline-fixture", file.Name()}, emptyEnv, &out)
	if err != nil {
		t.Fatal(err)
	}
	var got output
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Mode != "offline-fixture" || got.Status.DeviceSN != "SN123" {
		t.Fatalf("output = %#v", got)
	}
}

func emptyEnv(string) string {
	return ""
}

func mapEnv(values map[string]string) envGetter {
	return func(key string) string {
		return values[key]
	}
}
