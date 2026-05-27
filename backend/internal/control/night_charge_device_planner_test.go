package control

import (
	"strings"
	"testing"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

func TestApplyNightChargeDevicePlansAllocatesTargetByPriority(t *testing.T) {
	plan := &domain.NightChargePlan{
		StrategyState:             "NIGHT_CHARGE_WINDOW",
		EstimatedMorningLoadKWh:   1.2,
		MorningToPVStartLoadKWh:   0.8,
		EstimatedDeficitKWh:       1.0,
		SafetyMarginKWh:           0.5,
		RecommendedNightTargetKWh: 6.0,
		RecommendedNightTargetSoc: 50,
		MinimumReserveSoc:         20,
		CurrentBatteryEnergyKWh:   2.0,
		RequiredNightChargeKWh:    4.0,
		BatteryCapacityKWh:        12.288,
	}
	devices := []NightChargeDeviceInput{
		{
			DeviceID:              1,
			Name:                  "DELTA Pro 3",
			Kind:                  "ecoflow_delta_pro3",
			Priority:              1,
			Enabled:               true,
			ControlEnabled:        true,
			WriteTarget:           true,
			CapacityWh:            12288,
			CurrentSoc:            intPtr(20),
			BackupReserveMinSoc:   10,
			BackupReserveMaxSoc:   90,
			MinChargeW:            400,
			MaxChargeW:            1500,
			SupportsACChargeLimit: true,
			StatusAvailable:       true,
			DataSource:            "ecoflow_cloud",
		},
		{
			DeviceID:              2,
			Name:                  "DELTA 3 Plus",
			Kind:                  "ecoflow_delta3_plus",
			Priority:              2,
			Enabled:               true,
			ControlEnabled:        true,
			CapacityWh:            2048,
			CurrentSoc:            intPtr(20),
			BackupReserveMinSoc:   20,
			BackupReserveMaxSoc:   90,
			MinChargeW:            100,
			MaxChargeW:            1200,
			SupportsACChargeLimit: true,
			StatusAvailable:       true,
			DataSource:            "ecoflow_private_mqtt",
		},
	}

	ApplyNightChargeDevicePlans(plan, devices, DefaultSettings(), allowedNightChargeDeviceWriteGuard())

	if len(plan.DevicePlans) != 2 {
		t.Fatalf("DevicePlans len = %d, want 2", len(plan.DevicePlans))
	}
	if plan.DevicePlans[0].DeviceID != 1 {
		t.Fatalf("first DeviceID = %d, want priority device 1", plan.DevicePlans[0].DeviceID)
	}
	if plan.DevicePlans[0].RecommendedTargetSoc <= 20 {
		t.Fatalf("first RecommendedTargetSoc = %d, want increased target", plan.DevicePlans[0].RecommendedTargetSoc)
	}
	if !plan.DevicePlans[0].ShouldCharge || !plan.DevicePlans[0].WouldWrite {
		t.Fatalf("first plan ShouldCharge/WouldWrite = %v/%v, want true/true", plan.DevicePlans[0].ShouldCharge, plan.DevicePlans[0].WouldWrite)
	}
	if plan.TotalRequiredDeviceChargeKWh <= 0 {
		t.Fatalf("TotalRequiredDeviceChargeKWh = %f, want > 0", plan.TotalRequiredDeviceChargeKWh)
	}
	if plan.TotalDeviceCapacityKWh < 14 {
		t.Fatalf("TotalDeviceCapacityKWh = %f, want combined capacity", plan.TotalDeviceCapacityKWh)
	}
}

func TestApplyNightChargeDevicePlansBlocksDisabledControl(t *testing.T) {
	plan := &domain.NightChargePlan{
		StrategyState:             "NIGHT_CHARGE_WINDOW",
		RecommendedNightTargetKWh: 2.0,
		MinimumReserveSoc:         20,
	}

	ApplyNightChargeDevicePlans(plan, []NightChargeDeviceInput{{
		DeviceID:              1,
		Name:                  "DELTA 3 Plus",
		Enabled:               true,
		ControlEnabled:        false,
		CapacityWh:            2048,
		CurrentSoc:            intPtr(20),
		BackupReserveMinSoc:   20,
		BackupReserveMaxSoc:   90,
		MaxChargeW:            1200,
		SupportsACChargeLimit: true,
		StatusAvailable:       true,
	}}, DefaultSettings(), allowedNightChargeDeviceWriteGuard())

	if len(plan.DevicePlans) != 1 {
		t.Fatalf("DevicePlans len = %d, want 1", len(plan.DevicePlans))
	}
	if plan.DevicePlans[0].ShouldCharge {
		t.Fatal("ShouldCharge = true, want false because disabled devices are excluded from automatic night allocation")
	}
	if plan.DevicePlans[0].WouldWrite {
		t.Fatal("WouldWrite = true, want false for control disabled device")
	}
	if !strings.Contains(plan.DevicePlans[0].BlockReason, "control is disabled") {
		t.Fatalf("BlockReason = %q, want control disabled", plan.DevicePlans[0].BlockReason)
	}
}

func TestApplyNightChargeDevicePlansSkipsUnwritableDeviceAllocation(t *testing.T) {
	plan := &domain.NightChargePlan{
		StrategyState:             "NIGHT_CHARGE_WINDOW",
		RecommendedNightTargetKWh: 3.2,
		MinimumReserveSoc:         20,
		CurrentBatteryEnergyKWh:   0.4,
		RequiredNightChargeKWh:    2.8,
	}

	ApplyNightChargeDevicePlans(plan, []NightChargeDeviceInput{
		{
			DeviceID:              1,
			Name:                  "disabled high priority",
			Kind:                  "ecoflow_delta3_plus",
			Priority:              1,
			Enabled:               true,
			ControlEnabled:        false,
			CapacityWh:            2048,
			CurrentSoc:            intPtr(20),
			BackupReserveMinSoc:   20,
			BackupReserveMaxSoc:   90,
			MinChargeW:            100,
			MaxChargeW:            1200,
			SupportsACChargeLimit: true,
			StatusAvailable:       true,
		},
		{
			DeviceID:              2,
			Name:                  "controllable lower priority",
			Kind:                  "ecoflow_delta_pro3",
			Priority:              2,
			Enabled:               true,
			ControlEnabled:        true,
			WriteTarget:           true,
			CapacityWh:            12288,
			CurrentSoc:            intPtr(20),
			BackupReserveMinSoc:   20,
			BackupReserveMaxSoc:   90,
			MinChargeW:            400,
			MaxChargeW:            1500,
			SupportsACChargeLimit: true,
			StatusAvailable:       true,
		},
	}, DefaultSettings(), allowedNightChargeDeviceWriteGuard())

	if len(plan.DevicePlans) != 2 {
		t.Fatalf("DevicePlans len = %d, want 2", len(plan.DevicePlans))
	}
	if plan.DevicePlans[0].ShouldCharge || plan.DevicePlans[0].WouldWrite {
		t.Fatalf("disabled device ShouldCharge/WouldWrite = %v/%v, want false/false", plan.DevicePlans[0].ShouldCharge, plan.DevicePlans[0].WouldWrite)
	}
	if !plan.DevicePlans[1].ShouldCharge || !plan.DevicePlans[1].WouldWrite {
		t.Fatalf("controllable device ShouldCharge/WouldWrite = %v/%v, want true/true: %+v", plan.DevicePlans[1].ShouldCharge, plan.DevicePlans[1].WouldWrite, plan.DevicePlans[1])
	}
	if plan.DevicePlans[1].RecommendedTargetSoc <= 20 {
		t.Fatalf("controllable RecommendedTargetSoc = %d, want allocation above reserve", plan.DevicePlans[1].RecommendedTargetSoc)
	}
}

func TestApplyNightChargeDevicePlansBlocksMissingStatus(t *testing.T) {
	plan := &domain.NightChargePlan{
		StrategyState:             "NIGHT_CHARGE_WINDOW",
		RecommendedNightTargetKWh: 2.0,
		MinimumReserveSoc:         20,
	}

	ApplyNightChargeDevicePlans(plan, []NightChargeDeviceInput{{
		DeviceID:              1,
		Name:                  "DELTA 3 Plus",
		Enabled:               true,
		ControlEnabled:        true,
		CapacityWh:            2048,
		MaxChargeW:            1200,
		SupportsACChargeLimit: true,
		StatusAvailable:       true,
	}}, DefaultSettings(), allowedNightChargeDeviceWriteGuard())

	if len(plan.DevicePlans) != 1 {
		t.Fatalf("DevicePlans len = %d, want 1", len(plan.DevicePlans))
	}
	if !strings.Contains(plan.DevicePlans[0].BlockReason, "SOC is unavailable") {
		t.Fatalf("BlockReason = %q, want SOC unavailable", plan.DevicePlans[0].BlockReason)
	}
	if plan.DevicePlans[0].WouldWrite {
		t.Fatal("WouldWrite = true, want false")
	}
}

func TestApplyNightChargeDevicePlansBlocksOutsideNightChargeWindow(t *testing.T) {
	plan := &domain.NightChargePlan{
		StrategyState:             "NIGHT_PLAN_READY",
		RecommendedNightTargetKWh: 2.0,
		MinimumReserveSoc:         20,
	}

	ApplyNightChargeDevicePlans(plan, []NightChargeDeviceInput{{
		DeviceID:              1,
		Name:                  "DELTA 3 Plus",
		Enabled:               true,
		ControlEnabled:        true,
		CapacityWh:            2048,
		CurrentSoc:            intPtr(20),
		BackupReserveMinSoc:   20,
		BackupReserveMaxSoc:   90,
		MaxChargeW:            1200,
		SupportsACChargeLimit: true,
		StatusAvailable:       true,
	}}, DefaultSettings(), allowedNightChargeDeviceWriteGuard())

	if !plan.DevicePlans[0].ShouldCharge {
		t.Fatal("ShouldCharge = false, want true")
	}
	if plan.DevicePlans[0].WouldWrite {
		t.Fatal("WouldWrite = true, want false outside night charge window")
	}
	if !strings.Contains(plan.DevicePlans[0].BlockReason, "outside night charge window") {
		t.Fatalf("BlockReason = %q, want outside window", plan.DevicePlans[0].BlockReason)
	}
}

func TestApplyNightChargeDevicePlansDoesNotAddLoadOnTopOfCurrentEnergy(t *testing.T) {
	plan := &domain.NightChargePlan{
		StrategyState:             "NIGHT_CHARGE_WINDOW",
		RecommendedNightTargetKWh: 4.0,
		EstimatedMorningLoadKWh:   0.4,
		EstimatedDeficitKWh:       0.2,
		SafetyMarginKWh:           0.1,
		MinimumReserveSoc:         20,
	}

	ApplyNightChargeDevicePlans(plan, []NightChargeDeviceInput{{
		DeviceID:              1,
		Name:                  "DELTA Pro 3",
		Enabled:               true,
		ControlEnabled:        true,
		CapacityWh:            12288,
		CurrentSoc:            intPtr(80),
		BackupReserveMinSoc:   20,
		BackupReserveMaxSoc:   90,
		MaxChargeW:            1500,
		SupportsACChargeLimit: true,
		StatusAvailable:       true,
	}}, DefaultSettings(), allowedNightChargeDeviceWriteGuard())

	if len(plan.DevicePlans) != 1 {
		t.Fatalf("DevicePlans len = %d, want 1", len(plan.DevicePlans))
	}
	if plan.DevicePlans[0].ShouldCharge {
		t.Fatalf("ShouldCharge = true, want false when current energy already exceeds target: %+v", plan.DevicePlans[0])
	}
	if plan.TotalRequiredDeviceChargeKWh != 0 {
		t.Fatalf("TotalRequiredDeviceChargeKWh = %f, want 0", plan.TotalRequiredDeviceChargeKWh)
	}
}

func TestApplyNightChargeDevicePlansKeepsExecutablePro3PlanWhenAuxDemandCannotRun(t *testing.T) {
	plan := &domain.NightChargePlan{
		StrategyState:               "NIGHT_CHARGE_WINDOW",
		RecommendedNightTargetKWh:   12.0,
		RecommendedNightTargetSoc:   90,
		RecommendedACChargeLimitW:   1500,
		RecommendedBackupReserveSoc: intPtr(90),
		ShouldChargeTonight:         true,
		ShouldSetACChargeLimit:      true,
		ShouldSetBackupReserve:      true,
		WouldWrite:                  true,
		MinimumReserveSoc:           20,
		EstimatedMorningLoadKWh:     0.3,
		MorningToPVStartLoadKWh:     0.2,
		EstimatedDeficitKWh:         0.1,
		SafetyMarginKWh:             0.1,
		ShouldDisableEnergyModes:    true,
		ShouldEnableTOUMode:         false,
		ShouldEnableSelfPoweredMode: false,
	}

	ApplyNightChargeDevicePlans(plan, []NightChargeDeviceInput{
		{
			DeviceID:                1,
			Name:                    "DELTA Pro 3",
			Kind:                    "ecoflow_delta_pro3",
			Priority:                1,
			Enabled:                 true,
			ControlEnabled:          true,
			WriteTarget:             true,
			CapacityWh:              12288,
			CurrentSoc:              intPtr(80),
			CurrentACChargeLimitW:   intPtr(1500),
			CurrentBackupReserveSoc: intPtr(80),
			BackupReserveMinSoc:     20,
			BackupReserveMaxSoc:     90,
			MaxChargeW:              1500,
			SupportsACChargeLimit:   true,
			StatusAvailable:         true,
		},
		{
			DeviceID:              2,
			Name:                  "DELTA 3 Plus",
			Kind:                  "ecoflow_delta3_plus",
			Priority:              2,
			Enabled:               true,
			ControlEnabled:        true,
			CapacityWh:            2048,
			CurrentSoc:            intPtr(20),
			BackupReserveMinSoc:   20,
			BackupReserveMaxSoc:   90,
			MaxChargeW:            1200,
			SupportsACChargeLimit: true,
			StatusAvailable:       true,
		},
	}, DefaultSettings(), allowedNightChargeDeviceWriteGuard())

	if plan.RecommendedNightTargetSoc != 90 {
		t.Fatalf("RecommendedNightTargetSoc = %d, want original executable Pro 3 target 90", plan.RecommendedNightTargetSoc)
	}
	if !plan.ShouldChargeTonight {
		t.Fatal("ShouldChargeTonight = false, want original executable Pro 3 plan while aux demand has no executor")
	}
	if !plan.WouldWrite {
		t.Fatal("WouldWrite = false, want original executable Pro 3 write candidate")
	}
	if plan.ShouldSetACChargeLimit || !plan.ShouldSetBackupReserve {
		t.Fatalf("ShouldSet AC/reserve = %v/%v, want false/true", plan.ShouldSetACChargeLimit, plan.ShouldSetBackupReserve)
	}
}

func TestApplyNightChargeDevicePlansAllowsAuxChargeDuringNightRecoverWindow(t *testing.T) {
	plan := &domain.NightChargePlan{
		StrategyState:             "NIGHT_RECOVER",
		RecommendedNightTargetKWh: 4.0,
		MinimumReserveSoc:         20,
	}

	ApplyNightChargeDevicePlans(plan, []NightChargeDeviceInput{{
		DeviceID:              1,
		Name:                  "DELTA 3 Plus",
		Kind:                  "ecoflow_delta3_plus",
		Enabled:               true,
		ControlEnabled:        true,
		CapacityWh:            2048,
		CurrentSoc:            intPtr(20),
		BackupReserveMinSoc:   20,
		BackupReserveMaxSoc:   90,
		MaxChargeW:            1200,
		SupportsACChargeLimit: true,
		StatusAvailable:       true,
	}}, DefaultSettings(), allowedNightChargeDeviceWriteGuard())

	if !plan.DevicePlans[0].ShouldCharge {
		t.Fatal("ShouldCharge = false, want true")
	}
	if !plan.DevicePlans[0].WouldWrite {
		t.Fatalf("WouldWrite = false, want true during night recover window: %+v", plan.DevicePlans[0])
	}
}

func TestApplyNightChargeDevicePlansSyncsOnlyPro3WriteTarget(t *testing.T) {
	plan := &domain.NightChargePlan{
		StrategyState:             "NIGHT_CHARGE_WINDOW",
		RecommendedNightTargetKWh: 6.0,
		RecommendedNightTargetSoc: 50,
		MinimumReserveSoc:         20,
	}

	ApplyNightChargeDevicePlans(plan, []NightChargeDeviceInput{
		{
			DeviceID:              1,
			Name:                  "read only DELTA Pro 3",
			Kind:                  "ecoflow_delta_pro3",
			Priority:              1,
			Enabled:               true,
			ControlEnabled:        false,
			WriteTarget:           false,
			CapacityWh:            12288,
			CurrentSoc:            intPtr(85),
			BackupReserveMinSoc:   20,
			BackupReserveMaxSoc:   90,
			MaxChargeW:            1500,
			SupportsACChargeLimit: true,
			StatusAvailable:       true,
		},
		{
			DeviceID:                2,
			Name:                    "write DELTA Pro 3",
			Kind:                    "ecoflow_delta_pro3",
			Priority:                2,
			Enabled:                 true,
			ControlEnabled:          true,
			WriteTarget:             true,
			CapacityWh:              12288,
			CurrentSoc:              intPtr(30),
			CurrentACChargeLimitW:   intPtr(400),
			CurrentBackupReserveSoc: intPtr(30),
			BackupReserveMinSoc:     20,
			BackupReserveMaxSoc:     90,
			MaxChargeW:              1500,
			SupportsACChargeLimit:   true,
			StatusAvailable:         true,
		},
	}, DefaultSettings(), allowedNightChargeDeviceWriteGuard())

	if plan.RecommendedNightTargetSoc == 85 {
		t.Fatal("RecommendedNightTargetSoc synced read-only Pro 3 row, want write target row")
	}
	if plan.RecommendedNightTargetSoc <= 30 {
		t.Fatalf("RecommendedNightTargetSoc = %d, want write target allocation above current SOC", plan.RecommendedNightTargetSoc)
	}
}

func TestApplyNightChargeDevicePlansKeepsWriteBehindRealControlGate(t *testing.T) {
	plan := &domain.NightChargePlan{
		StrategyState:             "NIGHT_CHARGE_WINDOW",
		RecommendedNightTargetKWh: 2.0,
		MinimumReserveSoc:         20,
	}

	ApplyNightChargeDevicePlans(plan, []NightChargeDeviceInput{{
		DeviceID:              1,
		Name:                  "DELTA 3 Plus",
		Enabled:               true,
		ControlEnabled:        true,
		CapacityWh:            2048,
		CurrentSoc:            intPtr(20),
		BackupReserveMinSoc:   20,
		BackupReserveMaxSoc:   90,
		MaxChargeW:            1200,
		SupportsACChargeLimit: true,
		StatusAvailable:       true,
	}}, DefaultSettings(), NightChargeDeviceWriteGuard{SimulationMode: true})

	if !plan.DevicePlans[0].ShouldCharge {
		t.Fatal("ShouldCharge = false, want true because simulation mode blocks writes but keeps the calculated plan")
	}
	if plan.DevicePlans[0].WouldWrite {
		t.Fatal("WouldWrite = true, want false under simulation mode")
	}
	if !strings.Contains(plan.DevicePlans[0].BlockReason, "simulation mode") {
		t.Fatalf("BlockReason = %q, want simulation mode guard", plan.DevicePlans[0].BlockReason)
	}
}

func TestApplyNightChargeDevicePlansKeepsDelta3WriteBehindPrivateAPIGates(t *testing.T) {
	plan := &domain.NightChargePlan{
		StrategyState:             "NIGHT_CHARGE_WINDOW",
		RecommendedNightTargetKWh: 2.0,
		MinimumReserveSoc:         20,
	}

	guard := allowedNightChargeDeviceWriteGuard()
	guard.Delta3ExecuteWrite = false
	ApplyNightChargeDevicePlans(plan, []NightChargeDeviceInput{{
		DeviceID:              1,
		Name:                  "DELTA 3 Plus",
		Kind:                  "ecoflow_delta3_plus",
		Enabled:               true,
		ControlEnabled:        true,
		CapacityWh:            2048,
		CurrentSoc:            intPtr(20),
		BackupReserveMinSoc:   20,
		BackupReserveMaxSoc:   90,
		MaxChargeW:            1200,
		SupportsACChargeLimit: true,
		StatusAvailable:       true,
	}}, DefaultSettings(), guard)

	if plan.DevicePlans[0].ShouldCharge {
		t.Fatal("ShouldCharge = true, want false because disabled DELTA 3 write gates exclude it from automatic night allocation")
	}
	if plan.DevicePlans[0].WouldWrite {
		t.Fatal("WouldWrite = true, want false when DELTA 3 execute gate is disabled")
	}
	if !strings.Contains(plan.DevicePlans[0].BlockReason, "ECOFLOW_DELTA3_EXECUTE") {
		t.Fatalf("BlockReason = %q, want DELTA 3 execute gate", plan.DevicePlans[0].BlockReason)
	}
}

func TestApplyNightChargeDevicePlansKeepsDelta3WriteBehindAuxGate(t *testing.T) {
	plan := &domain.NightChargePlan{
		StrategyState:             "NIGHT_CHARGE_WINDOW",
		RecommendedNightTargetKWh: 2.0,
		MinimumReserveSoc:         20,
	}

	guard := allowedNightChargeDeviceWriteGuard()
	guard.Delta3AuxEnabled = false
	ApplyNightChargeDevicePlans(plan, []NightChargeDeviceInput{{
		DeviceID:              1,
		Name:                  "DELTA 3 Plus",
		Kind:                  "ecoflow_delta3_plus",
		Enabled:               true,
		ControlEnabled:        true,
		CapacityWh:            2048,
		CurrentSoc:            intPtr(20),
		BackupReserveMinSoc:   20,
		BackupReserveMaxSoc:   90,
		MaxChargeW:            1200,
		SupportsACChargeLimit: true,
		StatusAvailable:       true,
	}}, DefaultSettings(), guard)

	if plan.DevicePlans[0].ShouldCharge {
		t.Fatal("ShouldCharge = true, want false because DELTA3_AUX_ENABLED=false excludes it from automatic night allocation")
	}
	if plan.DevicePlans[0].WouldWrite {
		t.Fatal("WouldWrite = true, want false when DELTA3_AUX_ENABLED=false")
	}
	if !strings.Contains(plan.DevicePlans[0].BlockReason, "DELTA3_AUX_ENABLED") {
		t.Fatalf("BlockReason = %q, want DELTA3_AUX_ENABLED gate", plan.DevicePlans[0].BlockReason)
	}
}

func TestApplyNightChargeDevicePlansBlocksWhenDeviceSettingsAlreadyMatch(t *testing.T) {
	plan := &domain.NightChargePlan{
		StrategyState:             "NIGHT_CHARGE_WINDOW",
		RecommendedNightTargetKWh: 2.0,
		MinimumReserveSoc:         20,
	}

	ApplyNightChargeDevicePlans(plan, []NightChargeDeviceInput{{
		DeviceID:                1,
		Name:                    "DELTA 3 Plus",
		Kind:                    "ecoflow_delta3_plus",
		Enabled:                 true,
		ControlEnabled:          true,
		CapacityWh:              2048,
		CurrentSoc:              intPtr(20),
		CurrentACChargeLimitW:   intPtr(1200),
		CurrentBackupReserveSoc: intPtr(90),
		BackupReserveMinSoc:     20,
		BackupReserveMaxSoc:     90,
		MaxChargeW:              1200,
		SupportsACChargeLimit:   true,
		StatusAvailable:         true,
	}}, DefaultSettings(), allowedNightChargeDeviceWriteGuard())

	if !plan.DevicePlans[0].ShouldCharge {
		t.Fatal("ShouldCharge = false, want true")
	}
	if plan.DevicePlans[0].WouldWrite {
		t.Fatal("WouldWrite = true, want false when device settings already match")
	}
	if !strings.Contains(plan.DevicePlans[0].BlockReason, "already match") {
		t.Fatalf("BlockReason = %q, want already match", plan.DevicePlans[0].BlockReason)
	}
}

func TestApplyNightChargeDevicePlansBlocksDuringMinimumCommandInterval(t *testing.T) {
	plan := &domain.NightChargePlan{
		StrategyState:             "NIGHT_CHARGE_WINDOW",
		RecommendedNightTargetKWh: 2.0,
		MinimumReserveSoc:         20,
	}
	now := time.Date(2026, 5, 27, 23, 30, 0, 0, time.Local)
	guard := allowedNightChargeDeviceWriteGuard()
	guard.Now = now
	guard.Previous = &domain.NightChargePlanLog{
		MeasuredAt: now.Add(-30 * time.Second),
		WouldWrite: true,
	}

	ApplyNightChargeDevicePlans(plan, []NightChargeDeviceInput{{
		DeviceID:              1,
		Name:                  "DELTA 3 Plus",
		Kind:                  "ecoflow_delta3_plus",
		Enabled:               true,
		ControlEnabled:        true,
		CapacityWh:            2048,
		CurrentSoc:            intPtr(20),
		BackupReserveMinSoc:   20,
		BackupReserveMaxSoc:   90,
		MaxChargeW:            1200,
		SupportsACChargeLimit: true,
		StatusAvailable:       true,
	}}, DefaultSettings(), guard)

	if !plan.DevicePlans[0].ShouldCharge {
		t.Fatal("ShouldCharge = false, want true")
	}
	if plan.DevicePlans[0].WouldWrite {
		t.Fatal("WouldWrite = true, want false during minimum command interval")
	}
	if !strings.Contains(plan.DevicePlans[0].BlockReason, "minimum interval") {
		t.Fatalf("BlockReason = %q, want minimum interval", plan.DevicePlans[0].BlockReason)
	}
}

func TestApplyNightChargeDevicePlansBlocksDelta3DuringAuxMinimumCommandInterval(t *testing.T) {
	plan := &domain.NightChargePlan{
		StrategyState:             "NIGHT_CHARGE_WINDOW",
		RecommendedNightTargetKWh: 2.0,
		MinimumReserveSoc:         20,
	}
	now := time.Date(2026, 5, 27, 23, 30, 0, 0, time.Local)
	guard := allowedNightChargeDeviceWriteGuard()
	guard.Now = now
	guard.Delta3AuxMinInterval = 5 * time.Minute
	guard.Delta3AuxPrevious = &domain.Delta3AuxControlCommandLog{
		MeasuredAt: now.Add(-30 * time.Second),
		WouldWrite: true,
	}

	ApplyNightChargeDevicePlans(plan, []NightChargeDeviceInput{{
		DeviceID:              1,
		Name:                  "DELTA 3 Plus",
		Kind:                  "ecoflow_delta3_plus",
		Enabled:               true,
		ControlEnabled:        true,
		CapacityWh:            2048,
		CurrentSoc:            intPtr(20),
		BackupReserveMinSoc:   20,
		BackupReserveMaxSoc:   90,
		MaxChargeW:            1200,
		SupportsACChargeLimit: true,
		StatusAvailable:       true,
	}}, DefaultSettings(), guard)

	if !plan.DevicePlans[0].ShouldCharge {
		t.Fatal("ShouldCharge = false, want true")
	}
	if plan.DevicePlans[0].WouldWrite {
		t.Fatal("WouldWrite = true, want false during DELTA 3 Plus minimum command interval")
	}
	if !strings.Contains(plan.DevicePlans[0].BlockReason, "DELTA 3 Plus command suppressed") {
		t.Fatalf("BlockReason = %q, want DELTA 3 Plus minimum interval", plan.DevicePlans[0].BlockReason)
	}
}

func TestApplyNightChargeDevicePlansUsesForecastPVWindowForExpectedLoad(t *testing.T) {
	plan := &domain.NightChargePlan{
		StrategyState:                    "NIGHT_CHARGE_WINDOW",
		PVEffectiveStartAt:               "2026-05-20T10:00",
		PVEffectiveEndAt:                 "2026-05-20T12:00",
		CorrectedEstimatedPVToBatteryKWh: 0.3,
		MinimumReserveSoc:                20,
	}

	ApplyNightChargeDevicePlans(plan, []NightChargeDeviceInput{{
		DeviceID:              1,
		Name:                  "DELTA 3 Plus",
		Kind:                  "ecoflow_delta3_plus",
		Enabled:               true,
		ControlEnabled:        true,
		CapacityWh:            2048,
		CurrentSoc:            intPtr(20),
		BackupReserveMinSoc:   20,
		BackupReserveMaxSoc:   90,
		ExpectedDaytimeLoadW:  400,
		MaxChargeW:            1200,
		SupportsACChargeLimit: true,
		StatusAvailable:       true,
	}}, DefaultSettings(), allowedNightChargeDeviceWriteGuard())

	got := plan.DevicePlans[0]
	if got.DaytimeRequiredKWh != 0.8 {
		t.Fatalf("DaytimeRequiredKWh = %f, want 0.8 from 400W * 2h", got.DaytimeRequiredKWh)
	}
	if got.PVAllocatedKWh != 0.3 {
		t.Fatalf("PVAllocatedKWh = %f, want 0.3", got.PVAllocatedKWh)
	}
}

func TestApplyNightChargeDevicePlansIncludesHourlyRadiationEndSample(t *testing.T) {
	plan := &domain.NightChargePlan{
		StrategyState:                    "NIGHT_CHARGE_WINDOW",
		PVEffectiveStartAt:               "2026-05-20T10:00",
		PVEffectiveEndAt:                 "2026-05-20T12:00",
		PVEffectiveWindowSource:          "hourly-radiation",
		CorrectedEstimatedPVToBatteryKWh: 1.2,
		MinimumReserveSoc:                20,
	}

	ApplyNightChargeDevicePlans(plan, []NightChargeDeviceInput{{
		DeviceID:              1,
		Name:                  "DELTA 3 Plus",
		Kind:                  "ecoflow_delta3_plus",
		Enabled:               true,
		ControlEnabled:        true,
		CapacityWh:            2048,
		CurrentSoc:            intPtr(20),
		BackupReserveMinSoc:   20,
		BackupReserveMaxSoc:   90,
		ExpectedDaytimeLoadW:  400,
		MaxChargeW:            1200,
		SupportsACChargeLimit: true,
		StatusAvailable:       true,
	}}, DefaultSettings(), allowedNightChargeDeviceWriteGuard())

	got := plan.DevicePlans[0]
	if got.DaytimeRequiredKWh != 1.2 {
		t.Fatalf("DaytimeRequiredKWh = %f, want 1.2 from three hourly radiation samples", got.DaytimeRequiredKWh)
	}
}

func TestApplyNightChargeDevicePlansIsIdempotentForAggregateTotals(t *testing.T) {
	plan := &domain.NightChargePlan{
		StrategyState:                    "NIGHT_CHARGE_WINDOW",
		PVEffectiveStartAt:               "2026-05-20T09:00",
		PVEffectiveEndAt:                 "2026-05-20T16:00",
		CorrectedEstimatedPVToBatteryKWh: 0.4,
		MinimumReserveSoc:                20,
	}
	devices := []NightChargeDeviceInput{{
		DeviceID:              1,
		Name:                  "DELTA 3 Plus",
		Kind:                  "ecoflow_delta3_plus",
		Enabled:               true,
		ControlEnabled:        true,
		CapacityWh:            2048,
		CurrentSoc:            intPtr(40),
		BackupReserveMinSoc:   20,
		BackupReserveMaxSoc:   90,
		ExpectedDaytimeLoadW:  200,
		MaxChargeW:            1200,
		SupportsACChargeLimit: true,
		StatusAvailable:       true,
	}}

	ApplyNightChargeDevicePlans(plan, devices, DefaultSettings(), allowedNightChargeDeviceWriteGuard())
	firstRequired := plan.TotalDaytimeRequiredKWh
	firstAvailable := plan.TotalAvailableKWh
	firstDeficit := plan.TotalDeficitKWh
	ApplyNightChargeDevicePlans(plan, devices, DefaultSettings(), allowedNightChargeDeviceWriteGuard())

	if plan.TotalDaytimeRequiredKWh != firstRequired || plan.TotalAvailableKWh != firstAvailable || plan.TotalDeficitKWh != firstDeficit {
		t.Fatalf("aggregate totals changed on second apply: first %.3f/%.3f/%.3f, second %.3f/%.3f/%.3f",
			firstRequired, firstAvailable, firstDeficit,
			plan.TotalDaytimeRequiredKWh, plan.TotalAvailableKWh, plan.TotalDeficitKWh)
	}
}

func TestApplyNightChargeDevicePlansDoesNotDoubleAllocateAfterPVDistribution(t *testing.T) {
	plan := &domain.NightChargePlan{
		StrategyState:                    "NIGHT_CHARGE_WINDOW",
		PVEffectiveStartAt:               "2026-05-20T09:00",
		PVEffectiveEndAt:                 "2026-05-20T16:00",
		EstimatedMorningLoadKWh:          0.1,
		SafetyMarginKWh:                  0.1,
		CorrectedEstimatedPVToBatteryKWh: 0.0,
		RecommendedNightTargetKWh:        0.4,
		MinimumReserveSoc:                20,
	}

	ApplyNightChargeDevicePlans(plan, []NightChargeDeviceInput{{
		DeviceID:              1,
		Name:                  "DELTA 3 Plus",
		Kind:                  "ecoflow_delta3_plus",
		Enabled:               true,
		ControlEnabled:        true,
		CapacityWh:            2048,
		CurrentSoc:            intPtr(20),
		BackupReserveMinSoc:   20,
		BackupReserveMaxSoc:   90,
		ExpectedDaytimeLoadW:  100,
		MaxChargeW:            1200,
		SupportsACChargeLimit: true,
		StatusAvailable:       true,
	}}, DefaultSettings(), allowedNightChargeDeviceWriteGuard())

	got := plan.DevicePlans[0]
	if got.RecommendedTargetSoc >= 90 {
		t.Fatalf("RecommendedTargetSoc = %d, want below max; PV distribution should not be added twice", got.RecommendedTargetSoc)
	}
}

func TestApplyNightChargeDevicePlansDoesNotAllocatePVToBlockedDevice(t *testing.T) {
	plan := &domain.NightChargePlan{
		StrategyState:                    "NIGHT_CHARGE_WINDOW",
		PVEffectiveStartAt:               "2026-05-20T09:00",
		PVEffectiveEndAt:                 "2026-05-20T16:00",
		CorrectedEstimatedPVToBatteryKWh: 2.0,
		MinimumReserveSoc:                20,
	}

	ApplyNightChargeDevicePlans(plan, []NightChargeDeviceInput{
		{
			DeviceID:              1,
			Name:                  "blocked DELTA 3 Plus",
			Kind:                  "ecoflow_delta3_plus",
			Priority:              1,
			Enabled:               true,
			ControlEnabled:        false,
			CapacityWh:            2048,
			CurrentSoc:            intPtr(20),
			BackupReserveMinSoc:   20,
			BackupReserveMaxSoc:   90,
			ExpectedDaytimeLoadW:  400,
			MaxChargeW:            1200,
			SupportsACChargeLimit: true,
			StatusAvailable:       true,
		},
		{
			DeviceID:              2,
			Name:                  "controllable DELTA 3 Plus",
			Kind:                  "ecoflow_delta3_plus",
			Priority:              2,
			Enabled:               true,
			ControlEnabled:        true,
			CapacityWh:            2048,
			CurrentSoc:            intPtr(20),
			BackupReserveMinSoc:   20,
			BackupReserveMaxSoc:   90,
			ExpectedDaytimeLoadW:  400,
			MaxChargeW:            1200,
			SupportsACChargeLimit: true,
			StatusAvailable:       true,
		},
	}, DefaultSettings(), allowedNightChargeDeviceWriteGuard())

	if len(plan.DevicePlans) != 2 {
		t.Fatalf("DevicePlans len = %d, want 2", len(plan.DevicePlans))
	}
	if plan.DevicePlans[0].PVAllocatedKWh != 0 {
		t.Fatalf("blocked PVAllocatedKWh = %f, want 0", plan.DevicePlans[0].PVAllocatedKWh)
	}
	if plan.DevicePlans[1].PVAllocatedKWh <= 0 {
		t.Fatalf("controllable PVAllocatedKWh = %f, want > 0", plan.DevicePlans[1].PVAllocatedKWh)
	}
}

func TestApplyNightChargeDevicePlansUsesExistingAvailableEnergyBeforeRaisingTarget(t *testing.T) {
	plan := &domain.NightChargePlan{
		StrategyState:           "NIGHT_CHARGE_WINDOW",
		PVEffectiveStartAt:      "2026-05-20T09:00",
		PVEffectiveEndAt:        "2026-05-20T16:00",
		MinimumReserveSoc:       20,
		SafetyMarginKWh:         0.1,
		EstimatedMorningLoadKWh: 0.1,
	}

	ApplyNightChargeDevicePlans(plan, []NightChargeDeviceInput{{
		DeviceID:              1,
		Name:                  "DELTA 3 Plus",
		Kind:                  "ecoflow_delta3_plus",
		Enabled:               true,
		ControlEnabled:        true,
		CapacityWh:            2048,
		CurrentSoc:            intPtr(50),
		BackupReserveMinSoc:   20,
		BackupReserveMaxSoc:   90,
		ExpectedDaytimeLoadW:  50,
		MaxChargeW:            1200,
		SupportsACChargeLimit: true,
		StatusAvailable:       true,
	}}, DefaultSettings(), allowedNightChargeDeviceWriteGuard())

	got := plan.DevicePlans[0]
	if got.RecommendedTargetSoc != 50 {
		t.Fatalf("RecommendedTargetSoc = %d, want current SOC 50 because available energy covers need", got.RecommendedTargetSoc)
	}
	if got.ShouldCharge {
		t.Fatalf("ShouldCharge = true, want false: %+v", got)
	}
}

func TestApplyNightChargeDevicePlansAddsResidualNeedToCurrentEnergy(t *testing.T) {
	plan := &domain.NightChargePlan{
		StrategyState:      "NIGHT_CHARGE_WINDOW",
		PVEffectiveStartAt: "2026-05-20T09:00",
		PVEffectiveEndAt:   "2026-05-20T16:00",
		MinimumReserveSoc:  20,
	}

	ApplyNightChargeDevicePlans(plan, []NightChargeDeviceInput{{
		DeviceID:              1,
		Name:                  "DELTA 3 Plus",
		Kind:                  "ecoflow_delta3_plus",
		Enabled:               true,
		ControlEnabled:        true,
		CapacityWh:            2048,
		CurrentSoc:            intPtr(50),
		BackupReserveMinSoc:   20,
		BackupReserveMaxSoc:   90,
		ExpectedDaytimeLoadW:  200,
		MaxChargeW:            1200,
		SupportsACChargeLimit: true,
		StatusAvailable:       true,
	}}, DefaultSettings(), allowedNightChargeDeviceWriteGuard())

	got := plan.DevicePlans[0]
	if got.RecommendedTargetSoc < 88 {
		t.Fatalf("RecommendedTargetSoc = %d, want around 89; residual load must be added to current energy", got.RecommendedTargetSoc)
	}
}

func TestApplyNightChargeDevicePlansUsesAvailableEnergyForPrePVLoad(t *testing.T) {
	plan := &domain.NightChargePlan{
		StrategyState:           "NIGHT_CHARGE_WINDOW",
		PVEffectiveStartAt:      "2026-05-20T10:00",
		PVEffectiveEndAt:        "2026-05-20T11:00",
		MorningToPVStartLoadKWh: 0.3,
		MinimumReserveSoc:       20,
	}

	ApplyNightChargeDevicePlans(plan, []NightChargeDeviceInput{{
		DeviceID:              1,
		Name:                  "DELTA 3 Plus",
		Kind:                  "ecoflow_delta3_plus",
		Enabled:               true,
		ControlEnabled:        true,
		CapacityWh:            2048,
		CurrentSoc:            intPtr(50),
		BackupReserveMinSoc:   20,
		BackupReserveMaxSoc:   90,
		ExpectedDaytimeLoadW:  100,
		MaxChargeW:            1200,
		SupportsACChargeLimit: true,
		StatusAvailable:       true,
	}}, DefaultSettings(), allowedNightChargeDeviceWriteGuard())

	got := plan.DevicePlans[0]
	if got.MorningPrePVRequiredKWh != 0.1 {
		t.Fatalf("MorningPrePVRequiredKWh = %f, want 0.1 capped by daytime need", got.MorningPrePVRequiredKWh)
	}
	if got.RecommendedTargetSoc != 50 {
		t.Fatalf("RecommendedTargetSoc = %d, want current SOC because available energy covers pre-PV load", got.RecommendedTargetSoc)
	}
}

func TestApplyNightChargeDevicePlansTreatsSingleHourPVWindowAsOneHour(t *testing.T) {
	plan := &domain.NightChargePlan{
		StrategyState:      "NIGHT_CHARGE_WINDOW",
		PVEffectiveStartAt: "2026-05-20T10:00",
		PVEffectiveEndAt:   "2026-05-20T10:00",
		MinimumReserveSoc:  20,
	}

	ApplyNightChargeDevicePlans(plan, []NightChargeDeviceInput{{
		DeviceID:              1,
		Name:                  "DELTA 3 Plus",
		Kind:                  "ecoflow_delta3_plus",
		Enabled:               true,
		ControlEnabled:        true,
		CapacityWh:            2048,
		CurrentSoc:            intPtr(20),
		BackupReserveMinSoc:   20,
		BackupReserveMaxSoc:   90,
		ExpectedDaytimeLoadW:  400,
		MaxChargeW:            1200,
		SupportsACChargeLimit: true,
		StatusAvailable:       true,
	}}, DefaultSettings(), allowedNightChargeDeviceWriteGuard())

	got := plan.DevicePlans[0]
	if got.DaytimeRequiredKWh != 0.4 {
		t.Fatalf("DaytimeRequiredKWh = %f, want 0.4 from one-hour PV window", got.DaytimeRequiredKWh)
	}
}

func allowedNightChargeDeviceWriteGuard() NightChargeDeviceWriteGuard {
	return NightChargeDeviceWriteGuard{
		EnableRealControl:       true,
		AutoControl:             true,
		ConfirmEcoFlowWrite:     confirmEcoFlowWriteValue,
		RealControlTrialActive:  true,
		IsNightChargeTime:       true,
		Delta3AllowAutoWrite:    true,
		Delta3ExecuteWrite:      true,
		Delta3AllowPrivateWrite: true,
		Delta3AuxEnabled:        true,
	}
}
