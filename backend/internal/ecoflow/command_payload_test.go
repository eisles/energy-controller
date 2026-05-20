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

func TestBuildSetBackupReservePayload(t *testing.T) {
	payload, err := buildSetBackupReservePayload("DP3-SN", 82)
	if err != nil {
		t.Fatalf("buildSetBackupReservePayload failed: %v", err)
	}

	if payload.SN != "DP3-SN" {
		t.Fatalf("SN = %q, want DP3-SN", payload.SN)
	}
	if payload.CmdID != 17 || payload.CmdFunc != 254 || payload.DirDest != 1 || payload.DirSrc != 1 || payload.Dest != 2 || !payload.NeedAck {
		t.Fatalf("unexpected command envelope: %+v", payload)
	}
	if payload.Params[candidateBackupReserveParam] != 82 {
		t.Fatalf("params[%q] = %d, want 82", candidateBackupReserveParam, payload.Params[candidateBackupReserveParam])
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	want := `{"sn":"DP3-SN","cmdId":17,"cmdFunc":254,"dirDest":1,"dirSrc":1,"dest":2,"needAck":true,"params":{"cfgBackupReverseSoc":82}}`
	if string(body) != want {
		t.Fatalf("json = %s, want %s", body, want)
	}
}

func TestBuildSetTOUModePayload(t *testing.T) {
	payload, err := buildSetTOUModePayload("DP3-SN", false)
	if err != nil {
		t.Fatalf("buildSetTOUModePayload failed: %v", err)
	}

	if payload.SN != "DP3-SN" {
		t.Fatalf("SN = %q, want DP3-SN", payload.SN)
	}
	if payload.CmdID != 17 || payload.CmdFunc != 254 || payload.DirDest != 1 || payload.DirSrc != 1 || payload.Dest != 2 || !payload.NeedAck {
		t.Fatalf("unexpected command envelope: %+v", payload)
	}
	mode, ok := payload.Params[candidateEnergyModeParam].(map[string]any)
	if !ok {
		t.Fatalf("params[%q] = %T, want map", candidateEnergyModeParam, payload.Params[candidateEnergyModeParam])
	}
	if mode["operateTouModeOpen"] != false || mode["operateSelfPoweredOpen"] != false || mode["operateScheduledOpen"] != false || mode["operateIntelligentScheduleModeOpen"] != false {
		t.Fatalf("params[%q] = %+v, want all energy strategy modes false", candidateEnergyModeParam, mode)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	want := `{"sn":"DP3-SN","cmdId":17,"cmdFunc":254,"dirDest":1,"dirSrc":1,"dest":2,"needAck":true,"params":{"cfgEnergyStrategyOperateMode":{"operateIntelligentScheduleModeOpen":false,"operateScheduledOpen":false,"operateSelfPoweredOpen":false,"operateTouModeOpen":false}}}`
	if string(body) != want {
		t.Fatalf("json = %s, want %s", body, want)
	}
}

func TestBuildSetTOUModePayloadRejectsInvalidInput(t *testing.T) {
	if _, err := buildSetTOUModePayload("", false); err == nil {
		t.Fatal("buildSetTOUModePayload returned nil error, want error")
	}
}

func TestBuildSetSelfPoweredModePayload(t *testing.T) {
	payload, err := buildSetSelfPoweredModePayload("DP3-SN", true)
	if err != nil {
		t.Fatalf("buildSetSelfPoweredModePayload failed: %v", err)
	}

	if payload.SN != "DP3-SN" {
		t.Fatalf("SN = %q, want DP3-SN", payload.SN)
	}
	if payload.CmdID != 17 || payload.CmdFunc != 254 || payload.DirDest != 1 || payload.DirSrc != 1 || payload.Dest != 2 || !payload.NeedAck {
		t.Fatalf("unexpected command envelope: %+v", payload)
	}
	mode, ok := payload.Params[candidateEnergyModeParam].(map[string]any)
	if !ok {
		t.Fatalf("params[%q] = %T, want map", candidateEnergyModeParam, payload.Params[candidateEnergyModeParam])
	}
	if mode["operateTouModeOpen"] != false || mode["operateSelfPoweredOpen"] != true || mode["operateScheduledOpen"] != false || mode["operateIntelligentScheduleModeOpen"] != false {
		t.Fatalf("params[%q] = %+v, want self-powered true and other modes false", candidateEnergyModeParam, mode)
	}

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	want := `{"sn":"DP3-SN","cmdId":17,"cmdFunc":254,"dirDest":1,"dirSrc":1,"dest":2,"needAck":true,"params":{"cfgEnergyStrategyOperateMode":{"operateIntelligentScheduleModeOpen":false,"operateScheduledOpen":false,"operateSelfPoweredOpen":true,"operateTouModeOpen":false}}}`
	if string(body) != want {
		t.Fatalf("json = %s, want %s", body, want)
	}
}

func TestBuildSetSelfPoweredModePayloadRejectsInvalidInput(t *testing.T) {
	if _, err := buildSetSelfPoweredModePayload("", true); err == nil {
		t.Fatal("buildSetSelfPoweredModePayload returned nil error, want error")
	}
}

func TestBuildSetBackupReservePayloadRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name     string
		deviceSN string
		percent  int
	}{
		{name: "empty serial", deviceSN: "", percent: 80},
		{name: "negative percent", deviceSN: "DP3-SN", percent: -1},
		{name: "over 100 percent", deviceSN: "DP3-SN", percent: 101},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := buildSetBackupReservePayload(tt.deviceSN, tt.percent); err == nil {
				t.Fatal("buildSetBackupReservePayload returned nil error, want error")
			}
		})
	}
}
