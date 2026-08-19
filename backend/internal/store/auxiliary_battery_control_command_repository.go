package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

type Delta3AuxControlCommandRepository struct {
	db *sql.DB
}

type Delta3AuxControlCommandLogPageFilter struct {
	From *time.Time
	To   *time.Time
}

func NewDelta3AuxControlCommandRepository(db *sql.DB) *Delta3AuxControlCommandRepository {
	return &Delta3AuxControlCommandRepository{db: db}
}

func (r *Delta3AuxControlCommandRepository) InsertDelta3AuxControlCommandLog(ctx context.Context, log domain.Delta3AuxControlCommandLog) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO delta3_aux_control_command_logs (
		device_id, measured_at, strategy_state, command_fingerprint, grid_w, import_w, export_w, residual_export_w,
		delta3_soc, previous_ac_charge_limit_w, target_ac_charge_limit_w, previous_backup_reserve_soc,
		target_backup_reserve_soc, command_sent, dry_run, would_write, should_adjust_ac_charge_limit,
		should_set_backup_reserve, should_disable_backup_reserve, suppressed_reason, decision_reason, error_message, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		log.DeviceID,
		log.MeasuredAt.Format(time.RFC3339Nano),
		log.StrategyState,
		log.CommandFingerprint,
		log.GridW,
		log.ImportW,
		log.ExportW,
		log.ResidualExportW,
		nullableInt(log.Delta3Soc),
		nullableInt(log.PreviousACChargeLimitW),
		nullableInt(log.TargetACChargeLimitW),
		nullableInt(log.PreviousBackupReserveSoc),
		nullableInt(log.TargetBackupReserveSoc),
		boolToInt(log.CommandSent),
		boolToInt(log.DryRun),
		boolToInt(log.WouldWrite),
		boolToInt(log.ShouldAdjustACChargeLimit),
		boolToInt(log.ShouldSetBackupReserve),
		boolToInt(log.ShouldDisableBackupReserve),
		log.SuppressedReason,
		log.DecisionReason,
		nullableString(log.ErrorMessage),
		log.CreatedAt.Format(time.RFC3339Nano),
	)
	return err
}

func (r *Delta3AuxControlCommandRepository) LatestDelta3AuxControlCommandLog(ctx context.Context) (*domain.Delta3AuxControlCommandLog, error) {
	return r.latestDelta3AuxControlCommandLog(ctx, "")
}

func (r *Delta3AuxControlCommandRepository) LatestDelta3AuxControlWriteCandidateLog(ctx context.Context) (*domain.Delta3AuxControlCommandLog, error) {
	return r.latestDelta3AuxControlCommandLog(ctx, "WHERE would_write = 1 OR command_sent = 1 OR (error_message IS NOT NULL AND error_message <> '')")
}

func (r *Delta3AuxControlCommandRepository) LatestDelta3AuxReserveCommandLog(ctx context.Context) (*domain.Delta3AuxControlCommandLog, error) {
	return r.latestDelta3AuxControlCommandLog(ctx, "WHERE command_sent = 1 AND target_backup_reserve_soc IS NOT NULL AND (error_message IS NULL OR error_message = '')")
}

func (r *Delta3AuxControlCommandRepository) LatestDelta3AuxControlWriteCandidateLogForDevice(ctx context.Context, deviceID int64) (*domain.Delta3AuxControlCommandLog, error) {
	// device_id=0 is the pre-migration history. Treat it as a conservative
	// fallback for every device until a device-bound command is recorded, so a
	// deployment cannot immediately repeat a recent legacy write.
	return r.latestDelta3AuxControlCommandLogForDevice(ctx, "WHERE (device_id = ? OR device_id = 0) AND (would_write = 1 OR command_sent = 1 OR (error_message IS NOT NULL AND error_message <> ''))", deviceID)
}

func (r *Delta3AuxControlCommandRepository) LatestDelta3AuxReserveCommandLogForDevice(ctx context.Context, deviceID int64) (*domain.Delta3AuxControlCommandLog, error) {
	return r.latestDelta3AuxControlCommandLogForDevice(ctx, "WHERE (device_id = ? OR device_id = 0) AND command_sent = 1 AND target_backup_reserve_soc IS NOT NULL AND (error_message IS NULL OR error_message = '')", deviceID)
}

func (r *Delta3AuxControlCommandRepository) latestDelta3AuxControlCommandLog(ctx context.Context, whereClause string) (*domain.Delta3AuxControlCommandLog, error) {
	return r.latestDelta3AuxControlCommandLogForDevice(ctx, whereClause)
}

func (r *Delta3AuxControlCommandRepository) latestDelta3AuxControlCommandLogForDevice(ctx context.Context, whereClause string, args ...any) (*domain.Delta3AuxControlCommandLog, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT
		id, device_id, measured_at, strategy_state, command_fingerprint, grid_w, import_w, export_w, residual_export_w,
		delta3_soc, previous_ac_charge_limit_w, target_ac_charge_limit_w, previous_backup_reserve_soc,
		target_backup_reserve_soc, command_sent, dry_run, would_write, should_adjust_ac_charge_limit,
		should_set_backup_reserve, should_disable_backup_reserve,
		suppressed_reason, decision_reason, error_message, created_at
		FROM delta3_aux_control_command_logs
		`+whereClause+`
		ORDER BY measured_at DESC, id DESC
		LIMIT 1`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	logs, err := scanDelta3AuxControlCommandLogs(rows, 1)
	if err != nil {
		return nil, err
	}
	if len(logs) == 0 {
		return nil, nil
	}
	return &logs[0], nil
}

func (r *Delta3AuxControlCommandRepository) ListDelta3AuxControlCommandLogsPage(ctx context.Context, limit int, offset int, filter Delta3AuxControlCommandLogPageFilter) ([]domain.Delta3AuxControlCommandLog, int, error) {
	limit = normalizeLimit(limit)
	offset = normalizeOffset(offset)
	whereClause, queryArgs := delta3AuxControlCommandLogWhere(filter)
	args := append(queryArgs, limit, offset)
	rows, err := r.db.QueryContext(ctx, `SELECT
		id, device_id, measured_at, strategy_state, command_fingerprint, grid_w, import_w, export_w, residual_export_w,
		delta3_soc, previous_ac_charge_limit_w, target_ac_charge_limit_w, previous_backup_reserve_soc,
		target_backup_reserve_soc, command_sent, dry_run, would_write, should_adjust_ac_charge_limit,
		should_set_backup_reserve, should_disable_backup_reserve,
		suppressed_reason, decision_reason, error_message, created_at
		FROM delta3_aux_control_command_logs
		`+whereClause+`
		ORDER BY measured_at DESC, id DESC
		LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	logs, err := scanDelta3AuxControlCommandLogs(rows, limit)
	if err != nil {
		return nil, 0, err
	}
	total, err := r.countDelta3AuxControlCommandLogs(ctx, whereClause, queryArgs)
	if err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}

func scanDelta3AuxControlCommandLogs(rows *sql.Rows, capacity int) ([]domain.Delta3AuxControlCommandLog, error) {
	logs := make([]domain.Delta3AuxControlCommandLog, 0, capacity)
	for rows.Next() {
		var log domain.Delta3AuxControlCommandLog
		var measuredAt, createdAt string
		var delta3Soc, previousACChargeLimitW, targetACChargeLimitW, previousBackupReserveSoc, targetBackupReserveSoc sql.NullInt64
		var commandSent, dryRun, wouldWrite, shouldAdjustACChargeLimit, shouldSetBackupReserve, shouldDisableBackupReserve int
		var errorMessage sql.NullString
		if err := rows.Scan(
			&log.ID,
			&log.DeviceID,
			&measuredAt,
			&log.StrategyState,
			&log.CommandFingerprint,
			&log.GridW,
			&log.ImportW,
			&log.ExportW,
			&log.ResidualExportW,
			&delta3Soc,
			&previousACChargeLimitW,
			&targetACChargeLimitW,
			&previousBackupReserveSoc,
			&targetBackupReserveSoc,
			&commandSent,
			&dryRun,
			&wouldWrite,
			&shouldAdjustACChargeLimit,
			&shouldSetBackupReserve,
			&shouldDisableBackupReserve,
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
		log.Delta3Soc = intPtrFromNull(delta3Soc)
		log.PreviousACChargeLimitW = intPtrFromNull(previousACChargeLimitW)
		log.TargetACChargeLimitW = intPtrFromNull(targetACChargeLimitW)
		log.PreviousBackupReserveSoc = intPtrFromNull(previousBackupReserveSoc)
		log.TargetBackupReserveSoc = intPtrFromNull(targetBackupReserveSoc)
		log.CommandSent = commandSent != 0
		log.DryRun = dryRun != 0
		log.WouldWrite = wouldWrite != 0
		log.ShouldAdjustACChargeLimit = shouldAdjustACChargeLimit != 0
		log.ShouldSetBackupReserve = shouldSetBackupReserve != 0
		log.ShouldDisableBackupReserve = shouldDisableBackupReserve != 0
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

func (r *Delta3AuxControlCommandRepository) countDelta3AuxControlCommandLogs(ctx context.Context, whereClause string, args []any) (int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM delta3_aux_control_command_logs `+whereClause, args...).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

func delta3AuxControlCommandLogWhere(filter Delta3AuxControlCommandLogPageFilter) (string, []any) {
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
