package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/eisles/energy-controller/backend/internal/ecoflowprivate"
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

func TestRunDryRunRemainingDelta3SocCandidates(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		command string
		hex     string
	}{
		{
			name:    "min discharge",
			args:    []string{"--sn", "SN123", "--device-type", "DELTA_3", "--min-discharge-soc", "10"},
			command: "set_min_discharge_soc",
			hex:     "90020a",
		},
		{
			name:    "max charge",
			args:    []string{"--sn", "SN123", "--device-type", "DELTA_3", "--max-charge-soc", "95"},
			command: "set_max_charge_soc",
			hex:     "88025f",
		},
		{
			name:    "energy backup disabled",
			args:    []string{"--sn", "SN123", "--device-type", "DELTA_3", "--energy-backup-enabled=false", "--energy-backup-start-soc", "25"},
			command: "set_energy_backup_enabled",
			hex:     "da020408001019",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			err := run(context.Background(), tt.args, emptyEnv, &out)
			if err != nil {
				t.Fatal(err)
			}
			var got output
			if err := json.Unmarshal(out.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			if got.Mode != "dry-run" || got.Write["sent"] != false || got.Write["command"] != tt.command {
				t.Fatalf("output = %#v", got)
			}
			if !strings.Contains(got.Write["hex"].(string), tt.hex) {
				t.Fatalf("hex = %s, want %s", got.Write["hex"], tt.hex)
			}
		})
	}
}

func TestRunDryRunEnergyBackupEnabledRequiresStartSoc(t *testing.T) {
	err := run(context.Background(), []string{"--sn", "SN123", "--device-type", "DELTA_3", "--energy-backup-enabled=true"}, emptyEnv, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "--energy-backup-start-soc") {
		t.Fatalf("error = %v, want start SOC guard", err)
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
		"CONFIRM_ECOFLOW_WRITE":      ecoflowprivate.ConfirmWriteValue,
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
		"CONFIRM_ECOFLOW_WRITE":      ecoflowprivate.ConfirmWriteValue,
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
	payload := ecoflowprivate.BuildGetSnapshotPayload(1)
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

func TestRunOfflineFixtureCanInspectFields(t *testing.T) {
	payload, err := ecoflowprivate.BuildSetACChargePowerPayload("SN123", 100, 1)
	if err != nil {
		t.Fatal(err)
	}
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
	err = run(context.Background(), []string{"--sn", "SN123", "--device-type", "DELTA_3", "--offline-fixture", file.Name(), "--inspect-fields"}, emptyEnv, &out)
	if err != nil {
		t.Fatal(err)
	}
	var got output
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Mode != "offline-fixture" || len(got.Fields) == 0 {
		t.Fatalf("output = %#v, want inspected fields", got)
	}
}

func TestRunOfflineFixtureCanInspectCycleCandidatesWithoutFieldsOutput(t *testing.T) {
	payload := privateTelemetryFixturePayload(
		testIntField(8, 30),
		testIntField(888, 57),
	)
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
	err = run(context.Background(), []string{"--sn", "SN123", "--device-type", "DELTA_3", "--offline-fixture", file.Name(), "--inspect-cycle-candidates"}, emptyEnv, &out)
	if err != nil {
		t.Fatal(err)
	}
	var got output
	if err := json.Unmarshal(out.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Fields) != 0 {
		t.Fatalf("fields = %#v, want omitted fields without --inspect-fields", got.Fields)
	}
	if len(got.CycleFieldCandidates) != 1 {
		t.Fatalf("cycleFieldCandidates = %#v, want one candidate", got.CycleFieldCandidates)
	}
	candidate := got.CycleFieldCandidates[0]
	if candidate.CmdFunc != 254 || candidate.CmdID != 21 || candidate.Field != 888 || candidate.Value != 57 {
		t.Fatalf("candidate = %#v, want field 888 value 57", candidate)
	}
}

func privateTelemetryFixturePayload(pdataFields ...[]byte) []byte {
	var pdata []byte
	for _, field := range pdataFields {
		pdata = append(pdata, field...)
	}
	header := []byte{}
	header = append(header, testBytesField(1, pdata)...)
	header = append(header, testIntField(2, 2)...)
	header = append(header, testIntField(8, 254)...)
	header = append(header, testIntField(9, 21)...)
	header = append(header, testIntField(14, 1)...)
	return testBytesField(1, header)
}

func testIntField(field int, value int) []byte {
	out := testEncodeTag(field, 0)
	return append(out, testEncodeVarint(uint64(value))...)
}

func testBytesField(field int, value []byte) []byte {
	out := testEncodeTag(field, 2)
	out = append(out, testEncodeVarint(uint64(len(value)))...)
	return append(out, value...)
}

func testEncodeTag(field int, wire int) []byte {
	return testEncodeVarint(uint64(field<<3 | wire))
}

func testEncodeVarint(value uint64) []byte {
	out := []byte{}
	for {
		b := byte(value & 0x7f)
		value >>= 7
		if value != 0 {
			out = append(out, b|0x80)
			continue
		}
		return append(out, b)
	}
}

func TestSaveRawMessagesWritesPayloadsWithoutTopicSecretsInFilename(t *testing.T) {
	dir := t.TempDir()
	files, err := saveRawMessages(dir, []ecoflowprivate.MQTTMessage{
		{Topic: "/app/user-1/SN123/thing/property/get_reply", Payload: []byte{1, 2, 3}},
		{Topic: "/app/device/property/SN123", Payload: []byte{4, 5}},
	}, time.Date(2026, 5, 31, 8, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("files = %#v, want 2", files)
	}
	if !strings.Contains(files[0].File, "get-reply") || strings.Contains(files[0].File, "SN123") || files[0].Bytes != 3 {
		t.Fatalf("first file metadata = %#v", files[0])
	}
	raw, err := os.ReadFile(files[1].File)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(raw, []byte{4, 5}) {
		t.Fatalf("raw = %v, want [4 5]", raw)
	}
}

func TestSaveRawMessagesForSampleIncludesSampleIndex(t *testing.T) {
	dir := t.TempDir()
	files, err := saveRawMessagesForSample(dir, []ecoflowprivate.MQTTMessage{
		{Topic: "/app/user-1/SN123/thing/property/get_reply", Payload: []byte{1}},
	}, time.Date(2026, 5, 31, 8, 0, 0, 0, time.UTC), 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 {
		t.Fatalf("files = %#v, want 1", files)
	}
	if files[0].Sample != 2 || !strings.Contains(files[0].File, "sample02") || strings.Contains(files[0].File, "SN123") {
		t.Fatalf("sample raw file metadata = %#v", files[0])
	}
}

func TestBuildConfigUsesDelta3TimeoutEnvUnlessFlagSet(t *testing.T) {
	cfg := buildConfig(options{}, mapEnv(map[string]string{
		"ECOFLOW_DELTA3_TIMEOUT_SEC": "12",
	}))
	if cfg.Timeout != 12*time.Second {
		t.Fatalf("Timeout = %s, want 12s", cfg.Timeout)
	}
	cfg = buildConfig(options{timeout: 3 * time.Second, timeoutSet: true}, mapEnv(map[string]string{
		"ECOFLOW_DELTA3_TIMEOUT_SEC": "12",
	}))
	if cfg.Timeout != 3*time.Second {
		t.Fatalf("Timeout = %s, want explicit flag value 3s", cfg.Timeout)
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
