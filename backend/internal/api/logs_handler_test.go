package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"
	"github.com/eisles/energy-controller/backend/internal/store"
)

type stubLogProvider struct {
	limit  int
	offset int
	filter store.LogPageFilter
	since  *time.Time
}

func (p *stubLogProvider) ListPowerLogs(_ context.Context, limit int) ([]domain.PowerLog, error) {
	p.limit = limit
	return p.logs()
}

func (p *stubLogProvider) ListPowerLogsPage(_ context.Context, limit int, offset int, filter store.LogPageFilter) ([]domain.PowerLog, int, error) {
	p.limit = limit
	p.offset = offset
	p.filter = filter
	logs, err := p.logs()
	return logs, 42, err
}

func (p *stubLogProvider) ListPowerLogsSince(_ context.Context, since time.Time, limit int) ([]domain.PowerLog, error) {
	p.since = &since
	p.limit = limit
	return p.logs()
}

func (p *stubLogProvider) logs() ([]domain.PowerLog, error) {
	return []domain.PowerLog{
		{
			ID:             1,
			MeasuredAt:     time.Date(2026, 5, 18, 8, 0, 0, 0, time.UTC),
			GridW:          -850,
			ImportW:        0,
			ExportW:        850,
			TargetChargeW:  700,
			DecisionReason: "export power is above start threshold",
			Mode:           "mock",
			CommandSent:    false,
			CreatedAt:      time.Date(2026, 5, 18, 8, 0, 0, 0, time.UTC),
		},
	}, nil
}

func TestLogsHandlerReturnsJSON(t *testing.T) {
	provider := &stubLogProvider{}
	req := httptest.NewRequest(http.MethodGet, "/api/logs?limit=25", nil)
	rec := httptest.NewRecorder()

	logsHandler(provider, slog.Default())(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	if provider.limit != 25 {
		t.Fatalf("limit = %d, want 25", provider.limit)
	}
	var payload []map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if len(payload) != 1 || payload[0]["gridW"] != float64(-850) {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestLogsHandlerReturnsPagedJSONWhenOffsetIsSpecified(t *testing.T) {
	provider := &stubLogProvider{}
	req := httptest.NewRequest(http.MethodGet, "/api/logs?limit=25&offset=50", nil)
	rec := httptest.NewRecorder()

	logsHandler(provider, slog.Default())(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	if provider.limit != 25 || provider.offset != 50 {
		t.Fatalf("limit, offset = %d, %d; want 25, 50", provider.limit, provider.offset)
	}
	var payload struct {
		Items  []map[string]any `json:"items"`
		Total  int              `json:"total"`
		Limit  int              `json:"limit"`
		Offset int              `json:"offset"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("invalid json response: %v", err)
	}
	if payload.Total != 42 || payload.Limit != 25 || payload.Offset != 50 || len(payload.Items) != 1 {
		t.Fatalf("unexpected payload: %#v", payload)
	}
}

func TestLogsHandlerPassesSearchQueryToPagedLogs(t *testing.T) {
	provider := &stubLogProvider{}
	req := httptest.NewRequest(http.MethodGet, "/api/logs?limit=25&offset=0&q=error", nil)
	rec := httptest.NewRecorder()

	logsHandler(provider, slog.Default())(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	if provider.filter.Query != "error" {
		t.Fatalf("query = %q, want error", provider.filter.Query)
	}
}

func TestLogsHandlerPassesDateRangeToPagedLogs(t *testing.T) {
	provider := &stubLogProvider{}
	from := "2026-05-18T07:00:00Z"
	to := "2026-05-18T08:00:00Z"
	req := httptest.NewRequest(http.MethodGet, "/api/logs?limit=25&offset=0&from="+from+"&to="+to, nil)
	rec := httptest.NewRecorder()

	logsHandler(provider, slog.Default())(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	if provider.filter.From == nil || provider.filter.From.Format(time.RFC3339) != from {
		t.Fatalf("from = %v, want %s", provider.filter.From, from)
	}
	if provider.filter.To == nil || provider.filter.To.Format(time.RFC3339) != to {
		t.Fatalf("to = %v, want %s", provider.filter.To, to)
	}
}

func TestLogsHandlerAcceptsSinceTimestamp(t *testing.T) {
	provider := &stubLogProvider{}
	since := "2026-05-18T07:00:00Z"
	req := httptest.NewRequest(http.MethodGet, "/api/logs?since="+since, nil)
	rec := httptest.NewRecorder()

	logsHandler(provider, slog.Default())(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusOK)
	}
	if provider.since == nil || provider.since.Format(time.RFC3339) != since {
		t.Fatalf("since = %v, want %s", provider.since, since)
	}
	if provider.limit != 0 {
		t.Fatalf("limit = %d, want 0 when since is used without explicit limit", provider.limit)
	}
}

func TestLogsHandlerRejectsInvalidLimit(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/logs?limit=bad", nil)
	rec := httptest.NewRecorder()

	logsHandler(&stubLogProvider{}, slog.Default())(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestLogsHandlerRejectsInvalidOffset(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/logs?offset=-1", nil)
	rec := httptest.NewRecorder()

	logsHandler(&stubLogProvider{}, slog.Default())(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestLogsHandlerRejectsOffsetWithSince(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/logs?since=2026-05-18T07:00:00Z&offset=25", nil)
	rec := httptest.NewRecorder()

	logsHandler(&stubLogProvider{}, slog.Default())(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestLogsHandlerRejectsSearchQueryWithSince(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/logs?since=2026-05-18T07:00:00Z&q=mock", nil)
	rec := httptest.NewRecorder()

	logsHandler(&stubLogProvider{}, slog.Default())(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestLogsHandlerRejectsInvalidDateRange(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/logs?from=2026-05-18T08:00:00Z&to=2026-05-18T07:00:00Z", nil)
	rec := httptest.NewRecorder()

	logsHandler(&stubLogProvider{}, slog.Default())(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestLogsHandlerRejectsInvalidSince(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/logs?since=bad", nil)
	rec := httptest.NewRecorder()

	logsHandler(&stubLogProvider{}, slog.Default())(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status code = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}
