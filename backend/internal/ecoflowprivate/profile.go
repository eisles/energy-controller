package ecoflowprivate

import "strings"

type DeviceRange struct {
	MinACChargeW int `json:"minAcChargeW"`
	MaxACChargeW int `json:"maxAcChargeW"`
}

type DeviceProfile struct {
	DeviceType       string
	Family           string
	MinACChargeW     int
	MaxACChargeW     int
	SupportedCommand map[string]bool
}

func ProfileForDeviceType(deviceType string) (DeviceProfile, bool) {
	normalized := strings.ToUpper(strings.TrimSpace(deviceType))
	switch normalized {
	case "DELTA_3", "DELTA_3_PLUS":
		return delta3SeriesProfile(normalized, 100, 1500), true
	case "DELTA_3_1500", "DELTA_3_MAX_PLUS":
		return delta3SeriesProfile(normalized, 200, 1500), true
	default:
		return DeviceProfile{}, false
	}
}

func RangeForDeviceType(deviceType string) (DeviceRange, bool) {
	profile, ok := ProfileForDeviceType(deviceType)
	if !ok {
		return DeviceRange{}, false
	}
	return DeviceRange{MinACChargeW: profile.MinACChargeW, MaxACChargeW: profile.MaxACChargeW}, true
}

func delta3SeriesProfile(deviceType string, minACChargeW int, maxACChargeW int) DeviceProfile {
	return DeviceProfile{
		DeviceType:   deviceType,
		Family:       "delta3_series",
		MinACChargeW: minACChargeW,
		MaxACChargeW: maxACChargeW,
		SupportedCommand: map[string]bool{
			"set_ac_charge_power":       true,
			"set_backup_reserve_soc":    true,
			"set_grid_bypass_disabled":  true,
			"set_min_discharge_soc":     true,
			"set_max_charge_soc":        true,
			"set_energy_backup_enabled": true,
		},
	}
}
