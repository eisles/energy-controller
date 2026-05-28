package control

import (
	"testing"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

func TestBuildPro3ACOutputEventCapturesDiagnosticsAndPreviousCommand(t *testing.T) {
	now := time.Date(2026, 5, 28, 18, 45, 0, 0, time.UTC)
	previousAt := now.Add(-5 * time.Minute)
	targetAC := 800
	targetReserve := 40
	event, ok := BuildPro3ACOutputEvent(domain.Status{
		GridW:          120,
		ImportW:        120,
		BatterySoc:     44,
		BatteryInputW:  100,
		BatteryOutputW: 267,
		ACChargeLimitW: 400,
		UpdatedAt:      now,
		EcoFlowDiagnostics: map[string]any{
			"outputPowerOffMemory":     true,
			"bmsMaxCellTemp":           33,
			"bmsMaxMosTemp":            32.5,
			"acOutFreq":                60,
			"plugInInfoAcOutDsgPowMax": 3600,
		},
	}, &domain.SurplusControlCommandLog{
		MeasuredAt:             previousAt,
		CommandKind:            "set_ac_charge_limit",
		CommandSent:            true,
		WouldWrite:             true,
		TargetACChargeLimitW:   &targetAC,
		TargetBackupReserveSoc: &targetReserve,
		DecisionReason:         "test previous command",
	}, now)

	if !ok {
		t.Fatal("BuildPro3ACOutputEvent returned ok=false")
	}
	if event.EventType != Pro3ACOutputOffEventType || !event.OutputPowerOffMemory {
		t.Fatalf("unexpected event type/off memory: %+v", event)
	}
	if event.BMSMaxCellTempC == nil || *event.BMSMaxCellTempC != 33 {
		t.Fatalf("BMSMaxCellTempC = %v, want 33", event.BMSMaxCellTempC)
	}
	if event.ACOutDsgPowMaxW == nil || *event.ACOutDsgPowMaxW != 3600 {
		t.Fatalf("ACOutDsgPowMaxW = %v, want 3600", event.ACOutDsgPowMaxW)
	}
	if event.PreviousCommandMeasuredAt == nil || !event.PreviousCommandMeasuredAt.Equal(previousAt) {
		t.Fatalf("PreviousCommandMeasuredAt = %v, want %s", event.PreviousCommandMeasuredAt, previousAt)
	}
	if event.PreviousCommandTargetACChargeW == nil || *event.PreviousCommandTargetACChargeW != targetAC {
		t.Fatalf("PreviousCommandTargetACChargeW = %v, want %d", event.PreviousCommandTargetACChargeW, targetAC)
	}
}

func TestBuildPro3ACOutputEventSkipsWhenOffMemoryIsFalse(t *testing.T) {
	_, ok := BuildPro3ACOutputEvent(domain.Status{
		EcoFlowDiagnostics: map[string]any{"outputPowerOffMemory": false},
	}, nil, time.Now())
	if ok {
		t.Fatal("BuildPro3ACOutputEvent ok=true, want false")
	}
}

func TestBuildPro3ACOutputEventUsesExistingMasterTemperatureKey(t *testing.T) {
	event, ok := BuildPro3ACOutputEvent(domain.Status{
		UpdatedAt: time.Date(2026, 5, 28, 19, 0, 0, 0, time.UTC),
		EcoFlowDiagnostics: map[string]any{
			"outputPowerOffMemory": true,
			"bmsMasterTemp":        44.5,
		},
	}, nil, time.Now())
	if !ok {
		t.Fatal("BuildPro3ACOutputEvent returned ok=false")
	}
	if event.BMSMaxCellTempC == nil || *event.BMSMaxCellTempC != 44.5 {
		t.Fatalf("BMSMaxCellTempC = %v, want 44.5 from bmsMasterTemp", event.BMSMaxCellTempC)
	}
}
