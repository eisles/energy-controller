package nature

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/eisles/energy-controller/backend/internal/control"
	"github.com/eisles/energy-controller/backend/internal/domain"
)

func ParseInstantaneousPower(hexValue string) (domain.GridPower, error) {
	value := strings.TrimSpace(hexValue)
	value = strings.TrimPrefix(strings.ToLower(value), "0x")
	if value == "" {
		return domain.GridPower{}, fmt.Errorf("instantaneous power value is empty")
	}
	if len(value) > 8 {
		return domain.GridPower{}, fmt.Errorf("instantaneous power value is too long: %q", hexValue)
	}
	parsed, err := strconv.ParseUint(value, 16, 32)
	if err != nil {
		return domain.GridPower{}, fmt.Errorf("parse instantaneous power %q: %w", hexValue, err)
	}
	gridW := int(int32(uint32(parsed)))
	return control.CalculateGridPower(gridW), nil
}

func ParseCumulativeEnergyKWh(rawValue string, rawCoefficient string, rawUnit string) (float64, int, float64, error) {
	value, err := parseUnsignedHex(rawValue, 64)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("parse cumulative energy value %q: %w", rawValue, err)
	}
	coefficientValue, err := parseUnsignedHex(rawCoefficient, 32)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("parse cumulative energy coefficient %q: %w", rawCoefficient, err)
	}
	coefficient := int(coefficientValue)
	if coefficient <= 0 {
		return 0, 0, 0, fmt.Errorf("cumulative energy coefficient must be positive: %d", coefficient)
	}
	unitCode, err := parseUnsignedHex(rawUnit, 8)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("parse cumulative energy unit %q: %w", rawUnit, err)
	}
	unit, ok := cumulativeEnergyUnitKWh(byte(unitCode))
	if !ok {
		return 0, 0, 0, fmt.Errorf("unsupported cumulative energy unit code: 0x%02x", unitCode)
	}
	kwh := float64(value) * float64(coefficient) * unit
	return roundKWh(kwh), coefficient, unit, nil
}

func parseUnsignedHex(rawValue string, bitSize int) (uint64, error) {
	value := strings.TrimSpace(rawValue)
	value = strings.TrimPrefix(strings.ToLower(value), "0x")
	if value == "" {
		return 0, fmt.Errorf("hex value is empty")
	}
	return strconv.ParseUint(value, 16, bitSize)
}

func cumulativeEnergyUnitKWh(code byte) (float64, bool) {
	switch code {
	case 0x00:
		return 1, true
	case 0x01:
		return 0.1, true
	case 0x02:
		return 0.01, true
	case 0x03:
		return 0.001, true
	case 0x04:
		return 0.0001, true
	case 0x0a:
		return 10, true
	case 0x0b:
		return 100, true
	case 0x0c:
		return 1000, true
	case 0x0d:
		return 10000, true
	default:
		return 0, false
	}
}

func roundKWh(value float64) float64 {
	return math.Round(value*10000) / 10000
}
