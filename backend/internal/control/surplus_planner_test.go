package control

import (
	"strings"
	"testing"
)

func TestPlanSurplusChargingRecommendsACAndReserve(t *testing.T) {
	reserve := 30
	tou := true
	plan := PlanSurplusCharging(SurplusPlanInput{
		GridW:             -1200,
		BatterySoc:        78,
		BatteryInputW:     0,
		BatteryOutputW:    0,
		ACChargeLimitW:    0,
		BackupReserveSoc:  &reserve,
		TOUModeEnabled:    &tou,
		SimulationMode:    true,
		EnableRealControl: false,
	}, DefaultSettings())

	if plan.RecommendedACChargeLimitW != 1000 {
		t.Fatalf("RecommendedACChargeLimitW = %d, want 1000", plan.RecommendedACChargeLimitW)
	}
	if plan.RecommendedBackupReserveSoc == nil || *plan.RecommendedBackupReserveSoc != 80 {
		t.Fatalf("RecommendedBackupReserveSoc = %v, want 80", plan.RecommendedBackupReserveSoc)
	}
	if !plan.ShouldRaiseBackupReserve {
		t.Fatal("ShouldRaiseBackupReserve = false, want true")
	}
	if !plan.ShouldAdjustACChargeLimit {
		t.Fatal("ShouldAdjustACChargeLimit = false, want true")
	}
	if !plan.ShouldDisableEnergyModes {
		t.Fatal("ShouldDisableEnergyModes = false, want true")
	}
	for _, want := range []string{"バックアップリザーブを80%へ引き上げ", "AC充電上限を1000Wへ設定", "energy strategy modesを全OFF"} {
		if !strings.Contains(plan.ActionSummary, want) {
			t.Fatalf("ActionSummary = %q, want contains %q", plan.ActionSummary, want)
		}
	}
	if plan.WouldWrite {
		t.Fatal("WouldWrite = true in simulation mode")
	}
	if got := plan.Reason; got == "" || !strings.Contains(got, "energy strategy mode blocks surplus charging") {
		t.Fatalf("Reason = %q, want energy strategy mode note", got)
	}
}

func TestPlanSurplusChargingRestoresDefaultReserveWhenImporting(t *testing.T) {
	reserve := 90
	plan := PlanSurplusCharging(SurplusPlanInput{
		GridW:             900,
		BatterySoc:        78,
		ACChargeLimitW:    1500,
		BackupReserveSoc:  &reserve,
		DefaultReserveSoc: 30,
		SimulationMode:    true,
	}, DefaultSettings())

	if plan.RecommendedACChargeLimitW != 0 {
		t.Fatalf("RecommendedACChargeLimitW = %d, want 0", plan.RecommendedACChargeLimitW)
	}
	if plan.RecommendedBackupReserveSoc == nil || *plan.RecommendedBackupReserveSoc != 30 {
		t.Fatalf("RecommendedBackupReserveSoc = %v, want 30", plan.RecommendedBackupReserveSoc)
	}
	if !plan.ShouldAdjustACChargeLimit {
		t.Fatal("ShouldAdjustACChargeLimit = false, want true")
	}
	if !plan.ShouldLowerBackupReserve {
		t.Fatal("ShouldLowerBackupReserve = false, want true")
	}
	for _, want := range []string{"AC充電上限を0Wへ設定", "バックアップリザーブを30%へ戻す"} {
		if !strings.Contains(plan.ActionSummary, want) {
			t.Fatalf("ActionSummary = %q, want contains %q", plan.ActionSummary, want)
		}
	}
	if plan.ShouldRaiseBackupReserve || plan.WouldWrite {
		t.Fatalf("unexpected write recommendation: %+v", plan)
	}
}

func TestPlanSurplusChargingWouldWriteIncludesEnergyModeDisableWhenAllowed(t *testing.T) {
	tou := true
	reserve := 85
	plan := PlanSurplusCharging(SurplusPlanInput{
		GridW:             -1200,
		BatterySoc:        83,
		ACChargeLimitW:    1000,
		BackupReserveSoc:  &reserve,
		TOUModeEnabled:    &tou,
		SimulationMode:    false,
		EnableRealControl: true,
		AutoControl:       true,
	}, DefaultSettings())

	if !plan.ShouldDisableEnergyModes {
		t.Fatal("ShouldDisableEnergyModes = false, want true")
	}
	if !plan.WouldWrite {
		t.Fatal("WouldWrite = false, want true")
	}
}

func TestPlanSurplusChargingRecommendsEnergyModeDisableForNonTOUModes(t *testing.T) {
	tou := false
	selfPowered := true
	reserve := 85
	plan := PlanSurplusCharging(SurplusPlanInput{
		GridW:              -1200,
		BatterySoc:         83,
		ACChargeLimitW:     1000,
		BackupReserveSoc:   &reserve,
		TOUModeEnabled:     &tou,
		SelfPoweredEnabled: &selfPowered,
		SimulationMode:     false,
		EnableRealControl:  true,
		AutoControl:        true,
	}, DefaultSettings())

	if !plan.ShouldDisableEnergyModes {
		t.Fatal("ShouldDisableEnergyModes = false, want true")
	}
	if !plan.WouldWrite {
		t.Fatal("WouldWrite = false, want true")
	}
	if got := plan.Reason; !strings.Contains(got, "energy strategy mode blocks surplus charging") {
		t.Fatalf("Reason = %q, want energy strategy mode note", got)
	}
}

func TestPlanSurplusChargingDoesNotLowerReserveBelowDefaultWhenImporting(t *testing.T) {
	reserve := 30
	plan := PlanSurplusCharging(SurplusPlanInput{
		GridW:             900,
		BatterySoc:        78,
		ACChargeLimitW:    0,
		BackupReserveSoc:  &reserve,
		DefaultReserveSoc: 30,
		SimulationMode:    true,
	}, DefaultSettings())

	if plan.RecommendedBackupReserveSoc != nil {
		t.Fatalf("RecommendedBackupReserveSoc = %v, want nil", plan.RecommendedBackupReserveSoc)
	}
	if plan.ShouldRaiseBackupReserve || plan.ShouldLowerBackupReserve || plan.ShouldAdjustACChargeLimit || plan.WouldWrite {
		t.Fatalf("unexpected write recommendation: %+v", plan)
	}
	if plan.ActionSummary != "" {
		t.Fatalf("ActionSummary = %q, want empty", plan.ActionSummary)
	}
}
