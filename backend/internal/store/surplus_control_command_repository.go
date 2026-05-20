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

func (r *SurplusControlCommandRepository) InsertSurplusControlCommandLog(ctx context.Context, status domain.Status) error {
	if status.SurplusPlan == nil {
		return nil
	}
	log := surplusControlCommandLogFromStatus(status)
	_, err := r.db.ExecContext(ctx, `INSERT INTO surplus_control_command_logs (
		measured_at, strategy_state, grid_w, import_w, export_w,
		battery_soc, battery_input_w, battery_output_w,
		previous_ac_charge_limit_w, target_ac_charge_limit_w,
		previous_backup_reserve_soc, target_backup_reserve_soc,
		command_sent, dry_run, would_write, suppressed_reason, decision_reason,
		error_message, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		log.MeasuredAt.Format(time.RFC3339Nano),
		log.StrategyState,
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
		log.SuppressedReason,
		log.DecisionReason,
		nullableString(log.ErrorMessage),
		log.CreatedAt.Format(time.RFC3339Nano),
	)
	return err
}

func (r *SurplusControlCommandRepository) ListSurplusControlCommandLogsPage(ctx context.Context, limit int, offset int, filter SurplusControlCommandLogPageFilter) ([]domain.SurplusControlCommandLog, int, error) {
	limit = normalizeLimit(limit)
	offset = normalizeOffset(offset)
	whereClause, queryArgs := surplusControlCommandLogWhere(filter)
	args := append(queryArgs, limit, offset)
	rows, err := r.db.QueryContext(ctx, `SELECT
		id, measured_at, strategy_state, grid_w, import_w, export_w,
		battery_soc, battery_input_w, battery_output_w,
		previous_ac_charge_limit_w, target_ac_charge_limit_w,
		previous_backup_reserve_soc, target_backup_reserve_soc,
		command_sent, dry_run, would_write, suppressed_reason, decision_reason,
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

func surplusControlCommandLogFromStatus(status domain.Status) domain.SurplusControlCommandLog {
	plan := status.SurplusPlan
	log := domain.SurplusControlCommandLog{
		MeasuredAt:               status.UpdatedAt,
		StrategyState:            plan.StrategyState,
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
		WouldWrite:               plan.WouldWrite,
		SuppressedReason:         surplusSuppressedReason(status),
		DecisionReason:           firstNonEmpty(plan.ActionSummary, plan.Reason),
		ErrorMessage:             status.LastError,
		CreatedAt:                status.UpdatedAt,
	}
	if plan.ShouldAdjustACChargeLimit {
		log.TargetACChargeLimitW = intPtr(plan.RecommendedACChargeLimitW)
	}
	if plan.ShouldRaiseBackupReserve || plan.ShouldLowerBackupReserve || plan.ShouldAlignBackupReserve {
		log.TargetBackupReserveSoc = plan.RecommendedBackupReserveSoc
	}
	return log
}

func scanSurplusControlCommandLogs(rows *sql.Rows, capacity int) ([]domain.SurplusControlCommandLog, error) {
	logs := make([]domain.SurplusControlCommandLog, 0, capacity)
	for rows.Next() {
		var log domain.SurplusControlCommandLog
		var measuredAt, createdAt string
		var previousACChargeLimitW, targetACChargeLimitW, previousBackupReserveSoc, targetBackupReserveSoc sql.NullInt64
		var commandSent, dryRun, wouldWrite int
		var errorMessage sql.NullString
		if err := rows.Scan(
			&log.ID,
			&measuredAt,
			&log.StrategyState,
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

func surplusSuppressedReason(status domain.Status) string {
	if status.SurplusPlan == nil {
		return ""
	}
	if status.SurplusPlan.WouldWrite {
		return ""
	}
	if status.SurplusPlan.ActionSummary == "" {
		return "no command candidate"
	}
	if status.SurplusPlan.Reason != "" {
		return status.SurplusPlan.Reason
	}
	return "dry-run"
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
