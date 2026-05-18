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
	"github.com/eisles/energy-controller/backend/internal/ecoflow"
)

type StatusProvider struct {
	clock          config.Clock
	settings       control.Settings
	mockMode       bool
	simulationMode bool
	realControl    bool
	autoControl    bool
	mode           string
	gridReader     GridReader
	batteryReader  BatteryReader
	writeClient    ecoflow.WriteClient
	staleAfter     time.Duration
	mu             sync.Mutex
	previous       control.PreviousDecision
	actualCommandW *int
	commandSent    bool
}

type GridReader interface {
	CurrentGridPower(ctx context.Context) (domain.GridPower, time.Time, error)
}

type BatteryReader interface {
	GetBatteryStatus(ctx context.Context) (domain.BatteryStatus, error)
}

func NewStatusProvider(clock config.Clock, settings control.Settings, mockMode bool, simulationMode bool, realControl bool, autoControl bool) *StatusProvider {
	return &StatusProvider{
		clock:          clock,
		settings:       settings,
		mockMode:       mockMode,
		simulationMode: simulationMode,
		realControl:    realControl,
		autoControl:    autoControl,
		mode:           "mock",
		staleAfter:     5 * time.Minute,
	}
}

func NewStatusProviderWithReaders(clock config.Clock, settings control.Settings, mockMode bool, simulationMode bool, realControl bool, autoControl bool, gridReader GridReader, batteryReader BatteryReader, writeClient ecoflow.WriteClient, mode string) *StatusProvider {
	provider := NewStatusProvider(clock, settings, mockMode, simulationMode, realControl, autoControl)
	provider.gridReader = gridReader
	provider.batteryReader = batteryReader
	provider.writeClient = writeClient
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
		MockMode:          p.mockMode,
		SimulationMode:    p.simulationMode,
		EnableRealControl: p.realControl,
		AutoControl:       p.autoControl,
	}, p.settings)
	p.previous.ShouldCharge = result.Decision.ShouldCharge
	p.previous.TargetChargeW = result.Decision.TargetChargeW
	p.mu.Unlock()

	commandSent, commandRecorded, actualCommandW, commandError := p.executeCommand(ctx, result)
	if result.CommandAllowed {
		p.mu.Lock()
		p.previous.LastCommandAt = now
		p.previous.LastCommandTargetW = result.Decision.TargetChargeW
		p.mu.Unlock()
	}
	lastError = combineErrors(lastError, commandError)
	if result.CommandBlockReason != "" {
		result.Decision.Reason += "; " + result.CommandBlockReason
	}
	if commandSent {
		result.Decision.Reason += "; EcoFlow write command sent"
	}
	if commandRecorded {
		result.Decision.Reason += "; EcoFlow mock write adapter recorded would-send command"
	}
	p.setCommandStatus(commandSent, actualCommandW)

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

func (p *StatusProvider) LastCommandActualW() *int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.actualCommandW == nil {
		return nil
	}
	value := *p.actualCommandW
	return &value
}

func (p *StatusProvider) LastCommandSent() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.commandSent
}

func (p *StatusProvider) setCommandStatus(commandSent bool, actualCommandW *int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.commandSent = commandSent
	if actualCommandW == nil {
		p.actualCommandW = nil
		return
	}
	value := *actualCommandW
	p.actualCommandW = &value
}

func (p *StatusProvider) executeCommand(ctx context.Context, result control.Result) (bool, bool, *int, *string) {
	if !result.CommandAllowed {
		return false, false, nil, nil
	}
	if p.writeClient == nil {
		message := "EcoFlow write command allowed but write adapter is not configured"
		return false, false, nil, &message
	}
	if result.Decision.ShouldCharge {
		if err := p.writeClient.SetACChargePower(ctx, result.Decision.TargetChargeW); err != nil {
			message := fmt.Sprintf("EcoFlow write command failed: %v", err)
			return false, false, nil, &message
		}
		return p.commandSentFlag(), p.commandRecordedFlag(), p.actualCommandWatts(result.Decision.TargetChargeW), nil
	}
	if err := p.writeClient.StopOrMinimizeCharging(ctx); err != nil {
		message := fmt.Sprintf("EcoFlow stop/minimize command failed: %v", err)
		return false, false, nil, &message
	}
	return p.commandSentFlag(), p.commandRecordedFlag(), p.actualCommandWatts(0), nil
}

func (p *StatusProvider) commandSentFlag() bool {
	_, mockWriter := p.writeClient.(*ecoflow.MockWriteClient)
	return !mockWriter
}

func (p *StatusProvider) commandRecordedFlag() bool {
	_, mockWriter := p.writeClient.(*ecoflow.MockWriteClient)
	return mockWriter
}

func (p *StatusProvider) actualCommandWatts(watts int) *int {
	if !p.commandSentFlag() {
		return nil
	}
	return intPtr(watts)
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

func intPtr(value int) *int {
	return &value
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
