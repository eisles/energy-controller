package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

type StatusRepository struct {
	db *sql.DB
}

func NewStatusRepository(db *sql.DB) *StatusRepository {
	return &StatusRepository{db: db}
}

func (r *StatusRepository) CurrentStatus(ctx context.Context) (domain.Status, error) {
	var status domain.Status
	var updatedAt string
	var batterySoc, batteryInputW, batteryOutputW, acChargeLimitW, backupReserveSoc, energyBackupEnabled, touModeEnabled, selfPoweredEnabled, scheduledEnabled, intelligentEnabled, batteryFullEnergyWh sql.NullInt64
	var lastError, ecoflowDiagnosticsJSON, surplusPlanJSON, nightChargePlanJSON, delta3AuxPlanJSON sql.NullString

	err := r.db.QueryRowContext(ctx, `SELECT
		grid_w, import_w, export_w, battery_soc, battery_input_w,
		battery_output_w, ac_charge_limit_w, target_charge_w, state, mode,
		last_decision_reason, last_error, updated_at, backup_reserve_soc,
		energy_backup_enabled, tou_mode_enabled, self_powered_enabled, scheduled_enabled,
		intelligent_enabled, battery_full_energy_wh, ecoflow_diagnostics_json, surplus_plan_json,
		night_charge_plan_json, delta3_aux_plan_json
		FROM current_status WHERE id = 1`,
	).Scan(
		&status.GridW,
		&status.ImportW,
		&status.ExportW,
		&batterySoc,
		&batteryInputW,
		&batteryOutputW,
		&acChargeLimitW,
		&status.TargetChargeW,
		&status.State,
		&status.Mode,
		&status.LastDecisionReason,
		&lastError,
		&updatedAt,
		&backupReserveSoc,
		&energyBackupEnabled,
		&touModeEnabled,
		&selfPoweredEnabled,
		&scheduledEnabled,
		&intelligentEnabled,
		&batteryFullEnergyWh,
		&ecoflowDiagnosticsJSON,
		&surplusPlanJSON,
		&nightChargePlanJSON,
		&delta3AuxPlanJSON,
	)
	if err != nil {
		return domain.Status{}, err
	}

	status.BatterySoc = intFromNull(batterySoc)
	status.BatteryInputW = intFromNull(batteryInputW)
	status.BatteryOutputW = intFromNull(batteryOutputW)
	status.ACChargeLimitW = intFromNull(acChargeLimitW)
	status.BackupReserveSoc = intPtrFromNull(backupReserveSoc)
	status.EnergyBackupEnabled = boolPtrFromNull(energyBackupEnabled)
	status.TOUModeEnabled = boolPtrFromNull(touModeEnabled)
	status.SelfPoweredEnabled = boolPtrFromNull(selfPoweredEnabled)
	status.ScheduledEnabled = boolPtrFromNull(scheduledEnabled)
	status.IntelligentEnabled = boolPtrFromNull(intelligentEnabled)
	status.BatteryFullEnergyWh = intPtrFromNull(batteryFullEnergyWh)
	if ecoflowDiagnosticsJSON.Valid && ecoflowDiagnosticsJSON.String != "" {
		diagnostics, err := mapFromJSON(ecoflowDiagnosticsJSON.String)
		if err != nil {
			return domain.Status{}, err
		}
		status.EcoFlowDiagnostics = diagnostics
	}
	if surplusPlanJSON.Valid && surplusPlanJSON.String != "" {
		var plan domain.SurplusPlan
		if err := json.Unmarshal([]byte(surplusPlanJSON.String), &plan); err != nil {
			return domain.Status{}, err
		}
		status.SurplusPlan = &plan
	}
	if nightChargePlanJSON.Valid && nightChargePlanJSON.String != "" {
		var plan domain.NightChargePlan
		if err := json.Unmarshal([]byte(nightChargePlanJSON.String), &plan); err != nil {
			return domain.Status{}, err
		}
		status.NightChargePlan = &plan
	}
	if delta3AuxPlanJSON.Valid && delta3AuxPlanJSON.String != "" {
		var plan domain.Delta3AuxPlan
		if err := json.Unmarshal([]byte(delta3AuxPlanJSON.String), &plan); err != nil {
			return domain.Status{}, err
		}
		status.Delta3AuxPlan = &plan
	}
	if lastError.Valid {
		status.LastError = &lastError.String
	}
	parsed, err := parseTime(updatedAt)
	if err != nil {
		return domain.Status{}, err
	}
	status.UpdatedAt = parsed
	return status, nil
}

func (r *StatusRepository) UpdateCurrentStatus(ctx context.Context, status domain.Status) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO current_status (
		id, grid_w, import_w, export_w, battery_soc, battery_input_w,
		battery_output_w, ac_charge_limit_w, target_charge_w, state, mode, last_decision_reason,
		last_error, updated_at, backup_reserve_soc, energy_backup_enabled, tou_mode_enabled,
		self_powered_enabled, scheduled_enabled, intelligent_enabled, battery_full_energy_wh,
		ecoflow_diagnostics_json, surplus_plan_json, night_charge_plan_json, delta3_aux_plan_json
	) VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		grid_w = excluded.grid_w,
		import_w = excluded.import_w,
		export_w = excluded.export_w,
		battery_soc = excluded.battery_soc,
		battery_input_w = excluded.battery_input_w,
		battery_output_w = excluded.battery_output_w,
		ac_charge_limit_w = excluded.ac_charge_limit_w,
		target_charge_w = excluded.target_charge_w,
		state = excluded.state,
		mode = excluded.mode,
		last_decision_reason = excluded.last_decision_reason,
		last_error = excluded.last_error,
		backup_reserve_soc = excluded.backup_reserve_soc,
		energy_backup_enabled = excluded.energy_backup_enabled,
		tou_mode_enabled = excluded.tou_mode_enabled,
		self_powered_enabled = excluded.self_powered_enabled,
		scheduled_enabled = excluded.scheduled_enabled,
		intelligent_enabled = excluded.intelligent_enabled,
		battery_full_energy_wh = excluded.battery_full_energy_wh,
		ecoflow_diagnostics_json = excluded.ecoflow_diagnostics_json,
		surplus_plan_json = excluded.surplus_plan_json,
		night_charge_plan_json = excluded.night_charge_plan_json,
		delta3_aux_plan_json = excluded.delta3_aux_plan_json,
		updated_at = excluded.updated_at`,
		status.GridW,
		status.ImportW,
		status.ExportW,
		status.BatterySoc,
		status.BatteryInputW,
		status.BatteryOutputW,
		status.ACChargeLimitW,
		status.TargetChargeW,
		status.State,
		status.Mode,
		status.LastDecisionReason,
		nullableString(status.LastError),
		status.UpdatedAt.Format(time.RFC3339Nano),
		nullableInt(status.BackupReserveSoc),
		nullableBool(status.EnergyBackupEnabled),
		nullableBool(status.TOUModeEnabled),
		nullableBool(status.SelfPoweredEnabled),
		nullableBool(status.ScheduledEnabled),
		nullableBool(status.IntelligentEnabled),
		nullableInt(status.BatteryFullEnergyWh),
		nullableJSON(status.EcoFlowDiagnostics),
		nullableJSON(status.SurplusPlan),
		nullableJSON(status.NightChargePlan),
		nullableJSON(status.Delta3AuxPlan),
	)
	return err
}

func intFromNull(value sql.NullInt64) int {
	if !value.Valid {
		return 0
	}
	return int(value.Int64)
}

func boolPtrFromNull(value sql.NullInt64) *bool {
	if !value.Valid {
		return nil
	}
	converted := value.Int64 != 0
	return &converted
}

func parseTime(value string) (time.Time, error) {
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err == nil {
		return parsed, nil
	}
	return time.Parse(time.RFC3339, value)
}

func nullableString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}

func nullableBool(value *bool) any {
	if value == nil {
		return nil
	}
	if *value {
		return 1
	}
	return 0
}

func nullableJSON(value any) any {
	if value == nil {
		return nil
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	return string(encoded)
}

func mapFromJSON(value string) (map[string]any, error) {
	var decoded map[string]any
	if err := json.Unmarshal([]byte(value), &decoded); err != nil {
		return nil, err
	}
	if len(decoded) == 0 {
		return nil, nil
	}
	return decoded, nil
}
