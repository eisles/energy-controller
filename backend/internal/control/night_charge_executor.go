package control

import (
	"context"
	"fmt"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

type NightChargeCommandGuardInput struct {
	Plan                   domain.NightChargePlan
	MockMode               bool
	SimulationMode         bool
	EnableRealControl      bool
	AutoControl            bool
	ConfirmEcoFlowWrite    string
	RealControlTrialActive bool
	Previous               *domain.NightChargePlanLog
	Now                    time.Time
	Settings               Settings
}

type NightChargeWriteClient interface {
	SetACChargePower(ctx context.Context, watts int) error
	SetBackupReserveSoc(ctx context.Context, percent int) error
	SetTOUMode(ctx context.Context, enabled bool) error
	SetSelfPoweredMode(ctx context.Context, enabled bool) error
}

func GuardNightChargeCommand(input NightChargeCommandGuardInput) domain.NightChargePlan {
	plan := input.Plan
	if !plan.WouldWrite {
		return plan
	}

	switch {
	case input.MockMode:
		return blockNightChargeCommand(plan, "mock mode keeps EcoFlow write disabled")
	case input.SimulationMode:
		return blockNightChargeCommand(plan, "simulation mode keeps EcoFlow write disabled")
	case !input.EnableRealControl:
		return blockNightChargeCommand(plan, "ENABLE_REAL_CONTROL=false keeps EcoFlow write disabled")
	case !input.AutoControl:
		return blockNightChargeCommand(plan, "auto control disabled keeps EcoFlow write disabled")
	case input.ConfirmEcoFlowWrite != confirmEcoFlowWriteValue:
		return blockNightChargeCommand(plan, "CONFIRM_ECOFLOW_WRITE is not I_UNDERSTAND")
	case !input.RealControlTrialActive:
		return blockNightChargeCommand(plan, "real control trial window inactive")
	case nightChargeCommandIntervalSuppressed(input):
		plan.CommandSuppressed = true
		if nightChargeDuplicateCommand(input) {
			return blockNightChargeCommand(plan, "duplicate night charge command candidate")
		}
		return blockNightChargeCommand(plan, "night charge command suppressed by minimum interval")
	default:
		return plan
	}
}

func ExecuteNightChargeCommand(ctx context.Context, plan domain.NightChargePlan, writer NightChargeWriteClient) domain.NightChargePlan {
	if !plan.WouldWrite {
		return plan
	}
	if writer == nil {
		return nightChargeCommandError(plan, "EcoFlow write client is unavailable")
	}
	if plan.ShouldDisableEnergyModes {
		if err := writer.SetTOUMode(ctx, false); err != nil {
			return nightChargeCommandError(plan, fmt.Sprintf("disable energy strategy modes: %v", err))
		}
		plan.CommandSent = true
	}
	if plan.ShouldEnableTOUMode {
		if err := writer.SetTOUMode(ctx, true); err != nil {
			return nightChargeCommandError(plan, fmt.Sprintf("enable TOU mode: %v", err))
		}
		plan.CommandSent = true
	}
	if plan.ShouldSetACChargeLimit {
		if plan.RecommendedACChargeLimitW <= 0 {
			return nightChargeCommandError(plan, "recommended AC charge limit is missing")
		}
		if err := writer.SetACChargePower(ctx, plan.RecommendedACChargeLimitW); err != nil {
			return nightChargeCommandError(plan, fmt.Sprintf("set AC charge power: %v", err))
		}
		plan.CommandSent = true
	}
	if plan.ShouldSetBackupReserve {
		if plan.RecommendedBackupReserveSoc == nil {
			return nightChargeCommandError(plan, "recommended backup reserve SOC is missing")
		}
		if err := writer.SetBackupReserveSoc(ctx, *plan.RecommendedBackupReserveSoc); err != nil {
			return nightChargeCommandError(plan, fmt.Sprintf("set backup reserve SOC: %v", err))
		}
		plan.CommandSent = true
	}
	if plan.ShouldEnableSelfPoweredMode {
		if err := writer.SetSelfPoweredMode(ctx, true); err != nil {
			return nightChargeCommandError(plan, fmt.Sprintf("enable self-powered mode: %v", err))
		}
		plan.CommandSent = true
	}
	return plan
}

func nightChargeDuplicateCommand(input NightChargeCommandGuardInput) bool {
	return input.Previous != nil &&
		input.Previous.CommandSent &&
		input.Previous.CommandError == nil &&
		input.Previous.CommandFingerprint != "" &&
		input.Previous.CommandFingerprint == input.Plan.CommandFingerprint
}

func nightChargeCommandIntervalSuppressed(input NightChargeCommandGuardInput) bool {
	if input.Previous == nil || input.Previous.MeasuredAt.IsZero() {
		return false
	}
	settings := normalizeSettings(input.Settings)
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	if input.Previous.CommandSent || input.Previous.CommandError != nil || input.Previous.WouldWrite {
		return now.Sub(input.Previous.MeasuredAt) < settings.MinCommandInterval
	}
	return false
}

func blockNightChargeCommand(plan domain.NightChargePlan, reason string) domain.NightChargePlan {
	plan.WouldWrite = false
	plan.CommandBlockReason = reason
	if plan.Reason != "" {
		plan.Reason += "; " + reason
	} else {
		plan.Reason = reason
	}
	return plan
}

func nightChargeCommandError(plan domain.NightChargePlan, message string) domain.NightChargePlan {
	plan.WouldWrite = false
	plan.CommandError = &message
	if plan.Reason != "" {
		plan.Reason += "; " + message
	} else {
		plan.Reason = message
	}
	return plan
}
