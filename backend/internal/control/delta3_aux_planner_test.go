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
	plan := PlanDelta3AuxCharging(Delta3AuxPlanInput{
		Status: domain.Status{
			ImportW:     180,
			SurplusPlan: &domain.SurplusPlan{StrategyState: "RECOVERING"},
			UpdatedAt:   time.Date(2026, 5, 24, 10, 0, 0, 0, time.UTC),
		},
		Delta3: Delta3AuxStatus{
			Available:      true,
			SOC:            &soc,
			ACChargeLimitW: &currentLimit,
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
