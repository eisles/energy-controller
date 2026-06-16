package api

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eisles/energy-controller/backend/internal/config"
	"github.com/eisles/energy-controller/backend/internal/domain"
	"github.com/eisles/energy-controller/backend/internal/ecoflow"
	"github.com/eisles/energy-controller/backend/internal/ecoflowdeveloper"
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

func TestReadDelta3StatusMapsDelta3PlusCycleCountCandidate(t *testing.T) {
	soc := 82
	response := readDelta3Status(context.Background(), validDelta3Config(), fakeDelta3Client{status: ecoflowprivate.Status{
		DeviceType:    "DELTA_3_PLUS",
		CMSBatterySoc: &soc,
		FieldSummaries: []ecoflowprivate.TelemetryFieldSummary{
			{MessageIndex: 0, CmdFunc: 254, CmdID: 21, Field: 427, Wire: 0, Value: "79"},
			{MessageIndex: 0, CmdFunc: 254, CmdID: 22, Field: 280, Wire: 0, Value: "0"},
		},
	}}, nil)
	if response.CycleCountCandidate == nil {
		t.Fatal("CycleCountCandidate = nil, want DELTA 3 Plus private MQTT candidate")
	}
	candidate := response.CycleCountCandidate
	if candidate.Value != 79 || candidate.Source != cycleCountCandidateSource || candidate.CmdFunc != 254 || candidate.CmdID != 21 || candidate.Field != 427 {
		t.Fatalf("CycleCountCandidate = %+v, want 79 from 254/21/427", candidate)
	}
	if candidate.Confidence != "candidate" || !strings.Contains(candidate.Reason, "not accepted cycleCount") {
		t.Fatalf("CycleCountCandidate metadata = %+v, want candidate confidence and non-formal reason", candidate)
	}
	if len(response.CycleCountCandidates) != 2 {
		t.Fatalf("CycleCountCandidates length = %d, want 2", len(response.CycleCountCandidates))
	}
	if got := response.CycleCountCandidates[0]; got.Value != 79 || got.CmdFunc != 254 || got.CmdID != 21 || got.Field != 427 {
		t.Fatalf("CycleCountCandidates[0] = %+v, want 79 from 254/21/427", got)
	}
	if got := response.CycleCountCandidates[1]; got.Value != 0 || got.CmdFunc != 254 || got.CmdID != 22 || got.Field != 280 {
		t.Fatalf("CycleCountCandidates[1] = %+v, want 0 from 254/22/280", got)
	}
}

func TestReadDelta3StatusKeepsPrimaryCycleCandidatePlausible(t *testing.T) {
	soc := 82
	response := readDelta3Status(context.Background(), validDelta3Config(), fakeDelta3Client{status: ecoflowprivate.Status{
		DeviceType:    "DELTA_3_PLUS",
		CMSBatterySoc: &soc,
		FieldSummaries: []ecoflowprivate.TelemetryFieldSummary{
			{MessageIndex: 0, CmdFunc: 254, CmdID: 21, Field: 427, Wire: 0, Value: "0"},
			{MessageIndex: 0, CmdFunc: 254, CmdID: 22, Field: 280, Wire: 0, Value: "79"},
		},
	}}, nil)
	if response.CycleCountCandidate == nil {
		t.Fatal("CycleCountCandidate = nil, want plausible secondary candidate")
	}
	candidate := response.CycleCountCandidate
	if candidate.Value != 79 || candidate.CmdFunc != 254 || candidate.CmdID != 22 || candidate.Field != 280 {
		t.Fatalf("CycleCountCandidate = %+v, want 79 from plausible 254/22/280", candidate)
	}
	if len(response.CycleCountCandidates) != 2 {
		t.Fatalf("CycleCountCandidates length = %d, want 2", len(response.CycleCountCandidates))
	}
	if got := response.CycleCountCandidates[0]; got.Value != 0 || got.CmdFunc != 254 || got.CmdID != 21 || got.Field != 427 {
		t.Fatalf("CycleCountCandidates[0] = %+v, want diagnostic 0 from 254/21/427", got)
	}
}

func TestReadDelta3StatusRejectsImplausibleSummaryCycleCandidate(t *testing.T) {
	soc := 82
	response := readDelta3Status(context.Background(), validDelta3Config(), fakeDelta3Client{status: ecoflowprivate.Status{
		DeviceType:    "DELTA_3_PLUS",
		CMSBatterySoc: &soc,
		FieldSummaries: []ecoflowprivate.TelemetryFieldSummary{
			{MessageIndex: 0, CmdFunc: 254, CmdID: 21, Field: 427, Wire: 0, Value: "60000"},
			{MessageIndex: 0, CmdFunc: 254, CmdID: 22, Field: 280, Wire: 0, Value: "79"},
		},
	}}, nil)
	if response.CycleCountCandidate == nil {
		t.Fatal("CycleCountCandidate = nil, want plausible secondary candidate")
	}
	candidate := response.CycleCountCandidate
	if candidate.Value != 79 || candidate.CmdFunc != 254 || candidate.CmdID != 22 || candidate.Field != 280 {
		t.Fatalf("CycleCountCandidate = %+v, want 79 from plausible 254/22/280", candidate)
	}
	if len(response.CycleCountCandidates) != 1 {
		t.Fatalf("CycleCountCandidates length = %d, want only plausible preferred candidate", len(response.CycleCountCandidates))
	}
}

func TestReadDelta3StatusMapsDelta3MaxPlusCycleCountCandidate(t *testing.T) {
	soc := 82
	response := readDelta3Status(context.Background(), validDelta3Config(), fakeDelta3Client{status: ecoflowprivate.Status{
		DeviceType:    "DELTA_3_MAX_PLUS",
		CMSBatterySoc: &soc,
		FieldSummaries: []ecoflowprivate.TelemetryFieldSummary{
			{MessageIndex: 0, CmdFunc: 32, CmdID: 50, Field: 86, Wire: 0, Value: "8"},
			{MessageIndex: 0, CmdFunc: 32, CmdID: 50, Field: 85, Wire: 0, Value: "5"},
		},
	}}, nil)
	if response.CycleCountCandidate == nil {
		t.Fatal("CycleCountCandidate = nil, want DELTA 3 Max Plus private MQTT candidate")
	}
	candidate := response.CycleCountCandidate
	if candidate.Value != 5 || candidate.CmdFunc != 32 || candidate.CmdID != 50 || candidate.Field != 85 {
		t.Fatalf("CycleCountCandidate = %+v, want preferred 5 from 32/50/85", candidate)
	}
	if len(response.CycleCountCandidates) != 2 {
		t.Fatalf("CycleCountCandidates length = %d, want 2", len(response.CycleCountCandidates))
	}
	if got := response.CycleCountCandidates[0]; got.Value != 5 || got.CmdFunc != 32 || got.CmdID != 50 || got.Field != 85 {
		t.Fatalf("CycleCountCandidates[0] = %+v, want 5 from 32/50/85", got)
	}
	if got := response.CycleCountCandidates[1]; got.Value != 8 || got.CmdFunc != 32 || got.CmdID != 50 || got.Field != 86 {
		t.Fatalf("CycleCountCandidates[1] = %+v, want 8 from 32/50/86", got)
	}
}

func TestReadDelta3StatusMapsCycleCountCandidateFromUntruncatedCandidates(t *testing.T) {
	soc := 82
	response := readDelta3Status(context.Background(), validDelta3Config(), fakeDelta3Client{status: ecoflowprivate.Status{
		DeviceType:    "DELTA_3_PLUS",
		CMSBatterySoc: &soc,
		FieldSummaries: []ecoflowprivate.TelemetryFieldSummary{
			{MessageIndex: 0, CmdFunc: 254, CmdID: 21, Field: 7, Wire: 0, Value: "82"},
		},
		FieldCount:            120,
		FieldSummaryTruncated: true,
		CycleFieldCandidates: []ecoflowprivate.CycleFieldCandidate{
			{MessageIndex: 0, CmdFunc: 254, CmdID: 21, Field: 427, Value: 79},
		},
	}}, nil)
	if response.CycleCountCandidate == nil {
		t.Fatal("CycleCountCandidate = nil, want candidate from untruncated private MQTT fields")
	}
	candidate := response.CycleCountCandidate
	if candidate.Value != 79 || candidate.CmdFunc != 254 || candidate.CmdID != 21 || candidate.Field != 427 {
		t.Fatalf("CycleCountCandidate = %+v, want 79 from untruncated 254/21/427 candidate", candidate)
	}
	if len(response.CycleCountCandidates) != 1 {
		t.Fatalf("CycleCountCandidates length = %d, want 1", len(response.CycleCountCandidates))
	}
}

func TestReadDelta3StatusDoesNotExposeCandidateWhenCycleCountIsKnown(t *testing.T) {
	cycleCount := 412
	response := readDelta3Status(context.Background(), validDelta3Config(), fakeDelta3Client{status: ecoflowprivate.Status{
		DeviceType:       "DELTA_3_PLUS",
		CycleCount:       &cycleCount,
		CycleCountSource: "ecoflow_private_mqtt_named",
		FieldSummaries: []ecoflowprivate.TelemetryFieldSummary{
			{MessageIndex: 0, CmdFunc: 254, CmdID: 21, Field: 427, Wire: 0, Value: "79"},
		},
	}}, nil)
	if response.CycleCountCandidate != nil {
		t.Fatalf("CycleCountCandidate = %+v, want nil when formal CycleCount exists", response.CycleCountCandidate)
	}
	if len(response.CycleCountCandidates) != 0 {
		t.Fatalf("CycleCountCandidates = %+v, want empty when formal CycleCount exists", response.CycleCountCandidates)
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
	probedTimeouts := make(map[string]time.Duration)
	var probedMu sync.Mutex
	cfg := validDelta3Config()
	cfg.Delta3Timeout = 20 * time.Second
	reader := newDelta3StatusReader(cfg, nil, nil)
	reader.clientFactory = func(cfg config.Config) delta3ProbeClient {
		probedMu.Lock()
		probed = append(probed, cfg.Delta3DeviceSN+"/"+cfg.Delta3DeviceType)
		probedTimeouts[cfg.Delta3DeviceSN] = cfg.Delta3Timeout
		probedMu.Unlock()
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
	probedMu.Lock()
	probedSet := make(map[string]bool, len(probed))
	for _, got := range probed {
		probedSet[got] = true
	}
	probedMu.Unlock()
	for _, want := range []string{"SN123/DELTA_3_PLUS", "SN456/DELTA_3_PLUS", "SN789/DELTA_3_MAX_PLUS", "SN999/RIVER_2"} {
		if !probedSet[want] {
			t.Fatalf("probed configs = %v, missing %s", probed, want)
		}
	}
	for _, sn := range []string{"SN123", "SN456", "SN789", "SN999"} {
		if probedTimeouts[sn] != deviceStatusReadTimeout {
			t.Fatalf("probed timeout for %s = %s, want %s", sn, probedTimeouts[sn], deviceStatusReadTimeout)
		}
	}
}

func TestDelta3StatusReaderDeviceStatusesTimeoutDoesNotBlockOtherDevices(t *testing.T) {
	cfg := validDelta3Config()
	cfg.Delta3Timeout = 20 * time.Millisecond
	reader := newDelta3StatusReader(cfg, nil, nil)
	reader.clientFactory = func(cfg config.Config) delta3ProbeClient {
		if cfg.Delta3DeviceSN == "SLOW" {
			return blockingDelta3Client{}
		}
		return fakeDelta3Client{status: delta3StatusFixture(74)}
	}
	devices := []domain.ChargingDevice{
		{
			ID:              1,
			Name:            "slow",
			Kind:            "ecoflow_delta3_plus",
			Provider:        "ecoflow",
			Enabled:         true,
			SupportsSocRead: true,
			DeviceSN:        "SLOW",
			DeviceType:      "DELTA_3_PLUS",
			StatusSource:    "ecoflow_private_mqtt",
		},
		{
			ID:              2,
			Name:            "fast",
			Kind:            "ecoflow_delta3_plus",
			Provider:        "ecoflow",
			Enabled:         true,
			SupportsSocRead: true,
			DeviceSN:        "FAST",
			DeviceType:      "DELTA_3_PLUS",
			StatusSource:    "ecoflow_private_mqtt",
		},
	}

	start := time.Now()
	statuses := reader.CurrentDeviceStatuses(context.Background(), devices)
	elapsed := time.Since(start)

	if elapsed > 200*time.Millisecond {
		t.Fatalf("CurrentDeviceStatuses took %s, want bounded by per-device timeout", elapsed)
	}
	if len(statuses) != 2 || statuses[0].DeviceSN != "SLOW" || statuses[1].DeviceSN != "FAST" {
		t.Fatalf("statuses order = %+v, want input device order", statuses)
	}
	if statuses[0].Status.Available {
		t.Fatalf("slow status = %+v, want unavailable timeout", statuses[0].Status)
	}
	if !statuses[1].Status.Available {
		t.Fatalf("fast status = %+v, want available", statuses[1].Status)
	}
}

func TestDelta3StatusReaderDeviceStatusesReturnsStaleCacheOnTimeout(t *testing.T) {
	cfg := validDelta3Config()
	cfg.Delta3Timeout = 20 * time.Millisecond
	now := time.Date(2026, 5, 24, 13, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	client := &scriptedDelta3Client{probes: []func(context.Context) (ecoflowprivate.Status, error){
		func(context.Context) (ecoflowprivate.Status, error) {
			return delta3StatusFixture(71), nil
		},
		func(ctx context.Context) (ecoflowprivate.Status, error) {
			<-ctx.Done()
			return ecoflowprivate.Status{}, ctx.Err()
		},
	}}
	reader := newDelta3StatusReader(cfg, nil, nil)
	reader.now = func() time.Time { return now }
	reader.clientFactory = func(config.Config) delta3ProbeClient {
		return client
	}
	devices := []domain.ChargingDevice{
		{
			ID:              1,
			Name:            "DELTA 3 Plus",
			Kind:            "ecoflow_delta3_plus",
			Provider:        "ecoflow",
			Enabled:         true,
			SupportsSocRead: true,
			DeviceSN:        "SN123",
			DeviceType:      "DELTA_3_PLUS",
			StatusSource:    "ecoflow_private_mqtt",
		},
	}

	first := reader.CurrentDeviceStatuses(context.Background(), devices)
	now = now.Add(delta3StatusSuccessCacheTTL + time.Second)
	second := reader.CurrentDeviceStatuses(context.Background(), devices)
	controlStatus := reader.CurrentStatusForConfig(context.Background(), Delta3ConfigForDevice(cfg, devices[0]))

	if !first[0].Status.Available || first[0].Status.Cached {
		t.Fatalf("first status = %+v, want fresh available", first[0].Status)
	}
	if !second[0].Status.Available || !second[0].Status.Cached {
		t.Fatalf("second status = %+v, want stale cached available", second[0].Status)
	}
	if second[0].Status.SOC == nil || *second[0].Status.SOC != 71 {
		t.Fatalf("second SOC = %v, want stale 71", second[0].Status.SOC)
	}
	if !strings.Contains(second[0].Status.LastError, "refresh failed") {
		t.Fatalf("second LastError = %q, want refresh failure reason", second[0].Status.LastError)
	}
	if controlStatus.Available || !controlStatus.Cached || !strings.Contains(controlStatus.LastError, "refresh failed") {
		t.Fatalf("control status = %+v, want stale cache hidden from control reads", controlStatus)
	}
}

func TestDelta3StatusReaderDoesNotCacheStaleStatusOnCallerCancellation(t *testing.T) {
	cfg := validDelta3Config()
	now := time.Date(2026, 5, 24, 13, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	client := &scriptedDelta3Client{probes: []func(context.Context) (ecoflowprivate.Status, error){
		func(context.Context) (ecoflowprivate.Status, error) {
			return delta3StatusFixture(71), nil
		},
		func(ctx context.Context) (ecoflowprivate.Status, error) {
			<-ctx.Done()
			return ecoflowprivate.Status{}, ctx.Err()
		},
		func(context.Context) (ecoflowprivate.Status, error) {
			return delta3StatusFixture(72), nil
		},
	}}
	reader := newDelta3StatusReader(cfg, nil, nil)
	reader.now = func() time.Time { return now }
	reader.clientFactory = func(config.Config) delta3ProbeClient {
		return client
	}
	devices := []domain.ChargingDevice{
		{
			ID:              1,
			Name:            "DELTA 3 Plus",
			Kind:            "ecoflow_delta3_plus",
			Provider:        "ecoflow",
			Enabled:         true,
			SupportsSocRead: true,
			DeviceSN:        "SN123",
			DeviceType:      "DELTA_3_PLUS",
			StatusSource:    "ecoflow_private_mqtt",
		},
	}

	first := reader.CurrentDeviceStatuses(context.Background(), devices)
	now = now.Add(delta3StatusSuccessCacheTTL + time.Second)
	cancelled := reader.CurrentDeviceStatuses(cancelledCtx, devices)
	refreshed := reader.CurrentDeviceStatuses(context.Background(), devices)

	if !first[0].Status.Available || first[0].Status.SOC == nil || *first[0].Status.SOC != 71 {
		t.Fatalf("first status = %+v, want SOC 71", first[0].Status)
	}
	if cancelled[0].Status.Available || !strings.Contains(cancelled[0].Status.LastError, "context canceled") {
		t.Fatalf("cancelled status = %+v, want direct context canceled error", cancelled[0].Status)
	}
	if !refreshed[0].Status.Available || refreshed[0].Status.Cached || refreshed[0].Status.SOC == nil || *refreshed[0].Status.SOC != 72 {
		t.Fatalf("refreshed status = %+v, want fresh SOC 72 after caller cancellation", refreshed[0].Status)
	}
}

func TestDelta3StatusReaderStaleCachePreservesBusyBackoff(t *testing.T) {
	cfg := validDelta3Config()
	cfg.Delta3Timeout = 20 * time.Millisecond
	now := time.Date(2026, 5, 24, 13, 0, 0, 0, time.FixedZone("JST", 9*60*60))
	client := &scriptedDelta3Client{probes: []func(context.Context) (ecoflowprivate.Status, error){
		func(context.Context) (ecoflowprivate.Status, error) {
			return delta3StatusFixture(71), nil
		},
		func(context.Context) (ecoflowprivate.Status, error) {
			return ecoflowprivate.Status{}, errors.New("EcoFlow private login failed: server is too busy")
		},
		func(context.Context) (ecoflowprivate.Status, error) {
			return delta3StatusFixture(72), nil
		},
	}}
	reader := newDelta3StatusReader(cfg, nil, nil)
	reader.now = func() time.Time { return now }
	reader.clientFactory = func(config.Config) delta3ProbeClient {
		return client
	}
	devices := []domain.ChargingDevice{
		{
			ID:              1,
			Name:            "DELTA 3 Plus",
			Kind:            "ecoflow_delta3_plus",
			Provider:        "ecoflow",
			Enabled:         true,
			SupportsSocRead: true,
			DeviceSN:        "SN123",
			DeviceType:      "DELTA_3_PLUS",
			StatusSource:    "ecoflow_private_mqtt",
		},
	}

	first := reader.CurrentDeviceStatuses(context.Background(), devices)
	now = now.Add(delta3StatusSuccessCacheTTL + time.Second)
	second := reader.CurrentDeviceStatuses(context.Background(), devices)
	now = now.Add(delta3StatusErrorCacheTTL)
	third := reader.CurrentDeviceStatuses(context.Background(), devices)

	if !first[0].Status.Available || first[0].Status.Cached {
		t.Fatalf("first status = %+v, want fresh available", first[0].Status)
	}
	if !second[0].Status.Available || !second[0].Status.Cached || !strings.Contains(second[0].Status.LastError, "server is too busy") {
		t.Fatalf("second status = %+v, want stale busy fallback", second[0].Status)
	}
	if !third[0].Status.Available || !third[0].Status.Cached || !strings.Contains(third[0].Status.LastError, "server is too busy") {
		t.Fatalf("third status = %+v, want cached busy fallback without re-probe", third[0].Status)
	}
	client.mu.Lock()
	calls := client.calls
	client.mu.Unlock()
	if calls != 2 {
		t.Fatalf("probe calls = %d, want 2 before busy backoff expires", calls)
	}
}

func TestDelta3StatusReaderSerializesFixedMQTTClientIDDeviceStatuses(t *testing.T) {
	cfg := validDelta3Config()
	cfg.Delta3MQTTClientID = "fixed-client-id"
	client := &concurrentDelta3Client{delay: 10 * time.Millisecond}
	reader := newDelta3StatusReader(cfg, nil, nil)
	reader.clientFactory = func(config.Config) delta3ProbeClient {
		return client
	}
	devices := []domain.ChargingDevice{
		{
			ID:              1,
			Name:            "DELTA 3 Plus",
			Kind:            "ecoflow_delta3_plus",
			Provider:        "ecoflow",
			Enabled:         true,
			SupportsSocRead: true,
			DeviceSN:        "SN123",
			DeviceType:      "DELTA_3_PLUS",
			StatusSource:    "ecoflow_private_mqtt",
		},
		{
			ID:              2,
			Name:            "DELTA 3 Max Plus",
			Kind:            "ecoflow_delta3_plus",
			Provider:        "ecoflow",
			Enabled:         true,
			SupportsSocRead: true,
			DeviceSN:        "SN456",
			DeviceType:      "DELTA_3_MAX_PLUS",
			StatusSource:    "ecoflow_private_mqtt",
		},
	}

	statuses := reader.CurrentDeviceStatuses(context.Background(), devices)

	for _, status := range statuses {
		if !status.Status.Available {
			t.Fatalf("status = %+v, want available", status.Status)
		}
	}
	client.mu.Lock()
	maxActive := client.maxActive
	calls := client.calls
	client.mu.Unlock()
	if maxActive != 1 {
		t.Fatalf("max concurrent probes = %d, want 1 for fixed MQTT client ID", maxActive)
	}
	if calls != 2 {
		t.Fatalf("probe calls = %d, want one call per device", calls)
	}
}

func TestDelta3StatusReaderFixedMQTTClientIDTimeoutStartsAfterQueue(t *testing.T) {
	cfg := validDelta3Config()
	cfg.Delta3MQTTClientID = "fixed-client-id"
	cfg.Delta3Timeout = 20 * time.Millisecond
	client := &concurrentDelta3Client{delay: 15 * time.Millisecond}
	reader := newDelta3StatusReader(cfg, nil, nil)
	reader.clientFactory = func(config.Config) delta3ProbeClient {
		return client
	}
	devices := []domain.ChargingDevice{
		{
			ID:              1,
			Name:            "DELTA 3 Plus",
			Kind:            "ecoflow_delta3_plus",
			Provider:        "ecoflow",
			Enabled:         true,
			SupportsSocRead: true,
			DeviceSN:        "SN123",
			DeviceType:      "DELTA_3_PLUS",
			StatusSource:    "ecoflow_private_mqtt",
		},
		{
			ID:              2,
			Name:            "DELTA 3 Max Plus",
			Kind:            "ecoflow_delta3_plus",
			Provider:        "ecoflow",
			Enabled:         true,
			SupportsSocRead: true,
			DeviceSN:        "SN456",
			DeviceType:      "DELTA_3_MAX_PLUS",
			StatusSource:    "ecoflow_private_mqtt",
		},
	}

	statuses := reader.CurrentDeviceStatuses(context.Background(), devices)

	for _, status := range statuses {
		if !status.Status.Available {
			t.Fatalf("status = %+v, want available after its own read timeout starts", status.Status)
		}
	}
	client.mu.Lock()
	maxActive := client.maxActive
	calls := client.calls
	client.mu.Unlock()
	if maxActive != 1 {
		t.Fatalf("max concurrent probes = %d, want 1 for fixed MQTT client ID", maxActive)
	}
	if calls != 2 {
		t.Fatalf("probe calls = %d, want one call per device", calls)
	}
}

func TestDelta3StatusReaderSerializesSameDeviceRefresh(t *testing.T) {
	cfg := validDelta3Config()
	client := &concurrentDelta3Client{delay: 10 * time.Millisecond}
	reader := newDelta3StatusReader(cfg, nil, nil)
	reader.client = nil
	reader.clientFactory = func(config.Config) delta3ProbeClient {
		return client
	}

	var wg sync.WaitGroup
	wg.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			defer wg.Done()
			status := reader.CurrentStatusForConfig(context.Background(), cfg)
			if !status.Available {
				t.Errorf("status = %+v, want available", status)
			}
		}()
	}
	wg.Wait()

	client.mu.Lock()
	maxActive := client.maxActive
	calls := client.calls
	client.mu.Unlock()
	if maxActive != 1 {
		t.Fatalf("max concurrent probes = %d, want 1 for same device", maxActive)
	}
	if calls != 1 {
		t.Fatalf("probe calls = %d, want second caller to use refreshed cache", calls)
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

func TestDelta3StatusReaderReturnsStaleEcoFlowCloudStatusOnTimeout(t *testing.T) {
	now := time.Date(2026, 5, 26, 5, 30, 0, 0, time.UTC)
	calls := 0
	reader := newDelta3StatusReader(config.Config{
		EcoFlowAccessKey: "access",
		EcoFlowSecretKey: "secret",
		EcoFlowDeviceSN:  "ENV-SN",
		EcoFlowBaseURL:   "https://api.test",
		Delta3Timeout:    20 * time.Millisecond,
	}, nil, nil)
	reader.now = func() time.Time { return now }
	reader.ecoFlowCloudReaderFactory = func(cfg ecoflow.Config) ecoFlowCloudBatteryReader {
		calls++
		if calls == 1 {
			return fakeEcoFlowCloudReader{status: domain.BatteryStatus{
				Soc:            88,
				InputW:         410,
				OutputW:        120,
				ACChargeLimitW: 900,
				IsOnline:       true,
			}}
		}
		return fakeEcoFlowCloudReader{err: context.DeadlineExceeded}
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

	first := reader.CurrentDeviceStatuses(context.Background(), devices)
	now = now.Add(delta3StatusSuccessCacheTTL + time.Second)
	second := reader.CurrentDeviceStatuses(context.Background(), devices)
	third := reader.CurrentDeviceStatuses(context.Background(), devices)

	if !first[0].Status.Available || first[0].Status.Cached {
		t.Fatalf("first status = %+v, want fresh available cloud status", first[0].Status)
	}
	if !second[0].Status.Available || !second[0].Status.Cached || !strings.Contains(second[0].Status.LastError, "refresh failed") {
		t.Fatalf("second status = %+v, want stale cached cloud status after timeout", second[0].Status)
	}
	assertIntPtrResponse(t, "second SOC", second[0].Status.SOC, 88)
	if !third[0].Status.Available || !third[0].Status.Cached {
		t.Fatalf("third status = %+v, want cached stale cloud status", third[0].Status)
	}
	if calls != 2 {
		t.Fatalf("cloud status calls = %d, want no retry before stale backoff expires", calls)
	}
}

func TestFreshEcoFlowCloudCacheResponseRejectsStaleRefreshFailure(t *testing.T) {
	refreshStartedAt := time.Date(2026, 6, 15, 6, 30, 0, 0, time.UTC)
	entry := ecoFlowCloudStatusCacheEntry{
		response: Delta3StatusResponse{
			Available: true,
			Cached:    true,
			LastError: "refresh failed: context deadline exceeded",
			SOC:       intPtr(88),
		},
		cacheUntil: refreshStartedAt.Add(time.Minute),
	}

	response, ok := freshAvailableEcoFlowCloudCacheResponse(entry, refreshStartedAt)

	if ok {
		t.Fatalf("fresh cloud cache response = %+v, want stale refresh failure rejected", response)
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

func TestDelta3StatusReaderAugmentsPro3CycleCountFromDeveloperMQTTQuota(t *testing.T) {
	now := time.Date(2026, 6, 12, 19, 45, 0, 0, time.UTC)
	developerCycleCalls := 0
	reader := newDelta3StatusReader(config.Config{
		EcoFlowAccessKey:      "access",
		EcoFlowSecretKey:      "secret",
		EcoFlowBaseURL:        "https://api.test",
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
	reader.ecoFlowDeveloperCycleReaderFactory = func(cfg ecoflowdeveloper.Config) ecoFlowDeveloperCycleReader {
		if cfg.DeviceSN != "MASTER-SN" {
			t.Fatalf("Developer MQTT device SN = %q, want MASTER-SN", cfg.DeviceSN)
		}
		cycle := 12
		return fakeEcoFlowDeveloperCycleReader{
			status: ecoflowdeveloper.CycleStatus{
				CycleCount:       &cycle,
				CycleCountSource: ecoflowdeveloper.CycleCountSource,
				Key:              "cycles",
				QuotaKeyCount:    4,
			},
			calls: &developerCycleCalls,
		}
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

	first := reader.CurrentDeviceStatuses(context.Background(), devices)
	second := reader.CurrentDeviceStatuses(context.Background(), devices)

	assertIntPtrResponse(t, "CycleCount", first[0].Status.CycleCount, 12)
	if first[0].Status.CycleCountSource != ecoflowdeveloper.CycleCountSource {
		t.Fatalf("CycleCountSource = %q, want Developer MQTT quota", first[0].Status.CycleCountSource)
	}
	assertIntPtrResponse(t, "CycleCount", second[0].Status.CycleCount, 12)
	if developerCycleCalls != 1 {
		t.Fatalf("Developer cycle calls = %d, want cached single call", developerCycleCalls)
	}
}

func TestDelta3StatusReaderKeepsLastDeveloperMQTTCycleOnRefreshError(t *testing.T) {
	now := time.Date(2026, 6, 12, 19, 45, 0, 0, time.UTC)
	developerCycleCalls := 0
	reader := newDelta3StatusReader(config.Config{
		EcoFlowAccessKey:      "access",
		EcoFlowSecretKey:      "secret",
		EcoFlowBaseURL:        "https://api.test",
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
	reader.ecoFlowDeveloperCycleReaderFactory = func(cfg ecoflowdeveloper.Config) ecoFlowDeveloperCycleReader {
		developerCycleCalls++
		if developerCycleCalls == 1 {
			cycle := 12
			return fakeEcoFlowDeveloperCycleReader{status: ecoflowdeveloper.CycleStatus{
				CycleCount:       &cycle,
				CycleCountSource: ecoflowdeveloper.CycleCountSource,
				Key:              "cycles",
			}}
		}
		return fakeEcoFlowDeveloperCycleReader{err: errors.New("cycle quota missing")}
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

	first := reader.CurrentDeviceStatuses(context.Background(), devices)
	now = now.Add(delta3CycleSuccessCacheTTL + time.Second)
	second := reader.CurrentDeviceStatuses(context.Background(), devices)

	assertIntPtrResponse(t, "first CycleCount", first[0].Status.CycleCount, 12)
	assertIntPtrResponse(t, "second CycleCount", second[0].Status.CycleCount, 12)
	if second[0].Status.CycleCountSource != ecoflowdeveloper.CycleCountSource {
		t.Fatalf("CycleCountSource = %q, want Developer MQTT quota", second[0].Status.CycleCountSource)
	}
	if developerCycleCalls != 2 {
		t.Fatalf("Developer cycle calls = %d, want refresh after success cache expiry", developerCycleCalls)
	}
}

func TestDelta3StatusReaderKeepsPro3StatusWhenDeveloperMQTTCycleMissing(t *testing.T) {
	reader := newDelta3StatusReader(config.Config{
		EcoFlowAccessKey:      "access",
		EcoFlowSecretKey:      "secret",
		EcoFlowBaseURL:        "https://api.test",
		Delta3PrivateAPIHost:  "api.test",
		Delta3PrivateEmail:    "user@example.com",
		Delta3PrivatePassword: "secret",
		Delta3Timeout:         time.Second,
	}, nil, nil)
	reader.ecoFlowCloudReaderFactory = func(cfg ecoflow.Config) ecoFlowCloudBatteryReader {
		return fakeEcoFlowCloudReader{status: domain.BatteryStatus{
			Soc:            67,
			InputW:         100,
			OutputW:        320,
			ACChargeLimitW: 400,
			IsOnline:       true,
		}}
	}
	reader.ecoFlowDeveloperCycleReaderFactory = func(cfg ecoflowdeveloper.Config) ecoFlowDeveloperCycleReader {
		return fakeEcoFlowDeveloperCycleReader{status: ecoflowdeveloper.CycleStatus{QuotaKeyCount: 3}}
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

	if !statuses[0].Status.Available {
		t.Fatalf("status should remain available when Developer MQTT cycle is missing: %+v", statuses[0].Status)
	}
	if statuses[0].Status.CycleCount != nil || statuses[0].Status.CycleCountSource != "" {
		t.Fatalf("cycle = %v source=%q, want missing cycle only", statuses[0].Status.CycleCount, statuses[0].Status.CycleCountSource)
	}
}

func TestDelta3StatusReaderReusesPro3DeviceTimeoutForCycleAugment(t *testing.T) {
	timeout := 100 * time.Millisecond
	reader := newDelta3StatusReader(config.Config{
		EcoFlowAccessKey:      "access",
		EcoFlowSecretKey:      "secret",
		EcoFlowBaseURL:        "https://api.test",
		Delta3PrivateAPIHost:  "api.test",
		Delta3PrivateEmail:    "user@example.com",
		Delta3PrivatePassword: "secret",
		Delta3Timeout:         timeout,
	}, nil, nil)
	reader.ecoFlowCloudReaderFactory = func(cfg ecoflow.Config) ecoFlowCloudBatteryReader {
		return delayedEcoFlowCloudReader{
			delay: 80 * time.Millisecond,
			status: domain.BatteryStatus{
				Soc:            67,
				InputW:         100,
				OutputW:        320,
				ACChargeLimitW: 400,
				IsOnline:       true,
			},
		}
	}
	developerCycleCalls := 0
	reader.ecoFlowDeveloperCycleReaderFactory = func(cfg ecoflowdeveloper.Config) ecoFlowDeveloperCycleReader {
		return blockingEcoFlowDeveloperCycleReader{calls: &developerCycleCalls}
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

	startedAt := time.Now()
	statuses := reader.CurrentDeviceStatuses(context.Background(), devices)
	elapsed := time.Since(startedAt)

	if elapsed > 150*time.Millisecond {
		t.Fatalf("CurrentDeviceStatuses elapsed = %s, want shared device timeout near %s", elapsed, timeout)
	}
	if !statuses[0].Status.Available {
		t.Fatalf("status should remain available when cycle augment times out: %+v", statuses[0].Status)
	}
	if statuses[0].Status.CycleCount != nil || statuses[0].Status.CycleCountSource != "" {
		t.Fatalf("cycle = %v source=%q, want missing cycle after shared timeout", statuses[0].Status.CycleCount, statuses[0].Status.CycleCountSource)
	}
	if developerCycleCalls != 1 {
		t.Fatalf("Developer cycle calls = %d, want one cycle attempt", developerCycleCalls)
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

type blockingDelta3Client struct{}

type scriptedDelta3Client struct {
	mu     sync.Mutex
	probes []func(context.Context) (ecoflowprivate.Status, error)
	calls  int
}

type concurrentDelta3Client struct {
	mu        sync.Mutex
	active    int
	maxActive int
	calls     int
	delay     time.Duration
}

type fakeEcoFlowCloudReader struct {
	status domain.BatteryStatus
	err    error
}

type delayedEcoFlowCloudReader struct {
	status domain.BatteryStatus
	delay  time.Duration
}

type fakeEcoFlowDeveloperCycleReader struct {
	status ecoflowdeveloper.CycleStatus
	err    error
	calls  *int
}

type blockingEcoFlowDeveloperCycleReader struct {
	calls *int
}

func (r fakeEcoFlowCloudReader) GetBatteryStatus(context.Context) (domain.BatteryStatus, error) {
	return r.status, r.err
}

func (r delayedEcoFlowCloudReader) GetBatteryStatus(ctx context.Context) (domain.BatteryStatus, error) {
	timer := time.NewTimer(r.delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return domain.BatteryStatus{}, ctx.Err()
	case <-timer.C:
		return r.status, nil
	}
}

func (r fakeEcoFlowDeveloperCycleReader) ReadCycleStatus(context.Context) (ecoflowdeveloper.CycleStatus, error) {
	if r.calls != nil {
		*r.calls++
	}
	return r.status, r.err
}

func (r blockingEcoFlowDeveloperCycleReader) ReadCycleStatus(ctx context.Context) (ecoflowdeveloper.CycleStatus, error) {
	if r.calls != nil {
		*r.calls++
	}
	<-ctx.Done()
	return ecoflowdeveloper.CycleStatus{}, ctx.Err()
}

func (f fakeDelta3Client) Probe(context.Context) (ecoflowprivate.Status, error) {
	if f.calls != nil {
		*f.calls++
	}
	return f.status, f.err
}

func (blockingDelta3Client) Probe(ctx context.Context) (ecoflowprivate.Status, error) {
	<-ctx.Done()
	return ecoflowprivate.Status{}, ctx.Err()
}

func (f *scriptedDelta3Client) Probe(ctx context.Context) (ecoflowprivate.Status, error) {
	f.mu.Lock()
	index := f.calls
	f.calls++
	if index >= len(f.probes) {
		index = len(f.probes) - 1
	}
	probe := f.probes[index]
	f.mu.Unlock()
	return probe(ctx)
}

func (f *concurrentDelta3Client) Probe(ctx context.Context) (ecoflowprivate.Status, error) {
	f.mu.Lock()
	f.active++
	f.calls++
	if f.active > f.maxActive {
		f.maxActive = f.active
	}
	f.mu.Unlock()
	defer func() {
		f.mu.Lock()
		f.active--
		f.mu.Unlock()
	}()
	timer := time.NewTimer(f.delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ecoflowprivate.Status{}, ctx.Err()
	case <-timer.C:
		return delta3StatusFixture(73), nil
	}
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
