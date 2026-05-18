package store

import (
	"context"
	"database/sql"
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
	var batterySoc, batteryInputW, batteryOutputW sql.NullInt64
	var lastError sql.NullString

	err := r.db.QueryRowContext(ctx, `SELECT
		grid_w, import_w, export_w, battery_soc, battery_input_w,
		battery_output_w, target_charge_w, state, mode,
		last_decision_reason, last_error, updated_at
		FROM current_status WHERE id = 1`,
	).Scan(
		&status.GridW,
		&status.ImportW,
		&status.ExportW,
		&batterySoc,
		&batteryInputW,
		&batteryOutputW,
		&status.TargetChargeW,
		&status.State,
		&status.Mode,
		&status.LastDecisionReason,
		&lastError,
		&updatedAt,
	)
	if err != nil {
		return domain.Status{}, err
	}

	status.BatterySoc = intFromNull(batterySoc)
	status.BatteryInputW = intFromNull(batteryInputW)
	status.BatteryOutputW = intFromNull(batteryOutputW)
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
		battery_output_w, target_charge_w, state, mode, last_decision_reason,
		last_error, updated_at
	) VALUES (1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		grid_w = excluded.grid_w,
		import_w = excluded.import_w,
		export_w = excluded.export_w,
		battery_soc = excluded.battery_soc,
		battery_input_w = excluded.battery_input_w,
		battery_output_w = excluded.battery_output_w,
		target_charge_w = excluded.target_charge_w,
		state = excluded.state,
		mode = excluded.mode,
		last_decision_reason = excluded.last_decision_reason,
		last_error = excluded.last_error,
		updated_at = excluded.updated_at`,
		status.GridW,
		status.ImportW,
		status.ExportW,
		status.BatterySoc,
		status.BatteryInputW,
		status.BatteryOutputW,
		status.TargetChargeW,
		status.State,
		status.Mode,
		status.LastDecisionReason,
		nullableString(status.LastError),
		status.UpdatedAt.Format(time.RFC3339Nano),
	)
	return err
}

func intFromNull(value sql.NullInt64) int {
	if !value.Valid {
		return 0
	}
	return int(value.Int64)
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
