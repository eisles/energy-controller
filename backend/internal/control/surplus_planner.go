package control

import (
	"fmt"
	"strings"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

const minImportRecoveryChargeW = 400

type SurplusPlanInput struct {
	GridW                  int
	MockMode               bool
	BatterySoc             int
	BatteryInputW          int
	BatteryOutputW         int
	ACChargeLimitW         int
	BackupReserveSoc       *int
	DefaultReserveSoc      int
	MinDischargeReserveSoc int
	TOUModeEnabled         *bool
	SelfPoweredEnabled     *bool
	ScheduledEnabled       *bool
	IntelligentEnabled     *bool
	SimulationMode         bool
	EnableRealControl      bool
	AutoControl            bool
	TariffControl          *domain.TariffControlContext
}

func PlanSurplusCharging(input SurplusPlanInput, settings Settings) domain.SurplusPlan {
	settings = normalizeSettings(settings)
	gridPower := CalculateGridPower(input.GridW)
	netBatteryW := input.BatteryInputW - input.BatteryOutputW
	plan := domain.SurplusPlan{
		Mode:                  "read-only",
		StrategyState:         "IDLE",
		NetBatteryW:           netBatteryW,
		RequiredStartExportW:  conservativeStartExportW(input.BatteryOutputW, settings),
		AvailableStartMarginW: gridPower.ExportW - conservativeStartExportW(input.BatteryOutputW, settings),
		WouldWrite:            false,
	}
	applyTariffContextToSurplusPlan(&plan, input.TariffControl)

	defaultReserveSoc := defaultReserveSoc(input, settings)
	switch {
	case gridPower.ImportW > 0:
		plan.StrategyState = "RECOVERING"
		plan.RecommendedACChargeLimitW = calculateImportRecoveryChargeW(input.ACChargeLimitW, gridPower.ImportW, settings)
		plan.ShouldAdjustACChargeLimit = abs(input.ACChargeLimitW-plan.RecommendedACChargeLimitW) >= settings.MinCommandDiffW
		applyRecoveryModePlan(&plan, input, settings)
		recoveryReserveSoc := importRecoveryReserveSoc(input, settings)
		if input.BackupReserveSoc != nil && input.BatterySoc > recoveryReserveSoc && *input.BackupReserveSoc > recoveryReserveSoc {
			recommendedReserve := recoveryReserveSoc
			plan.RecommendedBackupReserveSoc = &recommendedReserve
			plan.ShouldLowerBackupReserve = true
		}
		plan.ActionSummary = surplusActionSummary(plan)
		plan.Reason = "importing from grid; recover by stopping surplus charge and allowing discharge to reserve floor"
		if tariffPrefersImportDischarge(input.TariffControl) {
			plan.TariffControlReason = "non-low-price period; prioritize battery discharge and suppress grid charging"
			plan.Reason += "; non-low-price period prioritizes battery discharge"
		}
		plan.WouldWrite = writeAllowed(input) && (plan.ShouldAdjustACChargeLimit || plan.ShouldLowerBackupReserve || plan.ShouldDisableEnergyModes || plan.ShouldEnableTOUMode)
		return plan
	case input.BatterySoc >= settings.TargetSoc:
		plan.StrategyState = "RECOVERING"
		applyRecoveryModePlan(&plan, input, settings)
		if input.BackupReserveSoc != nil && *input.BackupReserveSoc > defaultReserveSoc {
			recommendedReserve := defaultReserveSoc
			plan.RecommendedBackupReserveSoc = &recommendedReserve
			plan.ShouldLowerBackupReserve = true
		}
		plan.ActionSummary = surplusActionSummary(plan)
		plan.Reason = "battery soc is at or above target; stop surplus charge and restore default reserve"
		plan.WouldWrite = writeAllowed(input) && (plan.ShouldLowerBackupReserve || plan.ShouldDisableEnergyModes || plan.ShouldEnableTOUMode)
		return plan
	case gridPower.ExportW < settings.StopExportThresholdW && !isSurplusTrackingCharge(input, settings):
		plan.Reason = "export power is below stop threshold; keep TOU mode and wait"
		return plan
	}

	if isSurplusTrackingCharge(input, settings) {
		plan.StrategyState = "CHARGING"
	} else {
		if smallSurplusPassThroughCandidate(input, gridPower.ExportW, plan.RequiredStartExportW, settings) {
			plan.StrategyState = "PASSTHROUGH"
			recommendedReserve := input.BatterySoc
			plan.RecommendedBackupReserveSoc = &recommendedReserve
			plan.ShouldAlignBackupReserve = input.BackupReserveSoc == nil || *input.BackupReserveSoc != recommendedReserve
			plan.ActionSummary = surplusActionSummary(plan)
			plan.Reason = fmt.Sprintf("small surplus is below normal charge start requirement; keep TOU on and align backup reserve to current SOC for pass-through behavior (%dW < %dW)", gridPower.ExportW, plan.RequiredStartExportW)
			return plan
		}
		plan.StrategyState = "READY"
		if gridPower.ExportW < plan.RequiredStartExportW {
			plan.StrategyState = "IDLE"
			plan.Reason = fmt.Sprintf("export power is below conservative start requirement (%dW = battery output %dW + min charge %dW + margin %dW)", plan.RequiredStartExportW, input.BatteryOutputW, settings.MinChargeW, settings.SafetyMarginW)
			return plan
		}
	}

	recommendedAC := calculateStartACChargeW(gridPower.ExportW, input.BatteryOutputW, settings)
	if plan.StrategyState == "CHARGING" {
		recommendedAC = calculateTrackingACChargeW(input.ACChargeLimitW, gridPower.ExportW, settings)
	}
	plan.RecommendedACChargeLimitW = recommendedAC
	plan.ShouldAdjustACChargeLimit = abs(recommendedAC-input.ACChargeLimitW) >= settings.MinCommandDiffW

	if input.BackupReserveSoc != nil {
		recommendedReserve := clamp(input.BatterySoc+settings.ReserveRaiseStepPercent, *input.BackupReserveSoc, settings.TargetSoc)
		plan.RecommendedBackupReserveSoc = &recommendedReserve
		plan.ShouldRaiseBackupReserve = recommendedReserve > *input.BackupReserveSoc && recommendedReserve > input.BatterySoc
	}
	if hasEnabledEnergyMode(input) {
		plan.ShouldDisableEnergyModes = true
	}
	plan.ActionSummary = surplusActionSummary(plan)

	switch {
	case input.SimulationMode:
		plan.Reason = surplusPlanReason(plan.StrategyState, "simulation mode keeps EcoFlow write disabled")
	case input.MockMode:
		plan.Reason = surplusPlanReason(plan.StrategyState, "mock mode keeps EcoFlow write disabled")
	case !input.EnableRealControl:
		plan.Reason = surplusPlanReason(plan.StrategyState, "ENABLE_REAL_CONTROL=false keeps EcoFlow write disabled")
	case !input.AutoControl:
		plan.Reason = surplusPlanReason(plan.StrategyState, "auto control disabled keeps EcoFlow write disabled")
	default:
		plan.Reason = surplusPlanReason(plan.StrategyState, "planner recommends charging adjustments")
		plan.WouldWrite = plan.ShouldAdjustACChargeLimit || plan.ShouldRaiseBackupReserve || plan.ShouldDisableEnergyModes
	}

	if hasEnabledEnergyMode(input) {
		plan.Reason += "; EcoFlow energy strategy mode blocks surplus charging until disabled"
	}
	return plan
}

func applyTariffContextToSurplusPlan(plan *domain.SurplusPlan, tariff *domain.TariffControlContext) {
	if plan == nil || tariff == nil {
		return
	}
	plan.TariffPeriod = tariff.CurrentPeriod
	plan.TariffRateYen = tariff.CurrentRateYen
}

func surplusPlanReason(strategyState string, suffix string) string {
	if strategyState == "CHARGING" {
		return "surplus tracking condition met; " + suffix
	}
	return "conservative surplus start condition met; " + suffix
}

func surplusActionSummary(plan domain.SurplusPlan) string {
	actions := make([]string, 0, 3)
	if plan.ShouldRaiseBackupReserve && plan.RecommendedBackupReserveSoc != nil {
		actions = append(actions, fmt.Sprintf("バックアップリザーブを%d%%へ引き上げ", *plan.RecommendedBackupReserveSoc))
	}
	if plan.ShouldLowerBackupReserve && plan.RecommendedBackupReserveSoc != nil {
		actions = append(actions, fmt.Sprintf("バックアップリザーブを%d%%へ戻す", *plan.RecommendedBackupReserveSoc))
	}
	if plan.ShouldAlignBackupReserve && plan.RecommendedBackupReserveSoc != nil {
		actions = append(actions, fmt.Sprintf("バックアップリザーブを現在SOCの%d%%へ合わせる", *plan.RecommendedBackupReserveSoc))
	}
	if plan.ShouldAdjustACChargeLimit {
		actions = append(actions, fmt.Sprintf("AC充電上限を%dWへ設定", plan.RecommendedACChargeLimitW))
	}
	if plan.ShouldDisableEnergyModes {
		actions = append(actions, "energy strategy modesを全OFF")
	}
	if plan.ShouldEnableTOUMode {
		actions = append(actions, "TOUをONに戻す")
	}
	return strings.Join(actions, "; ")
}

func applyRecoveryModePlan(plan *domain.SurplusPlan, input SurplusPlanInput, settings Settings) {
	if tariffImportDischargeRecovery(input, settings) {
		return
	}
	if boolPtrTrue(input.TOUModeEnabled) {
		return
	}
	if hasEnabledNonTOUEnergyMode(input) {
		plan.ShouldDisableEnergyModes = true
		return
	}
	plan.ShouldEnableTOUMode = boolPtrFalse(input.TOUModeEnabled)
}

func tariffImportDischargeRecovery(input SurplusPlanInput, settings Settings) bool {
	return input.GridW > 0 &&
		tariffPrefersImportDischarge(input.TariffControl) &&
		input.BatterySoc > importRecoveryReserveSoc(input, settings)
}

func conservativeStartExportW(batteryOutputW int, settings Settings) int {
	if batteryOutputW < 0 {
		batteryOutputW = 0
	}
	return batteryOutputW + settings.MinChargeW + settings.SafetyMarginW
}

func smallSurplusPassThroughCandidate(input SurplusPlanInput, exportW int, requiredStartExportW int, settings Settings) bool {
	return settings.PassThroughEnabled &&
		exportW > 0 &&
		exportW < requiredStartExportW &&
		input.BatteryOutputW > 0 &&
		input.BackupReserveSoc != nil &&
		input.BatterySoc < settings.TargetSoc &&
		boolPtrTrue(input.TOUModeEnabled)
}

func calculateStartACChargeW(exportW int, batteryOutputW int, settings Settings) int {
	target := exportW - max(0, batteryOutputW) - settings.SafetyMarginW
	target = clamp(target, settings.MinChargeW, settings.MaxChargeW)
	return roundDownToHundred(target)
}

func calculateTrackingACChargeW(currentLimitW int, exportW int, settings Settings) int {
	target := currentLimitW + exportW - settings.TargetExportBufferW
	target = clamp(target, settings.MinChargeW, settings.MaxChargeW)
	target = limitStep(currentLimitW, target, settings.MaxIncreaseStepW, settings.MaxDecreaseStepW)
	return roundDownToHundred(target)
}

func limitStep(current int, target int, maxIncrease int, maxDecrease int) int {
	if target > current+maxIncrease {
		return current + maxIncrease
	}
	if target < current-maxDecrease {
		return current - maxDecrease
	}
	return target
}

func calculateImportRecoveryChargeW(currentLimitW int, importW int, settings Settings) int {
	if currentLimitW <= minImportRecoveryChargeW {
		return currentLimitW
	}
	recommended := currentLimitW - importW - settings.SafetyMarginW
	if recommended <= minImportRecoveryChargeW {
		return minImportRecoveryChargeW
	}
	return roundDownToHundred(recommended)
}

func hasEnabledEnergyMode(input SurplusPlanInput) bool {
	return boolPtrTrue(input.TOUModeEnabled) ||
		boolPtrTrue(input.SelfPoweredEnabled) ||
		boolPtrTrue(input.ScheduledEnabled) ||
		boolPtrTrue(input.IntelligentEnabled)
}

func hasEnabledNonTOUEnergyMode(input SurplusPlanInput) bool {
	return boolPtrTrue(input.SelfPoweredEnabled) ||
		boolPtrTrue(input.ScheduledEnabled) ||
		boolPtrTrue(input.IntelligentEnabled)
}

func isSurplusTrackingCharge(input SurplusPlanInput, settings Settings) bool {
	netBatteryW := input.BatteryInputW - input.BatteryOutputW
	if netBatteryW >= settings.EffectiveChargeThresholdW {
		return true
	}
	return input.ACChargeLimitW >= settings.MinChargeW &&
		input.BackupReserveSoc != nil &&
		*input.BackupReserveSoc > input.BatterySoc
}

func boolPtrTrue(value *bool) bool {
	return value != nil && *value
}

func boolPtrFalse(value *bool) bool {
	return value != nil && !*value
}

func normalizeReserveSoc(value int) int {
	if value <= 0 {
		return 30
	}
	if value > 100 {
		return 100
	}
	return value
}

func defaultReserveSoc(input SurplusPlanInput, settings Settings) int {
	if input.DefaultReserveSoc > 0 {
		return normalizeReserveSoc(input.DefaultReserveSoc)
	}
	return normalizeReserveSoc(settings.DefaultReserveSoc)
}

func importRecoveryReserveSoc(input SurplusPlanInput, settings Settings) int {
	if input.MinDischargeReserveSoc > 0 {
		return normalizeReserveSoc(input.MinDischargeReserveSoc)
	}
	return defaultReserveSoc(input, settings)
}

func writeAllowed(input SurplusPlanInput) bool {
	return !input.MockMode && !input.SimulationMode && input.EnableRealControl && input.AutoControl
}
