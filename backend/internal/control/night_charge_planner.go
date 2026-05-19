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
	ACChargeLimitW      int
	BackupReserveSoc    *int
	BatteryFullEnergyWh *int
	TOUModeEnabled      *bool
	SelfPoweredEnabled  *bool
	ScheduledEnabled    *bool
	IntelligentEnabled  *bool
	Forecast            *domain.WeatherForecast
	SolarSettings       *domain.WeatherLocation
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
		plan.RecommendedNightTargetSoc = clamp(70, minReserveSoc, settings.TargetSoc)
		plan.ShouldChargeTonight = input.BatterySoc < plan.RecommendedNightTargetSoc
		applyBatteryEnergyEstimate(&plan, input)
		plan.RequiredNightChargeKWh = requiredNightChargeKWh(plan.CurrentBatteryEnergyKWh, plan.RecommendedNightTargetKWh)
		plan.StrategyState = nightChargeStrategyState(input.Now, plan.ShouldChargeTonight, input.BatterySoc, plan.RecommendedNightTargetSoc)
		applyNightChargeCommandPlan(&plan, input, settings)
		plan.ActionSummary = nightChargeActionSummary(plan)
		plan.Reason = "weather forecast is not configured; keep a conservative night charge target"
		applyNightChargeWriteGuard(&plan, input, settings)
		return plan
	}

	score := SolarForecastScore(*input.Forecast)
	plan.SolarForecastScore = score
	applySolarEstimate(&plan, input)
	targetSoc := nightTargetSocForSolarScore(score, settings.TargetSoc)
	plan.RecommendedNightTargetSoc = clamp(targetSoc, minReserveSoc, settings.TargetSoc)
	plan.ShouldChargeTonight = input.BatterySoc < plan.RecommendedNightTargetSoc
	applyBatteryEnergyEstimate(&plan, input)
	plan.RequiredNightChargeKWh = requiredNightChargeKWh(plan.CurrentBatteryEnergyKWh, plan.RecommendedNightTargetKWh)
	plan.StrategyState = nightChargeStrategyState(input.Now, plan.ShouldChargeTonight, input.BatterySoc, plan.RecommendedNightTargetSoc)
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
		applyBatteryEnergyEstimate(plan, input)
		return
	}
	ratio := input.SolarSettings.PVPerformanceRatio
	if ratio <= 0 {
		ratio = 0.75
	}
	plan.EstimatedPVKWh = plan.SolarRadiationKWhPerM2 * input.SolarSettings.PVCapacityKW * ratio
	plan.EstimatedDaytimeLoadKWh = input.SolarSettings.DailyBaseLoadKWh
	plan.EstimatedSurplusKWh = plan.EstimatedPVKWh - plan.EstimatedDaytimeLoadKWh
	if plan.EstimatedSurplusKWh < 0 {
		plan.EstimatedDeficitKWh = -plan.EstimatedSurplusKWh
		plan.EstimatedSurplusKWh = 0
	}
	applyBatteryEnergyEstimate(plan, input)
	if plan.BatteryChargeHeadroomKWh > 0 && plan.EstimatedSurplusKWh > 0 {
		plan.EstimatedPVToBatteryKWh = minFloat(plan.EstimatedSurplusKWh, plan.BatteryChargeHeadroomKWh)
	}
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
	if plan.ShouldSetBackupReserve && plan.RecommendedBackupReserveSoc != nil {
		actions = append(actions, fmt.Sprintf("バックアップリザーブを%d%%へ設定", *plan.RecommendedBackupReserveSoc))
	}
	if plan.ShouldSetACChargeLimit {
		actions = append(actions, fmt.Sprintf("AC充電上限を%dWへ設定", plan.RecommendedACChargeLimitW))
	}
	if plan.EstimatedPVToBatteryKWh > 0 {
		actions = append(actions, fmt.Sprintf("翌日日中PVで最大%.1fkWh充電見込み", plan.EstimatedPVToBatteryKWh))
	}
	return strings.Join(actions, "; ")
}

func applyNightChargeCommandPlan(plan *domain.NightChargePlan, input NightChargePlanInput, settings Settings) {
	if plan.StrategyState != "NIGHT_CHARGE_WINDOW" || !plan.ShouldChargeTonight {
		return
	}
	plan.RecommendedACChargeLimitW = settings.MaxChargeW
	plan.ShouldSetACChargeLimit = input.ACChargeLimitW <= 0 || abs(input.ACChargeLimitW-plan.RecommendedACChargeLimitW) >= settings.MinCommandDiffW
	recommendedReserve := plan.RecommendedNightTargetSoc
	plan.RecommendedBackupReserveSoc = &recommendedReserve
	plan.ShouldSetBackupReserve = input.BackupReserveSoc == nil || *input.BackupReserveSoc != recommendedReserve
	plan.ShouldDisableEnergyModes = hasEnabledEnergyMode(SurplusPlanInput{
		TOUModeEnabled:     input.TOUModeEnabled,
		SelfPoweredEnabled: input.SelfPoweredEnabled,
		ScheduledEnabled:   input.ScheduledEnabled,
		IntelligentEnabled: input.IntelligentEnabled,
	})
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
	if !plan.ShouldSetACChargeLimit && !plan.ShouldSetBackupReserve && !plan.ShouldDisableEnergyModes {
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
	hasCandidateChange := plan.ShouldSetACChargeLimit || plan.ShouldSetBackupReserve || plan.ShouldDisableEnergyModes
	if !hasCandidateChange {
		return false, true
	}
	if !input.Previous.LastCommandAt.IsZero() && input.Now.Sub(input.Previous.LastCommandAt) < settings.MinCommandInterval {
		return false, true
	}
	return true, false
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
