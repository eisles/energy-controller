package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

type WeatherSettingsStore interface {
	CurrentWeatherLocation(ctx context.Context) (domain.WeatherLocation, error)
	UpdateWeatherLocation(ctx context.Context, location domain.WeatherLocation) error
}

func getWeatherLocationHandler(store WeatherSettingsStore, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		location, err := store.CurrentWeatherLocation(r.Context())
		if err != nil {
			logger.Error("failed to get weather location", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get weather location"})
			return
		}
		writeJSON(w, http.StatusOK, location)
	}
}

func putWeatherLocationHandler(store WeatherSettingsStore, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var location domain.WeatherLocation
		if err := json.NewDecoder(r.Body).Decode(&location); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid weather location payload"})
			return
		}
		location.Timezone = normalizeWeatherTimezone(location.Timezone)
		location = normalizeWeatherSettings(location)
		if !validWeatherLocation(location) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "weather location or solar settings are out of range"})
			return
		}
		if err := store.UpdateWeatherLocation(r.Context(), location); err != nil {
			logger.Error("failed to update weather location", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update weather location"})
			return
		}
		writeJSON(w, http.StatusOK, location)
	}
}

func validWeatherLocation(location domain.WeatherLocation) bool {
	return location.Latitude >= -90 &&
		location.Latitude <= 90 &&
		location.Longitude >= -180 &&
		location.Longitude <= 180 &&
		location.PVCapacityKW >= 0 &&
		location.PVCapacityKW <= 100 &&
		location.PVPerformanceRatio > 0 &&
		location.PVPerformanceRatio <= 1 &&
		location.DailyBaseLoadKWh >= 0 &&
		location.DailyBaseLoadKWh <= 200 &&
		location.BatteryCapacityKWh > 0 &&
		location.BatteryCapacityKWh <= 200 &&
		location.MinimumReserveSoc >= 0 &&
		location.MinimumReserveSoc <= 100 &&
		location.PVChargeCorrectionFactor > 0 &&
		location.PVChargeCorrectionFactor <= 1 &&
		location.PVChargeCorrectionMinSampleDays >= 1 &&
		location.PVChargeCorrectionMinSampleDays <= 365 &&
		location.PVChargeCorrectionMinFactor > 0 &&
		location.PVChargeCorrectionMinFactor <= 1 &&
		location.PVChargeCorrectionMaxFactor > 0 &&
		location.PVChargeCorrectionMaxFactor <= 1 &&
		location.PVChargeCorrectionMaxFactor >= location.PVChargeCorrectionMinFactor
}

func normalizeWeatherTimezone(value string) string {
	if value == "" {
		return "Asia/Tokyo"
	}
	return value
}

func normalizeWeatherSettings(location domain.WeatherLocation) domain.WeatherLocation {
	if location.PVPerformanceRatio <= 0 {
		location.PVPerformanceRatio = 0.75
	}
	if location.BatteryCapacityKWh <= 0 {
		location.BatteryCapacityKWh = 4.096
	}
	if location.MinimumReserveSoc <= 0 {
		location.MinimumReserveSoc = 30
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
	if location.PVChargeCorrectionFactor <= 0 {
		location.PVChargeCorrectionFactor = 0.7
	}
	if location.PVChargeCorrectionFactor < location.PVChargeCorrectionMinFactor {
		location.PVChargeCorrectionFactor = location.PVChargeCorrectionMinFactor
	}
	if location.PVChargeCorrectionFactor > location.PVChargeCorrectionMaxFactor {
		location.PVChargeCorrectionFactor = location.PVChargeCorrectionMaxFactor
	}
	return location
}
