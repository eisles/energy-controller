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
	"github.com/eisles/energy-controller/backend/internal/ecoflowprivate"
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
	acOutput1Enabled := false
	acOutput2Enabled := true
	acOutputProtectionChannel := 2
	response := readDelta3Status(context.Background(), validDelta3Config(), fakeDelta3Client{status: ecoflowprivate.Status{
		DeviceType:                "DELTA_3_MAX_PLUS",
		CMSBatterySoc:             &soc,
		ACInW:                     &acInW,
		ACOutW:                    &acOutW,
		ACChargeLimitW:            &acLimitW,
		GridBypassDisabled:        &gridBypassDisabled,
		ACOutputEnabled:           &acOutputEnabled,
		ACOutput1Enabled:          &acOutput1Enabled,
		ACOutput2Enabled:          &acOutput2Enabled,
		ACOutputProtectionChannel: &acOutputProtectionChannel,
	}}, nil)
	if !response.Available {
		t.Fatalf("Available = false, lastError=%q", response.LastError)
	}
	assertIntPtrResponse(t, "SOC", response.SOC, 82)
	assertIntPtrResponse(t, "ACInW", response.ACInW, 100)
	assertIntPtrResponse(t, "ACOutW", response.ACOutW, 380)
	assertIntPtrResponse(t, "ACChargeLimitW", response.ACChargeLimitW, 100)
	if response.CycleCount != nil {
		t.Fatalf("CycleCount = %v, want nil without named cycle quota", response.CycleCount)
	}
	if response.CycleCountSource != "" {
		t.Fatalf("CycleCountSource = %q, want empty source", response.CycleCountSource)
	}
	if response.GridBypassDisabled == nil || *response.GridBypassDisabled {
		t.Fatalf("GridBypassDisabled = %v, want false", response.GridBypassDisabled)
	}
	if response.ACOutputEnabled == nil || !*response.ACOutputEnabled {
		t.Fatalf("ACOutputEnabled = %v, want true", response.ACOutputEnabled)
	}
	if response.ACOutput1Enabled == nil || *response.ACOutput1Enabled {
		t.Fatalf("ACOutput1Enabled = %v, want false", response.ACOutput1Enabled)
	}
	if response.ACOutput2Enabled == nil || !*response.ACOutput2Enabled {
		t.Fatalf("ACOutput2Enabled = %v, want true", response.ACOutput2Enabled)
	}
	assertIntPtrResponse(t, "ACOutputProtectionChannel", response.ACOutputProtectionChannel, 2)
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
	now = now.Add(delta3StatusSuccessCacheTTL + time.Second)
	third := reader.CurrentStatus(context.Background())
	if !third.Available || third.Cached {
		t.Fatalf("third response = %+v, want fresh available response after success cache TTL", third)
	}
	if calls != 2 {
		t.Fatalf("Probe calls after cache expiry = %d, want 2", calls)
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

func TestDelta3StatusReaderMarksUnsupportedPrivateMQTTPayloadUnavailable(t *testing.T) {
	reader := newDelta3StatusReader(validDelta3Config(), nil, fakeDelta3Client{
		status: ecoflowprivate.Status{
			DeviceType:          "RIVER_2",
			ReplyCount:          1,
			UnsupportedMessages: 14,
			FieldSummaries: []ecoflowprivate.TelemetryFieldSummary{
				{MessageIndex: 0, CmdFunc: 254, CmdID: 21, Field: 999, Wire: 0, Value: "14"},
			},
		},
	})

	status := reader.CurrentStatus(context.Background())

	if status.Available {
		t.Fatalf("Available = true, want false for unsupported payload")
	}
	if !strings.Contains(status.LastError, "no supported telemetry fields") {
		t.Fatalf("LastError = %q, want unsupported telemetry message", status.LastError)
	}
	if status.TelemetryDiagnostics == nil {
		t.Fatal("TelemetryDiagnostics = nil, want unsupported field diagnostics")
	}
	if status.TelemetryDiagnostics.UnsupportedMessages != 14 || status.TelemetryDiagnostics.ReplyCount != 1 || status.TelemetryDiagnostics.FieldCount != 1 {
		t.Fatalf("TelemetryDiagnostics = %+v, want replyCount=1 unsupported=14 fieldCount=1", status.TelemetryDiagnostics)
	}
	if got := status.TelemetryDiagnostics.FieldSummaries[0]; got.Field != 999 || got.Value != "14" {
		t.Fatalf("Field summary = %+v, want field 999 value 14", got)
	}
}

func TestDelta3StatusReaderKeepsDiagnosticsForEmptyUnsupportedPrivateMQTTPayload(t *testing.T) {
	reader := newDelta3StatusReader(validDelta3Config(), nil, fakeDelta3Client{
		status: ecoflowprivate.Status{
			DeviceType: "DELTA_3_MAX_PLUS",
			ReplyCount: 1,
		},
	})

	status := reader.CurrentStatus(context.Background())

	if status.Available {
		t.Fatalf("Available = true, want false for empty unsupported payload")
	}
	if status.TelemetryDiagnostics == nil {
		t.Fatal("TelemetryDiagnostics = nil, want empty unsupported probe diagnostics")
	}
	if status.TelemetryDiagnostics.ReplyCount != 1 || status.TelemetryDiagnostics.DecodedMessages != 0 || status.TelemetryDiagnostics.FieldCount != 0 {
		t.Fatalf("TelemetryDiagnostics = %+v, want replyCount=1 decoded=0 fieldCount=0", status.TelemetryDiagnostics)
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
		{
			ID:              3,
			Name:            "DELTA 3 Max Plus",
			Kind:            "ecoflow_delta3_plus",
			Provider:        "ecoflow",
			Enabled:         true,
			SupportsSocRead: true,
			CredentialRef:   "max_plus",
			DeviceSN:        "SN789",
			DeviceType:      "DELTA_3_MAX_PLUS",
			StatusSource:    "ecoflow_private_mqtt",
			Priority:        40,
			ControlEnabled:  false,
		},
		{
			ID:              4,
			Name:            "RIVER 2",
			Kind:            "ecoflow_river2",
			Provider:        "ecoflow",
			Enabled:         true,
			SupportsSocRead: true,
			CredentialRef:   "river2",
			DeviceSN:        "SN999",
			DeviceType:      "RIVER_2",
			StatusSource:    "ecoflow_private_mqtt",
			Priority:        60,
			ControlEnabled:  false,
		},
	}

	statuses := reader.CurrentDeviceStatuses(context.Background(), devices)

	if len(statuses) != 4 {
		t.Fatalf("CurrentDeviceStatuses len = %d, want 4", len(statuses))
	}
	if statuses[0].DeviceSN != "SN123" || statuses[1].DeviceSN != "SN456" || statuses[2].DeviceSN != "SN789" || statuses[3].DeviceSN != "SN999" {
		t.Fatalf("device SNs = %q/%q/%q/%q, want SN123/SN456/SN789/SN999", statuses[0].DeviceSN, statuses[1].DeviceSN, statuses[2].DeviceSN, statuses[3].DeviceSN)
	}
	if statuses[0].Status.DeviceType != "DELTA_3" {
		t.Fatalf("status device type = %q, want mapped probe fixture type", statuses[0].Status.DeviceType)
	}
	if !statuses[0].Status.Available || !statuses[1].Status.Available || !statuses[2].Status.Available || !statuses[3].Status.Available {
		t.Fatalf("statuses should be available: %+v", statuses)
	}
	if strings.Join(probed, ",") != "SN123/DELTA_3_PLUS,SN456/DELTA_3_PLUS,SN789/DELTA_3_MAX_PLUS,SN999/RIVER_2" {
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
		cycleCount := 412
		touEnabled := true
		selfPoweredEnabled := false
		scheduledEnabled := false
		intelligentEnabled := false
		return fakeEcoFlowCloudReader{status: domain.BatteryStatus{
			Soc:                 88,
			InputW:              410,
			OutputW:             120,
			ACChargeLimitW:      900,
			MaxChargeSoc:        &maxChargeSoc,
			MinDischargeSoc:     &minDischargeSoc,
			BackupReserveSoc:    &reserveSoc,
			EnergyBackupEnabled: &backupEnabled,
			TOUModeEnabled:      &touEnabled,
			SelfPoweredEnabled:  &selfPoweredEnabled,
			ScheduledEnabled:    &scheduledEnabled,
			IntelligentEnabled:  &intelligentEnabled,
			CycleCount:          &cycleCount,
			CycleCountSource:    "ecoflow_cloud_quota",
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
	assertIntPtrResponse(t, "CycleCount", statuses[0].Status.CycleCount, 412)
	if statuses[0].Status.CycleCountSource != "ecoflow_cloud_quota" {
		t.Fatalf("CycleCountSource = %q, want cloud quota", statuses[0].Status.CycleCountSource)
	}
	assertBoolPtrResponse(t, "TOUModeEnabled", statuses[0].Status.TOUModeEnabled, true)
	assertBoolPtrResponse(t, "SelfPoweredEnabled", statuses[0].Status.SelfPoweredEnabled, false)
	assertBoolPtrResponse(t, "ScheduledEnabled", statuses[0].Status.ScheduledEnabled, false)
	assertBoolPtrResponse(t, "IntelligentEnabled", statuses[0].Status.IntelligentEnabled, false)
	if statuses[0].CapacityWh != 12288 {
		t.Fatalf("CapacityWh = %d, want 12288", statuses[0].CapacityWh)
	}
}

func TestDelta3StatusReaderDoesNotAugmentPro3CycleCountFromPrivateMQTTCandidate(t *testing.T) {
	now := time.Date(2026, 5, 26, 5, 30, 0, 0, time.UTC)
	cycleCount := 415
	privateProbeCalls := 0
	reader := newDelta3StatusReader(config.Config{
		EcoFlowAccessKey:      "access",
		EcoFlowSecretKey:      "secret",
		EcoFlowBaseURL:        "https://api.test",
		Delta3ReadEnabled:     true,
		Delta3PrivateAPIHost:  "api.test",
		Delta3PrivateEmail:    "user@example.com",
		Delta3PrivatePassword: "secret",
		Delta3Timeout:         time.Second,
	}, nil, nil)
	reader.now = func() time.Time { return now }
	reader.ecoFlowCloudReaderFactory = func(cfg ecoflow.Config) ecoFlowCloudBatteryReader {
		return fakeEcoFlowCloudReader{status: domain.BatteryStatus{
			Soc:            67,
			InputW:         100,
			OutputW:        320,
			ACChargeLimitW: 400,
			IsOnline:       true,
		}}
	}
	reader.clientFactory = func(cfg config.Config) delta3ProbeClient {
		return fakeDelta3Client{status: ecoflowprivate.Status{
			DeviceType:       "DELTA_PRO3",
			CycleCount:       &cycleCount,
			CycleCountSource: "ecoflow_private_mqtt_named",
		}, calls: &privateProbeCalls}
	}
	devices := []domain.ChargingDevice{
		{
			ID:              1,
			Name:            "DELTA Pro 3",
			Kind:            "ecoflow_delta_pro3",
			Provider:        "ecoflow",
			Enabled:         true,
			SupportsSocRead: true,
			DeviceSN:        "MASTER-SN",
			DeviceType:      "DELTA_PRO3",
			StatusSource:    "ecoflow_cloud",
		},
	}

	statuses := reader.CurrentDeviceStatuses(context.Background(), devices)

	if len(statuses) != 1 {
		t.Fatalf("CurrentDeviceStatuses len = %d, want 1", len(statuses))
	}
	if statuses[0].Status.CycleCount != nil {
		t.Fatalf("CycleCount = %v, want nil without named cloud quota cycle", statuses[0].Status.CycleCount)
	}
	if statuses[0].Status.CycleCountSource != "" {
		t.Fatalf("CycleCountSource = %q, want empty source", statuses[0].Status.CycleCountSource)
	}
	if privateProbeCalls != 0 {
		t.Fatalf("private probe calls = %d, want 0", privateProbeCalls)
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

func delta3StatusFixture(soc int) ecoflowprivate.Status {
	acInW := 100
	acOutW := 380
	acLimitW := 100
	gridBypassDisabled := false
	acOutputEnabled := true
	return ecoflowprivate.Status{
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
	status ecoflowprivate.Status
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

func (f fakeDelta3Client) Probe(context.Context) (ecoflowprivate.Status, error) {
	if f.calls != nil {
		*f.calls++
	}
	return f.status, f.err
}

type panicDelta3Client struct{}

func (panicDelta3Client) Probe(context.Context) (ecoflowprivate.Status, error) {
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

func assertBoolPtrResponse(t *testing.T, name string, got *bool, want bool) {
	t.Helper()
	if got == nil || *got != want {
		if got == nil {
			t.Fatalf("%s = nil, want %t", name, want)
		}
		t.Fatalf("%s = %t, want %t", name, *got, want)
	}
}
