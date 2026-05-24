package api

import (
	"context"
	"errors"
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

func TestDelta3StatusReaderCachesSuccessfulProbe(t *testing.T) {
	calls := 0
	now := time.Date(2026, 5, 24, 13, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	reader := newDelta3StatusReader(validDelta3Config(), nil, fakeDelta3Client{
		status: delta3StatusFixture(82),
		calls:  &calls,
	})
	reader.now = func() time.Time { return now }

	first := reader.CurrentStatus(context.Background())
	second := reader.CurrentStatus(context.Background())

	if !first.Available || !second.Available {
		t.Fatalf("responses should be available: first=%+v second=%+v", first, second)
	}
	if calls != 1 {
		t.Fatalf("Probe calls = %d, want 1", calls)
	}
	if second.Cached != true {
		t.Fatalf("second Cached = false, want true")
	}
}

func TestDelta3StatusReaderBacksOffBusyError(t *testing.T) {
	calls := 0
	now := time.Date(2026, 5, 24, 13, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	reader := newDelta3StatusReader(validDelta3Config(), nil, fakeDelta3Client{
		err:   errors.New("EcoFlow private login returned code=1001 message=Server is too busy"),
		calls: &calls,
	})
	reader.now = func() time.Time { return now }

	first := reader.CurrentStatus(context.Background())
	now = now.Add(9 * time.Minute)
	second := reader.CurrentStatus(context.Background())
	now = now.Add(2 * time.Minute)
	third := reader.CurrentStatus(context.Background())

	if first.Available || second.Available || third.Available {
		t.Fatalf("responses should be unavailable: first=%+v second=%+v third=%+v", first, second, third)
	}
	if calls != 2 {
		t.Fatalf("Probe calls = %d, want 2 after busy backoff expires", calls)
	}
	if !second.Cached {
		t.Fatalf("second Cached = false, want true during busy backoff")
	}
}

func TestDelta3StatusReaderDoesNotCacheContextCancellation(t *testing.T) {
	calls := 0
	now := time.Date(2026, 5, 24, 13, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	reader := newDelta3StatusReader(validDelta3Config(), nil, fakeDelta3Client{
		err:   context.Canceled,
		calls: &calls,
	})
	reader.now = func() time.Time { return now }

	first := reader.CurrentStatus(context.Background())
	second := reader.CurrentStatus(context.Background())

	if first.Available || second.Available {
		t.Fatalf("responses should be unavailable: first=%+v second=%+v", first, second)
	}
	if calls != 2 {
		t.Fatalf("Probe calls = %d, want 2 because context cancellation is not cached", calls)
	}
	if second.Cached {
		t.Fatalf("second Cached = true, want false for context cancellation")
	}
}

func TestDelta3StatusReaderCachesContextDeadlineExceeded(t *testing.T) {
	calls := 0
	now := time.Date(2026, 5, 24, 13, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	reader := newDelta3StatusReader(validDelta3Config(), nil, fakeDelta3Client{
		err:   context.DeadlineExceeded,
		calls: &calls,
	})
	reader.now = func() time.Time { return now }

	first := reader.CurrentStatus(context.Background())
	second := reader.CurrentStatus(context.Background())

	if first.Available || second.Available {
		t.Fatalf("responses should be unavailable: first=%+v second=%+v", first, second)
	}
	if calls != 1 {
		t.Fatalf("Probe calls = %d, want 1 because probe deadline errors are cached", calls)
	}
	if !second.Cached {
		t.Fatalf("second Cached = false, want true for probe deadline error")
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

func delta3StatusFixture(soc int) ecoflowdelta3.Status {
	acInW := 100
	acOutW := 380
	acLimitW := 100
	gridBypassDisabled := false
	acOutputEnabled := true
	return ecoflowdelta3.Status{
		DeviceType:         "DELTA_3",
		CMSBatterySoc:      &soc,
		ACInW:              &acInW,
		ACOutW:             &acOutW,
		ACChargeLimitW:     &acLimitW,
		GridBypassDisabled: &gridBypassDisabled,
		ACOutputEnabled:    &acOutputEnabled,
	}
}

type fakeDelta3Client struct {
	status ecoflowdelta3.Status
	err    error
	calls  *int
}

func (f fakeDelta3Client) Probe(context.Context) (ecoflowdelta3.Status, error) {
	if f.calls != nil {
		*f.calls++
	}
	return f.status, f.err
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
