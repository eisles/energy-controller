package nature

import (
	"fmt"
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
