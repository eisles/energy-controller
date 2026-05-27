package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

type WeatherSettingsRepository struct {
	db *sql.DB
}

func NewWeatherSettingsRepository(db *sql.DB) *WeatherSettingsRepository {
	return &WeatherSettingsRepository{db: db}
}

func (r *WeatherSettingsRepository) CurrentWeatherLocation(ctx context.Context) (domain.WeatherLocation, error) {
	var enabled int
	var latitude, longitude, pvCapacityKW, pvPerformanceRatio, dailyBaseLoadKWh, batteryCapacityKWh sql.NullFloat64
	var pvChargeCorrectionFactor, pvChargeCorrectionMinFactor, pvChargeCorrectionMaxFactor sql.NullFloat64
	var minimumReserveSoc, pvChargeCorrectionManual, pvChargeCorrectionMinSampleDays sql.NullInt64
	var timezone string
	var pvChargeCorrectionUpdatedAt sql.NullString
	err := r.db.QueryRowContext(ctx, `SELECT
		weather_forecast_enabled, weather_latitude, weather_longitude, weather_timezone,
		pv_capacity_kw, pv_performance_ratio, daily_base_load_kwh, battery_capacity_kwh, minimum_reserve_soc,
		pv_charge_correction_factor, pv_charge_correction_manual, pv_charge_correction_updated_at,
		pv_charge_correction_min_sample_days, pv_charge_correction_min_factor, pv_charge_correction_max_factor
		FROM settings WHERE id = 1`).Scan(
		&enabled,
		&latitude,
		&longitude,
		&timezone,
		&pvCapacityKW,
		&pvPerformanceRatio,
		&dailyBaseLoadKWh,
		&batteryCapacityKWh,
		&minimumReserveSoc,
		&pvChargeCorrectionFactor,
		&pvChargeCorrectionManual,
		&pvChargeCorrectionUpdatedAt,
		&pvChargeCorrectionMinSampleDays,
		&pvChargeCorrectionMinFactor,
		&pvChargeCorrectionMaxFactor,
	)
	if err != nil {
		return domain.WeatherLocation{}, err
	}
	return normalizeWeatherLocation(domain.WeatherLocation{
		Enabled:                         enabled != 0,
		Latitude:                        floatFromNull(latitude),
		Longitude:                       floatFromNull(longitude),
		Timezone:                        timezone,
		PVCapacityKW:                    floatFromNull(pvCapacityKW),
		PVPerformanceRatio:              floatFromNull(pvPerformanceRatio),
		DailyBaseLoadKWh:                floatFromNull(dailyBaseLoadKWh),
		BatteryCapacityKWh:              floatFromNull(batteryCapacityKWh),
		MinimumReserveSoc:               intFromNull(minimumReserveSoc),
		PVChargeCorrectionFactor:        floatFromNull(pvChargeCorrectionFactor),
		PVChargeCorrectionManual:        pvChargeCorrectionManual.Valid && pvChargeCorrectionManual.Int64 != 0,
		PVChargeCorrectionUpdatedAt:     stringFromNull(pvChargeCorrectionUpdatedAt),
		PVChargeCorrectionMinSampleDays: intFromNull(pvChargeCorrectionMinSampleDays),
		PVChargeCorrectionMinFactor:     floatFromNull(pvChargeCorrectionMinFactor),
		PVChargeCorrectionMaxFactor:     floatFromNull(pvChargeCorrectionMaxFactor),
	}), nil
}

func (r *WeatherSettingsRepository) UpdateWeatherLocation(ctx context.Context, location domain.WeatherLocation) error {
	location = normalizePVChargeCorrectionUpdatedAt(ctx, r, normalizeWeatherLocation(location))
	now := time.Now().Format(time.RFC3339Nano)
	_, err := r.db.ExecContext(ctx, `UPDATE settings SET
		weather_forecast_enabled = ?,
		weather_latitude = ?,
		weather_longitude = ?,
		weather_timezone = ?,
		pv_capacity_kw = ?,
		pv_performance_ratio = ?,
		daily_base_load_kwh = ?,
		battery_capacity_kwh = ?,
		minimum_reserve_soc = ?,
		pv_charge_correction_factor = ?,
		pv_charge_correction_manual = ?,
		pv_charge_correction_updated_at = ?,
		pv_charge_correction_min_sample_days = ?,
		pv_charge_correction_min_factor = ?,
		pv_charge_correction_max_factor = ?,
		updated_at = ?
		WHERE id = 1`,
		boolToInt(location.Enabled),
		location.Latitude,
		location.Longitude,
		normalizeTimezone(location.Timezone),
		location.PVCapacityKW,
		location.PVPerformanceRatio,
		location.DailyBaseLoadKWh,
		location.BatteryCapacityKWh,
		location.MinimumReserveSoc,
		location.PVChargeCorrectionFactor,
		boolToInt(location.PVChargeCorrectionManual),
		nullableEmptyString(location.PVChargeCorrectionUpdatedAt),
		location.PVChargeCorrectionMinSampleDays,
		location.PVChargeCorrectionMinFactor,
		location.PVChargeCorrectionMaxFactor,
		now,
	)
	return err
}

func normalizePVChargeCorrectionUpdatedAt(ctx context.Context, repository *WeatherSettingsRepository, location domain.WeatherLocation) domain.WeatherLocation {
	if !location.PVChargeCorrectionManual {
		location.PVChargeCorrectionUpdatedAt = ""
		return location
	}
	current, err := repository.CurrentWeatherLocation(ctx)
	if err != nil {
		if location.PVChargeCorrectionUpdatedAt == "" {
			location.PVChargeCorrectionUpdatedAt = time.Now().Format(time.RFC3339Nano)
		}
		return location
	}
	if !current.PVChargeCorrectionManual || current.PVChargeCorrectionFactor != location.PVChargeCorrectionFactor || location.PVChargeCorrectionUpdatedAt == "" {
		location.PVChargeCorrectionUpdatedAt = time.Now().Format(time.RFC3339Nano)
	}
	return location
}

func normalizeWeatherLocation(location domain.WeatherLocation) domain.WeatherLocation {
	location.Timezone = normalizeTimezone(location.Timezone)
	if location.PVPerformanceRatio <= 0 {
		location.PVPerformanceRatio = 0.75
	}
	if location.BatteryCapacityKWh <= 0 {
		location.BatteryCapacityKWh = 4.096
	}
	if location.MinimumReserveSoc <= 0 {
		location.MinimumReserveSoc = 30
	}
	if location.PVChargeCorrectionFactor <= 0 {
		location.PVChargeCorrectionFactor = 0.7
	}
	if location.PVChargeCorrectionMinSampleDays <= 0 {
		location.PVChargeCorrectionMinSampleDays = 7
	}
	if location.PVChargeCorrectionMinFactor <= 0 {
		location.PVChargeCorrectionMinFactor = 0.2
	}
	if location.PVChargeCorrectionMaxFactor <= 0 {
		location.PVChargeCorrectionMaxFactor = 0.9
	}
	if location.PVChargeCorrectionMaxFactor < location.PVChargeCorrectionMinFactor {
		location.PVChargeCorrectionMaxFactor = location.PVChargeCorrectionMinFactor
	}
	if location.PVChargeCorrectionFactor < location.PVChargeCorrectionMinFactor {
		location.PVChargeCorrectionFactor = location.PVChargeCorrectionMinFactor
	}
	if location.PVChargeCorrectionFactor > location.PVChargeCorrectionMaxFactor {
		location.PVChargeCorrectionFactor = location.PVChargeCorrectionMaxFactor
	}
	return location
}

func floatFromNull(value sql.NullFloat64) float64 {
	if !value.Valid {
		return 0
	}
	return value.Float64
}

func normalizeTimezone(value string) string {
	if value == "" {
		return "Asia/Tokyo"
	}
	return value
}

func stringFromNull(value sql.NullString) string {
	if !value.Valid {
		return ""
	}
	return value.String
}

func nullableEmptyString(value string) any {
	if value == "" {
		return nil
	}
	return value
}
