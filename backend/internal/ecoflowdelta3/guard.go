package ecoflowdelta3

import "fmt"

type WriteGuards struct {
	MockMode                bool
	SimulationMode          bool
	EnableRealControl       bool
	AutoControlEnabled      bool
	AllowAutoControlOverlap bool
	ConfirmEcoFlowWrite     string
	Execute                 bool
	AllowPrivateAPIWrite    bool
	Command                 string
	DeviceType              string
}

func (g WriteGuards) Validate() error {
	if !g.Execute {
		return fmt.Errorf("DELTA_3 private write disabled: --execute is required")
	}
	if !g.AllowPrivateAPIWrite {
		return fmt.Errorf("DELTA_3 private write disabled: --allow-private-api-write is required")
	}
	if g.MockMode {
		return fmt.Errorf("DELTA_3 private write disabled: MOCK_MODE=true")
	}
	if g.SimulationMode {
		return fmt.Errorf("DELTA_3 private write disabled: SIMULATION_MODE=true")
	}
	if !g.EnableRealControl {
		return fmt.Errorf("DELTA_3 private write disabled: ENABLE_REAL_CONTROL=false")
	}
	if g.AutoControlEnabled && !g.AllowAutoControlOverlap {
		return fmt.Errorf("DELTA_3 private write disabled: AUTO_CONTROL_ENABLED=true; set --allow-auto-control-overlap for one-shot DELTA_3 validation")
	}
	if g.ConfirmEcoFlowWrite != ConfirmWriteValue {
		return fmt.Errorf("DELTA_3 private write disabled: CONFIRM_ECOFLOW_WRITE is not %s", ConfirmWriteValue)
	}
	if !allowedCommand(g.Command) {
		return fmt.Errorf("DELTA_3 private write disabled: command %q is not allowlisted", g.Command)
	}
	if _, ok := RangeForDeviceType(g.DeviceType); !ok {
		return fmt.Errorf("DELTA_3 private write disabled: unsupported device type %q", g.DeviceType)
	}
	return nil
}

func allowedCommand(command string) bool {
	switch command {
	case "set_ac_charge_power", "set_backup_reserve_soc", "set_grid_bypass_disabled", "set_min_discharge_soc", "set_max_charge_soc", "set_energy_backup_enabled":
		return true
	default:
		return false
	}
}

func ValidateACChargePower(deviceType string, watts int) error {
	deviceRange, ok := RangeForDeviceType(deviceType)
	if !ok {
		return fmt.Errorf("unsupported DELTA_3 device type %q", deviceType)
	}
	if watts < deviceRange.MinACChargeW || watts > deviceRange.MaxACChargeW {
		return fmt.Errorf("AC charge power %dW is outside %s range %d-%dW", watts, deviceType, deviceRange.MinACChargeW, deviceRange.MaxACChargeW)
	}
	return nil
}

func ValidateBackupReserveSoc(percent int) error {
	if percent < 5 || percent > 100 {
		return fmt.Errorf("backup reserve SOC must be 5-100%%: %d", percent)
	}
	return nil
}

func ValidateMaxChargeSoc(percent int) error {
	if percent < 50 || percent > 100 {
		return fmt.Errorf("max charge SOC must be 50-100%%: %d", percent)
	}
	return nil
}

func ValidateMinDischargeSoc(percent int) error {
	if percent < 0 || percent > 30 {
		return fmt.Errorf("min discharge SOC must be 0-30%%: %d", percent)
	}
	return nil
}
