package control

import (
	"math"
	"strings"
	"testing"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

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
	if plan.BatteryCapacitySource != "device" {
		t.Fatalf("BatteryCapacitySource = %q, want device", plan.BatteryCapacitySource)
	}
	if plan.MinimumReserveSoc != 35 {
		t.Fatalf("MinimumReserveSoc = %d, want 35", plan.MinimumReserveSoc)
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
