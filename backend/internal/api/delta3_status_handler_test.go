package api

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/eisles/energy-controller/backend/internal/config"
	"github.com/eisles/energy-controller/backend/internal/ecoflowdelta3"
)

func TestReadDelta3StatusDisabledDoesNotProbe(t *testing.T) {
	response := readDelta3Status(context.Background(), config.Config{Delta3ReadEnabled: false}, panicDelta3Client{}, nil)
	if response.Available {
		t.Fatalf("Available = true, want false")
	}
	if !strings.Contains(response.LastError, "ECOFLOW_DELTA3_READ_ENABLED=false") {
		t.Fatalf("LastError = %q", response.LastError)
	}
}

func TestReadDelta3StatusRequiresCredentialsBeforeProbe(t *testing.T) {
	response := readDelta3Status(context.Background(), config.Config{
		Delta3ReadEnabled: true,
		Delta3DeviceType:  "DELTA_3",
		Delta3Timeout:     time.Second,
	}, panicDelta3Client{}, nil)
	if response.Available {
		t.Fatalf("Available = true, want false")
	}
	if !strings.Contains(response.LastError, "ECOFLOW_PRIVATE_EMAIL") {
		t.Fatalf("LastError = %q, want missing credential names", response.LastError)
	}
}

func TestReadDelta3StatusMapsReadOnlyFields(t *testing.T) {
	soc := 82
	acInW := 100
	acOutW := 380
	acLimitW := 100
	gridBypassDisabled := false
	acOutputEnabled := true
	response := readDelta3Status(context.Background(), validDelta3Config(), fakeDelta3Client{status: ecoflowdelta3.Status{
		DeviceType:         "DELTA_3",
		CMSBatterySoc:      &soc,
		ACInW:              &acInW,
		ACOutW:             &acOutW,
		ACChargeLimitW:     &acLimitW,
		GridBypassDisabled: &gridBypassDisabled,
		ACOutputEnabled:    &acOutputEnabled,
	}}, nil)
	if !response.Available {
		t.Fatalf("Available = false, lastError=%q", response.LastError)
	}
	assertIntPtrResponse(t, "SOC", response.SOC, 82)
	assertIntPtrResponse(t, "ACInW", response.ACInW, 100)
	assertIntPtrResponse(t, "ACOutW", response.ACOutW, 380)
	assertIntPtrResponse(t, "ACChargeLimitW", response.ACChargeLimitW, 100)
	if response.GridBypassDisabled == nil || *response.GridBypassDisabled {
		t.Fatalf("GridBypassDisabled = %v, want false", response.GridBypassDisabled)
	}
	if response.ACOutputEnabled == nil || !*response.ACOutputEnabled {
		t.Fatalf("ACOutputEnabled = %v, want true", response.ACOutputEnabled)
	}
}

func validDelta3Config() config.Config {
	return config.Config{
		Delta3ReadEnabled:     true,
		Delta3PrivateAPIHost:  "api.test",
		Delta3PrivateEmail:    "user@example.com",
		Delta3PrivatePassword: "secret",
		Delta3DeviceSN:        "SN123",
		Delta3DeviceType:      "DELTA_3",
		Delta3Timeout:         time.Second,
	}
}

type fakeDelta3Client struct {
	status ecoflowdelta3.Status
}

func (f fakeDelta3Client) Probe(context.Context) (ecoflowdelta3.Status, error) {
	return f.status, nil
}

type panicDelta3Client struct{}

func (panicDelta3Client) Probe(context.Context) (ecoflowdelta3.Status, error) {
	panic("DELTA_3 client should not be called")
}

func assertIntPtrResponse(t *testing.T, name string, got *int, want int) {
	t.Helper()
	if got == nil || *got != want {
		if got == nil {
			t.Fatalf("%s = nil, want %d", name, want)
		}
		t.Fatalf("%s = %d, want %d", name, *got, want)
	}
}
