package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

type fakeSolarForecastSettingsStore struct {
	location domain.WeatherLocation
}

func (s fakeSolarForecastSettingsStore) CurrentWeatherLocation(context.Context) (domain.WeatherLocation, error) {
	return s.location, nil
}

func (s fakeSolarForecastSettingsStore) UpdateWeatherLocation(context.Context, domain.WeatherLocation) error {
	return nil
}

type fakeSolarForecastClient struct {
	days int
}

func (c *fakeSolarForecastClient) ForecastDaysForLocation(_ context.Context, _ domain.WeatherLocation, days int) ([]domain.WeatherForecast, error) {
	c.days = days
	return []domain.WeatherForecast{
		{
			Provider:                    "open-meteo",
			Date:                        "2026-05-20",
			ShortwaveRadiationMJPerM2:   11.52,
			SunshineDurationHours:       4,
			CloudCoverMeanPercent:       60,
			PrecipitationProbabilityMax: 70,
			PrecipitationSumMM:          12,
		},
	}, nil
}

func TestSolarForecastHandlerReturnsPVEstimate(t *testing.T) {
	client := &fakeSolarForecastClient{}
	handler := solarForecastHandler(fakeSolarForecastSettingsStore{location: domain.WeatherLocation{
		Enabled:            true,
		Latitude:           35.362502,
		Longitude:          136.925363,
		Timezone:           "Asia/Tokyo",
		PVCapacityKW:       4,
		PVPerformanceRatio: 0.75,
		DailyBaseLoadKWh:   2.4,
	}}, client, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/api/weather/solar-forecast?days=7", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if client.days != 7 {
		t.Fatalf("days = %d, want 7", client.days)
	}
	var summary domain.SolarForecastSummary
	if err := json.NewDecoder(rec.Body).Decode(&summary); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(summary.Items) != 1 {
		t.Fatalf("items len = %d, want 1", len(summary.Items))
	}
	if summary.Items[0].EstimatedPVKWh != 9.6 {
		t.Fatalf("EstimatedPVKWh = %f, want 9.6", summary.Items[0].EstimatedPVKWh)
	}
	if summary.Items[0].PVEffectiveStartAt != "2026-05-20T09:00" || summary.Items[0].PVEffectiveEndAt != "2026-05-20T16:00" {
		t.Fatalf("PV effective window = %s-%s, want fallback 09:00-16:00", summary.Items[0].PVEffectiveStartAt, summary.Items[0].PVEffectiveEndAt)
	}
	if summary.Items[0].PVEffectiveWindowSource != "fallback" {
		t.Fatalf("PVEffectiveWindowSource = %q, want fallback", summary.Items[0].PVEffectiveWindowSource)
	}
	if summary.Items[0].EstimatedSurplusKWh != 7.199999999999999 {
		t.Fatalf("EstimatedSurplusKWh = %f, want 7.2", summary.Items[0].EstimatedSurplusKWh)
	}
	if summary.Items[0].SolarForecastScore < 40 || summary.Items[0].SolarForecastScore >= 70 {
		t.Fatalf("SolarForecastScore = %d, want moderate score", summary.Items[0].SolarForecastScore)
	}
}

func TestSolarForecastHandlerRejectsUnsupportedDays(t *testing.T) {
	handler := solarForecastHandler(fakeSolarForecastSettingsStore{}, &fakeSolarForecastClient{}, slog.Default())
	req := httptest.NewRequest(http.MethodGet, "/api/weather/solar-forecast?days=10", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
