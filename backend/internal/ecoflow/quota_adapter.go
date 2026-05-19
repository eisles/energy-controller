package ecoflow

import (
	"fmt"
	"math"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

func BatteryStatusFromQuotas(quotas map[string]any) (domain.BatteryStatus, error) {
	soc, ok := numberFromQuotas(quotas, "cmsBattSoc", "bmsBattSoc")
	if !ok {
		return domain.BatteryStatus{}, fmt.Errorf("EcoFlow quota does not include known battery SOC keys")
	}
	inputW, _ := numberFromQuotas(quotas, "powInSumW")
	outputW, _ := numberFromQuotas(quotas, "powOutSumW")
	acChargeLimitW, _ := numberFromQuotas(quotas, "plugInInfoAcInChgPowMax")
	backupReserveSoc := intPtrFromQuota(quotas, "energyBackupStartSoc", "backupReverseSoc")
	energyBackupEnabled := boolPtrFromQuota(quotas, "energyBackupEn")
	touModeEnabled := boolPtrFromQuota(quotas, "energyStrategyOperateMode.operateTouModeOpen")
	fullEnergyWh := intPtrFromQuota(quotas, "cmsBattFullEnergy")

	return domain.BatteryStatus{
		Soc:                 int(math.Round(soc)),
		InputW:              int(math.Round(inputW)),
		OutputW:             int(math.Round(outputW)),
		ACChargeLimitW:      int(math.Round(acChargeLimitW)),
		BackupReserveSoc:    backupReserveSoc,
		EnergyBackupEnabled: energyBackupEnabled,
		TOUModeEnabled:      touModeEnabled,
		FullEnergyWh:        fullEnergyWh,
		IsOnline:            true,
	}, nil
}

func numberFromQuotas(quotas map[string]any, keys ...string) (float64, bool) {
	for _, key := range keys {
		value, ok := quotas[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case float64:
			return typed, true
		case int:
			return float64(typed), true
		case int64:
			return float64(typed), true
		case jsonNumber:
			number, err := typed.Float64()
			return number, err == nil
		}
	}
	return 0, false
}

func intFromQuota(quotas map[string]any, key string) (int, bool) {
	value, ok := numberFromQuotas(quotas, key)
	if !ok {
		return 0, false
	}
	return int(math.Round(value)), true
}

func intPtrFromQuota(quotas map[string]any, keys ...string) *int {
	for _, key := range keys {
		value, ok := intFromQuota(quotas, key)
		if ok {
			return &value
		}
	}
	return nil
}

func boolFromQuota(quotas map[string]any, key string) (bool, bool) {
	value, ok := quotas[key]
	if !ok {
		return false, false
	}
	switch typed := value.(type) {
	case bool:
		return typed, true
	case float64:
		return typed != 0, true
	case int:
		return typed != 0, true
	case int64:
		return typed != 0, true
	case jsonNumber:
		number, err := typed.Float64()
		return number != 0, err == nil
	default:
		return false, false
	}
}

func boolPtrFromQuota(quotas map[string]any, key string) *bool {
	value, ok := boolFromQuota(quotas, key)
	if !ok {
		return nil
	}
	return &value
}

type jsonNumber interface {
	Float64() (float64, error)
}
