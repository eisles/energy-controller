package control

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

var jst = time.FixedZone("JST", 9*60*60)

func TestPlanNightChargingKeepsTargetLowForSunnyForecast(t *testing.T) {
	reserve := 30
	plan := PlanNightCharging(NightChargePlanInput{
		BatterySoc:        55,
		BackupReserveSoc:  &reserve,
		Forecast:          &domain.WeatherForecast{ShortwaveRadiationMJPerM2: 22, SunshineDurationHours: 10, CloudCoverMeanPercent: 10, PrecipitationProbabilityMax: 0},
		SimulationMode:    true,
		EnableRealControl: false,
		AutoControl:       false,
	}, DefaultSettings())

	if plan.SolarForecastScore < 70 {
		t.Fatalf("SolarForecastScore = %d, want >= 70", plan.SolarForecastScore)
	}
	if plan.RecommendedNightTargetSoc != 50 {
		t.Fatalf("RecommendedNightTargetSoc = %d, want 50", plan.RecommendedNightTargetSoc)
	}
	if plan.ShouldChargeTonight {
		t.Fatal("ShouldChargeTonight = true, want false")
	}
	if plan.WouldWrite {
		t.Fatal("WouldWrite = true, want false in simulation mode")
	}
}

func TestPlanNightChargingRaisesTargetForWeakForecast(t *testing.T) {
	reserve := 30
	plan := PlanNightCharging(NightChargePlanInput{
		BatterySoc:        55,
		BackupReserveSoc:  &reserve,
		Forecast:          &domain.WeatherForecast{ShortwaveRadiationMJPerM2: 3, SunshineDurationHours: 1, CloudCoverMeanPercent: 95, PrecipitationProbabilityMax: 80, PrecipitationSumMM: 12},
		SimulationMode:    true,
		EnableRealControl: false,
		AutoControl:       false,
	}, DefaultSettings())

	if plan.SolarForecastScore >= 40 {
		t.Fatalf("SolarForecastScore = %d, want < 40", plan.SolarForecastScore)
	}
	if plan.RecommendedNightTargetSoc != DefaultSettings().TargetSoc {
		t.Fatalf("RecommendedNightTargetSoc = %d, want %d", plan.RecommendedNightTargetSoc, DefaultSettings().TargetSoc)
	}
	if !plan.ShouldChargeTonight {
		t.Fatal("ShouldChargeTonight = false, want true")
	}
	if !strings.Contains(plan.Reason, "weak") {
		t.Fatalf("Reason = %q, want weak forecast reason", plan.Reason)
	}
}

func TestSolarForecastScoreKeepsRainyButBrightDayModerate(t *testing.T) {
	score := SolarForecastScore(domain.WeatherForecast{
		ShortwaveRadiationMJPerM2:   11.52,
		SunshineDurationHours:       4,
		CloudCoverMeanPercent:       60,
		PrecipitationProbabilityMax: 70,
		PrecipitationSumMM:          12,
	})

	if score < 40 || score >= 70 {
		t.Fatalf("SolarForecastScore = %d, want moderate score", score)
	}
}

func TestPlanNightChargingEstimatesPVGeneration(t *testing.T) {
	fullEnergyWh := 12288
	plan := PlanNightCharging(NightChargePlanInput{
		BatterySoc:          75,
		BatteryFullEnergyWh: &fullEnergyWh,
		Forecast: &domain.WeatherForecast{
			ShortwaveRadiationMJPerM2: 18,
			SunshineDurationHours:     8,
			CloudCoverMeanPercent:     20,
		},
		SolarSettings: &domain.WeatherLocation{
			PVCapacityKW:       5,
			PVPerformanceRatio: 0.8,
			DailyBaseLoadKWh:   6,
			BatteryCapacityKWh: 4,
			MinimumReserveSoc:  35,
		},
		SimulationMode: true,
	}, DefaultSettings())

	if !floatEqual(plan.SolarRadiationKWhPerM2, 5) {
		t.Fatalf("SolarRadiationKWhPerM2 = %f, want 5", plan.SolarRadiationKWhPerM2)
	}
	if !floatEqual(plan.EstimatedPVKWh, 20) {
		t.Fatalf("EstimatedPVKWh = %f, want 20", plan.EstimatedPVKWh)
	}
	if !floatEqual(plan.EstimatedSurplusKWh, 14) {
		t.Fatalf("EstimatedSurplusKWh = %f, want 14", plan.EstimatedSurplusKWh)
	}
	if !floatEqual(plan.BatteryChargeHeadroomKWh, 3.072) {
		t.Fatalf("BatteryChargeHeadroomKWh = %f, want 3.072", plan.BatteryChargeHeadroomKWh)
	}
	if !floatEqual(plan.BatteryCapacityKWh, 12.288) {
		t.Fatalf("BatteryCapacityKWh = %f, want 12.288", plan.BatteryCapacityKWh)
	}
	if !floatEqual(plan.CurrentBatteryEnergyKWh, 9.216) {
		t.Fatalf("CurrentBatteryEnergyKWh = %f, want 9.216", plan.CurrentBatteryEnergyKWh)
	}
	if !floatEqual(plan.RecommendedNightTargetKWh, 6.144) {
		t.Fatalf("RecommendedNightTargetKWh = %f, want 6.144", plan.RecommendedNightTargetKWh)
	}
	if !floatEqual(plan.MinimumReserveKWh, 4.3008) {
		t.Fatalf("MinimumReserveKWh = %f, want 4.3008", plan.MinimumReserveKWh)
	}
	if !floatEqual(plan.EstimatedPVToBatteryKWh, 3.072) {
		t.Fatalf("EstimatedPVToBatteryKWh = %f, want 3.072", plan.EstimatedPVToBatteryKWh)
	}
	if plan.BatteryCapacitySource != "device" {
		t.Fatalf("BatteryCapacitySource = %q, want device", plan.BatteryCapacitySource)
	}
	if plan.MinimumReserveSoc != 35 {
		t.Fatalf("MinimumReserveSoc = %d, want 35", plan.MinimumReserveSoc)
	}
}

func TestPlanNightChargingShowsRequiredNightChargeEnergyForWeakForecast(t *testing.T) {
	fullEnergyWh := 12288
	reserve := 30
	plan := PlanNightCharging(NightChargePlanInput{
		BatterySoc:          55,
		BackupReserveSoc:    &reserve,
		BatteryFullEnergyWh: &fullEnergyWh,
		Forecast: &domain.WeatherForecast{
			ShortwaveRadiationMJPerM2:   3,
			SunshineDurationHours:       1,
			CloudCoverMeanPercent:       95,
			PrecipitationProbabilityMax: 80,
			PrecipitationSumMM:          12,
		},
		SolarSettings: &domain.WeatherLocation{
			PVCapacityKW:       4,
			PVPerformanceRatio: 0.75,
			DailyBaseLoadKWh:   6,
		},
		SimulationMode: true,
	}, DefaultSettings())

	if plan.RecommendedNightTargetSoc != DefaultSettings().TargetSoc {
		t.Fatalf("RecommendedNightTargetSoc = %d, want %d", plan.RecommendedNightTargetSoc, DefaultSettings().TargetSoc)
	}
	if !floatEqual(plan.RequiredNightChargeKWh, 4.3008) {
		t.Fatalf("RequiredNightChargeKWh = %f, want 4.3008", plan.RequiredNightChargeKWh)
	}
	if plan.EstimatedDeficitKWh <= 0 {
		t.Fatalf("EstimatedDeficitKWh = %f, want positive", plan.EstimatedDeficitKWh)
	}
	if !strings.Contains(plan.ActionSummary, "深夜目標SOCを90%へ設定") {
		t.Fatalf("ActionSummary = %q, want target SOC action", plan.ActionSummary)
	}
}

func TestPlanNightChargingStrategyStatesByTime(t *testing.T) {
	forecast := &domain.WeatherForecast{ShortwaveRadiationMJPerM2: 3, SunshineDurationHours: 1, CloudCoverMeanPercent: 95, PrecipitationProbabilityMax: 80, PrecipitationSumMM: 12}
	settings := DefaultSettings()
	tests := []struct {
		name      string
		now       time.Time
		soc       int
		wantState string
	}{
		{name: "daytime observe", now: time.Date(2026, 5, 19, 15, 0, 0, 0, jst), soc: 55, wantState: "DAYTIME_OBSERVE"},
		{name: "plan ready", now: time.Date(2026, 5, 19, 22, 30, 0, 0, jst), soc: 55, wantState: "NIGHT_PLAN_READY"},
		{name: "charge window", now: time.Date(2026, 5, 19, 23, 30, 0, 0, jst), soc: 55, wantState: "NIGHT_CHARGE_WINDOW"},
		{name: "recover at seven", now: time.Date(2026, 5, 20, 7, 0, 0, 0, jst), soc: 55, wantState: "NIGHT_RECOVER"},
		{name: "recover when target reached", now: time.Date(2026, 5, 19, 23, 30, 0, 0, jst), soc: 95, wantState: "NIGHT_RECOVER"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := PlanNightCharging(NightChargePlanInput{
				Now:            tt.now,
				BatterySoc:     tt.soc,
				Forecast:       forecast,
				SimulationMode: true,
			}, settings)
			if plan.StrategyState != tt.wantState {
				t.Fatalf("StrategyState = %q, want %q", plan.StrategyState, tt.wantState)
			}
		})
	}
}

func TestPlanNightChargingWouldWriteOnlyInsideNightChargeWindow(t *testing.T) {
	forecast := &domain.WeatherForecast{ShortwaveRadiationMJPerM2: 3, SunshineDurationHours: 1, CloudCoverMeanPercent: 95, PrecipitationProbabilityMax: 80, PrecipitationSumMM: 12}
	baseInput := NightChargePlanInput{
		BatterySoc:        55,
		Forecast:          forecast,
		SimulationMode:    false,
		EnableRealControl: true,
		AutoControl:       true,
	}

	planReadyInput := baseInput
	planReadyInput.Now = time.Date(2026, 5, 19, 22, 30, 0, 0, jst)
	planReady := PlanNightCharging(planReadyInput, DefaultSettings())
	if planReady.WouldWrite {
		t.Fatalf("WouldWrite = true during %s, want false", planReady.StrategyState)
	}

	windowInput := baseInput
	windowInput.Now = time.Date(2026, 5, 19, 23, 30, 0, 0, jst)
	window := PlanNightCharging(windowInput, DefaultSettings())
	if !window.WouldWrite {
		t.Fatalf("WouldWrite = false during %s, want true", window.StrategyState)
	}
}

func floatEqual(got float64, want float64) bool {
	return math.Abs(got-want) < 0.000001
}

func TestPlanNightChargingUsesConservativeTargetWithoutForecast(t *testing.T) {
	plan := PlanNightCharging(NightChargePlanInput{BatterySoc: 60, Forecast: nil, SimulationMode: true}, DefaultSettings())

	if plan.RecommendedNightTargetSoc != 70 {
		t.Fatalf("RecommendedNightTargetSoc = %d, want 70", plan.RecommendedNightTargetSoc)
	}
	if !plan.ShouldChargeTonight {
		t.Fatal("ShouldChargeTonight = false, want true")
	}
}
