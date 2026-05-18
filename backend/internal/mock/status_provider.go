package mock

import (
	"context"
	"math"
	"sync"
	"time"

	"github.com/eisles/energy-controller/backend/internal/config"
	"github.com/eisles/energy-controller/backend/internal/control"
	"github.com/eisles/energy-controller/backend/internal/domain"
)

type StatusProvider struct {
	clock          config.Clock
	settings       control.Settings
	simulationMode bool
	realControl    bool
	mu             sync.Mutex
	previous       control.PreviousDecision
}

func NewStatusProvider(clock config.Clock, settings control.Settings, simulationMode bool, realControl bool) *StatusProvider {
	return &StatusProvider{
		clock:          clock,
		settings:       settings,
		simulationMode: simulationMode,
		realControl:    realControl,
	}
}

func (p *StatusProvider) CurrentStatus(_ context.Context) (domain.Status, error) {
	now := p.clock.Now()
	gridW := sampleGridW(now)
	batterySoc := sampleBatterySoc(now)

	p.mu.Lock()
	result := control.Evaluate(control.Input{
		GridW:             gridW,
		BatterySoc:        batterySoc,
		Previous:          p.previous,
		Now:               now,
		SimulationMode:    p.simulationMode,
		EnableRealControl: p.realControl,
	}, p.settings)
	p.previous.ShouldCharge = result.Decision.ShouldCharge
	p.previous.TargetChargeW = result.Decision.TargetChargeW
	if result.CommandAllowed {
		p.previous.LastCommandAt = now
		p.previous.LastCommandTargetW = result.Decision.TargetChargeW
	}
	p.mu.Unlock()

	return domain.Status{
		GridW:              result.GridPower.GridW,
		ImportW:            result.GridPower.ImportW,
		ExportW:            result.GridPower.ExportW,
		BatterySoc:         batterySoc,
		BatteryInputW:      result.Decision.TargetChargeW,
		BatteryOutputW:     0,
		TargetChargeW:      result.Decision.TargetChargeW,
		State:              "simulation",
		Mode:               "mock",
		LastDecisionReason: result.Decision.Reason,
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
