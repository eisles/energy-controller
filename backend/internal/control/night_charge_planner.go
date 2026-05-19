package control

import "github.com/eisles/energy-controller/backend/internal/domain"

type NightChargePlanInput struct {
	BatterySoc          int
	BackupReserveSoc    *int
	BatteryFullEnergyWh *int
	Forecast            *domain.WeatherForecast
	SolarSettings       *domain.WeatherLocation
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
		MinimumReserveSoc: minReserveSoc,
		WouldWrite:        false,
		TargetForecast:    input.Forecast,
	}
	if input.Forecast == nil {
		plan.RecommendedNightTargetSoc = clamp(70, minReserveSoc, settings.TargetSoc)
		plan.ShouldChargeTonight = input.BatterySoc < plan.RecommendedNightTargetSoc
		plan.Reason = "weather forecast is not configured; keep a conservative night charge target"
		return plan
	}

	score := SolarForecastScore(*input.Forecast)
	plan.SolarForecastScore = score
	applySolarEstimate(&plan, input)
	targetSoc := nightTargetSocForSolarScore(score, settings.TargetSoc)
	plan.RecommendedNightTargetSoc = clamp(targetSoc, minReserveSoc, settings.TargetSoc)
	plan.ShouldChargeTonight = input.BatterySoc < plan.RecommendedNightTargetSoc

	switch {
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

	switch {
	case input.SimulationMode:
		plan.Reason += "; simulation mode keeps EcoFlow write disabled"
	case !input.EnableRealControl:
		plan.Reason += "; ENABLE_REAL_CONTROL=false keeps EcoFlow write disabled"
	case !input.AutoControl:
		plan.Reason += "; auto control disabled keeps EcoFlow write disabled"
	default:
		plan.WouldWrite = plan.ShouldChargeTonight
	}
	return plan
}

func applySolarEstimate(plan *domain.NightChargePlan, input NightChargePlanInput) {
	if input.Forecast == nil {
		return
	}
	plan.SolarRadiationKWhPerM2 = input.Forecast.ShortwaveRadiationMJPerM2 / 3.6
	if input.SolarSettings == nil {
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
		plan.EstimatedSurplusKWh = 0
	}
	batteryCapacityKWh := 0.0
	if input.BatteryFullEnergyWh != nil && *input.BatteryFullEnergyWh > 0 {
		batteryCapacityKWh = float64(*input.BatteryFullEnergyWh) / 1000
		plan.BatteryCapacitySource = "device"
	} else if input.SolarSettings.BatteryCapacityKWh > 0 {
		batteryCapacityKWh = input.SolarSettings.BatteryCapacityKWh
		plan.BatteryCapacitySource = "manual"
	}
	if batteryCapacityKWh > 0 {
		plan.BatteryChargeHeadroomKWh = batteryCapacityKWh * float64(100-input.BatterySoc) / 100
	}
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
