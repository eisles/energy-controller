package weather

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

func TestOpenMeteoClientForecastTargetDaytimeUsesTodayBeforeNoon(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/forecast" {
			t.Fatalf("path = %s, want /v1/forecast", r.URL.Path)
		}
		if r.URL.Query().Get("latitude") != "35.1" || r.URL.Query().Get("longitude") != "139.2" {
			t.Fatalf("unexpected location query: %s", r.URL.RawQuery)
		}
		if r.URL.Query().Get("forecast_days") != "3" {
			t.Fatalf("forecast_days = %q, want 3", r.URL.Query().Get("forecast_days"))
		}
		if r.URL.Query().Get("hourly") != "shortwave_radiation" {
			t.Fatalf("hourly = %q, want shortwave_radiation", r.URL.Query().Get("hourly"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"daily": map[string]any{
				"time":                          []string{"2026-05-19", "2026-05-20"},
				"weather_code":                  []int{3, 1},
				"shortwave_radiation_sum":       []float64{12.5, 21.2},
				"sunshine_duration":             []float64{18000, 36000},
				"cloud_cover_mean":              []int{60, 20},
				"precipitation_probability_max": []int{40, 5},
				"precipitation_sum":             []float64{2.2, 0},
			},
			"hourly": map[string]any{
				"time":                []string{"2026-05-19T07:00", "2026-05-19T08:00", "2026-05-20T07:00"},
				"shortwave_radiation": []float64{205, 380, 140},
			},
		})
	}))
	defer server.Close()

	client := NewOpenMeteoClient(OpenMeteoConfig{
		Latitude:  35.1,
		Longitude: 139.2,
		BaseURL:   server.URL,
	})
	forecast, err := client.ForecastTargetDaytime(context.Background(), time.Date(2026, 5, 19, 2, 30, 0, 0, time.FixedZone("JST", 9*60*60)))
	if err != nil {
		t.Fatalf("ForecastTargetDaytime failed: %v", err)
	}

	if forecast.Provider != "open-meteo" || forecast.Date != "2026-05-19" {
		t.Fatalf("unexpected forecast metadata: %+v", forecast)
	}
	if forecast.ShortwaveRadiationMJPerM2 != 12.5 || forecast.SunshineDurationHours != 5 {
		t.Fatalf("unexpected solar values: %+v", forecast)
	}
	if forecast.CloudCoverMeanPercent != 60 || forecast.PrecipitationProbabilityMax != 40 {
		t.Fatalf("unexpected weather values: %+v", forecast)
	}
	if len(forecast.HourlyShortwaveRadiation) != 2 || forecast.HourlyShortwaveRadiation[0].ShortwaveRadiationWPerM2 != 205 {
		t.Fatalf("unexpected hourly radiation: %+v", forecast.HourlyShortwaveRadiation)
	}
}

func TestForecastFromResponseUsesNextDayAfterNoon(t *testing.T) {
	payload := forecastResponse{Daily: forecastDaily{
		Time:                        []string{"2026-05-19", "2026-05-20"},
		WeatherCode:                 []int{3, 1},
		ShortwaveRadiationSum:       []float64{12.5, 21.2},
		SunshineDuration:            []float64{18000, 36000},
		CloudCoverMean:              []int{60, 20},
		PrecipitationProbabilityMax: []int{40, 5},
		PrecipitationSum:            []float64{2.2, 0},
	}}
	forecast, err := forecastFromResponse(payload, targetDaytimeDate(time.Date(2026, 5, 19, 13, 0, 0, 0, time.UTC)))
	if err != nil {
		t.Fatalf("forecastFromResponse failed: %v", err)
	}
	if forecast.Date != "2026-05-20" {
		t.Fatalf("Date = %s, want 2026-05-20", forecast.Date)
	}
}

func TestOpenMeteoClientForecastDaysForLocationRequestsRequestedDays(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("forecast_days") != "16" {
			t.Fatalf("forecast_days = %q, want 16", r.URL.Query().Get("forecast_days"))
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"daily": map[string]any{
				"time":                          []string{"2026-05-19", "2026-05-20"},
				"weather_code":                  []int{1, 63},
				"shortwave_radiation_sum":       []float64{28, 11.53},
				"sunshine_duration":             []float64{45248.6, 15443.4},
				"cloud_cover_mean":              []int{5, 62},
				"precipitation_probability_max": []int{1, 73},
				"precipitation_sum":             []float64{0, 17},
			},
		})
	}))
	defer server.Close()

	client := NewOpenMeteoClient(OpenMeteoConfig{BaseURL: server.URL})
	forecasts, err := client.ForecastDaysForLocation(context.Background(), domainWeatherLocation(), 16)
	if err != nil {
		t.Fatalf("ForecastDaysForLocation failed: %v", err)
	}
	if len(forecasts) != 2 || forecasts[1].Date != "2026-05-20" {
		t.Fatalf("unexpected forecasts: %+v", forecasts)
	}
	if forecasts[1].SunshineDurationHours != 4.289833333333333 {
		t.Fatalf("SunshineDurationHours = %f", forecasts[1].SunshineDurationHours)
	}
}

func TestForecastFromResponseRequiresDailyTime(t *testing.T) {
	if _, err := forecastFromResponse(forecastResponse{}, time.Date(2026, 5, 19, 0, 0, 0, 0, time.UTC)); err == nil {
		t.Fatal("forecastFromResponse returned nil error without daily time")
	}
}

func TestForecastsFromResponseAttachesHourlyRadiationPerDay(t *testing.T) {
	payload := forecastResponse{
		Daily: forecastDaily{
			Time:                  []string{"2026-05-20", "2026-05-21"},
			ShortwaveRadiationSum: []float64{18, 6},
		},
		Hourly: forecastHourly{
			Time:               []string{"2026-05-20T07:00", "2026-05-20T08:00", "2026-05-21T10:00"},
			ShortwaveRadiation: []float64{120, 450, 80},
		},
	}
	forecasts, err := forecastsFromResponse(payload)
	if err != nil {
		t.Fatalf("forecastsFromResponse failed: %v", err)
	}
	if len(forecasts[0].HourlyShortwaveRadiation) != 2 {
		t.Fatalf("day 1 hourly len = %d, want 2", len(forecasts[0].HourlyShortwaveRadiation))
	}
	if len(forecasts[1].HourlyShortwaveRadiation) != 1 {
		t.Fatalf("day 2 hourly len = %d, want 1", len(forecasts[1].HourlyShortwaveRadiation))
	}
}

func domainWeatherLocation() domain.WeatherLocation {
	return domain.WeatherLocation{
		Enabled:   true,
		Latitude:  35.1,
		Longitude: 139.2,
		Timezone:  "Asia/Tokyo",
	}
}
