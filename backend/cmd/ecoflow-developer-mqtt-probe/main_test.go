package main

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/eisles/energy-controller/backend/internal/ecoflowdeveloper"
)

func TestBuildConfigUsesReadOnlyEnvAndExplicitSN(t *testing.T) {
	getenv := func(key string) string {
		values := map[string]string{
			"ECOFLOW_PRIVATE_API_HOST":   "api.ecoflow.com",
			"ECOFLOW_PRIVATE_EMAIL":      "user@example.com",
			"ECOFLOW_PRIVATE_PASSWORD":   "secret",
			"ECOFLOW_ACCESS_KEY":         "access-key",
			"ECOFLOW_SECRET_KEY":         "secret-key",
			"ECOFLOW_BASE_URL":           "https://api-e.ecoflow.com",
			"ECOFLOW_DEVICE_SN":          "ENV-SN",
			"ECOFLOW_DELTA3_TIMEOUT_SEC": "30",
		}
		return values[key]
	}
	cfg := buildConfig(options{sn: "CLI-SN"}, getenv)
	if cfg.DeviceSN != "CLI-SN" {
		t.Fatalf("DeviceSN = %q, want CLI-SN", cfg.DeviceSN)
	}
	if cfg.PrivateAPIHost != "api.ecoflow.com" || cfg.Email != "user@example.com" || cfg.Password != "secret" {
		t.Fatalf("cfg = %+v, want private API credentials from env", cfg)
	}
	if cfg.AccessKey != "access-key" || cfg.SecretKey != "secret-key" || cfg.BaseURL != "https://api-e.ecoflow.com" {
		t.Fatalf("cfg = %+v, want official Developer API credentials from env", cfg)
	}
	if cfg.Timeout != 30*time.Second {
		t.Fatalf("Timeout = %s, want 30s", cfg.Timeout)
	}
}

func TestWriteJSONIncludesCycleCandidates(t *testing.T) {
	var buf bytes.Buffer
	err := writeJSON(&buf, output{
		Mode:          "read-only",
		QuotaKeyCount: 2,
		CycleCandidates: []ecoflowdeveloper.CycleCandidate{
			{Key: "bms_bmsStatus.cycSoh", Value: 98},
		},
		Write: readOnlyWriteState("test"),
	})
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(buf.Bytes(), &decoded); err != nil {
		t.Fatal(err)
	}
	candidates, ok := decoded["cycleCandidates"].([]any)
	if !ok || len(candidates) != 1 {
		t.Fatalf("cycleCandidates = %#v, want one candidate", decoded["cycleCandidates"])
	}
	first, ok := candidates[0].(map[string]any)
	if !ok || first["key"] != "bms_bmsStatus.cycSoh" {
		t.Fatalf("first candidate = %#v, want cycSoh key", candidates[0])
	}
}
