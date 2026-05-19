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
	if plan.ShouldRaiseBackupReserve || plan.WouldWrite {
		t.Fatalf("unexpected write recommendation: %+v", plan)
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
}
