package nature

import (
	"context"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

type Client interface {
	CurrentGridPower(ctx context.Context) (domain.GridPower, time.Time, error)
}

type EnergyMeterClient interface {
	CurrentEnergyMeterReading(ctx context.Context) (domain.EnergyMeterReading, error)
}
