package control

import (
	"context"
	"fmt"
	"time"

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
	PreviousReserve        *domain.Delta3AuxControlCommandLog
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
		annotateDelta3AuxBackupReserveApplyState(plan, latestDelta3AuxReserveCommand(input), settings, status.UpdatedAt)
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
		stripDelta3AuxUnreflectedReserveRetry(&log, plan)
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
	if input.Status.Delta3AuxPlan != nil && delta3AuxBackupReserveCommandUnreflected(input.Status.Delta3AuxPlan) &&
		log.TargetBackupReserveSoc != nil && input.Status.Delta3AuxPlan.LastBackupReserveTargetSoc != nil &&
		*log.TargetBackupReserveSoc == *input.Status.Delta3AuxPlan.LastBackupReserveTargetSoc {
		return "previous backup reserve command was not reflected by device"
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

func annotateDelta3AuxBackupReserveApplyState(plan *domain.Delta3AuxPlan, previous *domain.Delta3AuxControlCommandLog, settings Delta3AuxSettings, now time.Time) {
	if plan == nil || previous == nil || previous.ErrorMessage != nil || !previous.CommandSent || previous.TargetBackupReserveSoc == nil {
		return
	}
	elapsed := time.Duration(0)
	hasElapsed := !now.IsZero() && !previous.MeasuredAt.IsZero()
	if hasElapsed {
		elapsed = now.Sub(previous.MeasuredAt)
	}
	plan.LastBackupReserveCommandAt = previous.MeasuredAt.Format(time.RFC3339Nano)
	plan.LastBackupReserveTargetSoc = previous.TargetBackupReserveSoc
	if previous.ShouldDisableBackupReserve {
		if plan.CurrentBackupReserveEnabled != nil && !*plan.CurrentBackupReserveEnabled {
			plan.BackupReserveApplyState = "applied"
			plan.BackupReserveApplyReason = "DELTA 3 Plus backup reserve command is reflected by the device"
			return
		}
		if !hasElapsed || elapsed < settings.MinCommandInterval {
			plan.BackupReserveApplyState = "pending"
			plan.BackupReserveApplyReason = "waiting for DELTA 3 Plus backup reserve command reflection"
			return
		}
		if delta3AuxBackupReserveCommandIsStale(elapsed, settings) {
			plan.BackupReserveApplyState = "stale"
			plan.BackupReserveApplyReason = "previous DELTA 3 Plus backup reserve command is still not reflected after the stale window"
			return
		}
		plan.BackupReserveApplyState = "failed"
		plan.BackupReserveApplyReason = "DELTA 3 Plus backup reserve command was not reflected by the device"
		return
	}
	if previous.ShouldSetBackupReserve && plan.CurrentBackupReserveEnabled != nil && !*plan.CurrentBackupReserveEnabled {
		if delta3AuxImportRecoveryMinReserveCommand(previous, settings) && plan.CurrentBackupReserveSoc != nil && *plan.CurrentBackupReserveSoc == 0 {
			if !hasElapsed || elapsed < settings.MinCommandInterval {
				plan.BackupReserveApplyState = "pending"
				plan.BackupReserveApplyReason = "waiting for DELTA 3 Plus backup reserve command reflection"
				return
			}
			plan.BackupReserveApplyState = "ignored"
			plan.BackupReserveApplyReason = "DELTA 3 Plus ignored the import-recovery backup reserve command or already has backup reserve disabled"
			return
		}
		if !hasElapsed || elapsed < settings.MinCommandInterval {
			plan.BackupReserveApplyState = "pending"
			plan.BackupReserveApplyReason = "waiting for DELTA 3 Plus backup reserve command reflection"
			return
		}
		if delta3AuxBackupReserveCommandIsStale(elapsed, settings) {
			plan.BackupReserveApplyState = "stale"
			plan.BackupReserveApplyReason = "previous DELTA 3 Plus backup reserve command is still not reflected after the stale window"
			return
		}
		plan.BackupReserveApplyState = "failed"
		plan.BackupReserveApplyReason = "DELTA 3 Plus backup reserve command was not reflected by the device"
		return
	}
	if plan.CurrentBackupReserveSoc == nil {
		plan.BackupReserveApplyState = "pending"
		plan.BackupReserveApplyReason = "DELTA 3 Plus backup reserve status is unavailable after the last command"
		return
	}
	if previous.ShouldSetBackupReserve && plan.CurrentBackupReserveEnabled == nil {
		if !hasElapsed || elapsed < settings.MinCommandInterval {
			plan.BackupReserveApplyState = "pending"
			plan.BackupReserveApplyReason = "waiting for DELTA 3 Plus backup reserve enabled status after the last command"
			return
		}
		if delta3AuxBackupReserveCommandIsStale(elapsed, settings) {
			plan.BackupReserveApplyState = "stale"
			plan.BackupReserveApplyReason = "previous DELTA 3 Plus backup reserve command is still not reflected after the stale window"
			return
		}
		plan.BackupReserveApplyState = "failed"
		plan.BackupReserveApplyReason = "DELTA 3 Plus backup reserve enabled status is unavailable after the last command"
		return
	}
	if *plan.CurrentBackupReserveSoc == *previous.TargetBackupReserveSoc {
		plan.BackupReserveApplyState = "applied"
		plan.BackupReserveApplyReason = "DELTA 3 Plus backup reserve command is reflected by the device"
		return
	}
	if !hasElapsed || elapsed < settings.MinCommandInterval {
		plan.BackupReserveApplyState = "pending"
		plan.BackupReserveApplyReason = "waiting for DELTA 3 Plus backup reserve command reflection"
		return
	}
	if delta3AuxBackupReserveCommandIsStale(elapsed, settings) {
		plan.BackupReserveApplyState = "stale"
		plan.BackupReserveApplyReason = "previous DELTA 3 Plus backup reserve command is still not reflected after the stale window"
		return
	}
	plan.BackupReserveApplyState = "failed"
	plan.BackupReserveApplyReason = "DELTA 3 Plus backup reserve command was not reflected by the device"
}

func delta3AuxBackupReserveCommandIsStale(elapsed time.Duration, settings Delta3AuxSettings) bool {
	window := 6 * settings.MinCommandInterval
	if window < 30*time.Minute {
		window = 30 * time.Minute
	}
	return elapsed >= window
}

func latestDelta3AuxReserveCommand(input Delta3AuxCommandGuardInput) *domain.Delta3AuxControlCommandLog {
	if input.PreviousReserve != nil {
		return input.PreviousReserve
	}
	if input.Previous != nil && input.Previous.TargetBackupReserveSoc != nil && (input.Previous.ShouldSetBackupReserve || input.Previous.ShouldDisableBackupReserve) {
		return input.Previous
	}
	return nil
}

func delta3AuxImportRecoveryMinReserveCommand(log *domain.Delta3AuxControlCommandLog, settings Delta3AuxSettings) bool {
	if log == nil || !log.ShouldSetBackupReserve || log.TargetBackupReserveSoc == nil {
		return false
	}
	settings = normalizeDelta3AuxSettings(settings)
	return (log.StrategyState == "RECOVERING" || log.StrategyState == "SAFE_LIMIT") && *log.TargetBackupReserveSoc == settings.BackupReserveMinSoc
}

func stripDelta3AuxUnreflectedReserveRetry(log *domain.Delta3AuxControlCommandLog, plan *domain.Delta3AuxPlan) {
	if log == nil || plan == nil || !delta3AuxBackupReserveCommandUnreflected(plan) || plan.LastBackupReserveTargetSoc == nil || log.TargetBackupReserveSoc == nil {
		return
	}
	if *log.TargetBackupReserveSoc != *plan.LastBackupReserveTargetSoc || !delta3AuxAllowsSafetyWriteDespiteReserveFailure(*log) {
		return
	}
	log.ShouldSetBackupReserve = false
	log.ShouldDisableBackupReserve = false
	log.TargetBackupReserveSoc = nil
}

func delta3AuxAllowsSafetyWriteDespiteReserveFailure(log domain.Delta3AuxControlCommandLog) bool {
	if !log.ShouldAdjustACChargeLimit || log.PreviousACChargeLimitW == nil || log.TargetACChargeLimitW == nil {
		return false
	}
	if log.StrategyState == "SAFE_LIMIT" {
		return true
	}
	return *log.TargetACChargeLimitW < *log.PreviousACChargeLimitW
}

func delta3AuxBackupReserveCommandUnreflected(plan *domain.Delta3AuxPlan) bool {
	if plan == nil {
		return false
	}
	return plan.BackupReserveApplyState == "failed" || plan.BackupReserveApplyState == "stale"
}
