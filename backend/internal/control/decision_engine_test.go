package control

import (
	"testing"
	"time"
)

func TestCalculateGridPower(t *testing.T) {
	tests := []struct {
		name    string
		gridW   int
		importW int
		exportW int
	}{
		{name: "import", gridW: 320, importW: 320, exportW: 0},
		{name: "export", gridW: -850, importW: 0, exportW: 850},
		{name: "neutral", gridW: 0, importW: 0, exportW: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateGridPower(tt.gridW)
			if got.GridW != tt.gridW || got.ImportW != tt.importW || got.ExportW != tt.exportW {
				t.Fatalf("CalculateGridPower(%d) = %+v, want import=%d export=%d", tt.gridW, got, tt.importW, tt.exportW)
			}
		})
	}
}

func TestEvaluateStartsChargingWhenExportExceedsThreshold(t *testing.T) {
	result := Evaluate(Input{
		GridW:             -1200,
		BatterySoc:        50,
		Now:               fixedTime(),
		SimulationMode:    true,
		EnableRealControl: false,
	}, DefaultSettings())

	if !result.Decision.ShouldCharge {
		t.Fatal("ShouldCharge = false, want true")
	}
	if result.Decision.TargetChargeW != 1000 {
		t.Fatalf("TargetChargeW = %d, want 1000", result.Decision.TargetChargeW)
	}
	if result.CommandAllowed {
		t.Fatal("CommandAllowed = true in simulation mode")
	}
}

func TestEvaluateStopsWhenBatterySocReachesTarget(t *testing.T) {
	result := Evaluate(Input{
		GridW:             -1500,
		BatterySoc:        90,
		Previous:          PreviousDecision{ShouldCharge: true, TargetChargeW: 1000},
		Now:               fixedTime(),
		SimulationMode:    true,
		EnableRealControl: false,
	}, DefaultSettings())

	if result.Decision.ShouldCharge {
		t.Fatal("ShouldCharge = true, want false")
	}
	if result.Decision.TargetChargeW != 0 {
		t.Fatalf("TargetChargeW = %d, want 0", result.Decision.TargetChargeW)
	}
}

func TestEvaluateDoesNotStartBelowStartThreshold(t *testing.T) {
	result := Evaluate(Input{
		GridW:             -650,
		BatterySoc:        50,
		Now:               fixedTime(),
		SimulationMode:    true,
		EnableRealControl: false,
	}, DefaultSettings())

	if result.Decision.ShouldCharge {
		t.Fatal("ShouldCharge = true, want false")
	}
}

func TestEvaluateContinuesChargingAboveStopThreshold(t *testing.T) {
	result := Evaluate(Input{
		GridW:             -500,
		BatterySoc:        50,
		Previous:          PreviousDecision{ShouldCharge: true, TargetChargeW: 700},
		Now:               fixedTime(),
		SimulationMode:    true,
		EnableRealControl: false,
	}, DefaultSettings())

	if !result.Decision.ShouldCharge {
		t.Fatal("ShouldCharge = false, want true due to hysteresis")
	}
	if result.Decision.TargetChargeW != 400 {
		t.Fatalf("TargetChargeW = %d, want 400", result.Decision.TargetChargeW)
	}
}

func TestEvaluateStopsBelowStopThresholdWhenCharging(t *testing.T) {
	result := Evaluate(Input{
		GridW:             -250,
		BatterySoc:        50,
		Previous:          PreviousDecision{ShouldCharge: true, TargetChargeW: 400},
		Now:               fixedTime(),
		SimulationMode:    true,
		EnableRealControl: false,
	}, DefaultSettings())

	if result.Decision.ShouldCharge {
		t.Fatal("ShouldCharge = true, want false below stop threshold")
	}
}

func TestEvaluateClampsAndRoundsTargetCharge(t *testing.T) {
	tests := []struct {
		name   string
		gridW  int
		target int
	}{
		{name: "minimum", gridW: -720, target: 500},
		{name: "rounded down", gridW: -1080, target: 900},
		{name: "maximum", gridW: -3000, target: 1500},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Evaluate(Input{
				GridW:             tt.gridW,
				BatterySoc:        50,
				Now:               fixedTime(),
				SimulationMode:    true,
				EnableRealControl: false,
			}, DefaultSettings())
			if result.Decision.TargetChargeW != tt.target {
				t.Fatalf("TargetChargeW = %d, want %d", result.Decision.TargetChargeW, tt.target)
			}
		})
	}
}

func TestEvaluateSuppressesCommandWithinMinimumInterval(t *testing.T) {
	now := fixedTime()
	result := Evaluate(Input{
		GridW:      -1400,
		BatterySoc: 50,
		Previous: PreviousDecision{
			LastCommandAt:      now.Add(-30 * time.Second),
			LastCommandTargetW: 800,
		},
		Now:               now,
		SimulationMode:    false,
		EnableRealControl: true,
	}, DefaultSettings())

	if result.CommandAllowed {
		t.Fatal("CommandAllowed = true, want false within minimum interval")
	}
	if !result.CommandSuppressed {
		t.Fatal("CommandSuppressed = false, want true")
	}
}

func TestEvaluateSuppressesSmallCommandDiff(t *testing.T) {
	now := fixedTime()
	result := Evaluate(Input{
		GridW:      -1030,
		BatterySoc: 50,
		Previous: PreviousDecision{
			LastCommandAt:      now.Add(-2 * time.Minute),
			LastCommandTargetW: 800,
		},
		Now:               now,
		SimulationMode:    false,
		EnableRealControl: true,
	}, DefaultSettings())

	if result.Decision.TargetChargeW != 800 {
		t.Fatalf("TargetChargeW = %d, want 800", result.Decision.TargetChargeW)
	}
	if result.CommandAllowed {
		t.Fatal("CommandAllowed = true, want false for small command diff")
	}
}

func TestEvaluateAllowsCommandAfterIntervalAndLargeDiff(t *testing.T) {
	now := fixedTime()
	result := Evaluate(Input{
		GridW:      -1600,
		BatterySoc: 50,
		Previous: PreviousDecision{
			LastCommandAt:      now.Add(-2 * time.Minute),
			LastCommandTargetW: 800,
		},
		Now:               now,
		AutoControl:       true,
		SimulationMode:    false,
		EnableRealControl: true,
	}, DefaultSettings())

	if !result.CommandAllowed {
		t.Fatal("CommandAllowed = false, want true")
	}
}

func TestEvaluateBlocksCommandUnlessAllRealControlGuardsPass(t *testing.T) {
	tests := []struct {
		name        string
		input       Input
		blockReason string
	}{
		{
			name:        "mock mode",
			input:       Input{MockMode: true, SimulationMode: false, EnableRealControl: true, AutoControl: true},
			blockReason: "mock mode, EcoFlow write disabled",
		},
		{
			name:        "simulation mode",
			input:       Input{MockMode: false, SimulationMode: true, EnableRealControl: true, AutoControl: true},
			blockReason: "simulation mode, EcoFlow write disabled",
		},
		{
			name:        "real control disabled",
			input:       Input{MockMode: false, SimulationMode: false, EnableRealControl: false, AutoControl: true},
			blockReason: "ENABLE_REAL_CONTROL=false, EcoFlow write disabled",
		},
		{
			name:        "auto control disabled",
			input:       Input{MockMode: false, SimulationMode: false, EnableRealControl: true, AutoControl: false},
			blockReason: "auto control disabled, EcoFlow write disabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.input.GridW = -1600
			tt.input.BatterySoc = 50
			tt.input.Now = fixedTime()

			result := Evaluate(tt.input, DefaultSettings())

			if result.CommandAllowed {
				t.Fatal("CommandAllowed = true, want false")
			}
			if result.CommandBlockReason != tt.blockReason {
				t.Fatalf("CommandBlockReason = %q, want %q", result.CommandBlockReason, tt.blockReason)
			}
		})
	}
}

func fixedTime() time.Time {
	return time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
}
