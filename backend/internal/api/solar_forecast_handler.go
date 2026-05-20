package api

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/eisles/energy-controller/backend/internal/control"
	"github.com/eisles/energy-controller/backend/internal/domain"
)

type SolarForecastWeatherClient interface {
	ForecastDaysForLocation(ctx context.Context, location domain.WeatherLocation, days int) ([]domain.WeatherForecast, error)
}

func solarForecastHandler(settings WeatherSettingsStore, forecast SolarForecastWeatherClient, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		days, ok := parseForecastDays(r)
		if !ok {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "days must be one of 3, 7, or 16"})
			return
		}
		location, err := settings.CurrentWeatherLocation(r.Context())
		if err != nil {
			logger.Error("failed to get weather location", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get weather location"})
			return
		}
		if !location.Enabled || location.Latitude == 0 || location.Longitude == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "weather forecast location is not configured"})
			return
		}
		forecasts, err := forecast.ForecastDaysForLocation(r.Context(), location, days)
		if err != nil {
			logger.Error("failed to get solar forecast", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get solar forecast"})
			return
		}
		writeJSON(w, http.StatusOK, solarForecastSummary(location, forecasts, days))
	}
}

func parseForecastDays(r *http.Request) (int, bool) {
	raw := r.URL.Query().Get("days")
	if raw == "" {
		return 3, true
	}
	days, err := strconv.Atoi(raw)
	if err != nil {
		return 0, false
	}
	switch days {
	case 3, 7, 16:
		return days, true
	default:
		return 0, false
	}
}

func solarForecastSummary(location domain.WeatherLocation, forecasts []domain.WeatherForecast, days int) domain.SolarForecastSummary {
	items := make([]domain.SolarForecastEstimate, 0, len(forecasts))
	for _, forecast := range forecasts {
		ratio := location.PVPerformanceRatio
		if ratio <= 0 {
			ratio = 0.75
		}
		pvEstimate := control.EstimatePVForecast(forecast, location)
		estimatedPVKWh := pvEstimate.DailyEstimatedPVKWh
		estimatedSurplusKWh := estimatedPVKWh - location.DailyBaseLoadKWh
		if estimatedSurplusKWh < 0 {
			estimatedSurplusKWh = 0
		}
		items = append(items, domain.SolarForecastEstimate{
			Forecast:                    forecast,
			SolarForecastScore:          control.SolarForecastScore(forecast),
			SolarRadiationKWhPerM2:      pvEstimate.SolarRadiationKWhPerM2,
			EstimatedPVKWh:              estimatedPVKWh,
			DailyEstimatedPVKWh:         estimatedPVKWh,
			PVEffectiveStartAt:          pvEstimate.PVEffectiveStartAt,
			PVEffectiveEndAt:            pvEstimate.PVEffectiveEndAt,
			PVEffectiveWindowSource:     pvEstimate.PVEffectiveWindowSource,
			PVEffectiveRadiationWPerM2:  pvEstimate.PVEffectiveRadiationWPerM2,
			EstimatedDaytimeLoadKWh:     location.DailyBaseLoadKWh,
			EstimatedSurplusKWh:         estimatedSurplusKWh,
			PVCapacityKW:                location.PVCapacityKW,
			PVPerformanceRatio:          ratio,
			PrecipitationProbabilityMax: forecast.PrecipitationProbabilityMax,
			PrecipitationSumMM:          forecast.PrecipitationSumMM,
		})
	}
	return domain.SolarForecastSummary{
		Days:     days,
		Location: location,
		Items:    items,
		Note:     "Open-Meteo daily shortwave radiation is converted with PV capacity and performance ratio. This is a read-only estimate, not a control command.",
	}
}
