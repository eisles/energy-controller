package ecoflow

import (
	"encoding/json"
	"testing"
)

func TestBuildSetACChargePowerPayload(t *testing.T) {
	payload, err := buildSetACChargePowerPayload("DP3-SN", 1000)
	if err != nil {
		t.Fatalf("buildSetACChargePowerPayload failed: %v", err)
	}

	if payload.SN != "DP3-SN" {
		t.Fatalf("SN = %q, want DP3-SN", payload.SN)
	}
	if payload.CmdID != 17 || payload.CmdFunc != 254 || payload.DirDest != 1 || payload.DirSrc != 1 || payload.Dest != 2 || !payload.NeedAck {
		t.Fatalf("unexpected command envelope: %+v", payload)
	}
	if payload.Params[candidateACChargePowerParam] != 1000 {
		t.Fatalf("params[%q] = %d, want 1000", candidateACChargePowerParam, payload.Params[candidateACChargePowerParam])
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	want := `{"sn":"DP3-SN","cmdId":17,"cmdFunc":254,"dirDest":1,"dirSrc":1,"dest":2,"needAck":true,"params":{"cfgPlugInInfoAcInChgPowMax":1000}}`
	if string(body) != want {
		t.Fatalf("json = %s, want %s", body, want)
	}
}

func TestBuildSetACChargePowerPayloadRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name     string
		deviceSN string
		watts    int
	}{
		{name: "empty serial", deviceSN: "", watts: 1000},
		{name: "zero watts", deviceSN: "DP3-SN", watts: 0},
		{name: "negative watts", deviceSN: "DP3-SN", watts: -100},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := buildSetACChargePowerPayload(tt.deviceSN, tt.watts); err == nil {
				t.Fatal("buildSetACChargePowerPayload returned nil error, want error")
			}
		})
	}
}
