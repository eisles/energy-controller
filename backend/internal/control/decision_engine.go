package control

import (
	"math"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

type Settings struct {
	StartExportThresholdW int
	StopExportThresholdW  int
	SafetyMarginW         int
	MinChargeW            int
	MaxChargeW            int
	TargetSoc             int
	MinCommandInterval    time.Duration
	MinCommandDiffW       int
	NightSafetyMarginKWh  float64
}

type Input struct {
	GridW             int
	BatterySoc        int
	Previous          PreviousDecision
	Now               time.Time
	MockMode          bool
	SimulationMode    bool
	EnableRealControl bool
	AutoControl       bool
}

type PreviousDecision struct {
	ShouldCharge       bool
	TargetChargeW      int
	LastCommandAt      time.Time
	LastCommandTargetW int
}

type Result struct {
	GridPower          domain.GridPower
	Decision           domain.ControlDecision
	CommandAllowed     bool
	CommandSuppressed  bool
	CommandBlockReason string
}

func DefaultSettings() Settings {
	return Settings{
		StartExportThresholdW: 700,
		StopExportThresholdW:  300,
		SafetyMarginW:         150,
		MinChargeW:            400,
		MaxChargeW:            1500,
		TargetSoc:             90,
		MinCommandInterval:    60 * time.Second,
		MinCommandDiffW:       100,
		NightSafetyMarginKWh:  0.5,
	}
}

func Evaluate(input Input, settings Settings) Result {
	settings = normalizeSettings(settings)
	gridPower := CalculateGridPower(input.GridW)

	decision := domain.ControlDecision{
		ShouldCharge:  false,
		TargetChargeW: 0,
		Reason:        "importing from grid, do not charge",
	}

	switch {
	case input.BatterySoc >= settings.TargetSoc:
		decision.Reason = "battery soc is at or above target"
	case input.Previous.ShouldCharge && gridPower.ExportW >= settings.StopExportThresholdW:
		target := calculateTargetChargeW(gridPower.ExportW, settings)
		decision = domain.ControlDecision{
			ShouldCharge:  true,
			TargetChargeW: target,
			Reason:        "export remains above stop threshold, continue charging",
		}
	case gridPower.ExportW >= settings.StartExportThresholdW:
		target := calculateTargetChargeW(gridPower.ExportW, settings)
		decision = domain.ControlDecision{
			ShouldCharge:  true,
			TargetChargeW: target,
			Reason:        "export power is above start threshold",
		}
	case gridPower.ExportW > 0:
		decision.Reason = "export power is below start threshold"
	}

	allowed, suppressed := commandGate(input, decision, settings)
	blockReason := ""
	switch {
	case input.MockMode:
		allowed = false
		blockReason = "mock mode, EcoFlow write disabled"
	case input.SimulationMode:
		allowed = false
		blockReason = "simulation mode, EcoFlow write disabled"
	case !input.EnableRealControl:
		allowed = false
		blockReason = "ENABLE_REAL_CONTROL=false, EcoFlow write disabled"
	case !input.AutoControl:
		allowed = false
		blockReason = "auto control disabled, EcoFlow write disabled"
	case suppressed:
		blockReason = "command suppressed by minimum interval or command diff"
	}

	return Result{
		GridPower:          gridPower,
		Decision:           decision,
		CommandAllowed:     allowed,
		CommandSuppressed:  suppressed,
		CommandBlockReason: blockReason,
	}
}

func CalculateGridPower(gridW int) domain.GridPower {
	return domain.GridPower{
		GridW:   gridW,
		ImportW: max(0, gridW),
		ExportW: max(0, -gridW),
	}
}

func calculateTargetChargeW(exportW int, settings Settings) int {
	target := exportW - settings.SafetyMarginW
	target = clamp(target, settings.MinChargeW, settings.MaxChargeW)
	return roundDownToHundred(target)
}

func commandGate(input Input, decision domain.ControlDecision, settings Settings) (bool, bool) {
	targetChanged := abs(decision.TargetChargeW-input.Previous.LastCommandTargetW) >= settings.MinCommandDiffW
	if decision.TargetChargeW == 0 && input.Previous.LastCommandTargetW != 0 {
		targetChanged = true
	}
	if !targetChanged {
		return false, true
	}
	if !input.Previous.LastCommandAt.IsZero() && input.Now.Sub(input.Previous.LastCommandAt) < settings.MinCommandInterval {
		return false, true
	}
	return true, false
}

func normalizeSettings(settings Settings) Settings {
	defaults := DefaultSettings()
	if settings.StartExportThresholdW <= 0 {
		settings.StartExportThresholdW = defaults.StartExportThresholdW
	}
	if settings.StopExportThresholdW <= 0 {
		settings.StopExportThresholdW = defaults.StopExportThresholdW
	}
	if settings.SafetyMarginW < 0 {
		settings.SafetyMarginW = defaults.SafetyMarginW
	}
	if settings.MinChargeW <= 0 {
		settings.MinChargeW = defaults.MinChargeW
	}
	if settings.MaxChargeW <= 0 {
		settings.MaxChargeW = defaults.MaxChargeW
	}
	if settings.MaxChargeW < settings.MinChargeW {
		settings.MaxChargeW = settings.MinChargeW
	}
	if settings.TargetSoc <= 0 {
		settings.TargetSoc = defaults.TargetSoc
	}
	if settings.MinCommandInterval <= 0 {
		settings.MinCommandInterval = defaults.MinCommandInterval
	}
	if settings.MinCommandDiffW <= 0 {
		settings.MinCommandDiffW = defaults.MinCommandDiffW
	}
	if settings.NightSafetyMarginKWh < 0 {
		settings.NightSafetyMarginKWh = defaults.NightSafetyMarginKWh
	}
	if settings.StopExportThresholdW > settings.StartExportThresholdW {
		settings.StopExportThresholdW = settings.StartExportThresholdW
	}
	return settings
}

func clamp(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func roundDownToHundred(value int) int {
	return value / 100 * 100
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func ceilToInt(value float64) int {
	return int(math.Ceil(value))
}
