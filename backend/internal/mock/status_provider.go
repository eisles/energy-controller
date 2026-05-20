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
	weatherReader  WeatherReader
	loadEstimator  EcoFlowLoadEstimator
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

type WeatherReader interface {
	ForecastTargetDaytime(ctx context.Context, now time.Time) (domain.WeatherForecast, error)
	CurrentWeatherLocation(ctx context.Context) (domain.WeatherLocation, error)
}

type EcoFlowLoadEstimator interface {
	EstimateEcoFlowLoad(ctx context.Context, now time.Time, days int) (domain.EcoFlowLoadEstimate, error)
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

func NewStatusProviderWithReaders(clock config.Clock, settings control.Settings, mockMode bool, simulationMode bool, realControl bool, autoControl bool, gridReader GridReader, batteryReader BatteryReader, writeClient ecoflow.WriteClient, mode string, weatherReader ...WeatherReader) *StatusProvider {
	provider := NewStatusProvider(clock, settings, mockMode, simulationMode, realControl, autoControl)
	provider.gridReader = gridReader
	provider.batteryReader = batteryReader
	provider.writeClient = writeClient
	provider.mode = mode
	if len(weatherReader) > 0 {
		provider.weatherReader = weatherReader[0]
	}
	return provider
}

func (p *StatusProvider) SetEcoFlowLoadEstimator(estimator EcoFlowLoadEstimator) {
	p.loadEstimator = estimator
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
	weatherForecast, solarSettings, weatherError := p.currentWeatherForecast(ctx, now)
	lastError = combineErrors(lastError, weatherError)
	ecoflowLoadEstimate, loadEstimateError := p.currentEcoFlowLoadEstimate(ctx, now)
	lastError = combineErrors(lastError, loadEstimateError)
	p.setCommandStatus(commandSent, actualCommandW)
	surplusPlan := control.PlanSurplusCharging(control.SurplusPlanInput{
		GridW:              gridPower.GridW,
		MockMode:           p.mockMode,
		BatterySoc:         batteryStatus.Soc,
		BatteryInputW:      batteryStatus.InputW,
		BatteryOutputW:     batteryStatus.OutputW,
		ACChargeLimitW:     batteryStatus.ACChargeLimitW,
		BackupReserveSoc:   batteryStatus.BackupReserveSoc,
		DefaultReserveSoc:  defaultReserveSoc(solarSettings),
		TOUModeEnabled:     batteryStatus.TOUModeEnabled,
		SelfPoweredEnabled: batteryStatus.SelfPoweredEnabled,
		ScheduledEnabled:   batteryStatus.ScheduledEnabled,
		IntelligentEnabled: batteryStatus.IntelligentEnabled,
		SimulationMode:     p.simulationMode,
		EnableRealControl:  p.realControl,
		AutoControl:        p.autoControl,
	}, p.settings)
	if result.CommandBlockReason != "" {
		result.Decision.Reason += "; " + result.CommandBlockReason
	}
	if surplusPlan.ActionSummary != "" {
		result.Decision.Reason += "; surplus dry-run plan: " + surplusPlan.ActionSummary
	}
	if commandSent {
		result.Decision.Reason += "; EcoFlow write command sent"
	}
	if commandRecorded {
		result.Decision.Reason += "; EcoFlow mock write adapter recorded would-send command"
	}
	nightChargePlan := control.PlanNightCharging(control.NightChargePlanInput{
		Now:                 now,
		BatterySoc:          batteryStatus.Soc,
		BatteryInputW:       batteryStatus.InputW,
		BatteryOutputW:      batteryStatus.OutputW,
		ACChargeLimitW:      batteryStatus.ACChargeLimitW,
		BackupReserveSoc:    batteryStatus.BackupReserveSoc,
		BatteryFullEnergyWh: batteryStatus.FullEnergyWh,
		TOUModeEnabled:      batteryStatus.TOUModeEnabled,
		SelfPoweredEnabled:  batteryStatus.SelfPoweredEnabled,
		ScheduledEnabled:    batteryStatus.ScheduledEnabled,
		IntelligentEnabled:  batteryStatus.IntelligentEnabled,
		Forecast:            weatherForecast,
		SolarSettings:       solarSettings,
		EcoFlowLoadEstimate: ecoflowLoadEstimate,
		Previous:            p.previousDecisionSnapshot(),
		MockMode:            p.mockMode,
		SimulationMode:      p.simulationMode,
		EnableRealControl:   p.realControl,
		AutoControl:         p.autoControl,
	}, p.settings)
	if nightChargePlan.ActionSummary != "" {
		result.Decision.Reason += "; night dry-run plan: " + nightChargePlan.ActionSummary
	}

	return domain.Status{
		GridW:               result.GridPower.GridW,
		ImportW:             result.GridPower.ImportW,
		ExportW:             result.GridPower.ExportW,
		BatterySoc:          batteryStatus.Soc,
		BatteryInputW:       batteryStatus.InputW,
		BatteryOutputW:      batteryStatus.OutputW,
		ACChargeLimitW:      batteryStatus.ACChargeLimitW,
		BackupReserveSoc:    batteryStatus.BackupReserveSoc,
		EnergyBackupEnabled: batteryStatus.EnergyBackupEnabled,
		TOUModeEnabled:      batteryStatus.TOUModeEnabled,
		BatteryFullEnergyWh: batteryStatus.FullEnergyWh,
		SurplusPlan:         &surplusPlan,
		NightChargePlan:     &nightChargePlan,
		TargetChargeW:       result.Decision.TargetChargeW,
		State:               "simulation",
		Mode:                p.mode,
		LastDecisionReason:  result.Decision.Reason,
		LastError:           lastError,
		UpdatedAt:           now,
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

func (p *StatusProvider) previousDecisionSnapshot() control.PreviousDecision {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.previous
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

func (p *StatusProvider) currentWeatherForecast(ctx context.Context, now time.Time) (*domain.WeatherForecast, *domain.WeatherLocation, *string) {
	if p.weatherReader == nil {
		return nil, nil, nil
	}
	weatherLocation, settingsErr := p.weatherReader.CurrentWeatherLocation(ctx)
	forecast, err := p.weatherReader.ForecastTargetDaytime(ctx, now)
	if settingsErr != nil && err != nil {
		message := fmt.Sprintf("weather settings read failed: %v; weather forecast read failed: %v", settingsErr, err)
		return nil, nil, &message
	}
	if settingsErr != nil {
		message := fmt.Sprintf("weather settings read failed: %v", settingsErr)
		return &forecast, nil, &message
	}
	if err != nil {
		message := fmt.Sprintf("weather forecast read failed: %v", err)
		return nil, &weatherLocation, &message
	}
	return &forecast, &weatherLocation, nil
}

func (p *StatusProvider) currentEcoFlowLoadEstimate(ctx context.Context, now time.Time) (*domain.EcoFlowLoadEstimate, *string) {
	if p.loadEstimator == nil {
		return nil, nil
	}
	estimate, err := p.loadEstimator.EstimateEcoFlowLoad(ctx, now, 7)
	if err != nil {
		message := fmt.Sprintf("EcoFlow load estimate failed: %v", err)
		return nil, &message
	}
	return &estimate, nil
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

func defaultReserveSoc(location *domain.WeatherLocation) int {
	if location == nil || location.MinimumReserveSoc <= 0 {
		return 30
	}
	return location.MinimumReserveSoc
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
