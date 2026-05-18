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

	return domain.BatteryStatus{
		Soc:            int(math.Round(soc)),
		InputW:         int(math.Round(inputW)),
		OutputW:        int(math.Round(outputW)),
		ACChargeLimitW: int(math.Round(acChargeLimitW)),
		IsOnline:       true,
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

type jsonNumber interface {
	Float64() (float64, error)
}
