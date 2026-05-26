package control

import (
	"fmt"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

type Delta3AuxSettings struct {
	Enabled                   bool
	MinChargeW                int
	MaxChargeW                int
	SafetyMarginW             int
	MinCommandDiffW           int
	MaxIncreaseStepW          int
	MaxDecreaseStepW          int
	MinCommandInterval        time.Duration
	StopImportThresholdW      int
	TargetMaxSocBufferPercent int
}

type Delta3AuxStatus struct {
	Available      bool
	DeviceType     string
	SOC            *int
	ACInW          *int
	ACOutW         *int
	ACChargeLimitW *int
	MaxChargeSoc   *int
	LastError      string
}

type Delta3AuxPlanInput struct {
	Status         domain.Status
	Delta3         Delta3AuxStatus
	IgnorePro3Wait bool

	Pro3PreviousCommand *domain.SurplusControlCommandLog
}

func DefaultDelta3AuxSettings() Delta3AuxSettings {
	return Delta3AuxSettings{
		Enabled:                   false,
		MinChargeW:                100,
		MaxChargeW:                1500,
		SafetyMarginW:             50,
		MinCommandDiffW:           100,
		MaxIncreaseStepW:          300,
		MaxDecreaseStepW:          500,
		MinCommandInterval:        120 * time.Second,
		StopImportThresholdW:      50,
		TargetMaxSocBufferPercent: 2,
	}
}

func PlanDelta3AuxCharging(input Delta3AuxPlanInput, settings Delta3AuxSettings, pro3Settings Settings) domain.Delta3AuxPlan {
	settings = normalizeDelta3AuxSettings(settings)
	pro3Settings = normalizeSettings(pro3Settings)
	status := input.Status
	plan := domain.Delta3AuxPlan{
		Mode:                      "read-only",
		StrategyState:             "IDLE",
		ResidualExportW:           status.ExportW,
		SafetyMarginW:             settings.SafetyMarginW,
		CurrentACChargeLimitW:     input.Delta3.ACChargeLimitW,
		Delta3Soc:                 input.Delta3.SOC,
		Delta3MaxChargeSoc:        input.Delta3.MaxChargeSoc,
		RecommendedACChargeLimitW: valueOrZero(input.Delta3.ACChargeLimitW),
		WouldWrite:                false,
	}

	if !settings.Enabled {
		plan.StrategyState = "DISABLED"
		plan.Reason = "DELTA3_AUX_ENABLED=false"
		return plan
	}
	if !input.Delta3.Available {
		plan.StrategyState = "UNAVAILABLE"
		plan.Reason = firstNonEmpty(input.Delta3.LastError, "DELTA 3 Plus status unavailable")
		return plan
	}
	if input.Delta3.SOC == nil || input.Delta3.ACChargeLimitW == nil {
		plan.StrategyState = "UNAVAILABLE"
		plan.Reason = "DELTA 3 Plus SOC or AC charge limit is unavailable"
		return plan
	}

	maxChargeSoc := 100
	if input.Delta3.MaxChargeSoc != nil && *input.Delta3.MaxChargeSoc > 0 && *input.Delta3.MaxChargeSoc < maxChargeSoc {
		maxChargeSoc = *input.Delta3.MaxChargeSoc
	}
	if *input.Delta3.SOC >= maxChargeSoc-settings.TargetMaxSocBufferPercent {
		plan.StrategyState = "FULL"
		plan.Reason = fmt.Sprintf("DELTA 3 Plus SOC %d%% is near max charge SOC %d%%", *input.Delta3.SOC, maxChargeSoc)
		return plan
	}

	currentLimitW := *input.Delta3.ACChargeLimitW
	if status.ImportW >= settings.StopImportThresholdW {
		plan.StrategyState = "RECOVERING"
		target := delta3ImportRecoveryChargeW(currentLimitW, status.ImportW, settings)
		plan.RecommendedACChargeLimitW = target
		plan.ShouldAdjustACChargeLimit = abs(target-currentLimitW) >= settings.MinCommandDiffW
		plan.WouldWrite = plan.ShouldAdjustACChargeLimit
		plan.Reason = "importing from grid; reduce DELTA 3 Plus auxiliary charge toward safe minimum"
		return plan
	}

	if !input.IgnorePro3Wait {
		if input.Pro3PreviousCommand != nil && pro3CommandSettling(input.Pro3PreviousCommand, status.UpdatedAt, pro3Settings.MinCommandInterval) {
			plan.StrategyState = "WAIT_PRO3"
			plan.Reason = "waiting for recent DELTA Pro 3 command to settle"
			return plan
		}
		if shouldWaitForPro3(status, pro3Settings) {
			plan.StrategyState = "WAIT_PRO3"
			plan.Reason = "DELTA Pro 3 still has priority surplus absorption candidate"
			return plan
		}
	}

	residualHeadroomW := status.ExportW - settings.SafetyMarginW
	if residualHeadroomW < settings.MinCommandDiffW {
		plan.StrategyState = "HOLD"
		plan.Reason = "residual export is below DELTA 3 Plus auxiliary adjustment threshold"
		return plan
	}

	target := currentLimitW + roundDownToHundred(residualHeadroomW)
	target = clamp(target, settings.MinChargeW, settings.MaxChargeW)
	target = limitStep(currentLimitW, target, settings.MaxIncreaseStepW, settings.MaxDecreaseStepW)
	target = roundDownToHundred(target)
	plan.StrategyState = "READY"
	plan.RecommendedACChargeLimitW = target
	plan.ShouldAdjustACChargeLimit = abs(target-currentLimitW) >= settings.MinCommandDiffW
	plan.WouldWrite = plan.ShouldAdjustACChargeLimit
	if !plan.ShouldAdjustACChargeLimit {
		plan.StrategyState = "HOLD"
		plan.WouldWrite = false
		plan.Reason = "DELTA 3 Plus auxiliary target is within command diff threshold"
		return plan
	}
	plan.Reason = "DELTA Pro 3 priority is satisfied; use DELTA 3 Plus to absorb residual export"
	return plan
}

func normalizeDelta3AuxSettings(settings Delta3AuxSettings) Delta3AuxSettings {
	defaults := DefaultDelta3AuxSettings()
	if settings.MinChargeW <= 0 {
		settings.MinChargeW = defaults.MinChargeW
	}
	if settings.MaxChargeW <= 0 {
		settings.MaxChargeW = defaults.MaxChargeW
	}
	if settings.MaxChargeW < settings.MinChargeW {
		settings.MaxChargeW = settings.MinChargeW
	}
	if settings.SafetyMarginW < 0 {
		settings.SafetyMarginW = defaults.SafetyMarginW
	}
	if settings.MinCommandDiffW <= 0 {
		settings.MinCommandDiffW = defaults.MinCommandDiffW
	}
	if settings.MaxIncreaseStepW <= 0 {
		settings.MaxIncreaseStepW = defaults.MaxIncreaseStepW
	}
	if settings.MaxDecreaseStepW <= 0 {
		settings.MaxDecreaseStepW = defaults.MaxDecreaseStepW
	}
	if settings.MinCommandInterval <= 0 {
		settings.MinCommandInterval = defaults.MinCommandInterval
	}
	if settings.StopImportThresholdW <= 0 {
		settings.StopImportThresholdW = defaults.StopImportThresholdW
	}
	if settings.TargetMaxSocBufferPercent < 0 {
		settings.TargetMaxSocBufferPercent = defaults.TargetMaxSocBufferPercent
	}
	return settings
}

func pro3CommandSettling(previous *domain.SurplusControlCommandLog, now time.Time, interval time.Duration) bool {
	if previous == nil || previous.MeasuredAt.IsZero() || now.IsZero() || interval <= 0 {
		return false
	}
	if now.Sub(previous.MeasuredAt) >= interval {
		return false
	}
	return previous.WouldWrite || previous.CommandSent || previous.ErrorMessage != nil
}

func shouldWaitForPro3(status domain.Status, settings Settings) bool {
	plan := status.SurplusPlan
	if plan == nil {
		return true
	}
	if plan.ShouldAdjustACChargeLimit && plan.RecommendedACChargeLimitW > status.ACChargeLimitW {
		return true
	}
	if (plan.ShouldRaiseBackupReserve || plan.ShouldDisableEnergyModes) && status.BatterySoc < settings.TargetSoc {
		return true
	}
	return false
}

func delta3ImportRecoveryChargeW(currentLimitW int, importW int, settings Delta3AuxSettings) int {
	if currentLimitW <= settings.MinChargeW {
		return currentLimitW
	}
	reduceW := roundUpToHundred(importW + settings.SafetyMarginW)
	target := currentLimitW - reduceW
	if target < settings.MinChargeW {
		target = settings.MinChargeW
	}
	return target
}

func roundUpToHundred(value int) int {
	if value <= 0 {
		return 0
	}
	return ((value + 99) / 100) * 100
}

func valueOrZero(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}
