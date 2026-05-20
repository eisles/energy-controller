package control

import (
	"fmt"
	"strings"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

type NightChargePlanInput struct {
	Now                 time.Time
	BatterySoc          int
	BatteryInputW       int
	BatteryOutputW      int
	ACChargeLimitW      int
	BackupReserveSoc    *int
	BatteryFullEnergyWh *int
	TOUModeEnabled      *bool
	SelfPoweredEnabled  *bool
	ScheduledEnabled    *bool
	IntelligentEnabled  *bool
	Forecast            *domain.WeatherForecast
	SolarSettings       *domain.WeatherLocation
	EcoFlowLoadEstimate *domain.EcoFlowLoadEstimate
	Previous            PreviousDecision
	MockMode            bool
	SimulationMode      bool
	EnableRealControl   bool
	AutoControl         bool
}

func PlanNightCharging(input NightChargePlanInput, settings Settings) domain.NightChargePlan {
	settings = normalizeSettings(settings)
	minReserveSoc := 30
	if input.BackupReserveSoc != nil && *input.BackupReserveSoc > minReserveSoc {
		minReserveSoc = *input.BackupReserveSoc
	}
	if input.SolarSettings != nil && input.SolarSettings.MinimumReserveSoc > minReserveSoc {
		minReserveSoc = input.SolarSettings.MinimumReserveSoc
	}

	plan := domain.NightChargePlan{
		Mode:              "read-only",
		StrategyState:     "DAYTIME_OBSERVE",
		MinimumReserveSoc: minReserveSoc,
		WouldWrite:        false,
		TargetForecast:    input.Forecast,
	}
	if input.Forecast == nil {
		plan.RecommendedNightTargetSoc = fallbackNightTargetSoc(input, plan, settings)
		plan.ShouldChargeTonight = input.BatterySoc < plan.RecommendedNightTargetSoc
		applyBatteryEnergyEstimate(&plan, input)
		applyNightEnergyTarget(&plan, input, settings)
		recalculateNightChargeState(&plan, input)
		plan.RequiredNightChargeKWh = requiredNightChargeKWh(plan.CurrentBatteryEnergyKWh, plan.RecommendedNightTargetKWh)
		applyNightModeRecommendation(&plan, input, settings)
		applyNightChargeCommandPlan(&plan, input, settings)
		plan.ActionSummary = nightChargeActionSummary(plan)
		plan.Reason = "weather forecast is not configured; keep a conservative night charge target"
		applyNightChargeWriteGuard(&plan, input, settings)
		return plan
	}

	score := SolarForecastScore(*input.Forecast)
	plan.SolarForecastScore = score
	applySolarEstimate(&plan, input)
	targetSoc := nightTargetSocForEnergy(plan, input, settings, score)
	plan.RecommendedNightTargetSoc = clamp(targetSoc, minReserveSoc, settings.TargetSoc)
	plan.ShouldChargeTonight = input.BatterySoc < plan.RecommendedNightTargetSoc
	applyBatteryEnergyEstimate(&plan, input)
	applyNightEnergyTarget(&plan, input, settings)
	recalculateNightChargeState(&plan, input)
	plan.RequiredNightChargeKWh = requiredNightChargeKWh(plan.CurrentBatteryEnergyKWh, plan.RecommendedNightTargetKWh)
	applyNightModeRecommendation(&plan, input, settings)
	applyNightChargeCommandPlan(&plan, input, settings)
	plan.ActionSummary = nightChargeActionSummary(plan)

	switch {
	case plan.EstimatedDeficitKWh > 0:
		plan.Reason = "target daytime solar forecast may not cover daytime load; reserve more energy during low-price night hours"
	case score >= 70:
		plan.Reason = "target daytime solar forecast is strong; keep night charging modest"
	case score >= 40:
		plan.Reason = "target daytime solar forecast is moderate; keep a balanced night charge target"
	default:
		plan.Reason = "target daytime solar forecast is weak; charge more during low-price night hours"
	}
	if !plan.ShouldChargeTonight {
		plan.Reason += "; current SOC is already above the recommended night target"
	}

	applyNightChargeWriteGuard(&plan, input, settings)
	return plan
}

func recalculateNightChargeState(plan *domain.NightChargePlan, input NightChargePlanInput) {
	plan.ShouldChargeTonight = input.BatterySoc < plan.RecommendedNightTargetSoc
	plan.StrategyState = nightChargeStrategyState(input.Now, plan.ShouldChargeTonight, input.BatterySoc, plan.RecommendedNightTargetSoc)
}

func nightChargeStrategyState(now time.Time, shouldChargeTonight bool, batterySoc int, targetSoc int) string {
	if now.IsZero() {
		return "DAYTIME_OBSERVE"
	}
	hour := now.Hour()
	if hour == 7 || batterySoc >= targetSoc {
		return "NIGHT_RECOVER"
	}
	if hour >= 21 && hour < 23 {
		return "NIGHT_PLAN_READY"
	}
	if hour >= 23 || hour < 7 {
		if shouldChargeTonight {
			return "NIGHT_CHARGE_WINDOW"
		}
		return "NIGHT_RECOVER"
	}
	return "DAYTIME_OBSERVE"
}

func applySolarEstimate(plan *domain.NightChargePlan, input NightChargePlanInput) {
	if input.Forecast == nil {
		applyBatteryEnergyEstimate(plan, input)
		return
	}
	plan.SolarRadiationKWhPerM2 = input.Forecast.ShortwaveRadiationMJPerM2 / 3.6
	if input.SolarSettings == nil {
		applyConsumptionEstimate(plan, input)
		applyBatteryEnergyEstimate(plan, input)
		return
	}
	pvEstimate := EstimatePVForecast(*input.Forecast, *input.SolarSettings)
	plan.SolarRadiationKWhPerM2 = pvEstimate.SolarRadiationKWhPerM2
	plan.EstimatedPVKWh = pvEstimate.DailyEstimatedPVKWh
	plan.DailyEstimatedPVKWh = pvEstimate.DailyEstimatedPVKWh
	plan.PVEffectiveStartAt = pvEstimate.PVEffectiveStartAt
	plan.PVEffectiveEndAt = pvEstimate.PVEffectiveEndAt
	plan.PVEffectiveWindowSource = pvEstimate.PVEffectiveWindowSource
	plan.PVEffectiveRadiationWPerM2 = pvEstimate.PVEffectiveRadiationWPerM2
	applyConsumptionEstimate(plan, input)
	plan.PVUsableForEcoFlowKWh = plan.EstimatedPVKWh
	plan.EstimatedSurplusKWh = plan.EstimatedPVKWh - plan.EstimatedDaytimeLoadKWh
	if plan.EstimatedSurplusKWh < 0 {
		plan.EstimatedDeficitKWh = -plan.EstimatedSurplusKWh
		plan.EstimatedSurplusKWh = 0
	}
	plan.ForecastDaytimeDeficitKWh = plan.EstimatedDeficitKWh
	applyBatteryEnergyEstimate(plan, input)
	if plan.BatteryChargeHeadroomKWh > 0 && plan.EstimatedSurplusKWh > 0 {
		plan.EstimatedPVToBatteryKWh = minFloat(plan.EstimatedSurplusKWh, plan.BatteryChargeHeadroomKWh)
	}
}

func applyConsumptionEstimate(plan *domain.NightChargePlan, input NightChargePlanInput) {
	if input.EcoFlowLoadEstimate != nil && input.EcoFlowLoadEstimate.SampleCount > 0 && input.EcoFlowLoadEstimate.AverageDaytimeOutputKWh > 0 {
		plan.EstimatedDaytimeLoadKWh = input.EcoFlowLoadEstimate.AverageDaytimeOutputKWh
		plan.ConsumptionSource = "ecoflow-output"
	} else if input.SolarSettings != nil && input.SolarSettings.DailyBaseLoadKWh > 0 {
		plan.EstimatedDaytimeLoadKWh = input.SolarSettings.DailyBaseLoadKWh
		plan.ConsumptionSource = "manual"
	} else {
		plan.ConsumptionSource = "fallback"
	}
	if input.EcoFlowLoadEstimate != nil && input.EcoFlowLoadEstimate.SampleCount > 0 && input.EcoFlowLoadEstimate.AverageNightOutputKWh > 0 {
		plan.EstimatedMorningLoadKWh = input.EcoFlowLoadEstimate.AverageNightOutputKWh * remainingNightLoadRatio(input.Now)
	}
	plan.MorningToPVStartLoadKWh = expectedMorningToPVStartLoadKWh(input, plan.PVEffectiveStartAt)
}

func applyBatteryEnergyEstimate(plan *domain.NightChargePlan, input NightChargePlanInput) {
	batteryCapacityKWh, source := batteryCapacityKWh(input)
	if batteryCapacityKWh <= 0 {
		return
	}
	plan.BatteryCapacityKWh = batteryCapacityKWh
	plan.BatteryCapacitySource = source
	plan.CurrentBatteryEnergyKWh = batteryCapacityKWh * float64(input.BatterySoc) / 100
	plan.BatteryChargeHeadroomKWh = batteryCapacityKWh * float64(100-input.BatterySoc) / 100
	if plan.RecommendedNightTargetSoc > 0 {
		plan.RecommendedNightTargetKWh = batteryCapacityKWh * float64(plan.RecommendedNightTargetSoc) / 100
	}
	if plan.MinimumReserveSoc > 0 {
		plan.MinimumReserveKWh = batteryCapacityKWh * float64(plan.MinimumReserveSoc) / 100
	}
}

func applyNightEnergyTarget(plan *domain.NightChargePlan, input NightChargePlanInput, settings Settings) {
	applyConsumptionEstimate(plan, input)
	if plan.BatteryCapacityKWh <= 0 {
		return
	}
	plan.SafetyMarginKWh = settings.NightSafetyMarginKWh
	if plan.SafetyMarginKWh < 0 {
		plan.SafetyMarginKWh = 0
	}
	targetKWh := plan.MinimumReserveKWh + plan.EstimatedMorningLoadKWh + plan.MorningToPVStartLoadKWh + plan.EstimatedDeficitKWh + plan.SafetyMarginKWh
	if targetKWh > plan.BatteryCapacityKWh {
		targetKWh = plan.BatteryCapacityKWh
	}
	if targetKWh > 0 {
		plan.RecommendedNightTargetSoc = clamp(ceilToInt(targetKWh/plan.BatteryCapacityKWh*100), plan.MinimumReserveSoc, settings.TargetSoc)
		plan.RecommendedNightTargetKWh = plan.BatteryCapacityKWh * float64(plan.RecommendedNightTargetSoc) / 100
	}
}

func nightTargetSocForEnergy(plan domain.NightChargePlan, input NightChargePlanInput, settings Settings, score int) int {
	batteryCapacityKWh, _ := batteryCapacityKWh(input)
	if batteryCapacityKWh <= 0 {
		return nightTargetSocForSolarScore(score, settings.TargetSoc)
	}
	targetKWh := plan.MinimumReserveKWh + plan.EstimatedMorningLoadKWh + plan.MorningToPVStartLoadKWh + plan.EstimatedDeficitKWh + settings.NightSafetyMarginKWh
	if targetKWh <= 0 {
		return nightTargetSocForSolarScore(score, settings.TargetSoc)
	}
	return ceilToInt(targetKWh / batteryCapacityKWh * 100)
}

func fallbackNightTargetSoc(input NightChargePlanInput, plan domain.NightChargePlan, settings Settings) int {
	batteryCapacityKWh, _ := batteryCapacityKWh(input)
	if batteryCapacityKWh <= 0 {
		return clamp(70, plan.MinimumReserveSoc, settings.TargetSoc)
	}
	targetKWh := plan.MinimumReserveKWh + estimatedMorningLoadKWh(input) + settings.NightSafetyMarginKWh
	return clamp(ceilToInt(targetKWh/batteryCapacityKWh*100), plan.MinimumReserveSoc, settings.TargetSoc)
}

func estimatedMorningLoadKWh(input NightChargePlanInput) float64 {
	if input.EcoFlowLoadEstimate == nil || input.EcoFlowLoadEstimate.SampleCount <= 0 {
		return 0
	}
	return input.EcoFlowLoadEstimate.AverageNightOutputKWh * remainingNightLoadRatio(input.Now)
}

func remainingNightLoadRatio(now time.Time) float64 {
	if now.IsZero() {
		return 1
	}
	const nightHours = 8.0
	hour := float64(now.Hour()) + float64(now.Minute())/60 + float64(now.Second())/3600
	switch {
	case hour >= 23:
		return (24 - hour + 7) / nightHours
	case hour < 7:
		return (7 - hour) / nightHours
	default:
		return 1
	}
}

func batteryCapacityKWh(input NightChargePlanInput) (float64, string) {
	if input.BatteryFullEnergyWh != nil && *input.BatteryFullEnergyWh > 0 {
		return float64(*input.BatteryFullEnergyWh) / 1000, "device"
	}
	if input.SolarSettings != nil && input.SolarSettings.BatteryCapacityKWh > 0 {
		return input.SolarSettings.BatteryCapacityKWh, "manual"
	}
	return 0, ""
}

func requiredNightChargeKWh(currentKWh, targetKWh float64) float64 {
	if targetKWh <= currentKWh {
		return 0
	}
	return targetKWh - currentKWh
}

func nightChargeActionSummary(plan domain.NightChargePlan) string {
	actions := make([]string, 0, 4)
	if plan.RecommendedMode != "" {
		actions = append(actions, fmt.Sprintf("推奨modeは%s", plan.RecommendedMode))
	}
	if plan.ShouldChargeTonight {
		actions = append(actions, fmt.Sprintf("深夜目標SOCを%d%%へ設定", plan.RecommendedNightTargetSoc))
		if plan.RequiredNightChargeKWh > 0 {
			actions = append(actions, fmt.Sprintf("不足分%.1fkWhを深夜に確保", plan.RequiredNightChargeKWh))
		}
	} else {
		actions = append(actions, fmt.Sprintf("深夜充電は抑制し%d%%を維持", plan.RecommendedNightTargetSoc))
	}
	if plan.ShouldDisableEnergyModes {
		actions = append(actions, "夜間充電前にenergy strategy modesを全OFF")
	}
	if plan.ShouldEnableTOUMode {
		actions = append(actions, "TOUをONに維持")
	}
	if plan.ShouldEnableSelfPoweredMode {
		actions = append(actions, "self-powered modeへ切り替え")
	}
	if plan.ShouldSetBackupReserve && plan.RecommendedBackupReserveSoc != nil {
		actions = append(actions, fmt.Sprintf("バックアップリザーブを%d%%へ設定", *plan.RecommendedBackupReserveSoc))
	}
	if plan.ShouldSetACChargeLimit {
		actions = append(actions, fmt.Sprintf("AC充電上限を%dWへ設定", plan.RecommendedACChargeLimitW))
	}
	if plan.EstimatedPVToBatteryKWh > 0 {
		actions = append(actions, fmt.Sprintf("翌日日中PVで最大%.1fkWh充電見込み", plan.EstimatedPVToBatteryKWh))
	}
	if plan.PVEffectiveStartAt != "" && plan.PVEffectiveEndAt != "" {
		actions = append(actions, fmt.Sprintf("PV有効時間帯%s-%s", timeLabel(plan.PVEffectiveStartAt), timeLabel(plan.PVEffectiveEndAt)))
	}
	return strings.Join(actions, "; ")
}

func applyNightModeRecommendation(plan *domain.NightChargePlan, input NightChargePlanInput, settings Settings) {
	if plan.StrategyState != "NIGHT_CHARGE_WINDOW" && !(plan.StrategyState == "NIGHT_RECOVER" && isNightChargeTime(input.Now)) {
		plan.RecommendedMode = "observe"
		return
	}
	if !plan.ShouldChargeTonight && plan.CurrentBatteryEnergyKWh > plan.RecommendedNightTargetKWh && plan.RecommendedNightTargetKWh > 0 {
		plan.RecommendedMode = "self-powered"
		plan.ShouldEnableSelfPoweredMode = !boolPtrTrue(input.SelfPoweredEnabled)
		plan.ShouldEnableTOUMode = false
		return
	}
	if plan.ShouldChargeTonight && touChargeIneffective(input, settings) {
		plan.RecommendedMode = "energy-strategy-off"
		plan.ShouldEnableTOUMode = false
		return
	}
	if boolPtrTrue(input.TOUModeEnabled) || plan.ShouldChargeTonight {
		plan.RecommendedMode = "tou"
		plan.ShouldEnableTOUMode = !boolPtrTrue(input.TOUModeEnabled)
		return
	}
	plan.RecommendedMode = "energy-strategy-off"
}

func touChargeIneffective(input NightChargePlanInput, settings Settings) bool {
	return boolPtrTrue(input.TOUModeEnabled) && input.BatteryInputW < settings.MinChargeW
}

func applyNightChargeCommandPlan(plan *domain.NightChargePlan, input NightChargePlanInput, settings Settings) {
	if plan.StrategyState != "NIGHT_CHARGE_WINDOW" && !(plan.StrategyState == "NIGHT_RECOVER" && isNightChargeTime(input.Now)) {
		return
	}
	if plan.RecommendedMode == "self-powered" {
		recommendedReserve := plan.RecommendedNightTargetSoc
		plan.RecommendedBackupReserveSoc = &recommendedReserve
		plan.ShouldSetBackupReserve = input.BackupReserveSoc == nil || *input.BackupReserveSoc != recommendedReserve
		return
	}
	if !plan.ShouldChargeTonight {
		return
	}
	plan.RecommendedACChargeLimitW = settings.MaxChargeW
	plan.ShouldSetACChargeLimit = input.ACChargeLimitW <= 0 || abs(input.ACChargeLimitW-plan.RecommendedACChargeLimitW) >= settings.MinCommandDiffW
	recommendedReserve := plan.RecommendedNightTargetSoc
	plan.RecommendedBackupReserveSoc = &recommendedReserve
	plan.ShouldSetBackupReserve = input.BackupReserveSoc == nil || *input.BackupReserveSoc != recommendedReserve
	plan.ShouldDisableEnergyModes = plan.RecommendedMode == "energy-strategy-off" && hasEnabledEnergyMode(SurplusPlanInput{
		TOUModeEnabled:     input.TOUModeEnabled,
		SelfPoweredEnabled: input.SelfPoweredEnabled,
		ScheduledEnabled:   input.ScheduledEnabled,
		IntelligentEnabled: input.IntelligentEnabled,
	})
}

func isNightChargeTime(now time.Time) bool {
	if now.IsZero() {
		return false
	}
	hour := now.Hour()
	return hour >= 23 || hour < 7
}

func applyNightChargeWriteGuard(plan *domain.NightChargePlan, input NightChargePlanInput, settings Settings) {
	if !plan.ShouldChargeTonight {
		plan.CommandBlockReason = "current SOC is already above the recommended night target"
		return
	}
	if plan.StrategyState != "NIGHT_CHARGE_WINDOW" {
		plan.CommandBlockReason = "outside night charge window"
		return
	}
	if !nightChargeHasCandidateChange(*plan) {
		plan.CommandBlockReason = "night charge settings already match plan"
		return
	}
	allowed, suppressed := nightChargeCommandGate(*plan, input, settings)
	plan.CommandSuppressed = suppressed

	switch {
	case input.MockMode:
		plan.CommandBlockReason = "mock mode keeps EcoFlow write disabled"
	case input.SimulationMode:
		plan.CommandBlockReason = "simulation mode keeps EcoFlow write disabled"
	case !input.EnableRealControl:
		plan.CommandBlockReason = "ENABLE_REAL_CONTROL=false keeps EcoFlow write disabled"
	case !input.AutoControl:
		plan.CommandBlockReason = "auto control disabled keeps EcoFlow write disabled"
	case suppressed:
		plan.CommandBlockReason = "command suppressed by minimum interval or command diff"
	case allowed:
		plan.WouldWrite = true
		plan.CommandBlockReason = ""
	}
	if plan.CommandBlockReason != "" {
		plan.Reason += "; " + plan.CommandBlockReason
	}
}

func nightChargeCommandGate(plan domain.NightChargePlan, input NightChargePlanInput, settings Settings) (bool, bool) {
	if !nightChargeHasCandidateChange(plan) {
		return false, true
	}
	if !input.Previous.LastCommandAt.IsZero() && input.Now.Sub(input.Previous.LastCommandAt) < settings.MinCommandInterval {
		return false, true
	}
	return true, false
}

func nightChargeHasCandidateChange(plan domain.NightChargePlan) bool {
	return plan.ShouldSetACChargeLimit ||
		plan.ShouldSetBackupReserve ||
		plan.ShouldDisableEnergyModes ||
		plan.ShouldEnableTOUMode ||
		plan.ShouldEnableSelfPoweredMode
}

func SolarForecastScore(forecast domain.WeatherForecast) int {
	radiationScore := int(forecast.ShortwaveRadiationMJPerM2 / 25 * 87)
	sunshineScore := int(forecast.SunshineDurationHours / 10 * 10)
	cloudScore := (100 - clamp(forecast.CloudCoverMeanPercent, 0, 100)) / 10
	penalty := forecast.PrecipitationProbabilityMax/20 + int(forecast.PrecipitationSumMM/5)
	return clamp(radiationScore+sunshineScore+cloudScore-penalty, 0, 100)
}

func nightTargetSocForSolarScore(score int, maxTargetSoc int) int {
	switch {
	case score >= 70:
		return 50
	case score >= 40:
		return 70
	default:
		return maxTargetSoc
	}
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func timeLabel(value string) string {
	parts := strings.Split(value, "T")
	if len(parts) != 2 || len(parts[1]) < 5 {
		return value
	}
	return parts[1][:5]
}
