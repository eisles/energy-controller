package ecoflow

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSignedWriteClientGuardBlocksWithoutRequest(t *testing.T) {
	tests := []struct {
		name    string
		guards  WriteGuards
		cfg     Config
		wantErr string
	}{
		{
			name:    "mock mode",
			guards:  WriteGuards{MockMode: true, SimulationMode: false, EnableRealControl: true, AutoControlEnabled: true},
			wantErr: "MOCK_MODE=true",
		},
		{
			name:    "simulation mode",
			guards:  WriteGuards{MockMode: false, SimulationMode: true, EnableRealControl: true, AutoControlEnabled: true},
			wantErr: "SIMULATION_MODE=true",
		},
		{
			name:    "real control disabled",
			guards:  WriteGuards{MockMode: false, SimulationMode: false, EnableRealControl: false, AutoControlEnabled: true},
			wantErr: "ENABLE_REAL_CONTROL=false",
		},
		{
			name:    "auto control disabled",
			guards:  WriteGuards{MockMode: false, SimulationMode: false, EnableRealControl: true, AutoControlEnabled: false},
			wantErr: "AUTO_CONTROL_ENABLED=false",
		},
		{
			name:    "access key empty",
			guards:  WriteGuards{MockMode: false, SimulationMode: false, EnableRealControl: true, AutoControlEnabled: true},
			cfg:     Config{AccessKey: "", SecretKey: "secret-key", DeviceSN: "DP3-SN"},
			wantErr: "access key, secret key, or device SN is empty",
		},
		{
			name:    "secret key empty",
			guards:  WriteGuards{MockMode: false, SimulationMode: false, EnableRealControl: true, AutoControlEnabled: true},
			cfg:     Config{AccessKey: "access-key", SecretKey: "", DeviceSN: "DP3-SN"},
			wantErr: "access key, secret key, or device SN is empty",
		},
		{
			name:    "device serial empty",
			guards:  WriteGuards{MockMode: false, SimulationMode: false, EnableRealControl: true, AutoControlEnabled: true},
			cfg:     Config{AccessKey: "access-key", SecretKey: "secret-key", DeviceSN: ""},
			wantErr: "access key, secret key, or device SN is empty",
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

			cfg := tt.cfg
			if cfg.AccessKey == "" && cfg.SecretKey == "" && cfg.DeviceSN == "" {
				cfg = Config{
					AccessKey: "access-key",
					SecretKey: "secret-key",
					DeviceSN:  "DP3-SN",
				}
			}
			cfg.BaseURL = server.URL
			client := NewSignedWriteClient(cfg, tt.guards)

			err := client.SetACChargePower(context.Background(), 1000)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("SetACChargePower error = %v, want contains %q", err, tt.wantErr)
			}
			if requests != 0 {
				t.Fatalf("requests = %d, want 0", requests)
			}
		})
	}
}

func TestSignedWriteClientSendsOneSignedRequestWhenGuardsPass(t *testing.T) {
	var gotBody string
	var gotHeaders http.Header
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodPut {
			t.Fatalf("method = %s, want PUT", r.Method)
		}
		if r.URL.Path != "/iot-open/sign/device/quota" {
			t.Fatalf("path = %s, want /iot-open/sign/device/quota", r.URL.Path)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll failed: %v", err)
		}
		gotBody = string(body)
		gotHeaders = r.Header.Clone()
		_ = json.NewEncoder(w).Encode(map[string]string{"code": "0", "message": "Success"})
	}))
	defer server.Close()

	client := NewSignedWriteClient(Config{
		AccessKey: "access-key",
		SecretKey: "secret-key",
		DeviceSN:  "DP3-SN",
		BaseURL:   server.URL,
	}, WriteGuards{
		MockMode:           false,
		SimulationMode:     false,
		EnableRealControl:  true,
		AutoControlEnabled: true,
	})
	client.client.nonce = func() string { return "123456" }
	client.client.now = func() time.Time { return time.UnixMilli(1700000000000) }

	if err := client.SetACChargePower(context.Background(), 1000); err != nil {
		t.Fatalf("SetACChargePower failed: %v", err)
	}

	if requests != 1 {
		t.Fatalf("requests = %d, want 1", requests)
	}
	wantBody := `{"sn":"DP3-SN","cmdId":17,"cmdFunc":254,"dirDest":1,"dirSrc":1,"dest":2,"needAck":true,"params":{"cfgPlugInInfoAcInChgPowMax":1000}}`
	if gotBody != wantBody {
		t.Fatalf("body = %s, want %s", gotBody, wantBody)
	}
	if gotHeaders.Get("accessKey") != "access-key" || gotHeaders.Get("nonce") != "123456" || gotHeaders.Get("timestamp") != "1700000000000" {
		t.Fatalf("unexpected auth headers: accessKey=%q nonce=%q timestamp=%q", gotHeaders.Get("accessKey"), gotHeaders.Get("nonce"), gotHeaders.Get("timestamp"))
	}
	wantSign := "5e710b7bda56e00de61d9c3190b14a77eb5275230fecc0fb010a89cc481a69f2"
	if gotHeaders.Get("sign") != wantSign {
		t.Fatalf("sign = %s, want %s", gotHeaders.Get("sign"), wantSign)
	}
}

func TestSignedWriteClientReturnsAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"code": "500", "message": "device rejected command"})
	}))
	defer server.Close()

	client := NewSignedWriteClient(Config{
		AccessKey: "access-key",
		SecretKey: "secret-key",
		DeviceSN:  "DP3-SN",
		BaseURL:   server.URL,
	}, WriteGuards{
		MockMode:           false,
		SimulationMode:     false,
		EnableRealControl:  true,
		AutoControlEnabled: true,
	})

	err := client.SetACChargePower(context.Background(), 1000)
	if err == nil || !strings.Contains(err.Error(), "device rejected command") {
		t.Fatalf("SetACChargePower error = %v, want device rejected command", err)
	}
}

func TestSignedWriteClientStopOrMinimizeIsNotImplemented(t *testing.T) {
	client := NewSignedWriteClient(Config{}, WriteGuards{})

	err := client.StopOrMinimizeCharging(context.Background())
	if err == nil || !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("StopOrMinimizeCharging error = %v, want not implemented", err)
	}
}
