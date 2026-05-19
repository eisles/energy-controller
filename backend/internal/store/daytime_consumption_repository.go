package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

const (
	defaultDaytimeDays      = 7
	defaultDaytimeStartHour = 9
	defaultDaytimeEndHour   = 16
	maxDaytimeSampleGap     = 5 * time.Minute
)

type DaytimeConsumptionRepository struct {
	db *sql.DB
}

func NewDaytimeConsumptionRepository(db *sql.DB) *DaytimeConsumptionRepository {
	return &DaytimeConsumptionRepository{db: db}
}

func (r *DaytimeConsumptionRepository) EstimateDaytimeConsumption(ctx context.Context, now time.Time, days int) (domain.DaytimeConsumptionEstimate, error) {
	if days <= 0 {
		days = defaultDaytimeDays
	}
	since := now.AddDate(0, 0, -days)
	rows, err := r.db.QueryContext(ctx, `SELECT
		measured_at, import_w, export_w, battery_input_w, battery_output_w
		FROM power_logs
		WHERE julianday(measured_at) >= julianday(?)
		ORDER BY measured_at ASC, id ASC`, since.Format(time.RFC3339Nano))
	if err != nil {
		return domain.DaytimeConsumptionEstimate{}, err
	}
	defer rows.Close()

	samples := make([]daytimeSample, 0)
	for rows.Next() {
		var measuredAt string
		var sample daytimeSample
		var batteryInputW, batteryOutputW sql.NullInt64
		if err := rows.Scan(&measuredAt, &sample.importW, &sample.exportW, &batteryInputW, &batteryOutputW); err != nil {
			return domain.DaytimeConsumptionEstimate{}, err
		}
		parsed, err := parseTime(measuredAt)
		if err != nil {
			return domain.DaytimeConsumptionEstimate{}, err
		}
		sample.measuredAt = parsed
		sample.batteryInputW = intFromNull(batteryInputW)
		sample.batteryOutputW = intFromNull(batteryOutputW)
		samples = append(samples, sample)
	}
	if err := rows.Err(); err != nil {
		return domain.DaytimeConsumptionEstimate{}, err
	}
	return estimateDaytimeFromSamples(samples, days), nil
}

func estimateDaytimeFromSamples(samples []daytimeSample, days int) domain.DaytimeConsumptionEstimate {
	byDate := map[string]*domain.DailyDaytimeConsumptionEstimate{}
	for i := 0; i < len(samples)-1; i++ {
		current := samples[i]
		next := samples[i+1]
		if current.measuredAt.Hour() < defaultDaytimeStartHour || current.measuredAt.Hour() >= defaultDaytimeEndHour {
			continue
		}
		duration := next.measuredAt.Sub(current.measuredAt)
		if duration <= 0 || duration > maxDaytimeSampleGap {
			continue
		}
		date := current.measuredAt.Format("2006-01-02")
		day := byDate[date]
		if day == nil {
			day = &domain.DailyDaytimeConsumptionEstimate{Date: date}
			byDate[date] = day
		}
		hours := duration.Hours()
		day.SampleCount++
		day.ImportKWh += wattsToKWh(current.importW, hours)
		day.ExportKWh += wattsToKWh(current.exportW, hours)
		day.BatteryChargeKWh += wattsToKWh(current.batteryInputW, hours)
		day.BatteryDischargeKWh += wattsToKWh(current.batteryOutputW, hours)
	}

	estimate := domain.DaytimeConsumptionEstimate{
		Days:      days,
		StartHour: defaultDaytimeStartHour,
		EndHour:   defaultDaytimeEndHour,
		Daily:     make([]domain.DailyDaytimeConsumptionEstimate, 0, len(byDate)),
	}
	for _, day := range byDate {
		day.EstimatedLoadKWh = day.ImportKWh + day.BatteryDischargeKWh - day.BatteryChargeKWh
		if day.EstimatedLoadKWh < 0 {
			day.EstimatedLoadKWh = 0
		}
		estimate.SampleCount += day.SampleCount
		estimate.AverageImportKWh += day.ImportKWh
		estimate.AverageExportKWh += day.ExportKWh
		estimate.AverageBatteryChargeKWh += day.BatteryChargeKWh
		estimate.AverageBatteryDischargeKWh += day.BatteryDischargeKWh
		estimate.AverageEstimatedLoadKWh += day.EstimatedLoadKWh
		estimate.Daily = append(estimate.Daily, *day)
	}
	if len(estimate.Daily) > 0 {
		count := float64(len(estimate.Daily))
		estimate.AverageImportKWh /= count
		estimate.AverageExportKWh /= count
		estimate.AverageBatteryChargeKWh /= count
		estimate.AverageBatteryDischargeKWh /= count
		estimate.AverageEstimatedLoadKWh /= count
	}
	estimate.SuggestedDailyBaseLoadKWh = estimate.AverageEstimatedLoadKWh
	return estimate
}

func wattsToKWh(watts int, hours float64) float64 {
	return float64(watts) * hours / 1000
}

type daytimeSample struct {
	measuredAt     time.Time
	importW        int
	exportW        int
	batteryInputW  int
	batteryOutputW int
}
