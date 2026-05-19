package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

const (
	defaultEcoFlowLoadDays       = 7
	defaultEcoFlowDayStartHour   = 9
	defaultEcoFlowDayEndHour     = 16
	defaultEcoFlowNightStartHour = 23
	defaultEcoFlowNightEndHour   = 7
)

type EcoFlowLoadRepository struct {
	db       *sql.DB
	location *time.Location
}

func NewEcoFlowLoadRepository(db *sql.DB) *EcoFlowLoadRepository {
	return NewEcoFlowLoadRepositoryWithTimezone(db, "Asia/Tokyo")
}

func NewEcoFlowLoadRepositoryWithTimezone(db *sql.DB, timezone string) *EcoFlowLoadRepository {
	return &EcoFlowLoadRepository{db: db, location: loadEcoFlowLoadLocation(timezone)}
}

func (r *EcoFlowLoadRepository) EstimateEcoFlowLoad(ctx context.Context, now time.Time, days int) (domain.EcoFlowLoadEstimate, error) {
	if days <= 0 {
		days = defaultEcoFlowLoadDays
	}
	since := now.AddDate(0, 0, -days)
	rows, err := r.db.QueryContext(ctx, `SELECT
		measured_at, battery_input_w, battery_output_w
		FROM power_logs
		WHERE julianday(measured_at) >= julianday(?)
		ORDER BY measured_at ASC, id ASC`, since.Format(time.RFC3339Nano))
	if err != nil {
		return domain.EcoFlowLoadEstimate{}, err
	}
	defer rows.Close()

	samples := make([]ecoflowLoadSample, 0)
	for rows.Next() {
		var measuredAt string
		var batteryInputW, batteryOutputW sql.NullInt64
		if err := rows.Scan(&measuredAt, &batteryInputW, &batteryOutputW); err != nil {
			return domain.EcoFlowLoadEstimate{}, err
		}
		parsed, err := parseTime(measuredAt)
		if err != nil {
			return domain.EcoFlowLoadEstimate{}, err
		}
		samples = append(samples, ecoflowLoadSample{
			measuredAt:     parsed,
			batteryInputW:  intFromNull(batteryInputW),
			batteryOutputW: intFromNull(batteryOutputW),
		})
	}
	if err := rows.Err(); err != nil {
		return domain.EcoFlowLoadEstimate{}, err
	}
	return estimateEcoFlowLoadFromSamples(samples, now, days, r.location), nil
}

func estimateEcoFlowLoadFromSamples(samples []ecoflowLoadSample, now time.Time, days int, location *time.Location) domain.EcoFlowLoadEstimate {
	if location == nil {
		location = loadEcoFlowLoadLocation("")
	}
	nowLocal := now.In(location)
	byDate := map[string]*domain.DailyEcoFlowLoadEstimate{}
	for i := 0; i < len(samples)-1; i++ {
		current := samples[i]
		next := samples[i+1]
		duration := next.measuredAt.Sub(current.measuredAt)
		if duration <= 0 || duration > maxDaytimeSampleGap {
			continue
		}
		localMeasuredAt := current.measuredAt.In(location)
		date := localMeasuredAt.Format("2006-01-02")
		day := byDate[date]
		if day == nil {
			day = &domain.DailyEcoFlowLoadEstimate{Date: date}
			byDate[date] = day
		}
		hours := duration.Hours()
		outputKWh := wattsToKWh(current.batteryOutputW, hours)
		inputKWh := wattsToKWh(current.batteryInputW, hours)
		day.SampleCount++
		day.DailyOutputKWh += outputKWh
		if isEcoFlowDaytime(localMeasuredAt.Hour()) {
			day.DaytimeOutputKWh += outputKWh
			day.DaytimeChargeKWh += inputKWh
			day.DaytimeSampleCount++
		} else if isEcoFlowNight(localMeasuredAt.Hour()) {
			day.NightOutputKWh += outputKWh
		} else {
			day.ShoulderOutputKWh += outputKWh
		}
	}

	estimate := domain.EcoFlowLoadEstimate{
		Days:             days,
		DaytimeStartHour: defaultEcoFlowDayStartHour,
		DaytimeEndHour:   defaultEcoFlowDayEndHour,
		NightStartHour:   defaultEcoFlowNightStartHour,
		NightEndHour:     defaultEcoFlowNightEndHour,
		Daily:            make([]domain.DailyEcoFlowLoadEstimate, 0, len(byDate)),
		Note:             "EcoFlow batteryOutputW is treated as the specific circuit load supplied by EcoFlow. Nature Remo E import/export remains whole-home grid flow.",
	}
	for _, day := range byDate {
		day.DaytimeNetLoadKWh = day.DaytimeOutputKWh
		if day.DaytimeNetLoadKWh < 0 {
			day.DaytimeNetLoadKWh = 0
		}
		estimate.SampleCount += day.SampleCount
		day.DaytimeComplete = isEcoFlowDaytimeComplete(day.Date, nowLocal, location)
		if day.DaytimeSampleCount > 0 {
			estimate.DaytimeSampleDays++
		}
		if day.DaytimeComplete && day.DaytimeSampleCount > 0 {
			estimate.CompleteDaytimeSampleDays++
			estimate.AverageDaytimeOutputKWh += day.DaytimeOutputKWh
			estimate.AverageDaytimeChargeKWh += day.DaytimeChargeKWh
		}
		estimate.AverageShoulderOutputKWh += day.ShoulderOutputKWh
		estimate.AverageNightOutputKWh += day.NightOutputKWh
		estimate.AverageDailyOutputKWh += day.DailyOutputKWh
		estimate.Daily = append(estimate.Daily, *day)
	}
	if len(estimate.Daily) > 0 {
		if estimate.CompleteDaytimeSampleDays > 0 {
			count := float64(estimate.CompleteDaytimeSampleDays)
			estimate.AverageDaytimeOutputKWh /= count
			estimate.AverageDaytimeChargeKWh /= count
		}
		count := float64(len(estimate.Daily))
		estimate.AverageShoulderOutputKWh /= count
		estimate.AverageNightOutputKWh /= count
		estimate.AverageDailyOutputKWh /= count
	}
	estimate.SuggestedDaytimeBaseLoadKWh = estimate.AverageDaytimeOutputKWh
	estimate.SuggestedOvernightReserveKWh = estimate.AverageNightOutputKWh
	return estimate
}

func isEcoFlowDaytime(hour int) bool {
	return hour >= defaultEcoFlowDayStartHour && hour < defaultEcoFlowDayEndHour
}

func isEcoFlowDaytimeComplete(date string, now time.Time, location *time.Location) bool {
	day, err := time.ParseInLocation("2006-01-02", date, location)
	if err != nil {
		return false
	}
	daytimeEnd := time.Date(day.Year(), day.Month(), day.Day(), defaultEcoFlowDayEndHour, 0, 0, 0, location)
	return !now.Before(daytimeEnd)
}

func isEcoFlowNight(hour int) bool {
	return hour >= defaultEcoFlowNightStartHour || hour < defaultEcoFlowNightEndHour
}

func loadEcoFlowLoadLocation(timezone string) *time.Location {
	if timezone == "" {
		timezone = "Asia/Tokyo"
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return time.FixedZone("JST", 9*60*60)
	}
	return location
}

type ecoflowLoadSample struct {
	measuredAt     time.Time
	batteryInputW  int
	batteryOutputW int
}
