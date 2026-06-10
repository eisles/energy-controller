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
	MinDischargeReserveSoc    int
	BackupReserveMinSoc       int
	BackupReserveMaxSoc       int
	AutoRecoverACOutput       bool
}

type Delta3AuxStatus struct {
	Available            bool
	DeviceType           string
	SOC                  *int
	ACInW                *int
	ACOutW               *int
	ACChargeLimitW       *int
	MaxChargeSoc         *int
	BackupReserveSoc     *int
	BackupReserveEnabled *bool
	ACOutputEnabled      *bool
	LastError            string
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
		MinDischargeReserveSoc:    20,
		BackupReserveMinSoc:       20,
		BackupReserveMaxSoc:       90,
	}
}

func PlanDelta3AuxCharging(input Delta3AuxPlanInput, settings Delta3AuxSettings, pro3Settings Settings) domain.Delta3AuxPlan {
	settings = normalizeDelta3AuxSettings(settings)
	pro3Settings = normalizeSettings(pro3Settings)
	status := input.Status
	plan := domain.Delta3AuxPlan{
		Mode:                        "read-only",
		StrategyState:               "IDLE",
		ResidualExportW:             status.ExportW,
		SafetyMarginW:               settings.SafetyMarginW,
		CurrentACChargeLimitW:       input.Delta3.ACChargeLimitW,
		CurrentBackupReserveSoc:     input.Delta3.BackupReserveSoc,
		CurrentBackupReserveEnabled: input.Delta3.BackupReserveEnabled,
		Delta3Soc:                   input.Delta3.SOC,
		Delta3MaxChargeSoc:          input.Delta3.MaxChargeSoc,
		Delta3ACOutputW:             positiveIntPtr(delta3ACOutputLoadW(input.Delta3)),
		Delta3ACOutputEnabled:       input.Delta3.ACOutputEnabled,
		SafeACChargeLimitW:          delta3SafeACChargeLimitW(input.Delta3, settings),
		RecommendedACChargeLimitW:   valueOrZero(input.Delta3.ACChargeLimitW),
		WouldWrite:                  false,
	}

	if !settings.Enabled {
		plan.StrategyState = "DISABLED"
		plan.Reason = "DELTA3_AUX_ENABLED=false"
		return plan
	}
	if !input.Delta3.Available {
		plan.StrategyState = "UNAVAILABLE"
		plan.Reason = firstNonEmpty(input.Delta3.LastError, "auxiliary battery status unavailable")
		return plan
	}
	if input.Delta3.SOC == nil || input.Delta3.ACChargeLimitW == nil {
		plan.StrategyState = "UNAVAILABLE"
		plan.Reason = "auxiliary battery SOC or AC charge limit is unavailable"
		return plan
	}
	currentLimitW := *input.Delta3.ACChargeLimitW
	safeChargeLimitW := delta3SafeACChargeLimitW(input.Delta3, settings)
	maxChargeSoc := 100
	if input.Delta3.MaxChargeSoc != nil && *input.Delta3.MaxChargeSoc > 0 && *input.Delta3.MaxChargeSoc < maxChargeSoc {
		maxChargeSoc = *input.Delta3.MaxChargeSoc
	}

	if status.ImportW >= settings.StopImportThresholdW {
		plan.StrategyState = "RECOVERING"
		target := delta3ImportRecoveryChargeW(currentLimitW, status.ImportW, settings)
		target = min(target, safeChargeLimitW)
		if currentLimitW > safeChargeLimitW {
			plan.StrategyState = "SAFE_LIMIT"
		}
		plan.RecommendedACChargeLimitW = target
		plan.ShouldAdjustACChargeLimit = currentLimitW > safeChargeLimitW || abs(target-currentLimitW) >= settings.MinCommandDiffW
		maybeSetDelta3DischargeReserve(&plan, input.Delta3, settings)
		plan.WouldWrite = plan.ShouldAdjustACChargeLimit || plan.ShouldSetBackupReserve
		if currentLimitW > safeChargeLimitW {
			plan.Reason = fmt.Sprintf("AC charge limit exceeds output-aware safe limit while importing from grid; reduce auxiliary battery charge (%dW = max %dW - AC output %dW)", safeChargeLimitW, settings.MaxChargeW, delta3ACOutputLoadW(input.Delta3))
			return plan
		}
		if plan.ShouldSetBackupReserve {
			plan.Reason = "importing from grid; lower auxiliary battery backup reserve to the master minimum so it can discharge"
			return plan
		}
		if !plan.ShouldAdjustACChargeLimit && !backupReserveEnabled(input.Delta3.BackupReserveEnabled) {
			plan.Reason = "importing from grid; backup reserve is already disabled; use existing discharge behavior"
			return plan
		}
		plan.Reason = "importing from grid; reduce auxiliary battery charge toward safe minimum"
		return plan
	}

	if currentLimitW > safeChargeLimitW {
		plan.StrategyState = "SAFE_LIMIT"
		plan.RecommendedACChargeLimitW = safeChargeLimitW
		plan.ShouldAdjustACChargeLimit = true
		maybeDisableDelta3BackupReserve(&plan, input.Delta3)
		plan.WouldWrite = plan.ShouldAdjustACChargeLimit || plan.ShouldDisableBackupReserve
		plan.Reason = fmt.Sprintf("auxiliary battery AC charge limit exceeds output-aware safe limit (%dW = max %dW - AC output %dW)", safeChargeLimitW, settings.MaxChargeW, delta3ACOutputLoadW(input.Delta3))
		return plan
	}

	if input.Delta3.ACOutputEnabled != nil && !*input.Delta3.ACOutputEnabled {
		plan.StrategyState = "AC_OUTPUT_OFF"
		if settings.AutoRecoverACOutput {
			plan.Reason = "auxiliary battery AC output is OFF; auto recovery is allowed for this device, but AC output write payload is not verified yet"
			return plan
		}
		plan.Reason = "auxiliary battery AC output is OFF; auto recovery is not allowed for this device"
		return plan
	}

	if *input.Delta3.SOC >= maxChargeSoc-settings.TargetMaxSocBufferPercent {
		plan.StrategyState = "FULL"
		maybeDisableDelta3BackupReserve(&plan, input.Delta3)
		plan.WouldWrite = plan.ShouldDisableBackupReserve
		plan.Reason = fmt.Sprintf("auxiliary battery SOC %d%% is near max charge SOC %d%%", *input.Delta3.SOC, maxChargeSoc)
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
			plan.Reason = "charging priority keeps DELTA Pro 3 first; auxiliary battery waits while Pro3 has a surplus absorption candidate"
			return plan
		}
	}

	residualHeadroomW := status.ExportW - settings.SafetyMarginW
	if residualHeadroomW < settings.MinCommandDiffW {
		plan.StrategyState = "HOLD"
		maybeDisableDelta3BackupReserve(&plan, input.Delta3)
		plan.WouldWrite = plan.ShouldDisableBackupReserve
		plan.Reason = "residual export is below auxiliary battery adjustment threshold"
		return plan
	}

	target := currentLimitW + roundDownToHundred(residualHeadroomW)
	target = clamp(target, settings.MinChargeW, settings.MaxChargeW)
	target = min(target, safeChargeLimitW)
	target = limitStep(currentLimitW, target, settings.MaxIncreaseStepW, settings.MaxDecreaseStepW)
	target = min(target, safeChargeLimitW)
	target = roundDownToHundred(target)
	if target < settings.MinChargeW {
		target = settings.MinChargeW
	}
	plan.StrategyState = "READY"
	plan.RecommendedACChargeLimitW = target
	plan.ShouldAdjustACChargeLimit = abs(target-currentLimitW) >= settings.MinCommandDiffW
	maybeSetDelta3BackupReserve(&plan, input.Delta3, settings, pro3Settings, safeChargeLimitW)
	plan.WouldWrite = plan.ShouldAdjustACChargeLimit || plan.ShouldSetBackupReserve
	if !plan.ShouldAdjustACChargeLimit {
		if plan.ShouldSetBackupReserve {
			plan.Reason = "auxiliary battery AC charge is maxed but passthrough; raise backup reserve above current SOC"
			return plan
		}
		plan.StrategyState = "HOLD"
		plan.WouldWrite = false
		plan.Reason = "auxiliary battery target is within command diff threshold"
		return plan
	}
	if plan.ShouldSetBackupReserve {
		plan.Reason = "DELTA Pro 3 priority is satisfied; set auxiliary battery AC charge and backup reserve to absorb export"
		return plan
	}
	plan.Reason = "DELTA Pro 3 priority is satisfied; use auxiliary battery to absorb residual export"
	return plan
}

func maybeSetDelta3BackupReserve(plan *domain.Delta3AuxPlan, status Delta3AuxStatus, settings Delta3AuxSettings, pro3Settings Settings, safeChargeLimitW int) {
	if plan == nil || status.SOC == nil || status.BackupReserveSoc == nil {
		return
	}
	if status.MaxChargeSoc != nil && *status.SOC >= *status.MaxChargeSoc-settings.TargetMaxSocBufferPercent {
		return
	}
	if !plan.ShouldAdjustACChargeLimit && plan.RecommendedACChargeLimitW < safeChargeLimitW {
		return
	}
	if !delta3LooksPassthrough(status, settings) && !plan.ShouldAdjustACChargeLimit {
		return
	}
	maxChargeSoc := 100
	if status.MaxChargeSoc != nil && *status.MaxChargeSoc > 0 && *status.MaxChargeSoc < maxChargeSoc {
		maxChargeSoc = *status.MaxChargeSoc
	}
	reserveCeiling := min(settings.BackupReserveMaxSoc, maxChargeSoc-settings.TargetMaxSocBufferPercent)
	if reserveCeiling <= *status.SOC {
		return
	}
	reserveFloor := max(settings.BackupReserveMinSoc, *status.SOC+1)
	if reserveFloor > reserveCeiling {
		return
	}
	target := reserveCeiling
	if *status.BackupReserveSoc > 0 && !backupReserveKnownDisabled(status.BackupReserveEnabled) {
		target = clamp(*status.SOC+pro3Settings.ReserveRaiseStepPercent, reserveFloor, reserveCeiling)
	}
	if *status.BackupReserveSoc >= target && !backupReserveKnownDisabled(status.BackupReserveEnabled) {
		return
	}
	plan.RecommendedBackupReserveSoc = &target
	plan.ShouldSetBackupReserve = true
}

func maybeSetDelta3DischargeReserve(plan *domain.Delta3AuxPlan, status Delta3AuxStatus, settings Delta3AuxSettings) {
	if plan == nil || status.SOC == nil || status.BackupReserveSoc == nil {
		return
	}
	if !backupReserveEnabled(status.BackupReserveEnabled) {
		return
	}
	target := settings.BackupReserveMinSoc
	if *status.SOC <= target {
		return
	}
	if *status.BackupReserveSoc == target {
		return
	}
	plan.RecommendedBackupReserveSoc = &target
	plan.ShouldSetBackupReserve = true
	plan.ShouldDisableBackupReserve = false
}

func maybeDisableDelta3BackupReserve(plan *domain.Delta3AuxPlan, status Delta3AuxStatus) {
	if plan == nil || status.BackupReserveSoc == nil || !backupReserveEnabled(status.BackupReserveEnabled) {
		return
	}
	target := 5
	if *status.BackupReserveSoc >= 5 {
		target = *status.BackupReserveSoc
	}
	plan.RecommendedBackupReserveSoc = &target
	plan.ShouldDisableBackupReserve = true
}

func delta3LooksPassthrough(status Delta3AuxStatus, settings Delta3AuxSettings) bool {
	if status.ACInW == nil || status.ACOutW == nil {
		return false
	}
	return abs(*status.ACInW-abs(*status.ACOutW)) < settings.MinCommandDiffW
}

func delta3SafeACChargeLimitW(status Delta3AuxStatus, settings Delta3AuxSettings) int {
	outputW := delta3ACOutputLoadW(status)
	target := settings.MaxChargeW - outputW
	if target < settings.MinChargeW {
		return settings.MinChargeW
	}
	target = clamp(target, settings.MinChargeW, settings.MaxChargeW)
	target = roundDownToHundred(target)
	if target < settings.MinChargeW {
		return settings.MinChargeW
	}
	return target
}

func delta3ACOutputLoadW(status Delta3AuxStatus) int {
	if status.ACOutW == nil {
		return 0
	}
	return abs(*status.ACOutW)
}

func positiveIntPtr(value int) *int {
	if value <= 0 {
		return nil
	}
	return &value
}

func backupReserveEnabled(value *bool) bool {
	return value != nil && *value
}

func backupReserveKnownDisabled(value *bool) bool {
	return value != nil && !*value
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
	if settings.MinDischargeReserveSoc <= 0 {
		settings.MinDischargeReserveSoc = defaults.MinDischargeReserveSoc
	}
	settings.MinDischargeReserveSoc = clamp(settings.MinDischargeReserveSoc, 5, 100)
	if settings.BackupReserveMinSoc <= 0 {
		settings.BackupReserveMinSoc = settings.MinDischargeReserveSoc
	}
	settings.BackupReserveMinSoc = clamp(settings.BackupReserveMinSoc, 5, 100)
	settings.MinDischargeReserveSoc = settings.BackupReserveMinSoc
	if settings.BackupReserveMaxSoc <= 0 {
		settings.BackupReserveMaxSoc = defaults.BackupReserveMaxSoc
	}
	settings.BackupReserveMaxSoc = clamp(settings.BackupReserveMaxSoc, 5, 100)
	if settings.BackupReserveMaxSoc < settings.BackupReserveMinSoc {
		settings.BackupReserveMaxSoc = settings.BackupReserveMinSoc
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
