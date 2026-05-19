package control

import "github.com/eisles/energy-controller/backend/internal/domain"

type SurplusPlanInput struct {
	GridW              int
	BatterySoc         int
	BatteryInputW      int
	BatteryOutputW     int
	ACChargeLimitW     int
	BackupReserveSoc   *int
	DefaultReserveSoc  int
	TOUModeEnabled     *bool
	SelfPoweredEnabled *bool
	ScheduledEnabled   *bool
	IntelligentEnabled *bool
	SimulationMode     bool
	EnableRealControl  bool
	AutoControl        bool
}

func PlanSurplusCharging(input SurplusPlanInput, settings Settings) domain.SurplusPlan {
	settings = normalizeSettings(settings)
	gridPower := CalculateGridPower(input.GridW)
	netBatteryW := input.BatteryInputW - input.BatteryOutputW
	plan := domain.SurplusPlan{
		Mode:        "read-only",
		NetBatteryW: netBatteryW,
		WouldWrite:  false,
	}

	defaultReserveSoc := normalizeReserveSoc(input.DefaultReserveSoc)
	switch {
	case gridPower.ImportW > 0:
		plan.RecommendedACChargeLimitW = 0
		plan.ShouldAdjustACChargeLimit = input.ACChargeLimitW >= settings.MinCommandDiffW
		if input.BackupReserveSoc != nil && *input.BackupReserveSoc > defaultReserveSoc {
			recommendedReserve := defaultReserveSoc
			plan.RecommendedBackupReserveSoc = &recommendedReserve
			plan.ShouldLowerBackupReserve = true
		}
		plan.Reason = "importing from grid; restore charging controls toward default reserve"
		plan.WouldWrite = writeAllowed(input) && (plan.ShouldAdjustACChargeLimit || plan.ShouldLowerBackupReserve)
		return plan
	case gridPower.ExportW < settings.StartExportThresholdW:
		plan.Reason = "export power is below start threshold"
		return plan
	case input.BatterySoc >= settings.TargetSoc:
		plan.Reason = "battery soc is at or above target"
		return plan
	}

	recommendedAC := calculateTargetChargeW(gridPower.ExportW, settings)
	plan.RecommendedACChargeLimitW = recommendedAC
	plan.ShouldAdjustACChargeLimit = abs(recommendedAC-input.ACChargeLimitW) >= settings.MinCommandDiffW

	if input.BackupReserveSoc != nil {
		recommendedReserve := clamp(input.BatterySoc+2, *input.BackupReserveSoc, settings.TargetSoc)
		plan.RecommendedBackupReserveSoc = &recommendedReserve
		plan.ShouldRaiseBackupReserve = recommendedReserve > *input.BackupReserveSoc && recommendedReserve > input.BatterySoc
	}
	if hasEnabledEnergyMode(input) {
		plan.ShouldDisableEnergyModes = true
	}

	switch {
	case input.SimulationMode:
		plan.Reason = "surplus detected; simulation mode keeps EcoFlow write disabled"
	case !input.EnableRealControl:
		plan.Reason = "surplus detected; ENABLE_REAL_CONTROL=false keeps EcoFlow write disabled"
	case !input.AutoControl:
		plan.Reason = "surplus detected; auto control disabled keeps EcoFlow write disabled"
	default:
		plan.Reason = "surplus detected; planner recommends charging adjustments"
		plan.WouldWrite = plan.ShouldAdjustACChargeLimit || plan.ShouldRaiseBackupReserve || plan.ShouldDisableEnergyModes
	}

	if hasEnabledEnergyMode(input) {
		plan.Reason += "; EcoFlow energy strategy mode blocks surplus charging until disabled"
	}
	return plan
}

func hasEnabledEnergyMode(input SurplusPlanInput) bool {
	return boolPtrTrue(input.TOUModeEnabled) ||
		boolPtrTrue(input.SelfPoweredEnabled) ||
		boolPtrTrue(input.ScheduledEnabled) ||
		boolPtrTrue(input.IntelligentEnabled)
}

func boolPtrTrue(value *bool) bool {
	return value != nil && *value
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

func writeAllowed(input SurplusPlanInput) bool {
	return !input.SimulationMode && input.EnableRealControl && input.AutoControl
}
