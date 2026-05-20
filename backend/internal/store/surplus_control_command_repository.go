package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

type SurplusControlCommandRepository struct {
	db *sql.DB
}

type SurplusControlCommandLogPageFilter struct {
	From *time.Time
	To   *time.Time
}

func NewSurplusControlCommandRepository(db *sql.DB) *SurplusControlCommandRepository {
	return &SurplusControlCommandRepository{db: db}
}

func (r *SurplusControlCommandRepository) InsertSurplusControlCommandLog(ctx context.Context, log domain.SurplusControlCommandLog) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO surplus_control_command_logs (
		measured_at, strategy_state, command_kind, command_fingerprint, grid_w, import_w, export_w,
		battery_soc, battery_input_w, battery_output_w,
		previous_ac_charge_limit_w, target_ac_charge_limit_w,
		previous_backup_reserve_soc, target_backup_reserve_soc,
		command_sent, dry_run, would_write,
		should_adjust_ac_charge_limit, should_set_backup_reserve,
		should_disable_energy_modes, should_enable_tou_mode, mode_guard_reason,
		suppressed_reason, decision_reason,
		error_message, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		log.MeasuredAt.Format(time.RFC3339Nano),
		log.StrategyState,
		log.CommandKind,
		log.CommandFingerprint,
		log.GridW,
		log.ImportW,
		log.ExportW,
		log.BatterySoc,
		log.BatteryInputW,
		log.BatteryOutputW,
		nullableInt(log.PreviousACChargeLimitW),
		nullableInt(log.TargetACChargeLimitW),
		nullableInt(log.PreviousBackupReserveSoc),
		nullableInt(log.TargetBackupReserveSoc),
		boolToInt(log.CommandSent),
		boolToInt(log.DryRun),
		boolToInt(log.WouldWrite),
		boolToInt(log.ShouldAdjustACChargeLimit),
		boolToInt(log.ShouldSetBackupReserve),
		boolToInt(log.ShouldDisableEnergyModes),
		boolToInt(log.ShouldEnableTOUMode),
		log.ModeGuardReason,
		log.SuppressedReason,
		log.DecisionReason,
		nullableString(log.ErrorMessage),
		log.CreatedAt.Format(time.RFC3339Nano),
	)
	return err
}

func (r *SurplusControlCommandRepository) LatestSurplusControlCommandLog(ctx context.Context) (*domain.SurplusControlCommandLog, error) {
	return r.latestSurplusControlCommandLog(ctx, "")
}

func (r *SurplusControlCommandRepository) LatestSurplusControlWriteCandidateLog(ctx context.Context) (*domain.SurplusControlCommandLog, error) {
	return r.latestSurplusControlCommandLog(ctx, "WHERE would_write = 1 OR command_sent = 1")
}

func (r *SurplusControlCommandRepository) latestSurplusControlCommandLog(ctx context.Context, whereClause string) (*domain.SurplusControlCommandLog, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT
			id, measured_at, strategy_state, command_kind, command_fingerprint, grid_w, import_w, export_w,
			battery_soc, battery_input_w, battery_output_w,
			previous_ac_charge_limit_w, target_ac_charge_limit_w,
		previous_backup_reserve_soc, target_backup_reserve_soc,
		command_sent, dry_run, would_write,
		should_adjust_ac_charge_limit, should_set_backup_reserve,
		should_disable_energy_modes, should_enable_tou_mode, mode_guard_reason,
			suppressed_reason, decision_reason,
			error_message, created_at
			FROM surplus_control_command_logs
			`+whereClause+`
			ORDER BY measured_at DESC, id DESC
			LIMIT 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	logs, err := scanSurplusControlCommandLogs(rows, 1)
	if err != nil {
		return nil, err
	}
	if len(logs) == 0 {
		return nil, nil
	}
	return &logs[0], nil
}

func (r *SurplusControlCommandRepository) ListSurplusControlCommandLogsPage(ctx context.Context, limit int, offset int, filter SurplusControlCommandLogPageFilter) ([]domain.SurplusControlCommandLog, int, error) {
	limit = normalizeLimit(limit)
	offset = normalizeOffset(offset)
	whereClause, queryArgs := surplusControlCommandLogWhere(filter)
	args := append(queryArgs, limit, offset)
	rows, err := r.db.QueryContext(ctx, `SELECT
		id, measured_at, strategy_state, command_kind, command_fingerprint, grid_w, import_w, export_w,
		battery_soc, battery_input_w, battery_output_w,
		previous_ac_charge_limit_w, target_ac_charge_limit_w,
		previous_backup_reserve_soc, target_backup_reserve_soc,
		command_sent, dry_run, would_write,
		should_adjust_ac_charge_limit, should_set_backup_reserve,
		should_disable_energy_modes, should_enable_tou_mode, mode_guard_reason,
		suppressed_reason, decision_reason,
		error_message, created_at
		FROM surplus_control_command_logs
		`+whereClause+`
		ORDER BY measured_at DESC, id DESC
		LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	logs, err := scanSurplusControlCommandLogs(rows, limit)
	if err != nil {
		return nil, 0, err
	}
	total, err := r.countSurplusControlCommandLogs(ctx, whereClause, queryArgs)
	if err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}

func scanSurplusControlCommandLogs(rows *sql.Rows, capacity int) ([]domain.SurplusControlCommandLog, error) {
	logs := make([]domain.SurplusControlCommandLog, 0, capacity)
	for rows.Next() {
		var log domain.SurplusControlCommandLog
		var measuredAt, createdAt string
		var previousACChargeLimitW, targetACChargeLimitW, previousBackupReserveSoc, targetBackupReserveSoc sql.NullInt64
		var commandSent, dryRun, wouldWrite, shouldAdjustACChargeLimit, shouldSetBackupReserve, shouldDisableEnergyModes, shouldEnableTOUMode int
		var errorMessage sql.NullString
		if err := rows.Scan(
			&log.ID,
			&measuredAt,
			&log.StrategyState,
			&log.CommandKind,
			&log.CommandFingerprint,
			&log.GridW,
			&log.ImportW,
			&log.ExportW,
			&log.BatterySoc,
			&log.BatteryInputW,
			&log.BatteryOutputW,
			&previousACChargeLimitW,
			&targetACChargeLimitW,
			&previousBackupReserveSoc,
			&targetBackupReserveSoc,
			&commandSent,
			&dryRun,
			&wouldWrite,
			&shouldAdjustACChargeLimit,
			&shouldSetBackupReserve,
			&shouldDisableEnergyModes,
			&shouldEnableTOUMode,
			&log.ModeGuardReason,
			&log.SuppressedReason,
			&log.DecisionReason,
			&errorMessage,
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
		log.PreviousACChargeLimitW = intPtrFromNull(previousACChargeLimitW)
		log.TargetACChargeLimitW = intPtrFromNull(targetACChargeLimitW)
		log.PreviousBackupReserveSoc = intPtrFromNull(previousBackupReserveSoc)
		log.TargetBackupReserveSoc = intPtrFromNull(targetBackupReserveSoc)
		log.CommandSent = commandSent != 0
		log.DryRun = dryRun != 0
		log.WouldWrite = wouldWrite != 0
		log.ShouldAdjustACChargeLimit = shouldAdjustACChargeLimit != 0
		log.ShouldSetBackupReserve = shouldSetBackupReserve != 0
		log.ShouldDisableEnergyModes = shouldDisableEnergyModes != 0
		log.ShouldEnableTOUMode = shouldEnableTOUMode != 0
		if errorMessage.Valid {
			log.ErrorMessage = &errorMessage.String
		}
		logs = append(logs, log)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return logs, nil
}

func (r *SurplusControlCommandRepository) countSurplusControlCommandLogs(ctx context.Context, whereClause string, args []any) (int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM surplus_control_command_logs `+whereClause, args...).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

func surplusControlCommandLogWhere(filter SurplusControlCommandLogPageFilter) (string, []any) {
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
