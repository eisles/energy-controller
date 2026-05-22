package api

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"

	"github.com/eisles/energy-controller/backend/internal/config"
	"github.com/eisles/energy-controller/backend/internal/domain"
	"github.com/eisles/energy-controller/backend/internal/store"
	"github.com/eisles/energy-controller/backend/internal/weather"
)

type StatusProvider interface {
	CurrentStatus(ctx context.Context) (domain.Status, error)
}

type Dependencies struct {
	Config         config.Config
	DB             *sql.DB
	StatusProvider StatusProvider
	Logger         *slog.Logger
}

func NewRouter(deps Dependencies) http.Handler {
	mux := http.NewServeMux()
	statusProvider := deps.StatusProvider
	logProvider := LogProvider(nil)
	if deps.DB != nil {
		statusProvider = store.NewStatusRepository(deps.DB)
		logProvider = store.NewLogRepository(deps.DB)
	}
	mux.HandleFunc("GET /api/status", statusHandler(statusProvider, deps.Logger, deps.Config))
	if logProvider != nil {
		mux.HandleFunc("GET /api/logs", logsHandler(logProvider, deps.Logger))
	}
	if deps.DB != nil {
		weatherSettings := store.NewWeatherSettingsRepository(deps.DB)
		weatherClient := weather.NewOpenMeteoClient(weather.OpenMeteoConfig{
			BaseURL: deps.Config.WeatherBaseURL,
		})
		tariffRepository := store.NewTariffRepository(deps.DB)
		mux.HandleFunc("GET /api/settings/weather-location", getWeatherLocationHandler(weatherSettings, deps.Logger))
		mux.HandleFunc("PUT /api/settings/weather-location", putWeatherLocationHandler(weatherSettings, deps.Logger))
		mux.HandleFunc("GET /api/weather/solar-forecast", solarForecastHandler(weatherSettings, weatherClient, deps.Logger))
		mux.HandleFunc("GET /api/analytics/daytime-consumption", daytimeConsumptionHandler(store.NewDaytimeConsumptionRepository(deps.DB), deps.Logger))
		mux.HandleFunc("GET /api/analytics/ecoflow-load", ecoFlowLoadHandler(store.NewEcoFlowLoadRepositoryWithTimezone(deps.DB, deps.Config.WeatherTimezone), deps.Logger))
		mux.HandleFunc("GET /api/energy-meter/logs", energyMeterLogsHandler(store.NewEnergyMeterRepository(deps.DB), deps.Logger))
		mux.HandleFunc("GET /api/surplus-control/commands", surplusControlCommandLogsHandler(store.NewSurplusControlCommandRepository(deps.DB), deps.Logger))
		mux.HandleFunc("GET /api/night-charge/plans", nightChargePlanLogsHandler(store.NewNightChargePlanRepository(deps.DB), deps.Logger))
		mux.HandleFunc("GET /api/night-charge/summaries", nightChargeSummariesHandler(store.NewNightChargeSummaryRepositoryWithTimezone(deps.DB, deps.Config.WeatherTimezone), deps.Logger))
		mux.HandleFunc("GET /api/tariff/summary", tariffSummaryHandler(tariffRepository, deps.Logger))
		mux.HandleFunc("GET /api/settings/tariff-plans", getTariffPlansHandler(tariffRepository, deps.Logger))
		mux.HandleFunc("POST /api/settings/tariff-plans", postTariffPlanHandler(tariffRepository, deps.Logger))
		mux.HandleFunc("DELETE /api/settings/tariff-plans/{id}", deleteTariffPlanHandler(tariffRepository, deps.Logger))
	}
	mux.HandleFunc("GET /api/health", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.Handle("/", staticHandler(deps.Config.FrontendDir))
	return mux
}

func staticHandler(frontendDir string) http.Handler {
	fileServer := http.FileServer(http.Dir(frontendDir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			path := filepath.Join(frontendDir, filepath.Clean(r.URL.Path))
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		indexPath := filepath.Join(frontendDir, "index.html")
		if _, err := os.Stat(indexPath); err != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("<!doctype html><title>Energy Controller</title><h1>Energy Controller</h1><p>frontend/out is not built yet.</p>"))
			return
		}
		http.ServeFile(w, r, indexPath)
	})
}
