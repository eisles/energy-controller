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
	var minimumReserveSoc sql.NullInt64
	var timezone string
	err := r.db.QueryRowContext(ctx, `SELECT
		weather_forecast_enabled, weather_latitude, weather_longitude, weather_timezone,
		pv_capacity_kw, pv_performance_ratio, daily_base_load_kwh, battery_capacity_kwh, minimum_reserve_soc
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
	)
	if err != nil {
		return domain.WeatherLocation{}, err
	}
	return normalizeWeatherLocation(domain.WeatherLocation{
		Enabled:            enabled != 0,
		Latitude:           floatFromNull(latitude),
		Longitude:          floatFromNull(longitude),
		Timezone:           timezone,
		PVCapacityKW:       floatFromNull(pvCapacityKW),
		PVPerformanceRatio: floatFromNull(pvPerformanceRatio),
		DailyBaseLoadKWh:   floatFromNull(dailyBaseLoadKWh),
		BatteryCapacityKWh: floatFromNull(batteryCapacityKWh),
		MinimumReserveSoc:  intFromNull(minimumReserveSoc),
	}), nil
}

func (r *WeatherSettingsRepository) UpdateWeatherLocation(ctx context.Context, location domain.WeatherLocation) error {
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
		time.Now().Format(time.RFC3339Nano),
	)
	return err
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
