package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

type EnergyMeterRepository struct {
	db *sql.DB
}

type EnergyMeterLogPageFilter struct {
	From *time.Time
	To   *time.Time
}

func NewEnergyMeterRepository(db *sql.DB) *EnergyMeterRepository {
	return &EnergyMeterRepository{db: db}
}

func (r *EnergyMeterRepository) InsertEnergyMeterReading(ctx context.Context, reading domain.EnergyMeterReading) error {
	previous, err := r.latestEnergyMeterLog(ctx)
	if err != nil {
		return err
	}
	log := domain.EnergyMeterLog{
		MeasuredAt:           reading.MeasuredAt,
		ImportCumulativeKWh:  reading.ImportCumulativeKWh,
		ExportCumulativeKWh:  reading.ExportCumulativeKWh,
		Coefficient:          reading.Coefficient,
		CumulativeUnit:       reading.CumulativeUnit,
		RawImportCumulative:  reading.RawImportCumulative,
		RawExportCumulative:  reading.RawExportCumulative,
		ImportValueUpdatedAt: reading.ImportValueUpdatedAt,
		ExportValueUpdatedAt: reading.ExportValueUpdatedAt,
		CreatedAt:            reading.MeasuredAt,
	}
	if previous != nil {
		if delta := reading.ImportCumulativeKWh - previous.ImportCumulativeKWh; delta >= 0 {
			log.ImportDeltaKWh = &delta
		}
		if delta := reading.ExportCumulativeKWh - previous.ExportCumulativeKWh; delta >= 0 {
			log.ExportDeltaKWh = &delta
		}
	}
	_, err = r.db.ExecContext(ctx, `INSERT OR IGNORE INTO energy_meter_logs (
		measured_at, import_cumulative_kwh, export_cumulative_kwh,
		import_delta_kwh, export_delta_kwh, coefficient, cumulative_unit,
		raw_import_cumulative, raw_export_cumulative,
		import_value_updated_at, export_value_updated_at, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		log.MeasuredAt.Format(time.RFC3339Nano),
		log.ImportCumulativeKWh,
		log.ExportCumulativeKWh,
		nullableFloat(log.ImportDeltaKWh),
		nullableFloat(log.ExportDeltaKWh),
		log.Coefficient,
		log.CumulativeUnit,
		log.RawImportCumulative,
		log.RawExportCumulative,
		log.ImportValueUpdatedAt.Format(time.RFC3339Nano),
		log.ExportValueUpdatedAt.Format(time.RFC3339Nano),
		log.CreatedAt.Format(time.RFC3339Nano),
	)
	return err
}

func (r *EnergyMeterRepository) latestEnergyMeterLog(ctx context.Context) (*domain.EnergyMeterLog, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT
		id, measured_at, import_cumulative_kwh, export_cumulative_kwh,
		import_delta_kwh, export_delta_kwh, coefficient, cumulative_unit,
		raw_import_cumulative, raw_export_cumulative,
		import_value_updated_at, export_value_updated_at, created_at
		FROM energy_meter_logs
		ORDER BY measured_at DESC, id DESC
		LIMIT 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	logs, err := scanEnergyMeterLogs(rows, 1)
	if err != nil {
		return nil, err
	}
	if len(logs) == 0 {
		return nil, nil
	}
	return &logs[0], nil
}

func (r *EnergyMeterRepository) ListEnergyMeterLogs(ctx context.Context, limit int) ([]domain.EnergyMeterLog, error) {
	logs, _, err := r.ListEnergyMeterLogsPage(ctx, limit, 0, EnergyMeterLogPageFilter{})
	return logs, err
}

func (r *EnergyMeterRepository) ListEnergyMeterLogsPage(ctx context.Context, limit int, offset int, filter EnergyMeterLogPageFilter) ([]domain.EnergyMeterLog, int, error) {
	limit = normalizeLimit(limit)
	offset = normalizeOffset(offset)
	whereClause, args := energyMeterLogWhere(filter)
	queryArgs := append(args, limit, offset)
	rows, err := r.db.QueryContext(ctx, `SELECT
		id, measured_at, import_cumulative_kwh, export_cumulative_kwh,
		import_delta_kwh, export_delta_kwh, coefficient, cumulative_unit,
		raw_import_cumulative, raw_export_cumulative,
		import_value_updated_at, export_value_updated_at, created_at
		FROM energy_meter_logs
		`+whereClause+`
		ORDER BY measured_at DESC, id DESC
		LIMIT ? OFFSET ?`, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	logs, err := scanEnergyMeterLogs(rows, limit)
	if err != nil {
		return nil, 0, err
	}
	total, err := r.countEnergyMeterLogs(ctx, whereClause, args)
	if err != nil {
		return nil, 0, err
	}
	return logs, total, nil
}

func scanEnergyMeterLogs(rows *sql.Rows, capacity int) ([]domain.EnergyMeterLog, error) {
	logs := make([]domain.EnergyMeterLog, 0, capacity)
	for rows.Next() {
		var log domain.EnergyMeterLog
		var measuredAt, importUpdatedAt, exportUpdatedAt, createdAt string
		var importDelta, exportDelta sql.NullFloat64
		if err := rows.Scan(
			&log.ID,
			&measuredAt,
			&log.ImportCumulativeKWh,
			&log.ExportCumulativeKWh,
			&importDelta,
			&exportDelta,
			&log.Coefficient,
			&log.CumulativeUnit,
			&log.RawImportCumulative,
			&log.RawExportCumulative,
			&importUpdatedAt,
			&exportUpdatedAt,
			&createdAt,
		); err != nil {
			return nil, err
		}
		parsedMeasuredAt, err := parseTime(measuredAt)
		if err != nil {
			return nil, err
		}
		parsedImportUpdatedAt, err := parseTime(importUpdatedAt)
		if err != nil {
			return nil, err
		}
		parsedExportUpdatedAt, err := parseTime(exportUpdatedAt)
		if err != nil {
			return nil, err
		}
		parsedCreatedAt, err := parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		log.MeasuredAt = parsedMeasuredAt
		log.ImportValueUpdatedAt = parsedImportUpdatedAt
		log.ExportValueUpdatedAt = parsedExportUpdatedAt
		log.CreatedAt = parsedCreatedAt
		log.ImportDeltaKWh = floatPtrFromNull(importDelta)
		log.ExportDeltaKWh = floatPtrFromNull(exportDelta)
		logs = append(logs, log)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return logs, nil
}

func floatPtrFromNull(value sql.NullFloat64) *float64 {
	if !value.Valid {
		return nil
	}
	result := value.Float64
	return &result
}

func nullableFloat(value *float64) any {
	if value == nil {
		return nil
	}
	return *value
}

func (r *EnergyMeterRepository) countEnergyMeterLogs(ctx context.Context, whereClause string, args []any) (int, error) {
	var total int
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM energy_meter_logs `+whereClause, args...).Scan(&total); err != nil {
		return 0, err
	}
	return total, nil
}

func energyMeterLogWhere(filter EnergyMeterLogPageFilter) (string, []any) {
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
