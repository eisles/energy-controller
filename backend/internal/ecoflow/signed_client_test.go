package ecoflow

import (
	"context"
	"io"
	"testing"
	"time"
)

func TestSignIncludesSortedParamsAndAuthFields(t *testing.T) {
	got := sign(map[string]string{"sn": "device-sn"}, "access-key", "secret-key", "123456", "1700000000000")
	want := "5230696a09e852661a82b1c3d3169e1ea981fa7c2f9998a73f2b9460e3e1aee3"
	if got != want {
		t.Fatalf("sign = %s, want %s", got, want)
	}
}

func TestFlattenSigningParamsForCommandPayload(t *testing.T) {
	payload, err := buildSetACChargePowerPayload("DP3-SN", 1000)
	if err != nil {
		t.Fatalf("buildSetACChargePowerPayload failed: %v", err)
	}

	params, err := flattenSigningParams(payload)
	if err != nil {
		t.Fatalf("flattenSigningParams failed: %v", err)
	}

	want := map[string]string{
		"sn":                                "DP3-SN",
		"cmdId":                             "17",
		"cmdFunc":                           "254",
		"dirDest":                           "1",
		"dirSrc":                            "1",
		"dest":                              "2",
		"needAck":                           "true",
		"params.cfgPlugInInfoAcInChgPowMax": "1000",
	}
	if len(params) != len(want) {
		t.Fatalf("flattened param count = %d, want %d: %#v", len(params), len(want), params)
	}
	for key, wantValue := range want {
		if params[key] != wantValue {
			t.Fatalf("params[%q] = %q, want %q", key, params[key], wantValue)
		}
	}
}

func TestFlattenSigningParamsForEnergyModePayload(t *testing.T) {
	payload, err := buildSetTOUModePayload("DP3-SN", false)
	if err != nil {
		t.Fatalf("buildSetTOUModePayload failed: %v", err)
	}

	params, err := flattenSigningParams(payload)
	if err != nil {
		t.Fatalf("flattenSigningParams failed: %v", err)
	}

	want := map[string]string{
		"sn":      "DP3-SN",
		"cmdId":   "17",
		"cmdFunc": "254",
		"dirDest": "1",
		"dirSrc":  "1",
		"dest":    "2",
		"needAck": "true",
		"params.cfgEnergyStrategyOperateMode.operateTouModeOpen":                 "false",
		"params.cfgEnergyStrategyOperateMode.operateSelfPoweredOpen":             "false",
		"params.cfgEnergyStrategyOperateMode.operateScheduledOpen":               "false",
		"params.cfgEnergyStrategyOperateMode.operateIntelligentScheduleModeOpen": "false",
	}
	if len(params) != len(want) {
		t.Fatalf("flattened param count = %d, want %d: %#v", len(params), len(want), params)
	}
	for key, wantValue := range want {
		if params[key] != wantValue {
			t.Fatalf("params[%q] = %q, want %q", key, params[key], wantValue)
		}
	}
}

func TestNewSignedPUTRequestBuildsRequestWithoutSending(t *testing.T) {
	payload, err := buildSetACChargePowerPayload("DP3-SN", 1000)
	if err != nil {
		t.Fatalf("buildSetACChargePowerPayload failed: %v", err)
	}
	client := NewSignedClient(Config{
		AccessKey: "access-key",
		SecretKey: "secret-key",
		BaseURL:   "https://api-e.ecoflow.test",
	})
	client.nonce = func() string { return "123456" }
	client.now = func() time.Time { return time.UnixMilli(1700000000000) }

	req, err := client.newSignedPUTRequest(context.Background(), "/iot-open/sign/device/quota", payload)
	if err != nil {
		t.Fatalf("newSignedPUTRequest failed: %v", err)
	}

	if req.Method != "PUT" {
		t.Fatalf("Method = %s, want PUT", req.Method)
	}
	if req.URL.String() != "https://api-e.ecoflow.test/iot-open/sign/device/quota" {
		t.Fatalf("URL = %s", req.URL.String())
	}
	if req.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", req.Header.Get("Content-Type"))
	}
	if req.Header.Get("accessKey") != "access-key" || req.Header.Get("nonce") != "123456" || req.Header.Get("timestamp") != "1700000000000" {
		t.Fatalf("unexpected auth headers: accessKey=%q nonce=%q timestamp=%q", req.Header.Get("accessKey"), req.Header.Get("nonce"), req.Header.Get("timestamp"))
	}
	wantSign := "5e710b7bda56e00de61d9c3190b14a77eb5275230fecc0fb010a89cc481a69f2"
	if req.Header.Get("sign") != wantSign {
		t.Fatalf("sign = %s, want %s", req.Header.Get("sign"), wantSign)
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("ReadAll body failed: %v", err)
	}
	wantBody := `{"sn":"DP3-SN","cmdId":17,"cmdFunc":254,"dirDest":1,"dirSrc":1,"dest":2,"needAck":true,"params":{"cfgPlugInInfoAcInChgPowMax":1000}}`
	if string(body) != wantBody {
		t.Fatalf("body = %s, want %s", body, wantBody)
	}
}

func TestNewSignedPUTRequestBuildsEnergyModeRequestWithoutSending(t *testing.T) {
	payload, err := buildSetTOUModePayload("DP3-SN", false)
	if err != nil {
		t.Fatalf("buildSetTOUModePayload failed: %v", err)
	}
	client := NewSignedClient(Config{
		AccessKey: "access-key",
		SecretKey: "secret-key",
		BaseURL:   "https://api-e.ecoflow.test",
	})
	client.nonce = func() string { return "123456" }
	client.now = func() time.Time { return time.UnixMilli(1700000000000) }

	req, err := client.newSignedPUTRequest(context.Background(), "/iot-open/sign/device/quota", payload)
	if err != nil {
		t.Fatalf("newSignedPUTRequest failed: %v", err)
	}

	if req.Method != "PUT" {
		t.Fatalf("Method = %s, want PUT", req.Method)
	}
	wantSign := "e83a89c0e70b63f8c7edc12cc8639e89747c0c292dd08990132eefcdd9923ed2"
	if req.Header.Get("sign") != wantSign {
		t.Fatalf("sign = %s, want %s", req.Header.Get("sign"), wantSign)
	}
	body, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatalf("ReadAll body failed: %v", err)
	}
	wantBody := `{"sn":"DP3-SN","cmdId":17,"cmdFunc":254,"dirDest":1,"dirSrc":1,"dest":2,"needAck":true,"params":{"cfgEnergyStrategyOperateMode":{"operateIntelligentScheduleModeOpen":false,"operateScheduledOpen":false,"operateSelfPoweredOpen":false,"operateTouModeOpen":false}}}`
	if string(body) != wantBody {
		t.Fatalf("body = %s, want %s", body, wantBody)
	}
}
