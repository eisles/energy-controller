package ecoflow

import (
	"context"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

type Client interface {
	GetBatteryStatus(ctx context.Context) (domain.BatteryStatus, error)
}
