package control

import (
	"context"
	"fmt"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

type Delta3AuxCommandGuardInput struct {
	Status                 domain.Status
	MockMode               bool
	SimulationMode         bool
	EnableRealControl      bool
	AutoControl            bool
	ConfirmEcoFlowWrite    string
	RealControlTrialActive bool
	Delta3ReadEnabled      bool
	Delta3ControlEnabled   bool
	AllowAutoControlWrite  bool
	Execute                bool
	AllowPrivateAPIWrite   bool
	Previous               *domain.Delta3AuxControlCommandLog
}

type Delta3AuxWriteClient interface {
	SetACChargePower(ctx context.Context, watts int) error
	SetEnergyBackupEnabled(ctx context.Context, enabled bool, startSoc int) error
}

func EvaluateDelta3AuxCommandGuard(input Delta3AuxCommandGuardInput, settings Delta3AuxSettings) domain.Delta3AuxControlCommandLog {
	settings = normalizeDelta3AuxSettings(settings)
	status := input.Status
	plan := status.Delta3AuxPlan
	log := domain.Delta3AuxControlCommandLog{
		MeasuredAt:       status.UpdatedAt,
		StrategyState:    "IDLE",
		GridW:            status.GridW,
		ImportW:          status.ImportW,
		ExportW:          status.ExportW,
		CommandSent:      false,
		DryRun:           true,
		CreatedAt:        status.UpdatedAt,
		DecisionReason:   "no DELTA 3 Plus auxiliary plan",
		SuppressedReason: "no DELTA 3 Plus auxiliary plan",
	}
	if plan != nil {
		log.StrategyState = plan.StrategyState
		log.ResidualExportW = plan.ResidualExportW
		log.Delta3Soc = plan.Delta3Soc
		log.PreviousACChargeLimitW = plan.CurrentACChargeLimitW
		log.PreviousBackupReserveSoc = plan.CurrentBackupReserveSoc
		if plan.ShouldAdjustACChargeLimit {
			log.TargetACChargeLimitW = intPtr(plan.RecommendedACChargeLimitW)
		}
		if plan.ShouldSetBackupReserve {
			log.TargetBackupReserveSoc = plan.RecommendedBackupReserveSoc
		}
		if plan.ShouldDisableBackupReserve {
			log.TargetBackupReserveSoc = plan.RecommendedBackupReserveSoc
		}
		log.ShouldAdjustACChargeLimit = plan.ShouldAdjustACChargeLimit
		log.ShouldSetBackupReserve = plan.ShouldSetBackupReserve
		log.ShouldDisableBackupReserve = plan.ShouldDisableBackupReserve
		log.DecisionReason = plan.Reason
	}
	log.CommandFingerprint = delta3AuxCommandFingerprint(log)
	log.SuppressedReason = delta3AuxCommandSuppressedReason(input, settings, log)
	log.WouldWrite = log.SuppressedReason == "" && (log.ShouldAdjustACChargeLimit || log.ShouldSetBackupReserve || log.ShouldDisableBackupReserve)
	return log
}

func delta3AuxCommandSuppressedReason(input Delta3AuxCommandGuardInput, settings Delta3AuxSettings, log domain.Delta3AuxControlCommandLog) string {
	if !log.ShouldAdjustACChargeLimit && !log.ShouldSetBackupReserve && !log.ShouldDisableBackupReserve {
		return "no command candidate"
	}
	if !settings.Enabled {
		return "DELTA3_AUX_ENABLED=false"
	}
	if input.MockMode {
		return "mock mode, DELTA 3 Plus write disabled"
	}
	if input.SimulationMode {
		return "simulation mode, DELTA 3 Plus write disabled"
	}
	if !input.EnableRealControl {
		return "ENABLE_REAL_CONTROL=false, DELTA 3 Plus write disabled"
	}
	if !input.AutoControl {
		return "auto control disabled, DELTA 3 Plus write disabled"
	}
	if input.ConfirmEcoFlowWrite != confirmEcoFlowWriteValue {
		return "CONFIRM_ECOFLOW_WRITE is not I_UNDERSTAND"
	}
	if !input.RealControlTrialActive {
		return "real control trial window inactive"
	}
	if !input.Delta3ReadEnabled {
		return "ECOFLOW_DELTA3_READ_ENABLED=false"
	}
	if !input.Delta3ControlEnabled {
		return "DELTA 3 Plus master write target disabled or unavailable"
	}
	if !input.AllowAutoControlWrite {
		return "ECOFLOW_DELTA3_ALLOW_WRITE_WITH_AUTO_CONTROL=false"
	}
	if !input.Execute {
		return "ECOFLOW_DELTA3_EXECUTE=false"
	}
	if !input.AllowPrivateAPIWrite {
		return "ECOFLOW_DELTA3_ALLOW_PRIVATE_API_WRITE=false"
	}
	if log.ShouldAdjustACChargeLimit && (log.PreviousACChargeLimitW == nil || log.TargetACChargeLimitW == nil) {
		return "DELTA 3 Plus current or target AC charge limit unavailable"
	}
	if (log.ShouldSetBackupReserve || log.ShouldDisableBackupReserve) && log.PreviousBackupReserveSoc == nil {
		return "DELTA 3 Plus current backup reserve unavailable"
	}
	if log.ShouldSetBackupReserve && log.TargetBackupReserveSoc == nil {
		return "DELTA 3 Plus target backup reserve unavailable"
	}
	if log.ShouldDisableBackupReserve && log.TargetBackupReserveSoc == nil {
		return "DELTA 3 Plus target backup reserve unavailable"
	}
	if log.ShouldAdjustACChargeLimit && log.StrategyState != "SAFE_LIMIT" && !log.ShouldSetBackupReserve && !log.ShouldDisableBackupReserve && abs(*log.TargetACChargeLimitW-*log.PreviousACChargeLimitW) < settings.MinCommandDiffW {
		return "command suppressed by minimum diff"
	}
	if previousDelta3AuxWriteCandidate(input.Previous) {
		if input.Previous.CommandFingerprint == log.CommandFingerprint {
			return "duplicate command candidate"
		}
		if !input.Previous.MeasuredAt.IsZero() && !input.Status.UpdatedAt.IsZero() && input.Status.UpdatedAt.Sub(input.Previous.MeasuredAt) < settings.MinCommandInterval {
			return "command suppressed by minimum interval"
		}
	}
	if previousDelta3AuxErroredCandidate(input.Previous) &&
		!input.Previous.MeasuredAt.IsZero() &&
		!input.Status.UpdatedAt.IsZero() &&
		input.Status.UpdatedAt.Sub(input.Previous.MeasuredAt) < settings.MinCommandInterval {
		return "command retry suppressed after previous error"
	}
	return ""
}

func ExecuteDelta3AuxCommand(ctx context.Context, log domain.Delta3AuxControlCommandLog, writer Delta3AuxWriteClient) domain.Delta3AuxControlCommandLog {
	if !log.WouldWrite {
		return log
	}
	log.DryRun = false
	if writer == nil {
		return delta3AuxCommandError(log, "DELTA 3 Plus write client is unavailable")
	}
	if log.ShouldAdjustACChargeLimit && log.TargetACChargeLimitW == nil {
		return delta3AuxCommandError(log, "target AC charge limit is missing")
	}
	if log.ShouldAdjustACChargeLimit {
		if err := writer.SetACChargePower(ctx, *log.TargetACChargeLimitW); err != nil {
			return delta3AuxCommandError(log, fmt.Sprintf("set DELTA 3 Plus AC charge power: %v", err))
		}
		log.CommandSent = true
	}
	if log.ShouldSetBackupReserve {
		if log.TargetBackupReserveSoc == nil {
			return delta3AuxCommandError(log, "target backup reserve SOC is missing")
		}
		if err := writer.SetEnergyBackupEnabled(ctx, true, *log.TargetBackupReserveSoc); err != nil {
			return delta3AuxCommandError(log, fmt.Sprintf("set DELTA 3 Plus backup reserve SOC: %v", err))
		}
		log.CommandSent = true
	}
	if log.ShouldDisableBackupReserve {
		if log.TargetBackupReserveSoc == nil {
			return delta3AuxCommandError(log, "target backup reserve SOC is missing")
		}
		if err := writer.SetEnergyBackupEnabled(ctx, false, *log.TargetBackupReserveSoc); err != nil {
			return delta3AuxCommandError(log, fmt.Sprintf("disable DELTA 3 Plus backup reserve SOC: %v", err))
		}
		log.CommandSent = true
	}
	return log
}

func delta3AuxCommandError(log domain.Delta3AuxControlCommandLog, message string) domain.Delta3AuxControlCommandLog {
	log.DryRun = false
	log.WouldWrite = false
	log.ErrorMessage = &message
	return log
}

func delta3AuxCommandFingerprint(log domain.Delta3AuxControlCommandLog) string {
	target := "-"
	if log.TargetACChargeLimitW != nil {
		target = fmt.Sprintf("%d", *log.TargetACChargeLimitW)
	}
	reserve := "-"
	if log.TargetBackupReserveSoc != nil {
		reserve = fmt.Sprintf("%d", *log.TargetBackupReserveSoc)
	}
	return fmt.Sprintf("delta3_aux;state=%s;ac=%s;reserve=%s;adjust_ac=%t;set_reserve=%t;disable_reserve=%t", log.StrategyState, target, reserve, log.ShouldAdjustACChargeLimit, log.ShouldSetBackupReserve, log.ShouldDisableBackupReserve)
}

func previousDelta3AuxWriteCandidate(previous *domain.Delta3AuxControlCommandLog) bool {
	return previous != nil && previous.ErrorMessage == nil && (previous.WouldWrite || previous.CommandSent) && previous.CommandFingerprint != ""
}

func previousDelta3AuxErroredCandidate(previous *domain.Delta3AuxControlCommandLog) bool {
	return previous != nil && previous.ErrorMessage != nil && previous.CommandFingerprint != ""
}
