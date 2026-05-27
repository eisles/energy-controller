package control

import (
	"testing"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

func TestPlanDelta3AuxChargingTracksResidualExportFromCurrentLimit(t *testing.T) {
	currentLimit := 700
	soc := 70
	maxSoc := 100
	plan := PlanDelta3AuxCharging(Delta3AuxPlanInput{
		Status: domain.Status{
			ExportW:        210,
			ACChargeLimitW: 1500,
			BatterySoc:     95,
			SurplusPlan:    &domain.SurplusPlan{StrategyState: "RECOVERING"},
			UpdatedAt:      time.Date(2026, 5, 24, 10, 0, 0, 0, time.UTC),
		},
		Delta3: Delta3AuxStatus{
			Available:      true,
			SOC:            &soc,
			ACChargeLimitW: &currentLimit,
			MaxChargeSoc:   &maxSoc,
		},
	}, Delta3AuxSettings{
		Enabled:          true,
		MinChargeW:       100,
		MaxChargeW:       1500,
		SafetyMarginW:    50,
		MinCommandDiffW:  100,
		MaxIncreaseStepW: 300,
		MaxDecreaseStepW: 500,
	}, DefaultSettings())

	if plan.RecommendedACChargeLimitW != 800 {
		t.Fatalf("RecommendedACChargeLimitW = %d, want 800", plan.RecommendedACChargeLimitW)
	}
	if !plan.ShouldAdjustACChargeLimit {
		t.Fatal("ShouldAdjustACChargeLimit = false, want true")
	}
}

func TestPlanDelta3AuxChargingReducesTowardMinimumWhenImporting(t *testing.T) {
	currentLimit := 700
	soc := 70
	reserve := 72
	enabled := true
	plan := PlanDelta3AuxCharging(Delta3AuxPlanInput{
		Status: domain.Status{
			ImportW:     180,
			SurplusPlan: &domain.SurplusPlan{StrategyState: "RECOVERING"},
			UpdatedAt:   time.Date(2026, 5, 24, 10, 0, 0, 0, time.UTC),
		},
		Delta3: Delta3AuxStatus{
			Available:            true,
			SOC:                  &soc,
			ACChargeLimitW:       &currentLimit,
			BackupReserveSoc:     &reserve,
			BackupReserveEnabled: &enabled,
		},
	}, Delta3AuxSettings{
		Enabled:              true,
		MinChargeW:           100,
		MaxChargeW:           1500,
		SafetyMarginW:        50,
		MinCommandDiffW:      100,
		MaxIncreaseStepW:     300,
		MaxDecreaseStepW:     500,
		StopImportThresholdW: 50,
	}, DefaultSettings())

	if plan.RecommendedACChargeLimitW != 400 {
		t.Fatalf("RecommendedACChargeLimitW = %d, want 400", plan.RecommendedACChargeLimitW)
	}
	if !plan.ShouldAdjustACChargeLimit {
		t.Fatal("ShouldAdjustACChargeLimit = false, want true")
	}
	if !plan.ShouldDisableBackupReserve {
		t.Fatal("ShouldDisableBackupReserve = false, want true when importing with backup reserve enabled")
	}
}

func TestPlanDelta3AuxChargingWaitsForPro3Candidate(t *testing.T) {
	currentLimit := 100
	soc := 70
	plan := PlanDelta3AuxCharging(Delta3AuxPlanInput{
		Status: domain.Status{
			ExportW:        900,
			ACChargeLimitW: 400,
			BatterySoc:     70,
			SurplusPlan: &domain.SurplusPlan{
				StrategyState:             "READY",
				ShouldAdjustACChargeLimit: true,
				RecommendedACChargeLimitW: 800,
			},
		},
		Delta3: Delta3AuxStatus{
			Available:      true,
			SOC:            &soc,
			ACChargeLimitW: &currentLimit,
		},
	}, Delta3AuxSettings{Enabled: true}, DefaultSettings())

	if plan.StrategyState != "WAIT_PRO3" {
		t.Fatalf("StrategyState = %q, want WAIT_PRO3", plan.StrategyState)
	}
	if plan.ShouldAdjustACChargeLimit {
		t.Fatal("ShouldAdjustACChargeLimit = true, want false")
	}
}

func TestPlanDelta3AuxChargingBypassesPro3WaitWhenHigherPriority(t *testing.T) {
	currentLimit := 100
	soc := 70
	plan := PlanDelta3AuxCharging(Delta3AuxPlanInput{
		Status: domain.Status{
			ExportW:        900,
			ACChargeLimitW: 400,
			BatterySoc:     70,
			SurplusPlan: &domain.SurplusPlan{
				StrategyState:             "READY",
				ShouldAdjustACChargeLimit: true,
				RecommendedACChargeLimitW: 800,
			},
		},
		Delta3: Delta3AuxStatus{
			Available:      true,
			SOC:            &soc,
			ACChargeLimitW: &currentLimit,
		},
		IgnorePro3Wait: true,
	}, Delta3AuxSettings{
		Enabled:          true,
		SafetyMarginW:    50,
		MinCommandDiffW:  100,
		MaxIncreaseStepW: 300,
		MaxDecreaseStepW: 500,
	}, DefaultSettings())

	if plan.StrategyState == "WAIT_PRO3" {
		t.Fatal("StrategyState = WAIT_PRO3, want DELTA 3 Plus priority to bypass Pro3 wait")
	}
	if !plan.ShouldAdjustACChargeLimit {
		t.Fatal("ShouldAdjustACChargeLimit = false, want true")
	}
}

func TestPlanDelta3AuxChargingDoesNotWriteNearMaxSoc(t *testing.T) {
	currentLimit := 100
	soc := 99
	maxSoc := 100
	plan := PlanDelta3AuxCharging(Delta3AuxPlanInput{
		Status: domain.Status{
			ExportW:     900,
			BatterySoc:  95,
			SurplusPlan: &domain.SurplusPlan{StrategyState: "RECOVERING"},
		},
		Delta3: Delta3AuxStatus{
			Available:      true,
			SOC:            &soc,
			ACChargeLimitW: &currentLimit,
			MaxChargeSoc:   &maxSoc,
		},
	}, Delta3AuxSettings{Enabled: true, TargetMaxSocBufferPercent: 2}, DefaultSettings())

	if plan.StrategyState != "FULL" {
		t.Fatalf("StrategyState = %q, want FULL", plan.StrategyState)
	}
}

func TestPlanDelta3AuxChargingDisablesBackupReserveNearMaxSoc(t *testing.T) {
	currentLimit := 1000
	soc := 99
	maxSoc := 100
	reserve := 40
	enabled := true
	plan := PlanDelta3AuxCharging(Delta3AuxPlanInput{
		Status: domain.Status{
			ExportW:     60,
			BatterySoc:  95,
			SurplusPlan: &domain.SurplusPlan{StrategyState: "HOLD"},
		},
		Delta3: Delta3AuxStatus{
			Available:            true,
			SOC:                  &soc,
			ACChargeLimitW:       &currentLimit,
			MaxChargeSoc:         &maxSoc,
			BackupReserveSoc:     &reserve,
			BackupReserveEnabled: &enabled,
		},
	}, Delta3AuxSettings{Enabled: true, TargetMaxSocBufferPercent: 2}, DefaultSettings())

	if plan.StrategyState != "FULL" {
		t.Fatalf("StrategyState = %q, want FULL", plan.StrategyState)
	}
	if !plan.ShouldDisableBackupReserve {
		t.Fatal("ShouldDisableBackupReserve = false, want true")
	}
	if plan.RecommendedBackupReserveSoc == nil || *plan.RecommendedBackupReserveSoc != reserve {
		t.Fatalf("RecommendedBackupReserveSoc = %v, want current reserve %d for disable command", plan.RecommendedBackupReserveSoc, reserve)
	}
	if !plan.WouldWrite {
		t.Fatal("WouldWrite = false, want true")
	}
}

func TestPlanDelta3AuxChargingReducesWhenCurrentLimitExceedsOutputAwareSafeLimit(t *testing.T) {
	currentLimit := 1500
	soc := 30
	maxSoc := 90
	acIn := 469
	acOut := 469
	reserve := 0
	plan := PlanDelta3AuxCharging(Delta3AuxPlanInput{
		Status: domain.Status{
			ExportW:        1200,
			ACChargeLimitW: 1500,
			BatterySoc:     95,
			SurplusPlan:    &domain.SurplusPlan{StrategyState: "CHARGING"},
			UpdatedAt:      time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC),
		},
		Delta3: Delta3AuxStatus{
			Available:        true,
			SOC:              &soc,
			ACInW:            &acIn,
			ACOutW:           &acOut,
			ACChargeLimitW:   &currentLimit,
			MaxChargeSoc:     &maxSoc,
			BackupReserveSoc: &reserve,
		},
	}, Delta3AuxSettings{
		Enabled:          true,
		MinChargeW:       100,
		MaxChargeW:       1500,
		SafetyMarginW:    50,
		MinCommandDiffW:  100,
		MaxIncreaseStepW: 300,
		MaxDecreaseStepW: 500,
	}, DefaultSettings())

	if plan.StrategyState != "SAFE_LIMIT" {
		t.Fatalf("StrategyState = %q, want SAFE_LIMIT", plan.StrategyState)
	}
	if plan.RecommendedACChargeLimitW != 1000 {
		t.Fatalf("RecommendedACChargeLimitW = %d, want 1000", plan.RecommendedACChargeLimitW)
	}
	if plan.SafeACChargeLimitW != 1000 {
		t.Fatalf("SafeACChargeLimitW = %d, want 1000", plan.SafeACChargeLimitW)
	}
	if plan.Delta3ACOutputW == nil || *plan.Delta3ACOutputW != 469 {
		t.Fatalf("Delta3ACOutputW = %v, want 469", plan.Delta3ACOutputW)
	}
	if !plan.ShouldAdjustACChargeLimit {
		t.Fatal("ShouldAdjustACChargeLimit = false, want true when current limit exceeds output-aware safe limit")
	}
	if !plan.WouldWrite {
		t.Fatal("WouldWrite = false, want true")
	}
}

func TestPlanDelta3AuxChargingCombinesImportRecoveryWithOutputAwareSafeLimit(t *testing.T) {
	currentLimit := 1500
	soc := 30
	maxSoc := 90
	acIn := 469
	acOut := 469
	reserve := 0
	plan := PlanDelta3AuxCharging(Delta3AuxPlanInput{
		Status: domain.Status{
			ImportW:        700,
			ACChargeLimitW: 1500,
			BatterySoc:     95,
			SurplusPlan:    &domain.SurplusPlan{StrategyState: "RECOVERING"},
			UpdatedAt:      time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC),
		},
		Delta3: Delta3AuxStatus{
			Available:        true,
			SOC:              &soc,
			ACInW:            &acIn,
			ACOutW:           &acOut,
			ACChargeLimitW:   &currentLimit,
			MaxChargeSoc:     &maxSoc,
			BackupReserveSoc: &reserve,
		},
	}, Delta3AuxSettings{
		Enabled:              true,
		MinChargeW:           100,
		MaxChargeW:           1500,
		SafetyMarginW:        50,
		MinCommandDiffW:      100,
		MaxIncreaseStepW:     300,
		MaxDecreaseStepW:     500,
		StopImportThresholdW: 50,
	}, DefaultSettings())

	if plan.StrategyState != "SAFE_LIMIT" {
		t.Fatalf("StrategyState = %q, want SAFE_LIMIT", plan.StrategyState)
	}
	if plan.RecommendedACChargeLimitW != 700 {
		t.Fatalf("RecommendedACChargeLimitW = %d, want 700", plan.RecommendedACChargeLimitW)
	}
	if plan.SafeACChargeLimitW != 1000 {
		t.Fatalf("SafeACChargeLimitW = %d, want 1000", plan.SafeACChargeLimitW)
	}
	if !plan.ShouldAdjustACChargeLimit {
		t.Fatal("ShouldAdjustACChargeLimit = false, want true when importing above threshold")
	}
	if !plan.WouldWrite {
		t.Fatal("WouldWrite = false, want true")
	}
}

func TestPlanDelta3AuxChargingAlwaysCutsUnsafeLimitDuringImportRecovery(t *testing.T) {
	currentLimit := 1050
	soc := 30
	maxSoc := 90
	acOut := 469
	reserve := 0
	plan := PlanDelta3AuxCharging(Delta3AuxPlanInput{
		Status: domain.Status{
			ImportW:        50,
			ACChargeLimitW: 1500,
			BatterySoc:     95,
			SurplusPlan:    &domain.SurplusPlan{StrategyState: "RECOVERING"},
			UpdatedAt:      time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC),
		},
		Delta3: Delta3AuxStatus{
			Available:        true,
			SOC:              &soc,
			ACOutW:           &acOut,
			ACChargeLimitW:   &currentLimit,
			MaxChargeSoc:     &maxSoc,
			BackupReserveSoc: &reserve,
		},
	}, Delta3AuxSettings{
		Enabled:              true,
		MinChargeW:           100,
		MaxChargeW:           1500,
		SafetyMarginW:        50,
		MinCommandDiffW:      120,
		MaxIncreaseStepW:     300,
		MaxDecreaseStepW:     500,
		StopImportThresholdW: 50,
	}, DefaultSettings())

	if plan.StrategyState != "SAFE_LIMIT" {
		t.Fatalf("StrategyState = %q, want SAFE_LIMIT", plan.StrategyState)
	}
	if plan.RecommendedACChargeLimitW != 950 {
		t.Fatalf("RecommendedACChargeLimitW = %d, want 950", plan.RecommendedACChargeLimitW)
	}
	if plan.SafeACChargeLimitW != 1000 {
		t.Fatalf("SafeACChargeLimitW = %d, want 1000", plan.SafeACChargeLimitW)
	}
	if !plan.ShouldAdjustACChargeLimit {
		t.Fatal("ShouldAdjustACChargeLimit = false, want true when unsafe even below recovery diff threshold")
	}
	if !plan.WouldWrite {
		t.Fatal("WouldWrite = false, want true")
	}
}

func TestPlanDelta3AuxChargingAlwaysCutsUnsafeOutputAwareLimit(t *testing.T) {
	currentLimit := 1050
	soc := 30
	maxSoc := 90
	acOut := 469
	reserve := 0
	plan := PlanDelta3AuxCharging(Delta3AuxPlanInput{
		Status: domain.Status{
			ExportW:        1200,
			ACChargeLimitW: 1500,
			BatterySoc:     95,
			SurplusPlan:    &domain.SurplusPlan{StrategyState: "CHARGING"},
			UpdatedAt:      time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC),
		},
		Delta3: Delta3AuxStatus{
			Available:        true,
			SOC:              &soc,
			ACOutW:           &acOut,
			ACChargeLimitW:   &currentLimit,
			MaxChargeSoc:     &maxSoc,
			BackupReserveSoc: &reserve,
		},
	}, Delta3AuxSettings{
		Enabled:          true,
		MinChargeW:       100,
		MaxChargeW:       1500,
		SafetyMarginW:    50,
		MinCommandDiffW:  100,
		MaxIncreaseStepW: 300,
		MaxDecreaseStepW: 500,
	}, DefaultSettings())

	if plan.StrategyState != "SAFE_LIMIT" {
		t.Fatalf("StrategyState = %q, want SAFE_LIMIT", plan.StrategyState)
	}
	if plan.RecommendedACChargeLimitW != 1000 {
		t.Fatalf("RecommendedACChargeLimitW = %d, want 1000", plan.RecommendedACChargeLimitW)
	}
	if !plan.ShouldAdjustACChargeLimit {
		t.Fatal("ShouldAdjustACChargeLimit = false, want true even below normal command diff threshold")
	}
	if !plan.WouldWrite {
		t.Fatal("WouldWrite = false, want true")
	}
}

func TestPlanDelta3AuxChargingCutsUnsafeOutputAwareLimitWhenNearFull(t *testing.T) {
	currentLimit := 1050
	soc := 88
	maxSoc := 90
	acOut := 469
	reserve := 0
	plan := PlanDelta3AuxCharging(Delta3AuxPlanInput{
		Status: domain.Status{
			ExportW:        1200,
			ACChargeLimitW: 1500,
			BatterySoc:     95,
			SurplusPlan:    &domain.SurplusPlan{StrategyState: "CHARGING"},
			UpdatedAt:      time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC),
		},
		Delta3: Delta3AuxStatus{
			Available:        true,
			SOC:              &soc,
			ACOutW:           &acOut,
			ACChargeLimitW:   &currentLimit,
			MaxChargeSoc:     &maxSoc,
			BackupReserveSoc: &reserve,
		},
	}, Delta3AuxSettings{
		Enabled:          true,
		MinChargeW:       100,
		MaxChargeW:       1500,
		SafetyMarginW:    50,
		MinCommandDiffW:  100,
		MaxIncreaseStepW: 300,
		MaxDecreaseStepW: 500,
	}, DefaultSettings())

	if plan.StrategyState != "SAFE_LIMIT" {
		t.Fatalf("StrategyState = %q, want SAFE_LIMIT", plan.StrategyState)
	}
	if plan.RecommendedACChargeLimitW != 1000 {
		t.Fatalf("RecommendedACChargeLimitW = %d, want 1000", plan.RecommendedACChargeLimitW)
	}
	if !plan.ShouldAdjustACChargeLimit {
		t.Fatal("ShouldAdjustACChargeLimit = false, want true even near full SOC")
	}
	if !plan.WouldWrite {
		t.Fatal("WouldWrite = false, want true")
	}
}

func TestPlanDelta3AuxChargingRaisesBackupReserveWhenPassthroughAtSafeACLimit(t *testing.T) {
	currentLimit := 1000
	soc := 30
	maxSoc := 90
	acIn := 469
	acOut := 469
	reserve := 0
	plan := PlanDelta3AuxCharging(Delta3AuxPlanInput{
		Status: domain.Status{
			ExportW:        1200,
			ACChargeLimitW: 1500,
			BatterySoc:     95,
			SurplusPlan:    &domain.SurplusPlan{StrategyState: "CHARGING"},
			UpdatedAt:      time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC),
		},
		Delta3: Delta3AuxStatus{
			Available:        true,
			SOC:              &soc,
			ACInW:            &acIn,
			ACOutW:           &acOut,
			ACChargeLimitW:   &currentLimit,
			MaxChargeSoc:     &maxSoc,
			BackupReserveSoc: &reserve,
		},
	}, Delta3AuxSettings{
		Enabled:          true,
		MinChargeW:       100,
		MaxChargeW:       1500,
		SafetyMarginW:    50,
		MinCommandDiffW:  100,
		MaxIncreaseStepW: 300,
		MaxDecreaseStepW: 500,
	}, DefaultSettings())

	if plan.RecommendedBackupReserveSoc == nil || *plan.RecommendedBackupReserveSoc != 32 {
		t.Fatalf("RecommendedBackupReserveSoc = %v, want 32", plan.RecommendedBackupReserveSoc)
	}
	if !plan.ShouldSetBackupReserve {
		t.Fatal("ShouldSetBackupReserve = false, want true")
	}
	if plan.ShouldAdjustACChargeLimit {
		t.Fatal("ShouldAdjustACChargeLimit = true, want false when AC is already at output-aware safe limit")
	}
	if !plan.WouldWrite {
		t.Fatal("WouldWrite = false, want true")
	}
}

func TestPlanDelta3AuxChargingDisablesBackupReserveWhenExportBelowThreshold(t *testing.T) {
	currentLimit := 1000
	soc := 30
	acIn := 469
	acOut := 469
	reserve := 40
	enabled := true
	plan := PlanDelta3AuxCharging(Delta3AuxPlanInput{
		Status: domain.Status{
			ExportW:        60,
			ACChargeLimitW: 1500,
			BatterySoc:     95,
			SurplusPlan:    &domain.SurplusPlan{StrategyState: "HOLD"},
			UpdatedAt:      time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC),
		},
		Delta3: Delta3AuxStatus{
			Available:            true,
			SOC:                  &soc,
			ACInW:                &acIn,
			ACOutW:               &acOut,
			ACChargeLimitW:       &currentLimit,
			BackupReserveSoc:     &reserve,
			BackupReserveEnabled: &enabled,
		},
	}, Delta3AuxSettings{
		Enabled:          true,
		MinChargeW:       100,
		MaxChargeW:       1500,
		SafetyMarginW:    50,
		MinCommandDiffW:  100,
		MaxIncreaseStepW: 300,
		MaxDecreaseStepW: 500,
	}, DefaultSettings())

	if !plan.ShouldDisableBackupReserve {
		t.Fatal("ShouldDisableBackupReserve = false, want true")
	}
	if plan.RecommendedBackupReserveSoc == nil || *plan.RecommendedBackupReserveSoc != reserve {
		t.Fatalf("RecommendedBackupReserveSoc = %v, want current reserve %d for disable command", plan.RecommendedBackupReserveSoc, reserve)
	}
	if !plan.WouldWrite {
		t.Fatal("WouldWrite = false, want true")
	}
}

func TestPlanDelta3AuxChargingClampsBackupReserveToWritableMinimum(t *testing.T) {
	currentLimit := 1000
	soc := 1
	acIn := 469
	acOut := 469
	reserve := 0
	plan := PlanDelta3AuxCharging(Delta3AuxPlanInput{
		Status: domain.Status{
			ExportW:        1200,
			ACChargeLimitW: 1500,
			BatterySoc:     95,
			SurplusPlan:    &domain.SurplusPlan{StrategyState: "CHARGING"},
			UpdatedAt:      time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC),
		},
		Delta3: Delta3AuxStatus{
			Available:        true,
			SOC:              &soc,
			ACInW:            &acIn,
			ACOutW:           &acOut,
			ACChargeLimitW:   &currentLimit,
			BackupReserveSoc: &reserve,
		},
	}, Delta3AuxSettings{
		Enabled:          true,
		MinChargeW:       100,
		MaxChargeW:       1500,
		SafetyMarginW:    50,
		MinCommandDiffW:  100,
		MaxIncreaseStepW: 300,
		MaxDecreaseStepW: 500,
	}, DefaultSettings())

	if plan.RecommendedBackupReserveSoc == nil || *plan.RecommendedBackupReserveSoc != 5 {
		t.Fatalf("RecommendedBackupReserveSoc = %v, want 5", plan.RecommendedBackupReserveSoc)
	}
	if !plan.ShouldSetBackupReserve {
		t.Fatal("ShouldSetBackupReserve = false, want true")
	}
}

func TestPlanDelta3AuxChargingDoesNotSetBackupReserveWhenCurrentReserveUnavailable(t *testing.T) {
	currentLimit := 1000
	soc := 30
	acIn := 469
	acOut := 469
	plan := PlanDelta3AuxCharging(Delta3AuxPlanInput{
		Status: domain.Status{
			ExportW:        1200,
			ACChargeLimitW: 1500,
			BatterySoc:     95,
			SurplusPlan:    &domain.SurplusPlan{StrategyState: "CHARGING"},
			UpdatedAt:      time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC),
		},
		Delta3: Delta3AuxStatus{
			Available:      true,
			SOC:            &soc,
			ACInW:          &acIn,
			ACOutW:         &acOut,
			ACChargeLimitW: &currentLimit,
		},
	}, Delta3AuxSettings{
		Enabled:          true,
		MinChargeW:       100,
		MaxChargeW:       1500,
		SafetyMarginW:    50,
		MinCommandDiffW:  100,
		MaxIncreaseStepW: 300,
		MaxDecreaseStepW: 500,
	}, DefaultSettings())

	if plan.ShouldSetBackupReserve {
		t.Fatal("ShouldSetBackupReserve = true, want false when current backup reserve is unavailable")
	}
	if plan.WouldWrite {
		t.Fatal("WouldWrite = true, want false when only reserve write would be possible but current reserve is unknown")
	}
}

func TestPlanDelta3AuxChargingClampsBackupReserveToMaxSocBuffer(t *testing.T) {
	currentLimit := 1000
	soc := 97
	maxSoc := 100
	acIn := 469
	acOut := 469
	reserve := 30
	plan := PlanDelta3AuxCharging(Delta3AuxPlanInput{
		Status: domain.Status{
			ExportW:        1200,
			ACChargeLimitW: 1500,
			BatterySoc:     95,
			SurplusPlan:    &domain.SurplusPlan{StrategyState: "CHARGING"},
			UpdatedAt:      time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC),
		},
		Delta3: Delta3AuxStatus{
			Available:        true,
			SOC:              &soc,
			ACInW:            &acIn,
			ACOutW:           &acOut,
			ACChargeLimitW:   &currentLimit,
			MaxChargeSoc:     &maxSoc,
			BackupReserveSoc: &reserve,
		},
	}, Delta3AuxSettings{
		Enabled:                   true,
		MinChargeW:                100,
		MaxChargeW:                1500,
		SafetyMarginW:             50,
		MinCommandDiffW:           100,
		MaxIncreaseStepW:          300,
		MaxDecreaseStepW:          500,
		TargetMaxSocBufferPercent: 2,
	}, DefaultSettings())

	if plan.RecommendedBackupReserveSoc == nil || *plan.RecommendedBackupReserveSoc != 98 {
		t.Fatalf("RecommendedBackupReserveSoc = %v, want 98", plan.RecommendedBackupReserveSoc)
	}
}

func TestPlanDelta3AuxChargingDoesNotTreatDischargeAsPassthrough(t *testing.T) {
	currentLimit := 1000
	soc := 30
	acIn := 0
	acOut := 500
	plan := PlanDelta3AuxCharging(Delta3AuxPlanInput{
		Status: domain.Status{
			ExportW:        1200,
			ACChargeLimitW: 1500,
			BatterySoc:     95,
			SurplusPlan:    &domain.SurplusPlan{StrategyState: "CHARGING"},
			UpdatedAt:      time.Date(2026, 5, 26, 12, 0, 0, 0, time.UTC),
		},
		Delta3: Delta3AuxStatus{
			Available:      true,
			SOC:            &soc,
			ACInW:          &acIn,
			ACOutW:         &acOut,
			ACChargeLimitW: &currentLimit,
		},
	}, Delta3AuxSettings{
		Enabled:          true,
		MinChargeW:       100,
		MaxChargeW:       1500,
		SafetyMarginW:    50,
		MinCommandDiffW:  100,
		MaxIncreaseStepW: 300,
		MaxDecreaseStepW: 500,
	}, DefaultSettings())

	if plan.ShouldSetBackupReserve {
		t.Fatal("ShouldSetBackupReserve = true, want false for ordinary discharge")
	}
	if plan.WouldWrite {
		t.Fatal("WouldWrite = true, want false when AC is max and DELTA 3 is discharging")
	}
}
