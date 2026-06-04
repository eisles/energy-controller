package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/eisles/energy-controller/backend/internal/config"
	"github.com/eisles/energy-controller/backend/internal/domain"
)

const controlDiagnosticsStaleAfter = 2 * time.Minute

func statusHandler(provider StatusProvider, logger *slog.Logger, cfg config.Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status, err := provider.CurrentStatus(r.Context())
		if err != nil {
			logger.Error("failed to get current status", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get current status"})
			return
		}
		applyRealControlTrialStatus(&status, cfg)
		status.ControlWriteReadiness = controlWriteReadiness(cfg, status.RealControlTrialActive)
		status.ControlDiagnostics = controlDiagnostics(status, apiNow(cfg))
		logger.Info("current status read", "mode", status.Mode, "state", status.State, "gridW", status.GridW, "targetChargeW", status.TargetChargeW, "reason", status.LastDecisionReason)
		writeJSON(w, http.StatusOK, status)
	}
}

func apiNow(cfg config.Config) time.Time {
	if cfg.Clock != nil {
		return cfg.Clock.Now()
	}
	return time.Now()
}

func applyRealControlTrialStatus(status *domain.Status, cfg config.Config) {
	if status == nil || cfg.RealControlTrialUntil.IsZero() {
		return
	}
	now := time.Now()
	if cfg.Clock != nil {
		now = cfg.Clock.Now()
	}
	until := cfg.RealControlTrialUntil
	status.RealControlTrialUntil = &until
	if now.Before(until) {
		status.RealControlTrialActive = true
		status.RealControlTrialRemainingSeconds = int64(until.Sub(now).Seconds())
	}
}

func controlDiagnostics(status domain.Status, now time.Time) *domain.ControlDiagnostics {
	gridState := controlGridState(status)
	dataFreshness := controlDataFreshness(status, now)
	writeReadiness := controlDiagnosticsReadiness(status.ControlWriteReadiness)
	pro3 := pro3ControlDiagnostics(status)
	auxiliary := auxiliaryControlDiagnostics(status)
	return &domain.ControlDiagnostics{
		GridState:      gridState,
		Summary:        controlDiagnosticsSummary(gridState, dataFreshness, writeReadiness, pro3, auxiliary),
		DataFreshness:  dataFreshness,
		WriteReadiness: writeReadiness,
		Pro3:           pro3,
		Auxiliary:      auxiliary,
	}
}

func controlGridState(status domain.Status) string {
	if status.ImportW > 0 || status.GridW > 0 {
		return "importing"
	}
	if status.ExportW > 0 || status.GridW < 0 {
		return "exporting"
	}
	return "neutral"
}

func controlDataFreshness(status domain.Status, now time.Time) domain.ControlDataFreshness {
	ageSeconds := int64(0)
	if !status.UpdatedAt.IsZero() {
		ageSeconds = int64(now.Sub(status.UpdatedAt).Seconds())
		if ageSeconds < 0 {
			ageSeconds = 0
		}
	}
	return domain.ControlDataFreshness{
		UpdatedAt:  status.UpdatedAt,
		AgeSeconds: ageSeconds,
		Stale:      status.UpdatedAt.IsZero() || time.Duration(ageSeconds)*time.Second > controlDiagnosticsStaleAfter,
		HasError:   status.LastError != nil,
		LastError:  status.LastError,
	}
}

func controlDiagnosticsReadiness(readiness *domain.ControlWriteReadiness) domain.ControlDiagnosticsReadiness {
	if readiness == nil {
		return domain.ControlDiagnosticsReadiness{
			Ready:          false,
			Mode:           "unknown",
			BlockedReason:  "control write readiness is unavailable",
			BlockedReasons: 1,
		}
	}
	blockedReason := ""
	if len(readiness.Reasons) > 0 {
		blockedReason = readiness.Reasons[0]
	}
	return domain.ControlDiagnosticsReadiness{
		Ready:          readiness.Ready,
		Mode:           readiness.Mode,
		BlockedReason:  blockedReason,
		BlockedReasons: len(readiness.Reasons),
	}
}

func pro3ControlDiagnostics(status domain.Status) domain.ControlDeviceDiagnostics {
	diagnostics := domain.ControlDeviceDiagnostics{
		Name:           "DELTA Pro 3",
		DeviceType:     "DELTA_PRO3",
		Action:         "hold",
		Reason:         status.LastDecisionReason,
		ControlSource:  "status",
		SOC:            intPtr(status.BatterySoc),
		ACInputW:       intPtr(status.BatteryInputW),
		ACOutputW:      intPtr(status.BatteryOutputW),
		TargetChargeW:  intPtr(status.TargetChargeW),
		WriteCandidate: false,
	}
	if status.LastError != nil {
		diagnostics.Action = "unavailable"
		diagnostics.Reason = "status has lastError"
		return diagnostics
	}
	if plan := status.NightChargePlan; plan != nil && plan.WouldWrite {
		diagnostics.Action = "night_charge_candidate"
		diagnostics.Reason = firstNonEmpty(plan.ActionSummary, plan.Reason, status.LastDecisionReason)
		diagnostics.ControlSource = "night_charge_plan"
		diagnostics.RecommendedACChargeLimitW = intPtr(plan.RecommendedACChargeLimitW)
		diagnostics.RecommendedBackupReserveSoc = plan.RecommendedBackupReserveSoc
		diagnostics.WriteCandidate = plan.WouldWrite
		return diagnostics
	}
	if plan := status.SurplusPlan; plan != nil {
		diagnostics.Reason = firstNonEmpty(plan.ActionSummary, plan.Reason, status.LastDecisionReason)
		diagnostics.ControlSource = "surplus_plan"
		diagnostics.RecommendedACChargeLimitW = intPtr(plan.RecommendedACChargeLimitW)
		diagnostics.RecommendedBackupReserveSoc = plan.RecommendedBackupReserveSoc
		diagnostics.WriteCandidate = plan.WouldWrite
		switch {
		case strings.EqualFold(plan.StrategyState, "PASSTHROUGH"):
			diagnostics.Action = "passthrough"
		case plan.ShouldAdjustACChargeLimit || plan.ShouldRaiseBackupReserve || plan.ShouldAlignBackupReserve:
			diagnostics.Action = "surplus_charge_candidate"
		case plan.ShouldLowerBackupReserve || plan.ShouldDisableEnergyModes || plan.ShouldEnableTOUMode:
			diagnostics.Action = "discharge_recovery_candidate"
		case strings.EqualFold(plan.StrategyState, "CHARGING"):
			diagnostics.Action = "charging"
		case strings.EqualFold(plan.StrategyState, "RECOVERING"):
			diagnostics.Action = "recovering"
		default:
			diagnostics.Action = "hold"
		}
		return diagnostics
	}
	switch {
	case status.BatteryInputW > status.BatteryOutputW:
		diagnostics.Action = "charging"
	case status.BatteryOutputW > status.BatteryInputW:
		diagnostics.Action = "discharging"
	}
	return diagnostics
}

func auxiliaryControlDiagnostics(status domain.Status) domain.ControlDeviceDiagnostics {
	diagnostics := domain.ControlDeviceDiagnostics{
		Name:           "補助バッテリー",
		Action:         "unavailable",
		Reason:         "auxiliary control plan is unavailable",
		ControlSource:  "delta3_aux_plan",
		WriteCandidate: false,
	}
	plan := status.Delta3AuxPlan
	if plan == nil {
		return diagnostics
	}
	diagnostics.Name = firstNonEmpty(plan.DeviceName, "補助バッテリー")
	diagnostics.DeviceType = plan.DeviceType
	diagnostics.Action = "hold"
	diagnostics.Reason = firstNonEmpty(plan.SuppressedReason, plan.Reason)
	diagnostics.SOC = plan.Delta3Soc
	diagnostics.ACOutputW = plan.Delta3ACOutputW
	diagnostics.RecommendedACChargeLimitW = intPtr(plan.RecommendedACChargeLimitW)
	diagnostics.RecommendedBackupReserveSoc = plan.RecommendedBackupReserveSoc
	diagnostics.WriteCandidate = plan.WouldWrite
	switch {
	case plan.ShouldAdjustACChargeLimit || plan.ShouldSetBackupReserve:
		diagnostics.Action = "surplus_charge_candidate"
	case plan.ShouldDisableBackupReserve:
		diagnostics.Action = "discharge_recovery_candidate"
	case strings.EqualFold(plan.StrategyState, "PASSTHROUGH"):
		diagnostics.Action = "passthrough"
	case strings.EqualFold(plan.StrategyState, "UNAVAILABLE"):
		diagnostics.Action = "unavailable"
	case strings.EqualFold(plan.StrategyState, "FULL"):
		diagnostics.Action = "full"
	}
	return diagnostics
}

func controlDiagnosticsSummary(
	gridState string,
	dataFreshness domain.ControlDataFreshness,
	writeReadiness domain.ControlDiagnosticsReadiness,
	pro3 domain.ControlDeviceDiagnostics,
	auxiliary domain.ControlDeviceDiagnostics,
) string {
	if dataFreshness.HasError {
		return "status_error"
	}
	if dataFreshness.Stale {
		return "stale_data"
	}
	if !writeReadiness.Ready {
		return "write_blocked"
	}
	if gridState == "exporting" && (pro3.WriteCandidate || auxiliary.WriteCandidate) {
		return "export_absorb_candidate"
	}
	if gridState == "importing" && (pro3.Action == "discharging" || pro3.Action == "discharge_recovery_candidate" || auxiliary.Action == "discharge_recovery_candidate") {
		return "import_discharge_candidate"
	}
	return "observing"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func controlWriteReadiness(cfg config.Config, realControlTrialActive bool) *domain.ControlWriteReadiness {
	gates := domain.ControlWriteGates{
		MockMode:                    cfg.MockMode,
		SimulationMode:              cfg.SimulationMode,
		EnableRealControl:           cfg.EnableRealControl,
		AutoControlEnabled:          cfg.AutoControlEnabled,
		ConfirmEcoFlowWriteAccepted: cfg.ConfirmEcoFlowWrite == "I_UNDERSTAND",
		RealControlTrialConfigured:  !cfg.RealControlTrialUntil.IsZero(),
		RealControlTrialActive:      realControlTrialActive,
		Delta3ReadEnabled:           cfg.Delta3ReadEnabled,
		Delta3AuxEnabled:            cfg.Delta3Aux.Enabled,
		Delta3ExecuteWrite:          cfg.Delta3ExecuteWrite,
		Delta3AllowPrivateWrite:     cfg.Delta3AllowPrivateWrite,
		Delta3AllowAutoWrite:        cfg.Delta3AllowAutoWrite,
	}
	reasons := make([]string, 0, 6)
	if gates.MockMode {
		reasons = append(reasons, "mock mode keeps device write disabled")
	}
	if gates.SimulationMode {
		reasons = append(reasons, "simulation mode keeps device write disabled")
	}
	if !gates.EnableRealControl {
		reasons = append(reasons, "ENABLE_REAL_CONTROL=false keeps device write disabled")
	}
	if !gates.AutoControlEnabled {
		reasons = append(reasons, "auto control disabled keeps device write disabled")
	}
	if !gates.ConfirmEcoFlowWriteAccepted {
		reasons = append(reasons, "CONFIRM_ECOFLOW_WRITE is not I_UNDERSTAND")
	}
	if !gates.RealControlTrialConfigured || !gates.RealControlTrialActive {
		reasons = append(reasons, "real control trial window inactive")
	}
	ready := len(reasons) == 0
	mode := "dry-run"
	if ready {
		mode = "ready"
	}
	return &domain.ControlWriteReadiness{
		Ready:   ready,
		Mode:    mode,
		Reasons: reasons,
		Gates:   gates,
	}
}

func writeJSON(w http.ResponseWriter, statusCode int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(value)
}
