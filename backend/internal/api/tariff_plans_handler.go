package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"
	"github.com/eisles/energy-controller/backend/internal/store"
)

type TariffPlanStore interface {
	ListTariffPlans(ctx context.Context) ([]domain.TariffPlan, error)
	UpsertTariffPlan(ctx context.Context, plan domain.TariffPlan) (domain.TariffPlan, error)
	DeleteTariffPlan(ctx context.Context, id int64) error
}

type tariffPlanPayload struct {
	PlanName      string    `json:"planName"`
	DayRateYen    *float64  `json:"dayRateYen"`
	HomeRateYen   *float64  `json:"homeRateYen"`
	NightRateYen  *float64  `json:"nightRateYen"`
	ExportRateYen *float64  `json:"exportRateYen"`
	Timezone      string    `json:"timezone"`
	EffectiveFrom time.Time `json:"effectiveFrom"`
}

func getTariffPlansHandler(store TariffPlanStore, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		plans, err := store.ListTariffPlans(r.Context())
		if err != nil {
			logger.Error("failed to list tariff plans", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list tariff plans"})
			return
		}
		writeJSON(w, http.StatusOK, plans)
	}
}

func postTariffPlanHandler(store TariffPlanStore, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var payload tariffPlanPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid tariff plan payload"})
			return
		}
		if !validTariffPlanPayload(payload) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "tariff plan is out of range"})
			return
		}
		plan := domain.TariffPlan{
			PlanName:      payload.PlanName,
			DayRateYen:    *payload.DayRateYen,
			HomeRateYen:   *payload.HomeRateYen,
			NightRateYen:  *payload.NightRateYen,
			ExportRateYen: *payload.ExportRateYen,
			Timezone:      payload.Timezone,
			EffectiveFrom: payload.EffectiveFrom,
		}
		plan.PlanName = strings.TrimSpace(plan.PlanName)
		plan.Timezone = normalizeWeatherTimezone(plan.Timezone)
		if !validTariffPlan(plan) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "tariff plan is out of range"})
			return
		}
		saved, err := store.UpsertTariffPlan(r.Context(), plan)
		if err != nil {
			logger.Error("failed to save tariff plan", "error", err)
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save tariff plan"})
			return
		}
		writeJSON(w, http.StatusOK, saved)
	}
}

func deleteTariffPlanHandler(tariffStore TariffPlanStore, logger *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil || id <= 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid tariff plan id"})
			return
		}
		if err := tariffStore.DeleteTariffPlan(r.Context(), id); err != nil {
			switch {
			case errors.Is(err, store.ErrCannotDeleteLastTariffPlan):
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot delete the last tariff plan"})
			case errors.Is(err, store.ErrTariffPlanCoverageRequired):
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cannot delete tariff plan because current time would not be covered"})
			case errors.Is(err, sql.ErrNoRows):
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "tariff plan not found"})
			default:
				logger.Error("failed to delete tariff plan", "error", err)
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete tariff plan"})
			}
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

func validTariffPlanPayload(payload tariffPlanPayload) bool {
	return payload.DayRateYen != nil &&
		payload.HomeRateYen != nil &&
		payload.NightRateYen != nil &&
		payload.ExportRateYen != nil
}

func validTariffPlan(plan domain.TariffPlan) bool {
	if _, err := time.LoadLocation(plan.Timezone); err != nil {
		return false
	}
	return plan.PlanName != "" &&
		plan.DayRateYen > 0 &&
		plan.DayRateYen <= 500 &&
		plan.HomeRateYen > 0 &&
		plan.HomeRateYen <= 500 &&
		plan.NightRateYen > 0 &&
		plan.NightRateYen <= 500 &&
		plan.ExportRateYen > 0 &&
		plan.ExportRateYen <= 500 &&
		plan.Timezone != "" &&
		!plan.EffectiveFrom.IsZero() &&
		!plan.EffectiveFrom.Before(time.Date(1970, 1, 1, 0, 0, 0, 0, time.UTC))
}
