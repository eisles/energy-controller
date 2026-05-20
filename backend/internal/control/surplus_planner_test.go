package control

import (
	"strings"
	"testing"
)

func TestPlanSurplusChargingRecommendsACAndReserve(t *testing.T) {
	reserve := 30
	tou := true
	plan := PlanSurplusCharging(SurplusPlanInput{
		GridW:             -1200,
		BatterySoc:        78,
		BatteryInputW:     0,
		BatteryOutputW:    0,
		ACChargeLimitW:    0,
		BackupReserveSoc:  &reserve,
		TOUModeEnabled:    &tou,
		SimulationMode:    true,
		EnableRealControl: false,
	}, SettingsWithTargetSoc(100))

	if plan.RecommendedACChargeLimitW != 1000 {
		t.Fatalf("RecommendedACChargeLimitW = %d, want 1000", plan.RecommendedACChargeLimitW)
	}
	if plan.StrategyState != "READY" {
		t.Fatalf("StrategyState = %q, want READY", plan.StrategyState)
	}
	if plan.RecommendedBackupReserveSoc == nil || *plan.RecommendedBackupReserveSoc != 80 {
		t.Fatalf("RecommendedBackupReserveSoc = %v, want 80", plan.RecommendedBackupReserveSoc)
	}
	if !plan.ShouldRaiseBackupReserve {
		t.Fatal("ShouldRaiseBackupReserve = false, want true")
	}
	if !plan.ShouldAdjustACChargeLimit {
		t.Fatal("ShouldAdjustACChargeLimit = false, want true")
	}
	if !plan.ShouldDisableEnergyModes {
		t.Fatal("ShouldDisableEnergyModes = false, want true")
	}
	for _, want := range []string{"バックアップリザーブを80%へ引き上げ", "AC充電上限を1000Wへ設定", "energy strategy modesを全OFF"} {
		if !strings.Contains(plan.ActionSummary, want) {
			t.Fatalf("ActionSummary = %q, want contains %q", plan.ActionSummary, want)
		}
	}
	if plan.WouldWrite {
		t.Fatal("WouldWrite = true in simulation mode")
	}
	if got := plan.Reason; got == "" || !strings.Contains(got, "energy strategy mode blocks surplus charging") {
		t.Fatalf("Reason = %q, want energy strategy mode note", got)
	}
}

func TestPlanSurplusChargingWaitsBelowConservativeStartRequirement(t *testing.T) {
	reserve := 85
	plan := PlanSurplusCharging(SurplusPlanInput{
		GridW:             -768,
		BatterySoc:        97,
		BatteryInputW:     0,
		BatteryOutputW:    359,
		ACChargeLimitW:    400,
		BackupReserveSoc:  &reserve,
		SimulationMode:    true,
		EnableRealControl: false,
	}, SettingsWithTargetSoc(100))

	if plan.StrategyState != "IDLE" {
		t.Fatalf("StrategyState = %q, want IDLE", plan.StrategyState)
	}
	if plan.RequiredStartExportW != 909 {
		t.Fatalf("RequiredStartExportW = %d, want 909", plan.RequiredStartExportW)
	}
	if plan.AvailableStartMarginW != -141 {
		t.Fatalf("AvailableStartMarginW = %d, want -141", plan.AvailableStartMarginW)
	}
	if plan.ActionSummary != "" {
		t.Fatalf("ActionSummary = %q, want empty", plan.ActionSummary)
	}
	if !strings.Contains(plan.Reason, "conservative start requirement") {
		t.Fatalf("Reason = %q, want conservative start requirement", plan.Reason)
	}
}

func TestPlanSurplusChargingUsesPassThroughForSmallSurplusWhenSocIsHigh(t *testing.T) {
	tou := true
	reserve := 85
	settings := DefaultSettings()
	settings.PassThroughEnabled = true
	plan := PlanSurplusCharging(SurplusPlanInput{
		GridW:             -768,
		BatterySoc:        97,
		BatteryInputW:     0,
		BatteryOutputW:    359,
		ACChargeLimitW:    400,
		BackupReserveSoc:  &reserve,
		TOUModeEnabled:    &tou,
		SimulationMode:    true,
		EnableRealControl: false,
	}, SettingsWithTargetSocAndPassThrough(100))

	if plan.StrategyState != "PASSTHROUGH" {
		t.Fatalf("StrategyState = %q, want PASSTHROUGH", plan.StrategyState)
	}
	if plan.RecommendedBackupReserveSoc == nil || *plan.RecommendedBackupReserveSoc != 97 {
		t.Fatalf("RecommendedBackupReserveSoc = %v, want 97", plan.RecommendedBackupReserveSoc)
	}
	if !plan.ShouldAlignBackupReserve {
		t.Fatal("ShouldAlignBackupReserve = false, want true")
	}
	if plan.WouldWrite {
		t.Fatal("WouldWrite = true, want false until pass-through write gets a dedicated feature flag")
	}
	if plan.ShouldDisableEnergyModes || plan.ShouldAdjustACChargeLimit {
		t.Fatalf("unexpected normal charging action: %+v", plan)
	}
	if !strings.Contains(plan.ActionSummary, "バックアップリザーブを現在SOCの97%へ合わせる") {
		t.Fatalf("ActionSummary = %q, want pass-through reserve action", plan.ActionSummary)
	}
	if !strings.Contains(plan.Reason, "pass-through behavior") {
		t.Fatalf("Reason = %q, want pass-through behavior", plan.Reason)
	}
}

func TestPlanSurplusChargingPassThroughDoesNothingWhenReserveAlreadyMatchesSoc(t *testing.T) {
	tou := true
	reserve := 97
	plan := PlanSurplusCharging(SurplusPlanInput{
		GridW:             -768,
		BatterySoc:        97,
		BatteryInputW:     0,
		BatteryOutputW:    359,
		ACChargeLimitW:    400,
		BackupReserveSoc:  &reserve,
		TOUModeEnabled:    &tou,
		SimulationMode:    true,
		EnableRealControl: false,
	}, SettingsWithTargetSocAndPassThrough(100))

	if plan.StrategyState != "PASSTHROUGH" {
		t.Fatalf("StrategyState = %q, want PASSTHROUGH", plan.StrategyState)
	}
	if plan.ShouldAlignBackupReserve {
		t.Fatal("ShouldAlignBackupReserve = true, want false")
	}
	if plan.ActionSummary != "" {
		t.Fatalf("ActionSummary = %q, want empty", plan.ActionSummary)
	}
}

func TestPlanSurplusChargingDoesNotUsePassThroughWithoutBackupReserveStatus(t *testing.T) {
	tou := true
	plan := PlanSurplusCharging(SurplusPlanInput{
		GridW:             -768,
		BatterySoc:        97,
		BatteryInputW:     0,
		BatteryOutputW:    359,
		ACChargeLimitW:    400,
		BackupReserveSoc:  nil,
		TOUModeEnabled:    &tou,
		SimulationMode:    true,
		EnableRealControl: false,
	}, SettingsWithTargetSoc(100))

	if plan.StrategyState == "PASSTHROUGH" {
		t.Fatalf("StrategyState = %q, want non-PASSTHROUGH when backup reserve is unavailable", plan.StrategyState)
	}
	if plan.ShouldAlignBackupReserve {
		t.Fatal("ShouldAlignBackupReserve = true, want false when backup reserve is unavailable")
	}
	if strings.Contains(plan.ActionSummary, "バックアップリザーブを現在SOC") {
		t.Fatalf("ActionSummary = %q, want no pass-through reserve action", plan.ActionSummary)
	}
}

func TestPlanSurplusChargingRecoversAtTargetSocBeforePassThrough(t *testing.T) {
	tou := true
	reserve := 97
	plan := PlanSurplusCharging(SurplusPlanInput{
		GridW:             -768,
		BatterySoc:        97,
		BatteryInputW:     0,
		BatteryOutputW:    359,
		ACChargeLimitW:    400,
		BackupReserveSoc:  &reserve,
		DefaultReserveSoc: 85,
		TOUModeEnabled:    &tou,
		SimulationMode:    true,
		EnableRealControl: false,
	}, SettingsWithTargetSoc(97))

	if plan.StrategyState != "RECOVERING" {
		t.Fatalf("StrategyState = %q, want RECOVERING", plan.StrategyState)
	}
	if plan.ShouldAlignBackupReserve {
		t.Fatal("ShouldAlignBackupReserve = true, want false")
	}
	if !plan.ShouldLowerBackupReserve {
		t.Fatal("ShouldLowerBackupReserve = false, want true")
	}
	if !strings.Contains(plan.Reason, "battery soc is at or above target") {
		t.Fatalf("Reason = %q, want target SOC recovery", plan.Reason)
	}
}

func TestPlanSurplusChargingStartsAtMinimumWhenConservativeRequirementMet(t *testing.T) {
	tou := true
	reserve := 85
	plan := PlanSurplusCharging(SurplusPlanInput{
		GridW:             -950,
		BatterySoc:        97,
		BatteryInputW:     0,
		BatteryOutputW:    359,
		ACChargeLimitW:    200,
		BackupReserveSoc:  &reserve,
		TOUModeEnabled:    &tou,
		SimulationMode:    true,
		EnableRealControl: false,
	}, SettingsWithTargetSoc(100))

	if plan.StrategyState != "READY" {
		t.Fatalf("StrategyState = %q, want READY", plan.StrategyState)
	}
	if plan.RecommendedACChargeLimitW != 400 {
		t.Fatalf("RecommendedACChargeLimitW = %d, want 400", plan.RecommendedACChargeLimitW)
	}
	if !plan.ShouldDisableEnergyModes {
		t.Fatal("ShouldDisableEnergyModes = false, want true")
	}
	if !strings.Contains(plan.ActionSummary, "AC充電上限を400Wへ設定") {
		t.Fatalf("ActionSummary = %q, want AC charge action", plan.ActionSummary)
	}
}

func TestPlanSurplusChargingTracksExportWhileAlreadyCharging(t *testing.T) {
	reserve := 79
	plan := PlanSurplusCharging(SurplusPlanInput{
		GridW:             -488,
		BatterySoc:        77,
		BatteryInputW:     743,
		BatteryOutputW:    246,
		ACChargeLimitW:    500,
		BackupReserveSoc:  &reserve,
		SimulationMode:    true,
		EnableRealControl: false,
	}, DefaultSettings())

	if plan.StrategyState != "CHARGING" {
		t.Fatalf("StrategyState = %q, want CHARGING", plan.StrategyState)
	}
	if plan.RecommendedACChargeLimitW != 800 {
		t.Fatalf("RecommendedACChargeLimitW = %d, want 800", plan.RecommendedACChargeLimitW)
	}
	if !plan.ShouldAdjustACChargeLimit {
		t.Fatal("ShouldAdjustACChargeLimit = false, want true")
	}
	if strings.Contains(plan.Reason, "conservative surplus start") {
		t.Fatalf("Reason = %q, want tracking reason", plan.Reason)
	}
}

func TestPlanSurplusChargingLimitsTrackingIncreaseStep(t *testing.T) {
	reserve := 79
	settings := DefaultSettings()
	plan := PlanSurplusCharging(SurplusPlanInput{
		GridW:             -1288,
		BatterySoc:        77,
		BatteryInputW:     740,
		BatteryOutputW:    252,
		ACChargeLimitW:    500,
		BackupReserveSoc:  &reserve,
		SimulationMode:    true,
		EnableRealControl: false,
	}, settings)

	want := 500 + settings.MaxIncreaseStepW
	if plan.RecommendedACChargeLimitW != want {
		t.Fatalf("RecommendedACChargeLimitW = %d, want %d", plan.RecommendedACChargeLimitW, want)
	}
}

func TestPlanSurplusChargingLimitsTrackingDecreaseStep(t *testing.T) {
	reserve := 79
	settings := DefaultSettings()
	settings.TargetExportBufferW = 900
	plan := PlanSurplusCharging(SurplusPlanInput{
		GridW:             -100,
		BatterySoc:        77,
		BatteryInputW:     900,
		BatteryOutputW:    300,
		ACChargeLimitW:    1500,
		BackupReserveSoc:  &reserve,
		SimulationMode:    true,
		EnableRealControl: false,
	}, settings)

	want := 1500 - settings.MaxDecreaseStepW
	if plan.RecommendedACChargeLimitW != want {
		t.Fatalf("RecommendedACChargeLimitW = %d, want %d", plan.RecommendedACChargeLimitW, want)
	}
}

func TestPlanSurplusChargingRestoresDefaultReserveWhenImporting(t *testing.T) {
	reserve := 90
	tou := false
	plan := PlanSurplusCharging(SurplusPlanInput{
		GridW:             900,
		BatterySoc:        78,
		ACChargeLimitW:    1500,
		BackupReserveSoc:  &reserve,
		DefaultReserveSoc: 30,
		TOUModeEnabled:    &tou,
		SimulationMode:    true,
	}, DefaultSettings())

	if plan.RecommendedACChargeLimitW != 400 {
		t.Fatalf("RecommendedACChargeLimitW = %d, want 400", plan.RecommendedACChargeLimitW)
	}
	if plan.RecommendedBackupReserveSoc == nil || *plan.RecommendedBackupReserveSoc != 30 {
		t.Fatalf("RecommendedBackupReserveSoc = %v, want 30", plan.RecommendedBackupReserveSoc)
	}
	if !plan.ShouldAdjustACChargeLimit {
		t.Fatal("ShouldAdjustACChargeLimit = false, want true")
	}
	if !plan.ShouldLowerBackupReserve {
		t.Fatal("ShouldLowerBackupReserve = false, want true")
	}
	if !plan.ShouldEnableTOUMode {
		t.Fatal("ShouldEnableTOUMode = false, want true")
	}
	if plan.StrategyState != "RECOVERING" {
		t.Fatalf("StrategyState = %q, want RECOVERING", plan.StrategyState)
	}
	for _, want := range []string{"AC充電上限を400Wへ設定", "バックアップリザーブを30%へ戻す", "TOUをONに戻す"} {
		if !strings.Contains(plan.ActionSummary, want) {
			t.Fatalf("ActionSummary = %q, want contains %q", plan.ActionSummary, want)
		}
	}
	if plan.ShouldRaiseBackupReserve || plan.ShouldAlignBackupReserve || plan.WouldWrite {
		t.Fatalf("unexpected write recommendation: %+v", plan)
	}
}

func TestPlanSurplusChargingStopsWhenTargetSocReached(t *testing.T) {
	reserve := 100
	tou := false
	plan := PlanSurplusCharging(SurplusPlanInput{
		GridW:             -1200,
		BatterySoc:        98,
		BatteryInputW:     600,
		BatteryOutputW:    200,
		ACChargeLimitW:    400,
		BackupReserveSoc:  &reserve,
		DefaultReserveSoc: 85,
		TOUModeEnabled:    &tou,
		SimulationMode:    true,
	}, DefaultSettings())

	if plan.StrategyState != "RECOVERING" {
		t.Fatalf("StrategyState = %q, want RECOVERING", plan.StrategyState)
	}
	if !plan.ShouldEnableTOUMode {
		t.Fatal("ShouldEnableTOUMode = false, want true")
	}
	if !plan.ShouldLowerBackupReserve {
		t.Fatal("ShouldLowerBackupReserve = false, want true")
	}
	for _, want := range []string{"バックアップリザーブを85%へ戻す", "TOUをONに戻す"} {
		if !strings.Contains(plan.ActionSummary, want) {
			t.Fatalf("ActionSummary = %q, want contains %q", plan.ActionSummary, want)
		}
	}
}

func TestPlanSurplusChargingDoesNotGoBelowAppMinimumWhenImporting(t *testing.T) {
	reserve := 90
	plan := PlanSurplusCharging(SurplusPlanInput{
		GridW:             900,
		BatterySoc:        78,
		ACChargeLimitW:    500,
		BackupReserveSoc:  &reserve,
		DefaultReserveSoc: 30,
		SimulationMode:    true,
	}, DefaultSettings())

	if plan.RecommendedACChargeLimitW != 400 {
		t.Fatalf("RecommendedACChargeLimitW = %d, want 400", plan.RecommendedACChargeLimitW)
	}
	if !strings.Contains(plan.ActionSummary, "AC充電上限を400Wへ設定") {
		t.Fatalf("ActionSummary = %q, want app-minimum AC action", plan.ActionSummary)
	}
}

func TestPlanSurplusChargingDoesNotRaiseBelowRecoveryLimitWhenImporting(t *testing.T) {
	reserve := 90
	plan := PlanSurplusCharging(SurplusPlanInput{
		GridW:             900,
		BatterySoc:        78,
		ACChargeLimitW:    80,
		BackupReserveSoc:  &reserve,
		DefaultReserveSoc: 30,
		SimulationMode:    true,
	}, DefaultSettings())

	if plan.RecommendedACChargeLimitW != 80 {
		t.Fatalf("RecommendedACChargeLimitW = %d, want 80", plan.RecommendedACChargeLimitW)
	}
	if plan.ShouldAdjustACChargeLimit {
		t.Fatal("ShouldAdjustACChargeLimit = true, want false")
	}
	if strings.Contains(plan.ActionSummary, "AC充電上限") {
		t.Fatalf("ActionSummary = %q, want no AC action", plan.ActionSummary)
	}
}

func TestPlanSurplusChargingWouldWriteIncludesEnergyModeDisableWhenAllowed(t *testing.T) {
	tou := true
	reserve := 85
	plan := PlanSurplusCharging(SurplusPlanInput{
		GridW:             -1200,
		BatterySoc:        83,
		ACChargeLimitW:    1000,
		BackupReserveSoc:  &reserve,
		TOUModeEnabled:    &tou,
		SimulationMode:    false,
		EnableRealControl: true,
		AutoControl:       true,
	}, DefaultSettings())

	if !plan.ShouldDisableEnergyModes {
		t.Fatal("ShouldDisableEnergyModes = false, want true")
	}
	if !plan.WouldWrite {
		t.Fatal("WouldWrite = false, want true")
	}
}

func TestPlanSurplusChargingRecommendsEnergyModeDisableForNonTOUModes(t *testing.T) {
	tou := false
	selfPowered := true
	reserve := 85
	plan := PlanSurplusCharging(SurplusPlanInput{
		GridW:              -1200,
		BatterySoc:         83,
		ACChargeLimitW:     1000,
		BackupReserveSoc:   &reserve,
		TOUModeEnabled:     &tou,
		SelfPoweredEnabled: &selfPowered,
		SimulationMode:     false,
		EnableRealControl:  true,
		AutoControl:        true,
	}, DefaultSettings())

	if !plan.ShouldDisableEnergyModes {
		t.Fatal("ShouldDisableEnergyModes = false, want true")
	}
	if !plan.WouldWrite {
		t.Fatal("WouldWrite = false, want true")
	}
	if got := plan.Reason; !strings.Contains(got, "energy strategy mode blocks surplus charging") {
		t.Fatalf("Reason = %q, want energy strategy mode note", got)
	}
}

func TestPlanSurplusChargingDoesNotLowerReserveBelowDefaultWhenImporting(t *testing.T) {
	reserve := 30
	plan := PlanSurplusCharging(SurplusPlanInput{
		GridW:             900,
		BatterySoc:        78,
		ACChargeLimitW:    0,
		BackupReserveSoc:  &reserve,
		DefaultReserveSoc: 30,
		SimulationMode:    true,
	}, DefaultSettings())

	if plan.RecommendedBackupReserveSoc != nil {
		t.Fatalf("RecommendedBackupReserveSoc = %v, want nil", plan.RecommendedBackupReserveSoc)
	}
	if plan.ShouldRaiseBackupReserve || plan.ShouldLowerBackupReserve || plan.ShouldAlignBackupReserve || plan.ShouldAdjustACChargeLimit || plan.WouldWrite {
		t.Fatalf("unexpected write recommendation: %+v", plan)
	}
	if plan.ActionSummary != "" {
		t.Fatalf("ActionSummary = %q, want empty", plan.ActionSummary)
	}
}

func TestPlanSurplusChargingWouldWriteBlockedInMockMode(t *testing.T) {
	reserve := 85
	plan := PlanSurplusCharging(SurplusPlanInput{
		GridW:             -1200,
		MockMode:          true,
		BatterySoc:        83,
		ACChargeLimitW:    1000,
		BackupReserveSoc:  &reserve,
		SimulationMode:    false,
		EnableRealControl: true,
		AutoControl:       true,
	}, DefaultSettings())

	if plan.WouldWrite {
		t.Fatal("WouldWrite = true, want false in mock mode")
	}
	if !strings.Contains(plan.Reason, "mock mode keeps EcoFlow write disabled") {
		t.Fatalf("Reason = %q, want mock mode guard", plan.Reason)
	}
}

func SettingsWithTargetSoc(targetSoc int) Settings {
	settings := DefaultSettings()
	settings.TargetSoc = targetSoc
	return settings
}

func SettingsWithTargetSocAndPassThrough(targetSoc int) Settings {
	settings := SettingsWithTargetSoc(targetSoc)
	settings.PassThroughEnabled = true
	return settings
}
