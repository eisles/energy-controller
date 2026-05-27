package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

type NightChargePlanRepository struct {
	db *sql.DB
}

type NightChargePlanLogPageFilter struct {
	From *time.Time
	To   *time.Time
}

func NewNightChargePlanRepository(db *sql.DB) *NightChargePlanRepository {
	return &NightChargePlanRepository{db: db}
}

func (r *NightChargePlanRepository) InsertNightChargePlanLog(ctx context.Context, status domain.Status) error {
	if status.NightChargePlan == nil {
		return nil
	}
	log := nightChargePlanLogFromStatus(status)
	_, err := r.db.ExecContext(ctx, `INSERT INTO night_charge_plan_logs (
		measured_at, strategy_state, recommended_mode, recommended_night_target_soc,
		recommended_night_target_kwh, current_battery_energy_kwh, required_night_charge_kwh,
		daily_estimated_pv_kwh, pv_charge_correction_factor, pv_charge_correction_source,
		corrected_estimated_pv_kwh, corrected_estimated_pv_to_battery_kwh,
		total_daytime_required_kwh, total_available_kwh, total_deficit_kwh,
		pv_effective_start_at, pv_effective_end_at, pv_effective_window_source,
		morning_to_pv_start_load_kwh, forecast_daytime_deficit_kwh,
		battery_soc, battery_input_w, battery_output_w, grid_w, import_w, export_w,
		should_charge_tonight, would_write, command_fingerprint, command_sent, command_error, command_block_reason, action_summary, reason,
		target_forecast_date, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		log.MeasuredAt.Format(time.RFC3339Nano),
		log.StrategyState,
		log.RecommendedMode,
		log.RecommendedNightTargetSoc,
		log.RecommendedNightTargetKWh,
		log.CurrentBatteryEnergyKWh,
		log.RequiredNightChargeKWh,
		log.DailyEstimatedPVKWh,
		log.PVChargeCorrectionFactor,
		log.PVChargeCorrectionSource,
		log.CorrectedEstimatedPVKWh,
		log.CorrectedEstimatedPVToBatteryKWh,
		log.TotalDaytimeRequiredKWh,
		log.TotalAvailableKWh,
		log.TotalDeficitKWh,
		log.PVEffectiveStartAt,
		log.PVEffectiveEndAt,
		log.PVEffectiveWindowSource,
		log.MorningToPVStartLoadKWh,
		log.ForecastDaytimeDeficitKWh,
		log.BatterySoc,
		log.BatteryInputW,
		log.BatteryOutputW,
		log.GridW,
		log.ImportW,
		log.ExportW,
		boolToInt(log.ShouldChargeTonight),
		boolToInt(log.WouldWrite),
		log.CommandFingerprint,
		boolToInt(log.CommandSent),
		nullableString(log.CommandError),
		log.CommandBlockReason,
		log.ActionSummary,
		log.Reason,
		nullableString(log.TargetForecastDate),
		log.CreatedAt.Format(time.RFC3339Nano),
	)
	return err
}

func (r *NightChargePlanRepository) ListNightChargePlanLogsPage(ctx context.Context, limit int, offset int, filter NightChargePlanLogPageFilter) ([]domain.NightChargePlanLog, int, error) {
	limit = normalizeLimit(limit)
	offset = normalizeOffset(offset)
	whereClause, queryArgs := nightChargePlanLogWhere(filter)
	args := append(queryArgs, limit, offset)
	rows, err := r.db.QueryContext(ctx, `SELECT
		id, measured_at, strategy_state, recommended_mode, recommended_night_target_soc,
		recommended_night_target_kwh, current_battery_energy_kwh, required_night_charge_kwh,
		daily_estimated_pv_kwh, pv_charge_correction_factor, pv_charge_correction_source,
		corrected_estimated_pv_kwh, corrected_estimated_pv_to_battery_kwh,
		total_daytime_required_kwh, total_available_kwh, total_deficit_kwh,
		pv_effective_start_at, pv_effective_end_at, pv_effective_window_source,
		morning_to_pv_start_load_kwh, forecast_daytime_deficit_kwh,
		battery_soc, battery_input_w, battery_output_w, grid_w, import_w, export_w,
		should_charge_tonight, would_write, command_fingerprint, command_sent, command_error, command_block_reason, action_summary, reason,
		target_forecast_date, created_at
		FROM night_charge_plan_logs
		`+whereClause+`
		ORDER BY measured_at DESC, id DESC
		LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	logs, err := scanNightChargePlanLogs(rows, limit)
	if err != nil {
		return nil, 0, err
	}
	total, err := r.countNightChargePlanLogs(ctx, whereClause, queryArgs)
	if err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}

func (r *NightChargePlanRepository) LatestNightChargePlanWriteCandidateLog(ctx context.Context) (*domain.NightChargePlanLog, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT
		id, measured_at, strategy_state, recommended_mode, recommended_night_target_soc,
		recommended_night_target_kwh, current_battery_energy_kwh, required_night_charge_kwh,
		daily_estimated_pv_kwh, pv_charge_correction_factor, pv_charge_correction_source,
		corrected_estimated_pv_kwh, corrected_estimated_pv_to_battery_kwh,
		total_daytime_required_kwh, total_available_kwh, total_deficit_kwh,
		pv_effective_start_at, pv_effective_end_at, pv_effective_window_source,
		morning_to_pv_start_load_kwh, forecast_daytime_deficit_kwh,
		battery_soc, battery_input_w, battery_output_w, grid_w, import_w, export_w,
		should_charge_tonight, would_write, command_fingerprint, command_sent, command_error, command_block_reason, action_summary, reason,
		target_forecast_date, created_at
		FROM night_charge_plan_logs
		WHERE would_write = 1 OR command_sent = 1 OR command_error IS NOT NULL
		ORDER BY measured_at DESC, id DESC
		LIMIT 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	logs, err := scanNightChargePlanLogs(rows, 1)
	if err != nil {
		return nil, err
	}
	if len(logs) == 0 {
		return nil, nil
	}
	return &logs[0], nil
}

func nightChargePlanLogFromStatus(status domain.Status) domain.NightChargePlanLog {
	plan := status.NightChargePlan
	log := domain.NightChargePlanLog{
		MeasuredAt:                       status.UpdatedAt,
		StrategyState:                    plan.StrategyState,
		RecommendedMode:                  plan.RecommendedMode,
		RecommendedNightTargetSoc:        plan.RecommendedNightTargetSoc,
		RecommendedNightTargetKWh:        plan.RecommendedNightTargetKWh,
		CurrentBatteryEnergyKWh:          plan.CurrentBatteryEnergyKWh,
		RequiredNightChargeKWh:           plan.RequiredNightChargeKWh,
		DailyEstimatedPVKWh:              plan.DailyEstimatedPVKWh,
		PVChargeCorrectionFactor:         plan.PVChargeCorrectionFactor,
		PVChargeCorrectionSource:         plan.PVChargeCorrectionSource,
		CorrectedEstimatedPVKWh:          plan.CorrectedEstimatedPVKWh,
		CorrectedEstimatedPVToBatteryKWh: plan.CorrectedEstimatedPVToBatteryKWh,
		TotalDaytimeRequiredKWh:          plan.TotalDaytimeRequiredKWh,
		TotalAvailableKWh:                plan.TotalAvailableKWh,
		TotalDeficitKWh:                  plan.TotalDeficitKWh,
		PVEffectiveStartAt:               plan.PVEffectiveStartAt,
		PVEffectiveEndAt:                 plan.PVEffectiveEndAt,
		PVEffectiveWindowSource:          plan.PVEffectiveWindowSource,
		MorningToPVStartLoadKWh:          plan.MorningToPVStartLoadKWh,
		ForecastDaytimeDeficitKWh:        plan.ForecastDaytimeDeficitKWh,
		BatterySoc:                       status.BatterySoc,
		BatteryInputW:                    status.BatteryInputW,
		BatteryOutputW:                   status.BatteryOutputW,
		GridW:                            status.GridW,
		ImportW:                          status.ImportW,
		ExportW:                          status.ExportW,
		ShouldChargeTonight:              plan.ShouldChargeTonight,
		WouldWrite:                       plan.WouldWrite,
		CommandFingerprint:               plan.CommandFingerprint,
		CommandSent:                      plan.CommandSent,
		CommandError:                     plan.CommandError,
		CommandBlockReason:               plan.CommandBlockReason,
		ActionSummary:                    plan.ActionSummary,
		Reason:                           plan.Reason,
		CreatedAt:                        status.UpdatedAt,
	}
	if log.CommandFingerprint == "" {
		log.CommandFingerprint = "none"
	}
	if plan.TargetForecast != nil {
		targetDate := plan.TargetForecast.Date
		log.TargetForecastDate = &targetDate
	}
	return log
}

func scanNightChargePlanLogs(rows *sql.Rows, capacity int) ([]domain.NightChargePlanLog, error) {
	logs := make([]domain.NightChargePlanLog, 0, capacity)
	for rows.Next() {
		var log domain.NightChargePlanLog
		var measuredAt, createdAt string
		var shouldChargeTonight, wouldWrite, commandSent int
		var commandError, targetForecastDate sql.NullString
		if err := rows.Scan(
			&log.ID,
			&measuredAt,
			&log.StrategyState,
			&log.RecommendedMode,
			&log.RecommendedNightTargetSoc,
			&log.RecommendedNightTargetKWh,
			&log.CurrentBatteryEnergyKWh,
			&log.RequiredNightChargeKWh,
			&log.DailyEstimatedPVKWh,
			&log.PVChargeCorrectionFactor,
			&log.PVChargeCorrectionSource,
			&log.CorrectedEstimatedPVKWh,
			&log.CorrectedEstimatedPVToBatteryKWh,
			&log.TotalDaytimeRequiredKWh,
			&log.TotalAvailableKWh,
			&log.TotalDeficitKWh,
			&log.PVEffectiveStartAt,
			&log.PVEffectiveEndAt,
			&log.PVEffectiveWindowSource,
			&log.MorningToPVStartLoadKWh,
			&log.ForecastDaytimeDeficitKWh,
			&log.BatterySoc,
			&log.BatteryInputW,
			&log.BatteryOutputW,
			&log.GridW,
			&log.ImportW,
			&log.ExportW,
			&shouldChargeTonight,
			&wouldWrite,
			&log.CommandFingerprint,
			&commandSent,
			&commandError,
			&log.CommandBlockReason,
			&log.ActionSummary,
			&log.Reason,
			&targetForecastDate,
			&createdAt,
		); err != nil {
			return nil, err
		}
		parsedMeasuredAt, err := parseTime(measuredAt)
		if err != nil {
			return nil, err
		}
		parsedCreatedAt, err := parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		log.MeasuredAt = parsedMeasuredAt
		log.CreatedAt = parsedCreatedAt
		log.ShouldChargeTonight = shouldChargeTonight != 0
		log.WouldWrite = wouldWrite != 0
		log.CommandSent = commandSent != 0
		if commandError.Valid {
			log.CommandError = &commandError.String
		}
		if targetForecastDate.Valid {
			log.TargetForecastDate = &targetForecastDate.String
		}
		logs = append(logs, log)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return logs, nil
}

func (r *NightChargePlanRepository) countNightChargePlanLogs(ctx context.Context, whereClause string, args []any) (int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM night_charge_plan_logs `+whereClause, args...).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

func nightChargePlanLogWhere(filter NightChargePlanLogPageFilter) (string, []any) {
	clauses := make([]string, 0, 2)
	args := make([]any, 0, 2)
	if filter.From != nil {
		clauses = append(clauses, "julianday(measured_at) >= julianday(?)")
		args = append(args, filter.From.Format(time.RFC3339Nano))
	}
	if filter.To != nil {
		clauses = append(clauses, "julianday(measured_at) <= julianday(?)")
		args = append(args, filter.To.Format(time.RFC3339Nano))
	}
	if len(clauses) == 0 {
		return "", nil
	}
	return "WHERE " + joinWithAnd(clauses), args
}
