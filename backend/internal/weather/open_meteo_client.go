package weather

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

const defaultOpenMeteoBaseURL = "https://api.open-meteo.com"

type OpenMeteoConfig struct {
	Latitude   float64
	Longitude  float64
	Timezone   string
	BaseURL    string
	HTTPClient *http.Client
}

type OpenMeteoClient struct {
	latitude   float64
	longitude  float64
	timezone   string
	baseURL    string
	httpClient *http.Client
}

func NewOpenMeteoClient(cfg OpenMeteoConfig) *OpenMeteoClient {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = defaultOpenMeteoBaseURL
	}
	timezone := cfg.Timezone
	if timezone == "" {
		timezone = "Asia/Tokyo"
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &OpenMeteoClient{
		latitude:   cfg.Latitude,
		longitude:  cfg.Longitude,
		timezone:   timezone,
		baseURL:    baseURL,
		httpClient: httpClient,
	}
}

func (c *OpenMeteoClient) ForecastTargetDaytime(ctx context.Context, now time.Time) (domain.WeatherForecast, error) {
	return c.forecastForTargetDate(ctx, c.latitude, c.longitude, c.timezone, targetDaytimeDate(now))
}

func (c *OpenMeteoClient) CurrentWeatherLocation(context.Context) (domain.WeatherLocation, error) {
	return domain.WeatherLocation{
		Enabled:            c.latitude != 0 && c.longitude != 0,
		Latitude:           c.latitude,
		Longitude:          c.longitude,
		Timezone:           c.timezone,
		PVPerformanceRatio: 0.75,
		BatteryCapacityKWh: 4.096,
		MinimumReserveSoc:  30,
	}, nil
}

func (c *OpenMeteoClient) ForecastTargetDaytimeForLocation(ctx context.Context, location domain.WeatherLocation, now time.Time) (domain.WeatherForecast, error) {
	return c.forecastForTargetDate(ctx, location.Latitude, location.Longitude, location.Timezone, targetDaytimeDate(now))
}

func (c *OpenMeteoClient) ForecastDaysForLocation(ctx context.Context, location domain.WeatherLocation, days int) ([]domain.WeatherForecast, error) {
	if days < 1 {
		days = 1
	}
	if days > 16 {
		days = 16
	}
	payload, err := c.forecastPayload(ctx, location.Latitude, location.Longitude, location.Timezone, days)
	if err != nil {
		return nil, err
	}
	return forecastsFromResponse(payload)
}

func (c *OpenMeteoClient) forecastForTargetDate(ctx context.Context, latitude float64, longitude float64, timezone string, targetDate time.Time) (domain.WeatherForecast, error) {
	payload, err := c.forecastPayload(ctx, latitude, longitude, timezone, 3)
	if err != nil {
		return domain.WeatherForecast{}, err
	}
	return forecastFromResponse(payload, targetDate)
}

func (c *OpenMeteoClient) forecastPayload(ctx context.Context, latitude float64, longitude float64, timezone string, days int) (forecastResponse, error) {
	reqURL, err := url.Parse(c.baseURL + "/v1/forecast")
	if err != nil {
		return forecastResponse{}, err
	}
	if timezone == "" {
		timezone = c.timezone
	}
	query := reqURL.Query()
	query.Set("latitude", strconv.FormatFloat(latitude, 'f', -1, 64))
	query.Set("longitude", strconv.FormatFloat(longitude, 'f', -1, 64))
	query.Set("daily", "weather_code,shortwave_radiation_sum,sunshine_duration,cloud_cover_mean,precipitation_probability_max,precipitation_sum")
	query.Set("hourly", "shortwave_radiation")
	query.Set("forecast_days", strconv.Itoa(days))
	query.Set("timezone", timezone)
	reqURL.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return forecastResponse{}, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return forecastResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return forecastResponse{}, fmt.Errorf("Open-Meteo forecast returned HTTP %d", resp.StatusCode)
	}

	var payload forecastResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return forecastResponse{}, err
	}
	return payload, nil
}

type LocationProvider interface {
	CurrentWeatherLocation(ctx context.Context) (domain.WeatherLocation, error)
}

type LocationForecastClient struct {
	locations *LocationProviderWrapper
	forecast  *OpenMeteoClient
}

type LocationProviderWrapper struct {
	Provider LocationProvider
}

func NewLocationForecastClient(provider LocationProvider, forecast *OpenMeteoClient) *LocationForecastClient {
	return &LocationForecastClient{
		locations: &LocationProviderWrapper{Provider: provider},
		forecast:  forecast,
	}
}

func (c *LocationForecastClient) ForecastTargetDaytime(ctx context.Context, now time.Time) (domain.WeatherForecast, error) {
	location, err := c.locations.Provider.CurrentWeatherLocation(ctx)
	if err != nil {
		return domain.WeatherForecast{}, err
	}
	if !location.Enabled || location.Latitude == 0 || location.Longitude == 0 {
		return domain.WeatherForecast{}, fmt.Errorf("weather forecast location is not configured")
	}
	return c.forecast.ForecastTargetDaytimeForLocation(ctx, location, now)
}

func (c *LocationForecastClient) CurrentWeatherLocation(ctx context.Context) (domain.WeatherLocation, error) {
	return c.locations.Provider.CurrentWeatherLocation(ctx)
}

func forecastFromResponse(payload forecastResponse, targetDate time.Time) (domain.WeatherForecast, error) {
	if len(payload.Daily.Time) == 0 {
		return domain.WeatherForecast{}, fmt.Errorf("Open-Meteo forecast response has no daily time")
	}
	targetDateString := targetDate.Format("2006-01-02")
	index := -1
	for i, value := range payload.Daily.Time {
		if value == targetDateString {
			index = i
			break
		}
	}
	if index == -1 {
		index = 0
	}
	forecast := domain.WeatherForecast{
		Provider:                    "open-meteo",
		Date:                        stringAt(payload.Daily.Time, index),
		WeatherCode:                 intAt(payload.Daily.WeatherCode, index),
		ShortwaveRadiationMJPerM2:   floatAt(payload.Daily.ShortwaveRadiationSum, index),
		SunshineDurationHours:       floatAt(payload.Daily.SunshineDuration, index) / 3600,
		CloudCoverMeanPercent:       intAt(payload.Daily.CloudCoverMean, index),
		PrecipitationProbabilityMax: intAt(payload.Daily.PrecipitationProbabilityMax, index),
		PrecipitationSumMM:          floatAt(payload.Daily.PrecipitationSum, index),
		HourlyShortwaveRadiation:    hourlyShortwaveRadiationForDate(payload.Hourly, stringAt(payload.Daily.Time, index)),
	}
	if forecast.Date == "" {
		return domain.WeatherForecast{}, fmt.Errorf("Open-Meteo forecast response has no target date")
	}
	return forecast, nil
}

func forecastsFromResponse(payload forecastResponse) ([]domain.WeatherForecast, error) {
	if len(payload.Daily.Time) == 0 {
		return nil, fmt.Errorf("Open-Meteo forecast response has no daily time")
	}
	forecasts := make([]domain.WeatherForecast, 0, len(payload.Daily.Time))
	for index := range payload.Daily.Time {
		forecast := domain.WeatherForecast{
			Provider:                    "open-meteo",
			Date:                        stringAt(payload.Daily.Time, index),
			WeatherCode:                 intAt(payload.Daily.WeatherCode, index),
			ShortwaveRadiationMJPerM2:   floatAt(payload.Daily.ShortwaveRadiationSum, index),
			SunshineDurationHours:       floatAt(payload.Daily.SunshineDuration, index) / 3600,
			CloudCoverMeanPercent:       intAt(payload.Daily.CloudCoverMean, index),
			PrecipitationProbabilityMax: intAt(payload.Daily.PrecipitationProbabilityMax, index),
			PrecipitationSumMM:          floatAt(payload.Daily.PrecipitationSum, index),
			HourlyShortwaveRadiation:    hourlyShortwaveRadiationForDate(payload.Hourly, stringAt(payload.Daily.Time, index)),
		}
		if forecast.Date == "" {
			return nil, fmt.Errorf("Open-Meteo forecast response has no target date")
		}
		forecasts = append(forecasts, forecast)
	}
	return forecasts, nil
}

func targetDaytimeDate(now time.Time) time.Time {
	if now.Hour() < 12 {
		return now
	}
	return now.AddDate(0, 0, 1)
}

func stringAt(values []string, index int) string {
	if index < 0 || index >= len(values) {
		return ""
	}
	return values[index]
}

func intAt(values []int, index int) int {
	if index < 0 || index >= len(values) {
		return 0
	}
	return values[index]
}

func floatAt(values []float64, index int) float64 {
	if index < 0 || index >= len(values) {
		return 0
	}
	return values[index]
}

func hourlyShortwaveRadiationForDate(hourly forecastHourly, date string) []domain.HourlyShortwaveRadiation {
	if date == "" || len(hourly.Time) == 0 {
		return nil
	}
	values := make([]domain.HourlyShortwaveRadiation, 0, 24)
	for i, value := range hourly.Time {
		if !strings.HasPrefix(value, date+"T") {
			continue
		}
		values = append(values, domain.HourlyShortwaveRadiation{
			Time:                     value,
			ShortwaveRadiationWPerM2: floatAt(hourly.ShortwaveRadiation, i),
		})
	}
	return values
}

type forecastResponse struct {
	Daily  forecastDaily  `json:"daily"`
	Hourly forecastHourly `json:"hourly"`
}

type forecastDaily struct {
	Time                        []string  `json:"time"`
	WeatherCode                 []int     `json:"weather_code"`
	ShortwaveRadiationSum       []float64 `json:"shortwave_radiation_sum"`
	SunshineDuration            []float64 `json:"sunshine_duration"`
	CloudCoverMean              []int     `json:"cloud_cover_mean"`
	PrecipitationProbabilityMax []int     `json:"precipitation_probability_max"`
	PrecipitationSum            []float64 `json:"precipitation_sum"`
}

type forecastHourly struct {
	Time               []string  `json:"time"`
	ShortwaveRadiation []float64 `json:"shortwave_radiation"`
}
