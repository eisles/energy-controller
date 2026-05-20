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
	if !floatEqual(plan.RecommendedNightTargetKWh, 4.9152) {
		t.Fatalf("RecommendedNightTargetKWh = %f, want 4.9152", plan.RecommendedNightTargetKWh)
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

func TestPlanNightChargingUsesHourlyRadiationForPVWindowAndMorningLoad(t *testing.T) {
	fullEnergyWh := 10000
	plan := PlanNightCharging(NightChargePlanInput{
		Now:                 time.Date(2026, 5, 19, 21, 30, 0, 0, jst),
		BatterySoc:          40,
		BatteryFullEnergyWh: &fullEnergyWh,
		Forecast: &domain.WeatherForecast{
			Date:                      "2026-05-20",
			ShortwaveRadiationMJPerM2: 18,
			SunshineDurationHours:     8,
			CloudCoverMeanPercent:     20,
			HourlyShortwaveRadiation: []domain.HourlyShortwaveRadiation{
				{Time: "2026-05-20T07:00", ShortwaveRadiationWPerM2: 100},
				{Time: "2026-05-20T08:00", ShortwaveRadiationWPerM2: 300},
				{Time: "2026-05-20T09:00", ShortwaveRadiationWPerM2: 600},
			},
		},
		SolarSettings: &domain.WeatherLocation{
			PVCapacityKW:       4,
			PVPerformanceRatio: 0.75,
			DailyBaseLoadKWh:   4,
			MinimumReserveSoc:  30,
		},
		EcoFlowLoadEstimate: &domain.EcoFlowLoadEstimate{
			SampleCount:           24,
			AverageDailyOutputKWh: 12,
		},
		SimulationMode: true,
	}, DefaultSettings())

	if !floatEqual(plan.DailyEstimatedPVKWh, 3) {
		t.Fatalf("DailyEstimatedPVKWh = %f, want 3", plan.DailyEstimatedPVKWh)
	}
	if plan.PVEffectiveStartAt != "2026-05-20T08:00" || plan.PVEffectiveEndAt != "2026-05-20T09:00" {
		t.Fatalf("PV effective window = %s-%s, want 08:00-09:00", plan.PVEffectiveStartAt, plan.PVEffectiveEndAt)
	}
	if plan.PVEffectiveWindowSource != "hourly-radiation" {
		t.Fatalf("PVEffectiveWindowSource = %q, want hourly-radiation", plan.PVEffectiveWindowSource)
	}
	if !floatEqual(plan.MorningToPVStartLoadKWh, 0.5) {
		t.Fatalf("MorningToPVStartLoadKWh = %f, want 0.5", plan.MorningToPVStartLoadKWh)
	}
	if !floatEqual(plan.ForecastDaytimeDeficitKWh, 1) {
		t.Fatalf("ForecastDaytimeDeficitKWh = %f, want 1", plan.ForecastDaytimeDeficitKWh)
	}
	if plan.RecommendedNightTargetSoc != 50 {
		t.Fatalf("RecommendedNightTargetSoc = %d, want 50", plan.RecommendedNightTargetSoc)
	}
}

func TestEstimatePVForecastUsesStrongestContiguousRadiationSegment(t *testing.T) {
	estimate := EstimatePVForecast(domain.WeatherForecast{
		Date: "2026-05-20",
		HourlyShortwaveRadiation: []domain.HourlyShortwaveRadiation{
			{Time: "2026-05-20T08:00", ShortwaveRadiationWPerM2: 260},
			{Time: "2026-05-20T09:00", ShortwaveRadiationWPerM2: 80},
			{Time: "2026-05-20T10:00", ShortwaveRadiationWPerM2: 240},
			{Time: "2026-05-20T11:00", ShortwaveRadiationWPerM2: 40},
			{Time: "2026-05-20T12:00", ShortwaveRadiationWPerM2: 60},
			{Time: "2026-05-20T13:00", ShortwaveRadiationWPerM2: 500},
			{Time: "2026-05-20T14:00", ShortwaveRadiationWPerM2: 450},
		},
	}, domain.WeatherLocation{PVCapacityKW: 4, PVPerformanceRatio: 0.75})

	if estimate.PVEffectiveStartAt != "2026-05-20T13:00" || estimate.PVEffectiveEndAt != "2026-05-20T14:00" {
		t.Fatalf("PV effective window = %s-%s, want strongest contiguous segment 13:00-14:00", estimate.PVEffectiveStartAt, estimate.PVEffectiveEndAt)
	}
	if !floatEqual(estimate.DailyEstimatedPVKWh, 4.89) {
		t.Fatalf("DailyEstimatedPVKWh = %f, want 4.89", estimate.DailyEstimatedPVKWh)
	}
}

func TestPlanNightChargingUsesFallbackPVStartWhenHourlyRadiationHasNoEffectiveWindow(t *testing.T) {
	fullEnergyWh := 10000
	plan := PlanNightCharging(NightChargePlanInput{
		Now:                 time.Date(2026, 5, 19, 21, 30, 0, 0, jst),
		BatterySoc:          40,
		BatteryFullEnergyWh: &fullEnergyWh,
		Forecast: &domain.WeatherForecast{
			Date: "2026-05-20",
			HourlyShortwaveRadiation: []domain.HourlyShortwaveRadiation{
				{Time: "2026-05-20T07:00", ShortwaveRadiationWPerM2: 40},
				{Time: "2026-05-20T08:00", ShortwaveRadiationWPerM2: 80},
				{Time: "2026-05-20T09:00", ShortwaveRadiationWPerM2: 120},
			},
		},
		SolarSettings: &domain.WeatherLocation{
			PVCapacityKW:       4,
			PVPerformanceRatio: 0.75,
			DailyBaseLoadKWh:   4,
			MinimumReserveSoc:  30,
		},
		EcoFlowLoadEstimate: &domain.EcoFlowLoadEstimate{
			SampleCount:           24,
			AverageDailyOutputKWh: 12,
		},
		SimulationMode: true,
	}, DefaultSettings())

	if plan.PVEffectiveWindowSource != "hourly-radiation-no-effective-window" {
		t.Fatalf("PVEffectiveWindowSource = %q, want hourly-radiation-no-effective-window", plan.PVEffectiveWindowSource)
	}
	if plan.PVEffectiveStartAt != "2026-05-20T09:00" || plan.PVEffectiveEndAt != "2026-05-20T16:00" {
		t.Fatalf("PV effective fallback window = %s-%s, want 09:00-16:00", plan.PVEffectiveStartAt, plan.PVEffectiveEndAt)
	}
	if !floatEqual(plan.MorningToPVStartLoadKWh, 1) {
		t.Fatalf("MorningToPVStartLoadKWh = %f, want 1", plan.MorningToPVStartLoadKWh)
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

	if plan.RecommendedNightTargetSoc != 63 {
		t.Fatalf("RecommendedNightTargetSoc = %d, want 63", plan.RecommendedNightTargetSoc)
	}
	if !floatEqual(plan.RequiredNightChargeKWh, 0.98304) {
		t.Fatalf("RequiredNightChargeKWh = %f, want 0.98304", plan.RequiredNightChargeKWh)
	}
	if plan.EstimatedDeficitKWh <= 0 {
		t.Fatalf("EstimatedDeficitKWh = %f, want positive", plan.EstimatedDeficitKWh)
	}
	if !strings.Contains(plan.ActionSummary, "深夜目標SOCを63%へ設定") {
		t.Fatalf("ActionSummary = %q, want target SOC action", plan.ActionSummary)
	}
}

func TestPlanNightChargingUsesEcoFlowLoadAndMorningLoadForKWhTarget(t *testing.T) {
	fullEnergyWh := 12288
	plan := PlanNightCharging(NightChargePlanInput{
		Now:                 time.Date(2026, 5, 19, 23, 0, 0, 0, jst),
		BatterySoc:          35,
		BatteryFullEnergyWh: &fullEnergyWh,
		Forecast: &domain.WeatherForecast{
			ShortwaveRadiationMJPerM2: 12,
			SunshineDurationHours:     4,
			CloudCoverMeanPercent:     60,
		},
		SolarSettings: &domain.WeatherLocation{
			PVCapacityKW:       4,
			PVPerformanceRatio: 0.75,
			DailyBaseLoadKWh:   8,
			MinimumReserveSoc:  30,
		},
		EcoFlowLoadEstimate: &domain.EcoFlowLoadEstimate{
			SampleCount:             10,
			AverageDaytimeOutputKWh: 5,
			AverageNightOutputKWh:   2,
		},
		SimulationMode: true,
	}, DefaultSettings())

	if plan.ConsumptionSource != "ecoflow-output" {
		t.Fatalf("ConsumptionSource = %q, want ecoflow-output", plan.ConsumptionSource)
	}
	if !floatEqual(plan.EstimatedDaytimeLoadKWh, 5) {
		t.Fatalf("EstimatedDaytimeLoadKWh = %f, want 5", plan.EstimatedDaytimeLoadKWh)
	}
	if !floatEqual(plan.EstimatedMorningLoadKWh, 2) {
		t.Fatalf("EstimatedMorningLoadKWh = %f, want 2", plan.EstimatedMorningLoadKWh)
	}
	if plan.RecommendedNightTargetSoc != 51 {
		t.Fatalf("RecommendedNightTargetSoc = %d, want 51", plan.RecommendedNightTargetSoc)
	}
}

func TestPlanNightChargingKeepsTargetEnergyConsistentWithClampedTargetSoc(t *testing.T) {
	fullEnergyWh := 12288
	settings := DefaultSettings()
	plan := PlanNightCharging(NightChargePlanInput{
		Now:                 time.Date(2026, 5, 19, 23, 0, 0, 0, jst),
		BatterySoc:          20,
		BatteryFullEnergyWh: &fullEnergyWh,
		Forecast:            &domain.WeatherForecast{ShortwaveRadiationMJPerM2: 1, SunshineDurationHours: 1, CloudCoverMeanPercent: 95, PrecipitationProbabilityMax: 90},
		SolarSettings:       &domain.WeatherLocation{PVCapacityKW: 1, PVPerformanceRatio: 0.7, DailyBaseLoadKWh: 20, MinimumReserveSoc: 30},
		EcoFlowLoadEstimate: &domain.EcoFlowLoadEstimate{SampleCount: 10, AverageDaytimeOutputKWh: 20, AverageNightOutputKWh: 4},
		SimulationMode:      true,
	}, settings)

	if plan.RecommendedNightTargetSoc != settings.TargetSoc {
		t.Fatalf("RecommendedNightTargetSoc = %d, want %d", plan.RecommendedNightTargetSoc, settings.TargetSoc)
	}
	wantKWh := float64(fullEnergyWh) / 1000 * float64(settings.TargetSoc) / 100
	if !floatEqual(plan.RecommendedNightTargetKWh, wantKWh) {
		t.Fatalf("RecommendedNightTargetKWh = %f, want %f", plan.RecommendedNightTargetKWh, wantKWh)
	}
}

func TestPlanNightChargingProratesMorningLoadDuringNightWindow(t *testing.T) {
	fullEnergyWh := 12288
	plan := PlanNightCharging(NightChargePlanInput{
		Now:                 time.Date(2026, 5, 20, 3, 0, 0, 0, jst),
		BatterySoc:          35,
		BatteryFullEnergyWh: &fullEnergyWh,
		Forecast:            &domain.WeatherForecast{ShortwaveRadiationMJPerM2: 12, SunshineDurationHours: 4, CloudCoverMeanPercent: 60},
		SolarSettings:       &domain.WeatherLocation{PVCapacityKW: 4, PVPerformanceRatio: 0.75, DailyBaseLoadKWh: 5, MinimumReserveSoc: 30},
		EcoFlowLoadEstimate: &domain.EcoFlowLoadEstimate{SampleCount: 10, AverageDaytimeOutputKWh: 5, AverageNightOutputKWh: 2},
		SimulationMode:      true,
	}, DefaultSettings())

	if !floatEqual(plan.EstimatedMorningLoadKWh, 1) {
		t.Fatalf("EstimatedMorningLoadKWh = %f, want 1", plan.EstimatedMorningLoadKWh)
	}
}

func TestPlanNightChargingRecommendsSelfPoweredWhenDischargeIsNeeded(t *testing.T) {
	fullEnergyWh := 12288
	tou := true
	reserve := 30
	plan := PlanNightCharging(NightChargePlanInput{
		Now:                 time.Date(2026, 5, 19, 23, 30, 0, 0, jst),
		BatterySoc:          85,
		BatteryFullEnergyWh: &fullEnergyWh,
		BackupReserveSoc:    &reserve,
		TOUModeEnabled:      &tou,
		Forecast:            &domain.WeatherForecast{ShortwaveRadiationMJPerM2: 22, SunshineDurationHours: 10, CloudCoverMeanPercent: 10},
		SolarSettings:       &domain.WeatherLocation{PVCapacityKW: 5, PVPerformanceRatio: 0.8, DailyBaseLoadKWh: 3, MinimumReserveSoc: 30},
		SimulationMode:      true,
	}, DefaultSettings())

	if plan.RecommendedMode != "self-powered" {
		t.Fatalf("RecommendedMode = %q, want self-powered", plan.RecommendedMode)
	}
	if !plan.ShouldEnableSelfPoweredMode {
		t.Fatal("ShouldEnableSelfPoweredMode = false, want true")
	}
	if plan.RecommendedBackupReserveSoc == nil || *plan.RecommendedBackupReserveSoc != plan.RecommendedNightTargetSoc {
		t.Fatalf("RecommendedBackupReserveSoc = %v, want %d", plan.RecommendedBackupReserveSoc, plan.RecommendedNightTargetSoc)
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
		ACChargeLimitW:    400,
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

func TestPlanNightChargingBuildsCommandPlanForNightChargeWindow(t *testing.T) {
	forecast := &domain.WeatherForecast{ShortwaveRadiationMJPerM2: 3, SunshineDurationHours: 1, CloudCoverMeanPercent: 95, PrecipitationProbabilityMax: 80, PrecipitationSumMM: 12}
	reserve := 30
	tou := true
	plan := PlanNightCharging(NightChargePlanInput{
		Now:               time.Date(2026, 5, 19, 23, 30, 0, 0, jst),
		BatterySoc:        55,
		BatteryInputW:     DefaultSettings().MinChargeW,
		ACChargeLimitW:    400,
		BackupReserveSoc:  &reserve,
		TOUModeEnabled:    &tou,
		Forecast:          forecast,
		SimulationMode:    true,
		EnableRealControl: false,
		AutoControl:       false,
	}, DefaultSettings())

	if plan.RecommendedACChargeLimitW != DefaultSettings().MaxChargeW {
		t.Fatalf("RecommendedACChargeLimitW = %d, want %d", plan.RecommendedACChargeLimitW, DefaultSettings().MaxChargeW)
	}
	if plan.RecommendedBackupReserveSoc == nil || *plan.RecommendedBackupReserveSoc != DefaultSettings().TargetSoc {
		t.Fatalf("RecommendedBackupReserveSoc = %v, want %d", plan.RecommendedBackupReserveSoc, DefaultSettings().TargetSoc)
	}
	if !plan.ShouldSetACChargeLimit || !plan.ShouldSetBackupReserve || plan.ShouldDisableEnergyModes {
		t.Fatalf("command plan flags = ac:%t reserve:%t modes:%t, want ac/reserve true and modes false for TOU priority", plan.ShouldSetACChargeLimit, plan.ShouldSetBackupReserve, plan.ShouldDisableEnergyModes)
	}
	for _, want := range []string{"推奨modeはtou", "バックアップリザーブを90%へ設定", "AC充電上限を1500Wへ設定"} {
		if !strings.Contains(plan.ActionSummary, want) {
			t.Fatalf("ActionSummary = %q, want %q", plan.ActionSummary, want)
		}
	}
}

func TestPlanNightChargingRecommendsEnergyStrategyOffWhenTOUIsNotCharging(t *testing.T) {
	forecast := &domain.WeatherForecast{ShortwaveRadiationMJPerM2: 3, SunshineDurationHours: 1, CloudCoverMeanPercent: 95, PrecipitationProbabilityMax: 80, PrecipitationSumMM: 12}
	reserve := 30
	tou := true
	plan := PlanNightCharging(NightChargePlanInput{
		Now:               time.Date(2026, 5, 19, 23, 30, 0, 0, jst),
		BatterySoc:        55,
		BatteryInputW:     0,
		ACChargeLimitW:    400,
		BackupReserveSoc:  &reserve,
		TOUModeEnabled:    &tou,
		Forecast:          forecast,
		SimulationMode:    true,
		EnableRealControl: false,
		AutoControl:       false,
	}, DefaultSettings())

	if plan.RecommendedMode != "energy-strategy-off" {
		t.Fatalf("RecommendedMode = %q, want energy-strategy-off", plan.RecommendedMode)
	}
	if !plan.ShouldDisableEnergyModes {
		t.Fatal("ShouldDisableEnergyModes = false, want true when TOU is not charging")
	}
	if plan.ShouldEnableTOUMode {
		t.Fatal("ShouldEnableTOUMode = true, want false when disabling modes")
	}
}

func TestPlanNightChargingBlocksWriteUnlessAllGuardsPass(t *testing.T) {
	forecast := &domain.WeatherForecast{ShortwaveRadiationMJPerM2: 3, SunshineDurationHours: 1, CloudCoverMeanPercent: 95, PrecipitationProbabilityMax: 80, PrecipitationSumMM: 12}
	tests := []struct {
		name  string
		input NightChargePlanInput
		want  string
	}{
		{
			name: "mock mode",
			input: NightChargePlanInput{
				MockMode:          true,
				SimulationMode:    false,
				EnableRealControl: true,
				AutoControl:       true,
			},
			want: "mock mode",
		},
		{
			name: "simulation mode",
			input: NightChargePlanInput{
				SimulationMode:    true,
				EnableRealControl: true,
				AutoControl:       true,
			},
			want: "simulation mode",
		},
		{
			name: "real control disabled",
			input: NightChargePlanInput{
				SimulationMode:    false,
				EnableRealControl: false,
				AutoControl:       true,
			},
			want: "ENABLE_REAL_CONTROL=false",
		},
		{
			name: "auto control disabled",
			input: NightChargePlanInput{
				SimulationMode:    false,
				EnableRealControl: true,
				AutoControl:       false,
			},
			want: "auto control disabled",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := tt.input
			input.Now = time.Date(2026, 5, 19, 23, 30, 0, 0, jst)
			input.BatterySoc = 55
			input.BatteryInputW = DefaultSettings().MinChargeW
			input.ACChargeLimitW = 400
			input.Forecast = forecast
			plan := PlanNightCharging(input, DefaultSettings())
			if plan.WouldWrite {
				t.Fatal("WouldWrite = true, want false")
			}
			if !strings.Contains(plan.CommandBlockReason, tt.want) {
				t.Fatalf("CommandBlockReason = %q, want contains %q", plan.CommandBlockReason, tt.want)
			}
		})
	}
}

func TestPlanNightChargingSuppressesWithinMinimumInterval(t *testing.T) {
	forecast := &domain.WeatherForecast{ShortwaveRadiationMJPerM2: 3, SunshineDurationHours: 1, CloudCoverMeanPercent: 95, PrecipitationProbabilityMax: 80, PrecipitationSumMM: 12}
	now := time.Date(2026, 5, 19, 23, 30, 0, 0, jst)
	plan := PlanNightCharging(NightChargePlanInput{
		Now:               now,
		BatterySoc:        55,
		ACChargeLimitW:    400,
		Forecast:          forecast,
		Previous:          PreviousDecision{LastCommandAt: now.Add(-30 * time.Second), LastCommandTargetW: 1500},
		SimulationMode:    false,
		EnableRealControl: true,
		AutoControl:       true,
	}, DefaultSettings())

	if plan.WouldWrite {
		t.Fatal("WouldWrite = true, want false inside minimum interval")
	}
	if !plan.CommandSuppressed {
		t.Fatal("CommandSuppressed = false, want true")
	}
	if !strings.Contains(plan.CommandBlockReason, "command suppressed") {
		t.Fatalf("CommandBlockReason = %q, want command suppressed", plan.CommandBlockReason)
	}
}

func TestPlanNightChargingDoesNotSuppressReserveOrModeChangeWhenACTargetIsUnchanged(t *testing.T) {
	forecast := &domain.WeatherForecast{ShortwaveRadiationMJPerM2: 3, SunshineDurationHours: 1, CloudCoverMeanPercent: 95, PrecipitationProbabilityMax: 80, PrecipitationSumMM: 12}
	now := time.Date(2026, 5, 19, 23, 30, 0, 0, jst)
	reserve := 30
	tou := true
	plan := PlanNightCharging(NightChargePlanInput{
		Now:               now,
		BatterySoc:        55,
		BatteryInputW:     DefaultSettings().MinChargeW,
		ACChargeLimitW:    DefaultSettings().MaxChargeW,
		BackupReserveSoc:  &reserve,
		TOUModeEnabled:    &tou,
		Forecast:          forecast,
		Previous:          PreviousDecision{LastCommandAt: now.Add(-2 * time.Minute), LastCommandTargetW: DefaultSettings().MaxChargeW},
		SimulationMode:    false,
		EnableRealControl: true,
		AutoControl:       true,
	}, DefaultSettings())

	if plan.ShouldSetACChargeLimit {
		t.Fatal("ShouldSetACChargeLimit = true, want false because current AC limit already matches")
	}
	if !plan.ShouldSetBackupReserve || plan.ShouldDisableEnergyModes {
		t.Fatalf("reserve/mode flags = reserve:%t modes:%t, want reserve true and modes false for TOU priority", plan.ShouldSetBackupReserve, plan.ShouldDisableEnergyModes)
	}
	if plan.CommandSuppressed {
		t.Fatal("CommandSuppressed = true, want false for reserve/mode changes after interval")
	}
	if !plan.WouldWrite {
		t.Fatal("WouldWrite = false, want true for reserve/mode changes after interval")
	}
}

func TestPlanNightChargingKeepsModeOnlyCandidateOutOfSettingsMatchNoop(t *testing.T) {
	forecast := &domain.WeatherForecast{ShortwaveRadiationMJPerM2: 3, SunshineDurationHours: 1, CloudCoverMeanPercent: 95, PrecipitationProbabilityMax: 80, PrecipitationSumMM: 12}
	now := time.Date(2026, 5, 19, 23, 30, 0, 0, jst)
	reserve := DefaultSettings().TargetSoc
	tou := false
	plan := PlanNightCharging(NightChargePlanInput{
		Now:               now,
		BatterySoc:        55,
		BatteryInputW:     0,
		ACChargeLimitW:    DefaultSettings().MaxChargeW,
		BackupReserveSoc:  &reserve,
		TOUModeEnabled:    &tou,
		Forecast:          forecast,
		SimulationMode:    true,
		EnableRealControl: false,
		AutoControl:       false,
	}, DefaultSettings())

	if !plan.ShouldEnableTOUMode {
		t.Fatal("ShouldEnableTOUMode = false, want true")
	}
	if plan.CommandBlockReason == "night charge settings already match plan" {
		t.Fatalf("CommandBlockReason = %q, want mode-only candidate to pass no-op guard", plan.CommandBlockReason)
	}
	if !strings.Contains(plan.CommandBlockReason, "simulation mode") {
		t.Fatalf("CommandBlockReason = %q, want simulation guard", plan.CommandBlockReason)
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
