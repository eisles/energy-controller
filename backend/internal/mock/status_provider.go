package mock

import (
	"context"
	"math"
	"time"

	"github.com/eisles/energy-controller/backend/internal/config"
	"github.com/eisles/energy-controller/backend/internal/domain"
)

type StatusProvider struct {
	clock config.Clock
}

func NewStatusProvider(clock config.Clock) *StatusProvider {
	return &StatusProvider{clock: clock}
}

func (p *StatusProvider) CurrentStatus(_ context.Context) (domain.Status, error) {
	now := p.clock.Now()
	gridW := sampleGridW(now)
	importW := max(0, gridW)
	exportW := max(0, -gridW)
	targetChargeW := 0
	reason := "mock import state, simulation only"
	if exportW >= 700 {
		targetChargeW = min(1500, max(400, exportW-150))
		reason = "mock export power is enough, simulation only"
	}

	return domain.Status{
		GridW:              gridW,
		ImportW:            importW,
		ExportW:            exportW,
		BatterySoc:         sampleBatterySoc(now),
		BatteryInputW:      targetChargeW,
		BatteryOutputW:     0,
		TargetChargeW:      targetChargeW,
		State:              "simulation",
		Mode:               "mock",
		LastDecisionReason: reason,
		LastError:          nil,
		UpdatedAt:          now,
	}, nil
}

func sampleGridW(now time.Time) int {
	seconds := float64(now.Unix() % 120)
	wave := math.Sin(seconds / 120 * 2 * math.Pi)
	return int(math.Round(wave * 1100))
}

func sampleBatterySoc(now time.Time) int {
	return 55 + int(now.Unix()/15%25)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
