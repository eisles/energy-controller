package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

const defaultLogLimit = 100
const maxLogLimit = 500

type LogRepository struct {
	db *sql.DB
}

type LogPageFilter struct {
	Query string
	From  *time.Time
	To    *time.Time
}

func NewLogRepository(db *sql.DB) *LogRepository {
	return &LogRepository{db: db}
}

func (r *LogRepository) InsertPowerLog(ctx context.Context, log domain.PowerLog) error {
	if log.CreatedAt.IsZero() {
		log.CreatedAt = log.MeasuredAt
	}
	_, err := r.db.ExecContext(ctx, `INSERT INTO power_logs (
		measured_at, grid_w, import_w, export_w, battery_soc,
		battery_input_w, battery_output_w, ac_charge_limit_w, target_charge_w,
		actual_command_w, decision_reason, mode, command_sent,
		error_message, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		log.MeasuredAt.Format(time.RFC3339Nano),
		log.GridW,
		log.ImportW,
		log.ExportW,
		nullableInt(log.BatterySoc),
		nullableInt(log.BatteryInputW),
		nullableInt(log.BatteryOutputW),
		nullableInt(log.ACChargeLimitW),
		log.TargetChargeW,
		nullableInt(log.ActualCommandW),
		log.DecisionReason,
		log.Mode,
		boolToInt(log.CommandSent),
		nullableString(log.ErrorMessage),
		log.CreatedAt.Format(time.RFC3339Nano),
	)
	return err
}

func (r *LogRepository) ListPowerLogs(ctx context.Context, limit int) ([]domain.PowerLog, error) {
	logs, _, err := r.ListPowerLogsPage(ctx, limit, 0, LogPageFilter{})
	return logs, err
}

func (r *LogRepository) ListPowerLogsPage(ctx context.Context, limit int, offset int, filter LogPageFilter) ([]domain.PowerLog, int, error) {
	limit = normalizeLimit(limit)
	offset = normalizeOffset(offset)
	whereClause, queryArgs := logSearchWhere(filter)
	args := append(queryArgs, limit, offset)
	rows, err := r.db.QueryContext(ctx, `SELECT
		id, measured_at, grid_w, import_w, export_w, battery_soc,
		battery_input_w, battery_output_w, ac_charge_limit_w, target_charge_w,
		actual_command_w, decision_reason, mode, command_sent,
		error_message, created_at
		FROM power_logs
		`+whereClause+`
		ORDER BY measured_at DESC, id DESC
		LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	logs, err := scanPowerLogs(rows, limit)
	if err != nil {
		return nil, 0, err
	}
	total, err := r.countPowerLogs(ctx, whereClause, queryArgs)
	if err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}

func (r *LogRepository) ListPowerLogsSince(ctx context.Context, since time.Time, limit int) ([]domain.PowerLog, error) {
	var rows *sql.Rows
	var err error
	args := []any{since.Format(time.RFC3339Nano)}
	limitClause := ""
	if limit > 0 {
		limit = normalizeLimit(limit)
		limitClause = " LIMIT ?"
		args = append(args, limit)
	}

	rows, err = r.db.QueryContext(ctx, `SELECT
		id, measured_at, grid_w, import_w, export_w, battery_soc,
		battery_input_w, battery_output_w, ac_charge_limit_w, target_charge_w,
		actual_command_w, decision_reason, mode, command_sent,
		error_message, created_at
		FROM power_logs
		WHERE julianday(measured_at) >= julianday(?)
		ORDER BY measured_at DESC, id DESC`+limitClause, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanPowerLogs(rows, normalizeLogCapacity(limit))
}

func scanPowerLogs(rows *sql.Rows, capacity int) ([]domain.PowerLog, error) {
	logs := make([]domain.PowerLog, 0, capacity)
	for rows.Next() {
		var log domain.PowerLog
		var measuredAt, createdAt string
		var batterySoc, batteryInputW, batteryOutputW, acChargeLimitW, actualCommandW sql.NullInt64
		var commandSent int
		var errorMessage sql.NullString
		if err := rows.Scan(
			&log.ID,
			&measuredAt,
			&log.GridW,
			&log.ImportW,
			&log.ExportW,
			&batterySoc,
			&batteryInputW,
			&batteryOutputW,
			&acChargeLimitW,
			&log.TargetChargeW,
			&actualCommandW,
			&log.DecisionReason,
			&log.Mode,
			&commandSent,
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
		log.BatterySoc = intPtrFromNull(batterySoc)
		log.BatteryInputW = intPtrFromNull(batteryInputW)
		log.BatteryOutputW = intPtrFromNull(batteryOutputW)
		log.ACChargeLimitW = intPtrFromNull(acChargeLimitW)
		log.ActualCommandW = intPtrFromNull(actualCommandW)
		if errorMessage.Valid {
			log.ErrorMessage = &errorMessage.String
		}
		log.CommandSent = commandSent != 0
		logs = append(logs, log)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return logs, nil
}

func normalizeLogCapacity(limit int) int {
	if limit <= 0 {
		return defaultLogLimit
	}
	return normalizeLimit(limit)
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return defaultLogLimit
	}
	if limit > maxLogLimit {
		return maxLogLimit
	}
	return limit
}

func normalizeOffset(offset int) int {
	if offset < 0 {
		return 0
	}
	return offset
}

func (r *LogRepository) countPowerLogs(ctx context.Context, whereClause string, args []any) (int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM power_logs `+whereClause, args...).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

func logSearchWhere(filter LogPageFilter) (string, []any) {
	clauses := make([]string, 0, 3)
	args := make([]any, 0, 16)
	if filter.Query != "" {
		pattern := "%" + filter.Query + "%"
		searchClauses := []string{
			"measured_at LIKE ?",
			"CAST(grid_w AS TEXT) LIKE ?",
			"CAST(import_w AS TEXT) LIKE ?",
			"CAST(export_w AS TEXT) LIKE ?",
			"CAST(battery_soc AS TEXT) LIKE ?",
			"CAST(battery_input_w AS TEXT) LIKE ?",
			"CAST(battery_output_w AS TEXT) LIKE ?",
			"CAST(ac_charge_limit_w AS TEXT) LIKE ?",
			"CAST(target_charge_w AS TEXT) LIKE ?",
			"CAST(actual_command_w AS TEXT) LIKE ?",
			"decision_reason LIKE ?",
			"mode LIKE ?",
			"CAST(command_sent AS TEXT) LIKE ?",
			"error_message LIKE ?",
		}
		clauses = append(clauses, "("+joinWithOr(searchClauses)+")")
		for range searchClauses {
			args = append(args, pattern)
		}
	}
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

func joinWithOr(values []string) string {
	return joinWith(values, " OR ")
}

func joinWithAnd(values []string) string {
	return joinWith(values, " AND ")
}

func joinWith(values []string, separator string) string {
	if len(values) == 0 {
		return ""
	}
	result := values[0]
	for _, value := range values[1:] {
		result += separator + value
	}
	return result
}

func intPtrFromNull(value sql.NullInt64) *int {
	if !value.Valid {
		return nil
	}
	result := int(value.Int64)
	return &result
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func nullableInt(value *int) any {
	if value == nil {
		return nil
	}
	return *value
}
