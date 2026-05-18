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
	limit = normalizeLimit(limit)
	rows, err := r.db.QueryContext(ctx, `SELECT
		id, measured_at, grid_w, import_w, export_w, battery_soc,
		battery_input_w, battery_output_w, ac_charge_limit_w, target_charge_w,
		actual_command_w, decision_reason, mode, command_sent,
		error_message, created_at
		FROM power_logs
		ORDER BY measured_at DESC, id DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	logs := make([]domain.PowerLog, 0, limit)
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

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return defaultLogLimit
	}
	if limit > maxLogLimit {
		return maxLogLimit
	}
	return limit
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
