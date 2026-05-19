package control

import "testing"

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
	if plan.WouldWrite {
		t.Fatal("WouldWrite = true in simulation mode")
	}
}

func TestPlanSurplusChargingDoesNothingWhenImporting(t *testing.T) {
	plan := PlanSurplusCharging(SurplusPlanInput{
		GridW:      900,
		BatterySoc: 78,
	}, DefaultSettings())

	if plan.RecommendedACChargeLimitW != 0 {
		t.Fatalf("RecommendedACChargeLimitW = %d, want 0", plan.RecommendedACChargeLimitW)
	}
	if plan.ShouldRaiseBackupReserve || plan.ShouldAdjustACChargeLimit || plan.WouldWrite {
		t.Fatalf("unexpected write recommendation: %+v", plan)
	}
}
