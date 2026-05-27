package control

import (
	"sort"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

type NightChargeDeviceInput struct {
	DeviceID                int64
	Name                    string
	Kind                    string
	Priority                int
	Enabled                 bool
	ControlEnabled          bool
	WriteTarget             bool
	CapacityWh              int
	CurrentSoc              *int
	CurrentACChargeLimitW   *int
	CurrentBackupReserveSoc *int
	ReserveSoc              int
	TargetSoc               int
	BackupReserveMinSoc     int
	BackupReserveMaxSoc     int
	MinChargeW              int
	MaxChargeW              int
	SupportsACChargeLimit   bool
	StatusAvailable         bool
	StatusUnavailableReason string
	DataSource              string
}

type NightChargeDeviceWriteGuard struct {
	MockMode                bool
	SimulationMode          bool
	EnableRealControl       bool
	AutoControl             bool
	ConfirmEcoFlowWrite     string
	RealControlTrialActive  bool
	IsNightChargeTime       bool
	Delta3AllowAutoWrite    bool
	Delta3ExecuteWrite      bool
	Delta3AllowPrivateWrite bool
	Delta3AuxEnabled        bool
	Delta3AuxPrevious       *domain.Delta3AuxControlCommandLog
	Delta3AuxMinInterval    time.Duration
	Previous                *domain.NightChargePlanLog
	Now                     time.Time
}

func ApplyNightChargeDevicePlans(plan *domain.NightChargePlan, devices []NightChargeDeviceInput, settings Settings, guard NightChargeDeviceWriteGuard) {
	if plan == nil {
		return
	}
	settings = normalizeSettings(settings)
	devicePlans := buildNightChargeDevicePlans(*plan, devices, settings, guard)
	plan.DevicePlans = devicePlans
	plan.TotalDeviceCapacityKWh = 0
	plan.TotalCurrentDeviceEnergyKWh = 0
	plan.TotalRecommendedTargetKWh = 0
	plan.TotalRequiredDeviceChargeKWh = 0
	for _, devicePlan := range devicePlans {
		plan.TotalDeviceCapacityKWh += devicePlan.CapacityKWh
		plan.TotalCurrentDeviceEnergyKWh += devicePlan.CurrentEnergyKWh
		plan.TotalRecommendedTargetKWh += devicePlan.RecommendedTargetKWh
		plan.TotalRequiredDeviceChargeKWh += devicePlan.RequiredChargeKWh
	}
	syncPrimaryPro3NightChargePlan(plan, devicePlans, devices, settings, guard)
}

func buildNightChargeDevicePlans(plan domain.NightChargePlan, devices []NightChargeDeviceInput, settings Settings, guard NightChargeDeviceWriteGuard) []domain.NightChargeDevicePlan {
	active := make([]NightChargeDeviceInput, 0, len(devices))
	for _, device := range devices {
		if device.Enabled {
			active = append(active, device)
		}
	}
	sort.SliceStable(active, func(i, j int) bool {
		if active[i].Priority == active[j].Priority {
			return active[i].DeviceID < active[j].DeviceID
		}
		return active[i].Priority < active[j].Priority
	})

	devicePlans := make([]domain.NightChargeDevicePlan, 0, len(active))
	currentTargetKWh := 0.0
	minimumTargetKWh := 0.0
	maxTargetKWh := 0.0
	for _, device := range active {
		devicePlan := initialNightChargeDevicePlan(plan, device, settings)
		if blockReason := nightChargeDeviceAllocationBlockReason(devicePlan, device, guard); blockReason != "" {
			devicePlan.BlockReason = blockReason
		}
		if nightChargeDeviceAllocationEligible(devicePlan) {
			currentTargetKWh += devicePlan.RecommendedTargetKWh
			minimumTargetKWh += devicePlan.CapacityKWh * float64(devicePlan.MinTargetSoc) / 100
			maxTargetKWh += devicePlan.CapacityKWh * float64(devicePlan.MaxTargetSoc) / 100
		}
		devicePlans = append(devicePlans, devicePlan)
	}

	totalTargetKWh := targetNightDeviceEnergyKWh(plan, minimumTargetKWh, maxTargetKWh)
	remainingKWh := totalTargetKWh - currentTargetKWh
	if remainingKWh < 0 {
		remainingKWh = 0
	}
	for i := range devicePlans {
		if remainingKWh <= 0 || !nightChargeDeviceAllocationEligible(devicePlans[i]) {
			continue
		}
		maxKWh := devicePlans[i].CapacityKWh * float64(devicePlans[i].MaxTargetSoc) / 100
		headroomKWh := maxKWh - devicePlans[i].RecommendedTargetKWh
		if headroomKWh <= 0 {
			continue
		}
		addKWh := minFloat(remainingKWh, headroomKWh)
		devicePlans[i].RecommendedTargetKWh += addKWh
		devicePlans[i].RecommendedTargetSoc = clamp(ceilToInt(devicePlans[i].RecommendedTargetKWh/devicePlans[i].CapacityKWh*100), devicePlans[i].MinTargetSoc, devicePlans[i].MaxTargetSoc)
		remainingKWh -= addKWh
	}

	for i := range devicePlans {
		finalizeNightChargeDevicePlan(&devicePlans[i], active[i], plan, settings, guard)
	}
	return devicePlans
}

func nightChargeDeviceAllocationEligible(devicePlan domain.NightChargeDevicePlan) bool {
	return devicePlan.BlockReason == "" && devicePlan.CurrentSoc != nil && devicePlan.CapacityKWh > 0
}

func nightChargeDeviceAllocationBlockReason(devicePlan domain.NightChargeDevicePlan, device NightChargeDeviceInput, guard NightChargeDeviceWriteGuard) string {
	switch {
	case devicePlan.BlockReason != "":
		return devicePlan.BlockReason
	case !device.ControlEnabled:
		return "device control is disabled"
	case devicePlan.RecommendedACChargeLimitW <= 0:
		return "device AC charge control is unavailable"
	case device.Kind == "ecoflow_delta_pro3" && !device.WriteTarget:
		return "DELTA Pro 3 is not the write target"
	case nightChargeDeviceSpecificBlockReason(device.Kind, guard) != "":
		return nightChargeDeviceSpecificBlockReason(device.Kind, guard)
	default:
		return ""
	}
}

func initialNightChargeDevicePlan(plan domain.NightChargePlan, device NightChargeDeviceInput, settings Settings) domain.NightChargeDevicePlan {
	minTargetSoc := device.BackupReserveMinSoc
	if minTargetSoc <= 0 {
		minTargetSoc = device.ReserveSoc
	}
	if minTargetSoc <= 0 {
		minTargetSoc = plan.MinimumReserveSoc
	}
	if minTargetSoc <= 0 {
		minTargetSoc = 30
	}
	minTargetSoc = clamp(minTargetSoc, 5, 100)

	maxTargetSoc := device.BackupReserveMaxSoc
	if maxTargetSoc <= 0 {
		maxTargetSoc = device.TargetSoc
	}
	if maxTargetSoc <= 0 {
		maxTargetSoc = settings.TargetSoc
	}
	maxTargetSoc = clamp(maxTargetSoc, minTargetSoc, 100)

	devicePlan := domain.NightChargeDevicePlan{
		DeviceID:                  device.DeviceID,
		Name:                      device.Name,
		Kind:                      device.Kind,
		Priority:                  device.Priority,
		ControlEnabled:            device.ControlEnabled,
		ReserveSoc:                device.ReserveSoc,
		TargetSoc:                 device.TargetSoc,
		MinTargetSoc:              minTargetSoc,
		MaxTargetSoc:              maxTargetSoc,
		RecommendedACChargeLimitW: recommendedNightDeviceChargeLimitW(device, settings),
		DataSource:                firstNonEmpty(device.DataSource, "device-master"),
	}
	if device.CapacityWh <= 0 {
		devicePlan.BlockReason = "device capacity is unavailable"
		return devicePlan
	}
	devicePlan.CapacityKWh = float64(device.CapacityWh) / 1000
	if device.CurrentSoc == nil {
		devicePlan.BlockReason = "device SOC is unavailable"
		return devicePlan
	}
	currentSoc := clamp(*device.CurrentSoc, 0, 100)
	devicePlan.CurrentSoc = intPtr(currentSoc)
	devicePlan.CurrentEnergyKWh = devicePlan.CapacityKWh * float64(currentSoc) / 100

	baseTargetSoc := max(currentSoc, minTargetSoc)
	devicePlan.RecommendedTargetSoc = clamp(baseTargetSoc, minTargetSoc, maxTargetSoc)
	devicePlan.RecommendedTargetKWh = devicePlan.CapacityKWh * float64(devicePlan.RecommendedTargetSoc) / 100
	if !device.StatusAvailable {
		devicePlan.BlockReason = firstNonEmpty(device.StatusUnavailableReason, "device status is unavailable")
	}
	return devicePlan
}

func targetNightDeviceEnergyKWh(plan domain.NightChargePlan, minimumTargetKWh float64, maxTargetKWh float64) float64 {
	requiredEnergyKWh := plan.EstimatedMorningLoadKWh + plan.MorningToPVStartLoadKWh + plan.EstimatedDeficitKWh + plan.SafetyMarginKWh
	targetKWh := minimumTargetKWh + requiredEnergyKWh
	if plan.RecommendedNightTargetKWh > targetKWh {
		targetKWh = plan.RecommendedNightTargetKWh
	}
	if maxTargetKWh > 0 && targetKWh > maxTargetKWh {
		targetKWh = maxTargetKWh
	}
	return targetKWh
}

func finalizeNightChargeDevicePlan(devicePlan *domain.NightChargeDevicePlan, device NightChargeDeviceInput, plan domain.NightChargePlan, settings Settings, guard NightChargeDeviceWriteGuard) {
	if devicePlan == nil || devicePlan.BlockReason != "" || devicePlan.CurrentSoc == nil || devicePlan.CapacityKWh <= 0 {
		return
	}
	settings = normalizeSettings(settings)
	devicePlan.RequiredChargeKWh = requiredNightChargeKWh(devicePlan.CurrentEnergyKWh, devicePlan.RecommendedTargetKWh)
	devicePlan.ShouldCharge = devicePlan.RequiredChargeKWh > 0.01
	if !devicePlan.ShouldCharge {
		return
	}
	switch {
	case !devicePlan.ControlEnabled:
		devicePlan.BlockReason = "device control is disabled"
	case devicePlan.RecommendedACChargeLimitW <= 0:
		devicePlan.BlockReason = "device AC charge control is unavailable"
	case !nightChargeDeviceWindowActive(plan, guard):
		devicePlan.BlockReason = "outside night charge window"
	case nightChargeDeviceGuardBlockReason(guard) != "":
		devicePlan.BlockReason = nightChargeDeviceGuardBlockReason(guard)
	case nightChargeDeviceSpecificBlockReason(devicePlan.Kind, guard) != "":
		devicePlan.BlockReason = nightChargeDeviceSpecificBlockReason(devicePlan.Kind, guard)
	case nightChargeDeviceCandidateBlockReason(*devicePlan, device, settings, guard) != "":
		devicePlan.BlockReason = nightChargeDeviceCandidateBlockReason(*devicePlan, device, settings, guard)
	default:
		devicePlan.WouldWrite = true
	}
}

func syncPrimaryPro3NightChargePlan(plan *domain.NightChargePlan, devicePlans []domain.NightChargeDevicePlan, devices []NightChargeDeviceInput, settings Settings, guard NightChargeDeviceWriteGuard) {
	if plan == nil {
		return
	}
	settings = normalizeSettings(settings)
	deviceByID := make(map[int64]NightChargeDeviceInput, len(devices))
	for _, device := range devices {
		deviceByID[device.DeviceID] = device
	}
	for _, devicePlan := range devicePlans {
		input := deviceByID[devicePlan.DeviceID]
		if devicePlan.Kind != "ecoflow_delta_pro3" || !input.WriteTarget || devicePlan.CurrentSoc == nil || devicePlan.CapacityKWh <= 0 {
			continue
		}
		if !devicePlan.ShouldCharge && hasUnhandledNightChargeDeviceDemand(devicePlans) {
			return
		}
		plan.RecommendedNightTargetSoc = devicePlan.RecommendedTargetSoc
		plan.RecommendedNightTargetKWh = devicePlan.RecommendedTargetKWh
		plan.RequiredNightChargeKWh = devicePlan.RequiredChargeKWh
		plan.ShouldChargeTonight = devicePlan.ShouldCharge
		plan.ShouldSetACChargeLimit = false
		plan.ShouldSetBackupReserve = false
		if !devicePlan.ShouldCharge {
			plan.ShouldDisableEnergyModes = false
			plan.ShouldEnableTOUMode = false
			plan.ShouldEnableSelfPoweredMode = false
			plan.WouldWrite = false
			plan.CommandBlockReason = "current SOC is already above the allocated DELTA Pro 3 night target"
			plan.CommandFingerprint = NightChargeCommandFingerprint(*plan)
			plan.ActionSummary = nightChargeActionSummary(*plan)
			return
		}

		plan.RecommendedACChargeLimitW = devicePlan.RecommendedACChargeLimitW
		plan.ShouldSetACChargeLimit = input.CurrentACChargeLimitW == nil || abs(*input.CurrentACChargeLimitW-plan.RecommendedACChargeLimitW) >= settings.MinCommandDiffW
		recommendedReserve := devicePlan.RecommendedTargetSoc
		plan.RecommendedBackupReserveSoc = &recommendedReserve
		plan.ShouldSetBackupReserve = input.CurrentBackupReserveSoc == nil || *input.CurrentBackupReserveSoc != recommendedReserve

		switch {
		case !nightChargeHasCandidateChange(*plan):
			plan.WouldWrite = false
			plan.CommandBlockReason = "night charge settings already match allocated DELTA Pro 3 plan"
		case !nightChargeDeviceWindowActive(*plan, guard):
			plan.WouldWrite = false
			plan.CommandBlockReason = "outside night charge window"
		case nightChargeDeviceGuardBlockReason(guard) != "":
			plan.WouldWrite = false
			plan.CommandBlockReason = nightChargeDeviceGuardBlockReason(guard)
		case nightChargeDeviceSpecificBlockReason(devicePlan.Kind, guard) != "":
			plan.WouldWrite = false
			plan.CommandBlockReason = nightChargeDeviceSpecificBlockReason(devicePlan.Kind, guard)
		default:
			plan.WouldWrite = true
			plan.CommandBlockReason = ""
		}
		plan.CommandFingerprint = NightChargeCommandFingerprint(*plan)
		plan.ActionSummary = nightChargeActionSummary(*plan)
		return
	}
}

func hasUnhandledNightChargeDeviceDemand(devicePlans []domain.NightChargeDevicePlan) bool {
	for _, devicePlan := range devicePlans {
		if devicePlan.Kind != "ecoflow_delta_pro3" && devicePlan.ShouldCharge {
			return true
		}
	}
	return false
}

func nightChargeDeviceWindowActive(plan domain.NightChargePlan, guard NightChargeDeviceWriteGuard) bool {
	return plan.StrategyState == "NIGHT_CHARGE_WINDOW" || (plan.StrategyState == "NIGHT_RECOVER" && guard.IsNightChargeTime)
}

func nightChargeDeviceGuardBlockReason(guard NightChargeDeviceWriteGuard) string {
	switch {
	case guard.MockMode:
		return "mock mode keeps device write disabled"
	case guard.SimulationMode:
		return "simulation mode keeps device write disabled"
	case !guard.EnableRealControl:
		return "ENABLE_REAL_CONTROL=false keeps device write disabled"
	case !guard.AutoControl:
		return "auto control disabled keeps device write disabled"
	case guard.ConfirmEcoFlowWrite != confirmEcoFlowWriteValue:
		return "CONFIRM_ECOFLOW_WRITE is not I_UNDERSTAND"
	case !guard.RealControlTrialActive:
		return "real control trial window inactive"
	default:
		return ""
	}
}

func nightChargeDeviceSpecificBlockReason(kind string, guard NightChargeDeviceWriteGuard) string {
	if kind != "ecoflow_delta3_plus" {
		return ""
	}
	switch {
	case !guard.Delta3AllowAutoWrite:
		return "ECOFLOW_DELTA3_ALLOW_WRITE_WITH_AUTO_CONTROL=false"
	case !guard.Delta3ExecuteWrite:
		return "ECOFLOW_DELTA3_EXECUTE=false"
	case !guard.Delta3AllowPrivateWrite:
		return "ECOFLOW_DELTA3_ALLOW_PRIVATE_API_WRITE=false"
	case !guard.Delta3AuxEnabled:
		return "DELTA3_AUX_ENABLED=false"
	default:
		return ""
	}
}

func nightChargeDeviceCandidateBlockReason(devicePlan domain.NightChargeDevicePlan, device NightChargeDeviceInput, settings Settings, guard NightChargeDeviceWriteGuard) string {
	if !nightChargeDeviceHasCandidateChange(devicePlan, device, settings) {
		return "night charge settings already match allocated device plan"
	}
	if devicePlan.Kind == "ecoflow_delta3_plus" && nightChargeDeviceDelta3AuxCommandIntervalSuppressed(guard, settings) {
		return "DELTA 3 Plus command suppressed by minimum interval"
	}
	if nightChargeDeviceCommandIntervalSuppressed(guard, settings) {
		return "night charge command suppressed by minimum interval"
	}
	return ""
}

func nightChargeDeviceHasCandidateChange(devicePlan domain.NightChargeDevicePlan, device NightChargeDeviceInput, settings Settings) bool {
	if device.CurrentACChargeLimitW == nil || abs(*device.CurrentACChargeLimitW-devicePlan.RecommendedACChargeLimitW) >= settings.MinCommandDiffW {
		return true
	}
	return device.CurrentBackupReserveSoc == nil || *device.CurrentBackupReserveSoc != devicePlan.RecommendedTargetSoc
}

func nightChargeDeviceCommandIntervalSuppressed(guard NightChargeDeviceWriteGuard, settings Settings) bool {
	if guard.Previous == nil || guard.Previous.MeasuredAt.IsZero() {
		return false
	}
	now := guard.Now
	if now.IsZero() {
		now = time.Now()
	}
	if guard.Previous.CommandSent || guard.Previous.CommandError != nil || guard.Previous.WouldWrite {
		return now.Sub(guard.Previous.MeasuredAt) < settings.MinCommandInterval
	}
	return false
}

func nightChargeDeviceDelta3AuxCommandIntervalSuppressed(guard NightChargeDeviceWriteGuard, settings Settings) bool {
	if guard.Delta3AuxPrevious == nil || guard.Delta3AuxPrevious.MeasuredAt.IsZero() {
		return false
	}
	interval := guard.Delta3AuxMinInterval
	if interval <= 0 {
		interval = normalizeSettings(settings).MinCommandInterval
	}
	now := guard.Now
	if now.IsZero() {
		now = time.Now()
	}
	if guard.Delta3AuxPrevious.CommandSent || guard.Delta3AuxPrevious.ErrorMessage != nil || guard.Delta3AuxPrevious.WouldWrite {
		return now.Sub(guard.Delta3AuxPrevious.MeasuredAt) < interval
	}
	return false
}

func recommendedNightDeviceChargeLimitW(device NightChargeDeviceInput, settings Settings) int {
	if !device.SupportsACChargeLimit {
		return 0
	}
	limit := device.MaxChargeW
	if limit <= 0 {
		limit = settings.MaxChargeW
	}
	if device.MinChargeW > 0 && limit < device.MinChargeW {
		limit = device.MinChargeW
	}
	if limit <= 0 {
		return 0
	}
	return limit
}
