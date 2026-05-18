package mock

import (
	"context"
	"fmt"
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
	mode           string
	gridReader     GridReader
	batteryReader  BatteryReader
	staleAfter     time.Duration
	mu             sync.Mutex
	previous       control.PreviousDecision
}

type GridReader interface {
	CurrentGridPower(ctx context.Context) (domain.GridPower, time.Time, error)
}

type BatteryReader interface {
	GetBatteryStatus(ctx context.Context) (domain.BatteryStatus, error)
}

func NewStatusProvider(clock config.Clock, settings control.Settings, simulationMode bool, realControl bool) *StatusProvider {
	return &StatusProvider{
		clock:          clock,
		settings:       settings,
		simulationMode: simulationMode,
		realControl:    realControl,
		mode:           "mock",
		staleAfter:     5 * time.Minute,
	}
}

func NewStatusProviderWithReaders(clock config.Clock, settings control.Settings, simulationMode bool, realControl bool, gridReader GridReader, batteryReader BatteryReader, mode string) *StatusProvider {
	provider := NewStatusProvider(clock, settings, simulationMode, realControl)
	provider.gridReader = gridReader
	provider.batteryReader = batteryReader
	provider.mode = mode
	return provider
}

func (p *StatusProvider) CurrentStatus(ctx context.Context) (domain.Status, error) {
	now := p.clock.Now()
	gridPower, lastError := p.currentGridPower(ctx, now)
	batteryStatus, batteryError := p.currentBatteryStatus(ctx, now)
	lastError = combineErrors(lastError, batteryError)

	p.mu.Lock()
	result := control.Evaluate(control.Input{
		GridW:             gridPower.GridW,
		BatterySoc:        batteryStatus.Soc,
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
		BatterySoc:         batteryStatus.Soc,
		BatteryInputW:      batteryStatus.InputW,
		BatteryOutputW:     batteryStatus.OutputW,
		ACChargeLimitW:     batteryStatus.ACChargeLimitW,
		TargetChargeW:      result.Decision.TargetChargeW,
		State:              "simulation",
		Mode:               p.mode,
		LastDecisionReason: result.Decision.Reason,
		LastError:          lastError,
		UpdatedAt:          now,
	}, nil
}

func (p *StatusProvider) currentBatteryStatus(ctx context.Context, now time.Time) (domain.BatteryStatus, *string) {
	if p.batteryReader == nil {
		return domain.BatteryStatus{
			Soc:            sampleBatterySoc(now),
			InputW:         0,
			OutputW:        0,
			ACChargeLimitW: 0,
			IsOnline:       true,
		}, nil
	}
	batteryStatus, err := p.batteryReader.GetBatteryStatus(ctx)
	if err != nil {
		message := fmt.Sprintf("EcoFlow battery status read failed: %v", err)
		return domain.BatteryStatus{
			Soc:            sampleBatterySoc(now),
			InputW:         0,
			OutputW:        0,
			ACChargeLimitW: 0,
			IsOnline:       false,
		}, &message
	}
	return batteryStatus, nil
}

func combineErrors(first *string, second *string) *string {
	switch {
	case first == nil:
		return second
	case second == nil:
		return first
	default:
		combined := *first + "; " + *second
		return &combined
	}
}

func (p *StatusProvider) currentGridPower(ctx context.Context, now time.Time) (domain.GridPower, *string) {
	if p.gridReader == nil {
		return control.CalculateGridPower(sampleGridW(now)), nil
	}
	gridPower, updatedAt, err := p.gridReader.CurrentGridPower(ctx)
	if err != nil {
		message := fmt.Sprintf("Nature Remo grid power read failed: %v", err)
		return control.CalculateGridPower(0), &message
	}
	if !updatedAt.IsZero() && now.Sub(updatedAt) > p.staleAfter {
		message := fmt.Sprintf("Nature Remo grid power is stale: updatedAt=%s", updatedAt.Format(time.RFC3339))
		return gridPower, &message
	}
	return gridPower, nil
}

func sampleGridW(now time.Time) int {
	seconds := float64(now.Unix() % 120)
	wave := math.Sin(seconds / 120 * 2 * math.Pi)
	return int(math.Round(wave * 1100))
}

func sampleBatterySoc(now time.Time) int {
	return 55 + int(now.Unix()/15%25)
}
