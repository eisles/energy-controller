package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

type Pro3ACOutputEventRepository struct {
	db *sql.DB
}

func NewPro3ACOutputEventRepository(db *sql.DB) *Pro3ACOutputEventRepository {
	return &Pro3ACOutputEventRepository{db: db}
}

func (r *Pro3ACOutputEventRepository) InsertPro3ACOutputEvent(ctx context.Context, event domain.Pro3ACOutputEvent) error {
	_, err := r.db.ExecContext(ctx, `INSERT OR IGNORE INTO pro3_ac_output_event_logs (
		measured_at, event_type, output_power_off_memory, grid_w, import_w, export_w,
		battery_soc, battery_input_w, battery_output_w, ac_charge_limit_w,
		bms_max_cell_temp_c, bms_max_mos_temp_c, ac_out_freq_hz, ac_out_dsg_pow_max_w,
		previous_command_measured_at, previous_command_kind, previous_command_sent,
		previous_command_would_write, previous_command_target_ac_charge_w,
		previous_command_target_reserve_soc, previous_command_reason, message, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		event.MeasuredAt.Format(time.RFC3339Nano),
		event.EventType,
		boolToInt(event.OutputPowerOffMemory),
		event.GridW,
		event.ImportW,
		event.ExportW,
		event.BatterySoc,
		event.BatteryInputW,
		event.BatteryOutputW,
		event.ACChargeLimitW,
		nullableFloat(event.BMSMaxCellTempC),
		nullableFloat(event.BMSMaxMosTempC),
		nullableFloat(event.ACOutFreqHz),
		nullableInt(event.ACOutDsgPowMaxW),
		nullableTime(event.PreviousCommandMeasuredAt),
		event.PreviousCommandKind,
		boolToInt(event.PreviousCommandSent),
		boolToInt(event.PreviousCommandWouldWrite),
		nullableInt(event.PreviousCommandTargetACChargeW),
		nullableInt(event.PreviousCommandTargetReserveSoc),
		event.PreviousCommandReason,
		event.Message,
		event.CreatedAt.Format(time.RFC3339Nano),
	)
	return err
}

func (r *Pro3ACOutputEventRepository) LatestPro3ACOutputEvent(ctx context.Context) (*domain.Pro3ACOutputEvent, error) {
	return r.latestPro3ACOutputEvent(ctx, "")
}

func (r *Pro3ACOutputEventRepository) LatestPro3ACOutputEventByType(ctx context.Context, eventType string) (*domain.Pro3ACOutputEvent, error) {
	return r.latestPro3ACOutputEvent(ctx, "WHERE event_type = ?", eventType)
}

func (r *Pro3ACOutputEventRepository) latestPro3ACOutputEvent(ctx context.Context, whereClause string, args ...any) (*domain.Pro3ACOutputEvent, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT
		id, measured_at, event_type, output_power_off_memory, grid_w, import_w, export_w,
		battery_soc, battery_input_w, battery_output_w, ac_charge_limit_w,
		bms_max_cell_temp_c, bms_max_mos_temp_c, ac_out_freq_hz, ac_out_dsg_pow_max_w,
		previous_command_measured_at, previous_command_kind, previous_command_sent,
		previous_command_would_write, previous_command_target_ac_charge_w,
		previous_command_target_reserve_soc, previous_command_reason, message, created_at
		FROM pro3_ac_output_event_logs
		`+whereClause+`
		ORDER BY measured_at DESC, id DESC
		LIMIT 1`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events, err := scanPro3ACOutputEvents(rows, 1)
	if err != nil {
		return nil, err
	}
	if len(events) == 0 {
		return nil, nil
	}
	return &events[0], nil
}

func (r *Pro3ACOutputEventRepository) ListPro3ACOutputEventsPage(ctx context.Context, limit int, offset int) ([]domain.Pro3ACOutputEvent, int, error) {
	limit = normalizeLimit(limit)
	offset = normalizeOffset(offset)
	rows, err := r.db.QueryContext(ctx, `SELECT
		id, measured_at, event_type, output_power_off_memory, grid_w, import_w, export_w,
		battery_soc, battery_input_w, battery_output_w, ac_charge_limit_w,
		bms_max_cell_temp_c, bms_max_mos_temp_c, ac_out_freq_hz, ac_out_dsg_pow_max_w,
		previous_command_measured_at, previous_command_kind, previous_command_sent,
		previous_command_would_write, previous_command_target_ac_charge_w,
		previous_command_target_reserve_soc, previous_command_reason, message, created_at
		FROM pro3_ac_output_event_logs
		ORDER BY measured_at DESC, id DESC
		LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	events, err := scanPro3ACOutputEvents(rows, limit)
	if err != nil {
		return nil, 0, err
	}
	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM pro3_ac_output_event_logs`).Scan(&total); err != nil {
		return nil, 0, err
	}
	return events, total, nil
}

func scanPro3ACOutputEvents(rows *sql.Rows, capacity int) ([]domain.Pro3ACOutputEvent, error) {
	events := make([]domain.Pro3ACOutputEvent, 0, capacity)
	for rows.Next() {
		var event domain.Pro3ACOutputEvent
		var measuredAt, createdAt string
		var outputPowerOffMemory, previousCommandSent, previousCommandWouldWrite int
		var bmsMaxCellTempC, bmsMaxMosTempC, acOutFreqHz sql.NullFloat64
		var acOutDsgPowMaxW, previousCommandTargetACChargeW, previousCommandTargetReserveSoc sql.NullInt64
		var previousCommandMeasuredAt sql.NullString
		if err := rows.Scan(
			&event.ID,
			&measuredAt,
			&event.EventType,
			&outputPowerOffMemory,
			&event.GridW,
			&event.ImportW,
			&event.ExportW,
			&event.BatterySoc,
			&event.BatteryInputW,
			&event.BatteryOutputW,
			&event.ACChargeLimitW,
			&bmsMaxCellTempC,
			&bmsMaxMosTempC,
			&acOutFreqHz,
			&acOutDsgPowMaxW,
			&previousCommandMeasuredAt,
			&event.PreviousCommandKind,
			&previousCommandSent,
			&previousCommandWouldWrite,
			&previousCommandTargetACChargeW,
			&previousCommandTargetReserveSoc,
			&event.PreviousCommandReason,
			&event.Message,
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
		event.MeasuredAt = parsedMeasuredAt
		event.CreatedAt = parsedCreatedAt
		event.OutputPowerOffMemory = outputPowerOffMemory != 0
		event.BMSMaxCellTempC = floatPtrFromNull(bmsMaxCellTempC)
		event.BMSMaxMosTempC = floatPtrFromNull(bmsMaxMosTempC)
		event.ACOutFreqHz = floatPtrFromNull(acOutFreqHz)
		event.ACOutDsgPowMaxW = intPtrFromNull(acOutDsgPowMaxW)
		if previousCommandMeasuredAt.Valid && previousCommandMeasuredAt.String != "" {
			parsed, err := parseTime(previousCommandMeasuredAt.String)
			if err != nil {
				return nil, err
			}
			event.PreviousCommandMeasuredAt = &parsed
		}
		event.PreviousCommandSent = previousCommandSent != 0
		event.PreviousCommandWouldWrite = previousCommandWouldWrite != 0
		event.PreviousCommandTargetACChargeW = intPtrFromNull(previousCommandTargetACChargeW)
		event.PreviousCommandTargetReserveSoc = intPtrFromNull(previousCommandTargetReserveSoc)
		events = append(events, event)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func nullableTime(value *time.Time) any {
	if value == nil || value.IsZero() {
		return nil
	}
	return value.Format(time.RFC3339Nano)
}
