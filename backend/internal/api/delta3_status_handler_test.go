package api

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/eisles/energy-controller/backend/internal/config"
	"github.com/eisles/energy-controller/backend/internal/domain"
	"github.com/eisles/energy-controller/backend/internal/ecoflow"
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
	acOutW := -380
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

func TestDelta3StatusReaderCacheIsScopedByDeviceIdentity(t *testing.T) {
	calls := 0
	now := time.Date(2026, 5, 24, 13, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	reader := newDelta3StatusReader(validDelta3Config(), nil, fakeDelta3Client{
		status: delta3StatusFixture(82),
		calls:  &calls,
	})
	reader.clientFactory = func(config.Config) delta3ProbeClient {
		return fakeDelta3Client{
			status: delta3StatusFixture(82),
			calls:  &calls,
		}
	}
	reader.now = func() time.Time { return now }
	firstConfig := validDelta3Config()
	secondConfig := validDelta3Config()
	secondConfig.Delta3DeviceSN = "SN456"

	_ = reader.CurrentStatusForConfig(context.Background(), firstConfig)
	_ = reader.CurrentStatusForConfig(context.Background(), secondConfig)
	_ = reader.CurrentStatusForConfig(context.Background(), firstConfig)

	if calls != 2 {
		t.Fatalf("Probe calls = %d, want 2 for two device identities", calls)
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

func TestDelta3StatusReaderReturnsDeviceStatuses(t *testing.T) {
	var probed []string
	reader := newDelta3StatusReader(validDelta3Config(), nil, nil)
	reader.clientFactory = func(cfg config.Config) delta3ProbeClient {
		probed = append(probed, cfg.Delta3DeviceSN+"/"+cfg.Delta3DeviceType)
		return fakeDelta3Client{status: delta3StatusFixture(82)}
	}
	devices := []domain.ChargingDevice{
		{
			ID:              1,
			Name:            "DELTA 3 Plus",
			Kind:            "ecoflow_delta3_plus",
			Provider:        "ecoflow",
			Enabled:         true,
			SupportsSocRead: true,
			CredentialRef:   "primary",
			DeviceSN:        "SN123",
			DeviceType:      "DELTA_3_PLUS",
			StatusSource:    "ecoflow_private_mqtt",
			Priority:        20,
			ControlEnabled:  false,
		},
		{
			ID:              2,
			Name:            "DELTA 3 Plus 2",
			Kind:            "ecoflow_delta3_plus",
			Provider:        "ecoflow",
			Enabled:         true,
			SupportsSocRead: true,
			CredentialRef:   "secondary",
			DeviceSN:        "SN456",
			DeviceType:      "DELTA_3_PLUS",
			StatusSource:    "ecoflow_private_mqtt",
			Priority:        30,
			ControlEnabled:  false,
		},
	}

	statuses := reader.CurrentDeviceStatuses(context.Background(), devices)

	if len(statuses) != 2 {
		t.Fatalf("CurrentDeviceStatuses len = %d, want 2", len(statuses))
	}
	if statuses[0].DeviceSN != "SN123" || statuses[1].DeviceSN != "SN456" {
		t.Fatalf("device SNs = %q/%q, want SN123/SN456", statuses[0].DeviceSN, statuses[1].DeviceSN)
	}
	if statuses[0].Status.DeviceType != "DELTA_3" {
		t.Fatalf("status device type = %q, want mapped probe fixture type", statuses[0].Status.DeviceType)
	}
	if !statuses[0].Status.Available || !statuses[1].Status.Available {
		t.Fatalf("statuses should be available: %+v", statuses)
	}
	if strings.Join(probed, ",") != "SN123/DELTA_3_PLUS,SN456/DELTA_3_PLUS" {
		t.Fatalf("probed configs = %v, want each device SN/type", probed)
	}
}

func TestDelta3StatusReaderReturnsEcoFlowCloudDeviceStatus(t *testing.T) {
	var requested ecoflow.Config
	now := time.Date(2026, 5, 26, 5, 30, 0, 0, time.UTC)
	reader := newDelta3StatusReader(config.Config{
		EcoFlowAccessKey: "access",
		EcoFlowSecretKey: "secret",
		EcoFlowDeviceSN:  "ENV-SN",
		EcoFlowBaseURL:   "https://api.test",
	}, nil, nil)
	reader.now = func() time.Time { return now }
	reader.ecoFlowCloudReaderFactory = func(cfg ecoflow.Config) ecoFlowCloudBatteryReader {
		requested = cfg
		reserveSoc := 30
		backupEnabled := true
		maxChargeSoc := 100
		minDischargeSoc := 0
		return fakeEcoFlowCloudReader{status: domain.BatteryStatus{
			Soc:                 88,
			InputW:              410,
			OutputW:             120,
			ACChargeLimitW:      900,
			MaxChargeSoc:        &maxChargeSoc,
			MinDischargeSoc:     &minDischargeSoc,
			BackupReserveSoc:    &reserveSoc,
			EnergyBackupEnabled: &backupEnabled,
			IsOnline:            true,
		}}
	}
	devices := []domain.ChargingDevice{
		{
			ID:                    1,
			Name:                  "DELTA Pro 3",
			Kind:                  "ecoflow_delta_pro3",
			Provider:              "ecoflow",
			Enabled:               true,
			SupportsSocRead:       true,
			SupportsACChargeLimit: true,
			CredentialRef:         "pro3",
			DeviceSN:              "MASTER-SN",
			DeviceType:            "DELTA_PRO3",
			StatusSource:          "ecoflow_cloud",
			Priority:              10,
			CapacityWh:            12288,
			ControlEnabled:        true,
		},
	}

	statuses := reader.CurrentDeviceStatuses(context.Background(), devices)

	if len(statuses) != 1 {
		t.Fatalf("CurrentDeviceStatuses len = %d, want 1", len(statuses))
	}
	if requested.DeviceSN != "MASTER-SN" {
		t.Fatalf("EcoFlow Cloud device SN = %q, want MASTER-SN", requested.DeviceSN)
	}
	if !statuses[0].Status.Available {
		t.Fatalf("status should be available: %+v", statuses[0].Status)
	}
	assertIntPtrResponse(t, "SOC", statuses[0].Status.SOC, 88)
	assertIntPtrResponse(t, "ACInW", statuses[0].Status.ACInW, 410)
	assertIntPtrResponse(t, "ACOutW", statuses[0].Status.ACOutW, 120)
	assertIntPtrResponse(t, "ACChargeLimitW", statuses[0].Status.ACChargeLimitW, 900)
	assertIntPtrResponse(t, "MaxChargeSoc", statuses[0].Status.MaxChargeSoc, 100)
	assertIntPtrResponse(t, "MinDischargeSoc", statuses[0].Status.MinDischargeSoc, 0)
	if statuses[0].CapacityWh != 12288 {
		t.Fatalf("CapacityWh = %d, want 12288", statuses[0].CapacityWh)
	}
}

func TestEcoFlowCloudConfigForDeviceDoesNotFallbackToEnvSN(t *testing.T) {
	cfg := EcoFlowCloudConfigForDevice(config.Config{EcoFlowDeviceSN: "ENV-SN"}, domain.ChargingDevice{DeviceSN: "   "})
	if cfg.EcoFlowDeviceSN != "" {
		t.Fatalf("EcoFlowDeviceSN = %q, want empty trimmed master SN without env fallback", cfg.EcoFlowDeviceSN)
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

type fakeEcoFlowCloudReader struct {
	status domain.BatteryStatus
	err    error
}

func (r fakeEcoFlowCloudReader) GetBatteryStatus(context.Context) (domain.BatteryStatus, error) {
	return r.status, r.err
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
