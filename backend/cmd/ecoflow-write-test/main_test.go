package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRunDryRunDoesNotRequireEnvironmentOrSendRequests(t *testing.T) {
	var out bytes.Buffer

	err := run(context.Background(), []string{"--watts", "1000", "--expected-current-limit", "1500", "--reserve-soc", "82", "--expected-current-reserve", "80"}, emptyEnv, &out)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if !strings.Contains(out.String(), "dry-run") || !strings.Contains(out.String(), "backup reserve to 82%") || !strings.Contains(out.String(), "no request sent") {
		t.Fatalf("output = %q, want dry-run no request sent", out.String())
	}
}

func TestRunDryRunAllowsExplicitZeroReserve(t *testing.T) {
	var out bytes.Buffer

	err := run(context.Background(), []string{"--watts", "1000", "--expected-current-limit", "1500", "--reserve-soc", "0", "--expected-current-reserve", "30"}, emptyEnv, &out)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if !strings.Contains(out.String(), "backup reserve to 0%") {
		t.Fatalf("output = %q, want explicit zero reserve", out.String())
	}
}

func TestRunExecuteRequiresSafetyEnvironmentBeforeRequests(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(map[string]string)
		wantErr string
	}{
		{
			name:    "mock mode must be false",
			mutate:  func(env map[string]string) { env["MOCK_MODE"] = "true" },
			wantErr: "MOCK_MODE",
		},
		{
			name:    "simulation mode must be false",
			mutate:  func(env map[string]string) { env["SIMULATION_MODE"] = "true" },
			wantErr: "SIMULATION_MODE",
		},
		{
			name:    "real control must be enabled",
			mutate:  func(env map[string]string) { env["ENABLE_REAL_CONTROL"] = "false" },
			wantErr: "ENABLE_REAL_CONTROL",
		},
		{
			name:    "auto control must stay disabled",
			mutate:  func(env map[string]string) { env["AUTO_CONTROL_ENABLED"] = "true" },
			wantErr: "AUTO_CONTROL_ENABLED",
		},
		{
			name:    "confirmation is required",
			mutate:  func(env map[string]string) { delete(env, "CONFIRM_ECOFLOW_WRITE") },
			wantErr: "CONFIRM_ECOFLOW_WRITE",
		},
		{
			name:    "access key is required",
			mutate:  func(env map[string]string) { delete(env, "ECOFLOW_ACCESS_KEY") },
			wantErr: "ECOFLOW_ACCESS_KEY",
		},
		{
			name:    "secret key is required",
			mutate:  func(env map[string]string) { delete(env, "ECOFLOW_SECRET_KEY") },
			wantErr: "ECOFLOW_SECRET_KEY",
		},
		{
			name:    "device serial is required",
			mutate:  func(env map[string]string) { delete(env, "ECOFLOW_DEVICE_SN") },
			wantErr: "ECOFLOW_DEVICE_SN",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			requests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests++
				w.WriteHeader(http.StatusOK)
			}))
			defer server.Close()

			env := validEnv(server.URL)
			tt.mutate(env)

			err := run(context.Background(), []string{"--execute", "--watts", "1000", "--expected-current-limit", "1500"}, mapEnv(env), io.Discard)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("run error = %v, want %s", err, tt.wantErr)
			}
			if requests != 0 {
				t.Fatalf("requests = %d, want 0", requests)
			}
		})
	}
}

func TestRunExecuteRejectsNumericOnlySerialBeforeRequests(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	env := validEnv(server.URL)
	env["ECOFLOW_DEVICE_SN"] = "1234567890"

	err := run(context.Background(), []string{"--execute", "--watts", "1000", "--expected-current-limit", "1500"}, mapEnv(env), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "numeric-only") {
		t.Fatalf("run error = %v, want numeric-only serial rejection", err)
	}
	if requests != 0 {
		t.Fatalf("requests = %d, want 0", requests)
	}
}

func TestRunExecuteRefusesWhenCurrentLimitDoesNotMatch(t *testing.T) {
	var getRequests, putRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getRequests++
			writeQuotaAll(t, w, 1200, 25)
		case http.MethodPut:
			putRequests++
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected method: %s", r.Method)
		}
	}))
	defer server.Close()

	err := run(context.Background(), []string{"--execute", "--watts", "1000", "--expected-current-limit", "1500"}, mapEnv(validEnv(server.URL)), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "current AC charge limit is 1200W") {
		t.Fatalf("run error = %v, want current limit mismatch", err)
	}
	if getRequests != 1 {
		t.Fatalf("GET requests = %d, want 1", getRequests)
	}
	if putRequests != 0 {
		t.Fatalf("PUT requests = %d, want 0", putRequests)
	}
}

func TestRunExecuteRefusesWhenCurrentReserveDoesNotMatch(t *testing.T) {
	var getRequests, putRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getRequests++
			writeQuotaAll(t, w, 1500, 25)
		case http.MethodPut:
			putRequests++
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected method: %s", r.Method)
		}
	}))
	defer server.Close()

	err := run(context.Background(), []string{"--execute", "--watts", "1000", "--expected-current-limit", "1500", "--reserve-soc", "30", "--expected-current-reserve", "20"}, mapEnv(validEnv(server.URL)), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "current backup reserve SOC is 25%") {
		t.Fatalf("run error = %v, want current reserve mismatch", err)
	}
	if getRequests != 1 {
		t.Fatalf("GET requests = %d, want 1", getRequests)
	}
	if putRequests != 0 {
		t.Fatalf("PUT requests = %d, want 0", putRequests)
	}
}

func TestRunExecuteSendsOneCommandAfterReadCheck(t *testing.T) {
	var getRequests, putRequests int
	var putBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getRequests++
			if r.URL.Path != "/iot-open/sign/device/quota/all" {
				t.Fatalf("GET path = %s", r.URL.Path)
			}
			writeQuotaAll(t, w, 1500, 25)
		case http.MethodPut:
			putRequests++
			if r.URL.Path != "/iot-open/sign/device/quota" {
				t.Fatalf("PUT path = %s", r.URL.Path)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("ReadAll failed: %v", err)
			}
			putBody = string(body)
			_ = json.NewEncoder(w).Encode(map[string]string{"code": "0", "message": "Success"})
		default:
			t.Fatalf("unexpected method: %s", r.Method)
		}
	}))
	defer server.Close()

	var out bytes.Buffer
	err := run(context.Background(), []string{"--execute", "--watts", "1000", "--expected-current-limit", "1500"}, mapEnv(validEnv(server.URL)), &out)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if getRequests != 1 || putRequests != 1 {
		t.Fatalf("requests GET=%d PUT=%d, want 1/1", getRequests, putRequests)
	}
	wantBody := `{"sn":"DP3-SN","cmdId":17,"cmdFunc":254,"dirDest":1,"dirSrc":1,"dest":2,"needAck":true,"params":{"cfgPlugInInfoAcInChgPowMax":1000}}`
	if putBody != wantBody {
		t.Fatalf("PUT body = %s, want %s", putBody, wantBody)
	}
	if !strings.Contains(out.String(), "sent EcoFlow command(s): AC charge power 1000W") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestRunExecuteSendsChargeAndReserveCommandsAfterReadCheck(t *testing.T) {
	var getRequests, putRequests int
	var putBodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getRequests++
			writeQuotaAll(t, w, 1500, 25)
		case http.MethodPut:
			putRequests++
			if r.URL.Path != "/iot-open/sign/device/quota" {
				t.Fatalf("PUT path = %s", r.URL.Path)
			}
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("ReadAll failed: %v", err)
			}
			putBodies = append(putBodies, string(body))
			_ = json.NewEncoder(w).Encode(map[string]string{"code": "0", "message": "Success"})
		default:
			t.Fatalf("unexpected method: %s", r.Method)
		}
	}))
	defer server.Close()

	var out bytes.Buffer
	err := run(context.Background(), []string{"--execute", "--watts", "1000", "--expected-current-limit", "1500", "--reserve-soc", "30", "--expected-current-reserve", "25"}, mapEnv(validEnv(server.URL)), &out)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if getRequests != 1 || putRequests != 2 {
		t.Fatalf("requests GET=%d PUT=%d, want 1/2", getRequests, putRequests)
	}
	wantChargeBody := `{"sn":"DP3-SN","cmdId":17,"cmdFunc":254,"dirDest":1,"dirSrc":1,"dest":2,"needAck":true,"params":{"cfgPlugInInfoAcInChgPowMax":1000}}`
	wantReserveBody := `{"sn":"DP3-SN","cmdId":17,"cmdFunc":254,"dirDest":1,"dirSrc":1,"dest":2,"needAck":true,"params":{"cfgBackupReverseSoc":30}}`
	if len(putBodies) != 2 || putBodies[0] != wantChargeBody || putBodies[1] != wantReserveBody {
		t.Fatalf("PUT bodies = %+v, want charge then reserve", putBodies)
	}
	if !strings.Contains(out.String(), "backup reserve 30%") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestRunExecuteSendsZeroReserveCommandAfterReadCheck(t *testing.T) {
	var getRequests, putRequests int
	var putBodies []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getRequests++
			writeQuotaAll(t, w, 1500, 25)
		case http.MethodPut:
			putRequests++
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("ReadAll failed: %v", err)
			}
			putBodies = append(putBodies, string(body))
			_ = json.NewEncoder(w).Encode(map[string]string{"code": "0", "message": "Success"})
		default:
			t.Fatalf("unexpected method: %s", r.Method)
		}
	}))
	defer server.Close()

	var out bytes.Buffer
	err := run(context.Background(), []string{"--execute", "--watts", "1000", "--expected-current-limit", "1500", "--reserve-soc", "0", "--expected-current-reserve", "25"}, mapEnv(validEnv(server.URL)), &out)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if getRequests != 1 || putRequests != 2 {
		t.Fatalf("requests GET=%d PUT=%d, want 1/2", getRequests, putRequests)
	}
	wantReserveBody := `{"sn":"DP3-SN","cmdId":17,"cmdFunc":254,"dirDest":1,"dirSrc":1,"dest":2,"needAck":true,"params":{"cfgBackupReverseSoc":0}}`
	if len(putBodies) != 2 || putBodies[1] != wantReserveBody {
		t.Fatalf("PUT bodies = %+v, want second reserve zero", putBodies)
	}
	if !strings.Contains(out.String(), "backup reserve 0%") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestRunExecuteReportsPartialSuccessWhenReserveCommandFails(t *testing.T) {
	var getRequests, putRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getRequests++
			writeQuotaAll(t, w, 1500, 25)
		case http.MethodPut:
			putRequests++
			if putRequests == 2 {
				_ = json.NewEncoder(w).Encode(map[string]string{"code": "500", "message": "reserve rejected"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]string{"code": "0", "message": "Success"})
		default:
			t.Fatalf("unexpected method: %s", r.Method)
		}
	}))
	defer server.Close()

	err := run(context.Background(), []string{"--execute", "--watts", "1000", "--expected-current-limit", "1500", "--reserve-soc", "30", "--expected-current-reserve", "25"}, mapEnv(validEnv(server.URL)), io.Discard)
	if err == nil {
		t.Fatal("run succeeded, want reserve failure")
	}
	for _, want := range []string{"prior command(s) were already sent (AC charge power 1000W)", "reserve rejected", "verify current EcoFlow settings before retrying"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("run error = %v, want contains %q", err, want)
		}
	}
	if getRequests != 1 || putRequests != 2 {
		t.Fatalf("requests GET=%d PUT=%d, want 1/2", getRequests, putRequests)
	}
}

func TestRunExecuteReportsReserveFailureWithoutPartialSuccessWhenNoPriorCommand(t *testing.T) {
	var getRequests, putRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getRequests++
			writeQuotaAll(t, w, 1500, 25)
		case http.MethodPut:
			putRequests++
			_ = json.NewEncoder(w).Encode(map[string]string{"code": "500", "message": "reserve rejected"})
		default:
			t.Fatalf("unexpected method: %s", r.Method)
		}
	}))
	defer server.Close()

	err := run(context.Background(), []string{"--execute", "--reserve-soc", "30", "--expected-current-reserve", "25"}, mapEnv(validEnv(server.URL)), io.Discard)
	if err == nil {
		t.Fatal("run succeeded, want reserve failure")
	}
	for _, want := range []string{"set EcoFlow backup reserve SOC", "reserve rejected"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("run error = %v, want contains %q", err, want)
		}
	}
	if strings.Contains(err.Error(), "prior command(s) were already sent") {
		t.Fatalf("run error = %v, want no partial success warning", err)
	}
	if getRequests != 1 || putRequests != 1 {
		t.Fatalf("requests GET=%d PUT=%d, want 1/1", getRequests, putRequests)
	}
}

func TestRunExecuteDisablesTOUAfterReadCheck(t *testing.T) {
	var getRequests, putRequests int
	var putBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getRequests++
			writeQuotaAll(t, w, 1500, 85)
		case http.MethodPut:
			putRequests++
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("ReadAll failed: %v", err)
			}
			putBody = string(body)
			_ = json.NewEncoder(w).Encode(map[string]string{"code": "0", "message": "Success"})
		default:
			t.Fatalf("unexpected method: %s", r.Method)
		}
	}))
	defer server.Close()

	var out bytes.Buffer
	err := run(context.Background(), []string{
		"--execute",
		"--disable-energy-modes",
		"--expected-tou-mode=true",
		"--expected-self-powered-mode=false",
		"--expected-scheduled-mode=false",
		"--expected-intelligent-schedule-mode=false",
	}, mapEnv(validEnv(server.URL)), &out)
	if err != nil {
		t.Fatalf("run failed: %v", err)
	}
	if getRequests != 1 || putRequests != 1 {
		t.Fatalf("requests GET=%d PUT=%d, want 1/1", getRequests, putRequests)
	}
	wantBody := `{"sn":"DP3-SN","cmdId":17,"cmdFunc":254,"dirDest":1,"dirSrc":1,"dest":2,"needAck":true,"params":{"cfgEnergyStrategyOperateMode":{"operateIntelligentScheduleModeOpen":false,"operateScheduledOpen":false,"operateSelfPoweredOpen":false,"operateTouModeOpen":false}}}`
	if putBody != wantBody {
		t.Fatalf("PUT body = %s, want %s", putBody, wantBody)
	}
	if !strings.Contains(out.String(), "energy strategy modes false") {
		t.Fatalf("output = %q", out.String())
	}
}

func TestRunExecuteRefusesWhenCurrentTOUDoesNotMatch(t *testing.T) {
	var getRequests, putRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			getRequests++
			writeQuotaAllWithEnergyModes(t, w, 1500, 85, false, false, false, false)
		case http.MethodPut:
			putRequests++
			w.WriteHeader(http.StatusOK)
		default:
			t.Fatalf("unexpected method: %s", r.Method)
		}
	}))
	defer server.Close()

	err := run(context.Background(), []string{
		"--execute",
		"--disable-energy-modes",
		"--expected-tou-mode=true",
		"--expected-self-powered-mode=false",
		"--expected-scheduled-mode=false",
		"--expected-intelligent-schedule-mode=false",
	}, mapEnv(validEnv(server.URL)), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "current TOU mode is false") {
		t.Fatalf("run error = %v, want current TOU mismatch", err)
	}
	if getRequests != 1 || putRequests != 0 {
		t.Fatalf("requests GET=%d PUT=%d, want 1/0", getRequests, putRequests)
	}
}

func writeQuotaAll(t *testing.T, w http.ResponseWriter, acChargeLimitW int, backupReserveSoc int) {
	t.Helper()
	writeQuotaAllWithEnergyModes(t, w, acChargeLimitW, backupReserveSoc, true, false, false, false)
}

func writeQuotaAllWithEnergyModes(t *testing.T, w http.ResponseWriter, acChargeLimitW int, backupReserveSoc int, touMode bool, selfPowered bool, scheduled bool, intelligent bool) {
	t.Helper()
	_ = json.NewEncoder(w).Encode(map[string]any{
		"code":    "0",
		"message": "Success",
		"data": map[string]any{
			"cmsBattSoc":              80,
			"powInSumW":               0,
			"powOutSumW":              0,
			"plugInInfoAcInChgPowMax": acChargeLimitW,
			"energyBackupStartSoc":    backupReserveSoc,
			"energyStrategyOperateMode.operateTouModeOpen":                 touMode,
			"energyStrategyOperateMode.operateSelfPoweredOpen":             selfPowered,
			"energyStrategyOperateMode.operateScheduledOpen":               scheduled,
			"energyStrategyOperateMode.operateIntelligentScheduleModeOpen": intelligent,
		},
	})
}

func validEnv(baseURL string) map[string]string {
	return map[string]string{
		"MOCK_MODE":             "false",
		"SIMULATION_MODE":       "false",
		"ENABLE_REAL_CONTROL":   "true",
		"AUTO_CONTROL_ENABLED":  "false",
		"CONFIRM_ECOFLOW_WRITE": confirmWriteValue,
		"ECOFLOW_ACCESS_KEY":    "access-key",
		"ECOFLOW_SECRET_KEY":    "secret-key",
		"ECOFLOW_DEVICE_SN":     "DP3-SN",
		"ECOFLOW_BASE_URL":      baseURL,
	}
}

func mapEnv(values map[string]string) envGetter {
	return func(key string) string {
		return values[key]
	}
}

func emptyEnv(string) string {
	return ""
}
