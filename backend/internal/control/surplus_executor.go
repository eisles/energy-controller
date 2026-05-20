package control

import (
	"fmt"
	"strings"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

const confirmEcoFlowWriteValue = "I_UNDERSTAND"

type SurplusCommandGuardInput struct {
	Status              domain.Status
	MockMode            bool
	SimulationMode      bool
	EnableRealControl   bool
	AutoControl         bool
	ConfirmEcoFlowWrite string
	Previous            *domain.SurplusControlCommandLog
}

func EvaluateSurplusCommandGuard(input SurplusCommandGuardInput, settings Settings) domain.SurplusControlCommandLog {
	settings = normalizeSettings(settings)
	status := input.Status
	plan := status.SurplusPlan
	log := domain.SurplusControlCommandLog{
		MeasuredAt:               status.UpdatedAt,
		StrategyState:            "IDLE",
		CommandKind:              "none",
		GridW:                    status.GridW,
		ImportW:                  status.ImportW,
		ExportW:                  status.ExportW,
		BatterySoc:               status.BatterySoc,
		BatteryInputW:            status.BatteryInputW,
		BatteryOutputW:           status.BatteryOutputW,
		PreviousACChargeLimitW:   intPtr(status.ACChargeLimitW),
		PreviousBackupReserveSoc: status.BackupReserveSoc,
		CommandSent:              false,
		DryRun:                   true,
		CreatedAt:                status.UpdatedAt,
	}
	if plan == nil {
		log.SuppressedReason = "no surplus plan"
		log.DecisionReason = log.SuppressedReason
		log.CommandFingerprint = commandFingerprint(log)
		return log
	}

	log.StrategyState = plan.StrategyState
	log.ShouldAdjustACChargeLimit = plan.ShouldAdjustACChargeLimit
	log.ShouldSetBackupReserve = plan.ShouldRaiseBackupReserve || plan.ShouldLowerBackupReserve || plan.ShouldAlignBackupReserve
	log.ShouldDisableEnergyModes = plan.ShouldDisableEnergyModes
	log.ShouldEnableTOUMode = plan.ShouldEnableTOUMode
	log.DecisionReason = firstNonEmpty(plan.ActionSummary, plan.Reason)
	log.CommandKind = commandKind(log)
	if log.ShouldAdjustACChargeLimit {
		log.TargetACChargeLimitW = intPtr(plan.RecommendedACChargeLimitW)
	}
	if log.ShouldSetBackupReserve {
		log.TargetBackupReserveSoc = plan.RecommendedBackupReserveSoc
	}
	if log.ShouldDisableEnergyModes || log.ShouldEnableTOUMode {
		if modeReason := modeGuardReason(status, log); modeReason != "" {
			log.ModeGuardReason = modeReason
		} else {
			log.ModeGuardReason = "mode status verified"
		}
	}
	log.CommandFingerprint = commandFingerprint(log)
	log.SuppressedReason = surplusCommandSuppressedReason(input, settings, log)
	log.WouldWrite = log.SuppressedReason == "" && log.CommandKind != "none"
	return log
}

func surplusCommandSuppressedReason(input SurplusCommandGuardInput, settings Settings, log domain.SurplusControlCommandLog) string {
	if log.CommandKind == "none" {
		return "no command candidate"
	}
	if input.MockMode {
		return "mock mode, EcoFlow write disabled"
	}
	if input.SimulationMode {
		return "simulation mode, EcoFlow write disabled"
	}
	if !input.EnableRealControl {
		return "ENABLE_REAL_CONTROL=false, EcoFlow write disabled"
	}
	if !input.AutoControl {
		return "auto control disabled, EcoFlow write disabled"
	}
	if input.ConfirmEcoFlowWrite != confirmEcoFlowWriteValue {
		return "CONFIRM_ECOFLOW_WRITE is not I_UNDERSTAND"
	}
	if log.ShouldSetBackupReserve && input.Status.BackupReserveSoc == nil {
		return "backup reserve status unavailable"
	}
	if log.ShouldDisableEnergyModes || log.ShouldEnableTOUMode {
		if log.ModeGuardReason != "mode status verified" {
			return log.ModeGuardReason
		}
	}
	if previousWriteCandidate(input.Previous) {
		if input.Previous.CommandFingerprint == log.CommandFingerprint {
			return "duplicate command candidate"
		}
		if !input.Previous.MeasuredAt.IsZero() && !input.Status.UpdatedAt.IsZero() && input.Status.UpdatedAt.Sub(input.Previous.MeasuredAt) < settings.MinCommandInterval {
			return "command suppressed by minimum interval"
		}
	}
	return ""
}

func modeGuardReason(status domain.Status, log domain.SurplusControlCommandLog) string {
	missing := make([]string, 0, 4)
	if status.TOUModeEnabled == nil {
		missing = append(missing, "tou")
	}
	if status.SelfPoweredEnabled == nil {
		missing = append(missing, "self-powered")
	}
	if status.ScheduledEnabled == nil {
		missing = append(missing, "scheduled")
	}
	if status.IntelligentEnabled == nil {
		missing = append(missing, "intelligent")
	}
	if len(missing) > 0 {
		return "mode status unavailable: " + strings.Join(missing, ", ")
	}
	if log.ShouldEnableTOUMode {
		if boolPtrTrue(status.TOUModeEnabled) {
			return "TOU mode is already enabled"
		}
		if hasEnabledEnergyMode(SurplusPlanInput{
			SelfPoweredEnabled: status.SelfPoweredEnabled,
			ScheduledEnabled:   status.ScheduledEnabled,
			IntelligentEnabled: status.IntelligentEnabled,
		}) {
			return "TOU mode enable expected all other energy modes disabled"
		}
	}
	if log.ShouldDisableEnergyModes && !hasEnabledEnergyMode(SurplusPlanInput{
		TOUModeEnabled:     status.TOUModeEnabled,
		SelfPoweredEnabled: status.SelfPoweredEnabled,
		ScheduledEnabled:   status.ScheduledEnabled,
		IntelligentEnabled: status.IntelligentEnabled,
	}) {
		return "energy modes already disabled"
	}
	return ""
}

func previousWriteCandidate(previous *domain.SurplusControlCommandLog) bool {
	return previous != nil &&
		previous.WouldWrite &&
		previous.CommandKind != "none" &&
		previous.CommandFingerprint != ""
}

func commandKind(log domain.SurplusControlCommandLog) string {
	count := 0
	kind := "none"
	if log.ShouldAdjustACChargeLimit {
		count++
		kind = "ac_charge_limit"
	}
	if log.ShouldSetBackupReserve {
		count++
		kind = "backup_reserve"
	}
	if log.ShouldDisableEnergyModes || log.ShouldEnableTOUMode {
		count++
		kind = "energy_mode"
	}
	if count > 1 {
		return "mixed"
	}
	return kind
}

func commandFingerprint(log domain.SurplusControlCommandLog) string {
	return fmt.Sprintf(
		"kind=%s;ac=%s;reserve=%s;adjust_ac=%t;set_reserve=%t;disable_modes=%t;enable_tou=%t",
		log.CommandKind,
		intPtrFingerprint(log.TargetACChargeLimitW),
		intPtrFingerprint(log.TargetBackupReserveSoc),
		log.ShouldAdjustACChargeLimit,
		log.ShouldSetBackupReserve,
		log.ShouldDisableEnergyModes,
		log.ShouldEnableTOUMode,
	)
}

func intPtrFingerprint(value *int) string {
	if value == nil {
		return "-"
	}
	return fmt.Sprintf("%d", *value)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func intPtr(value int) *int {
	return &value
}
