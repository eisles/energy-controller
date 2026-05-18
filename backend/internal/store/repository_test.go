package store

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"

	_ "modernc.org/sqlite"
)

func TestStatusRepositoryUpdatesAndReadsCurrentStatus(t *testing.T) {
	db := openTestDB(t)
	repo := NewStatusRepository(db)
	now := time.Date(2026, 5, 18, 8, 15, 0, 123, time.UTC)
	lastError := "sample error"

	want := domain.Status{
		GridW:              -850,
		ImportW:            0,
		ExportW:            850,
		BatterySoc:         62,
		BatteryInputW:      500,
		BatteryOutputW:     0,
		ACChargeLimitW:     1500,
		TargetChargeW:      700,
		State:              "simulation",
		Mode:               "mock",
		LastDecisionReason: "export power is above start threshold",
		LastError:          &lastError,
		UpdatedAt:          now,
	}

	if err := repo.UpdateCurrentStatus(context.Background(), want); err != nil {
		t.Fatalf("UpdateCurrentStatus failed: %v", err)
	}
	got, err := repo.CurrentStatus(context.Background())
	if err != nil {
		t.Fatalf("CurrentStatus failed: %v", err)
	}

	if got.GridW != want.GridW || got.ExportW != want.ExportW || got.TargetChargeW != want.TargetChargeW || got.ACChargeLimitW != want.ACChargeLimitW {
		t.Fatalf("status mismatch: got %+v want %+v", got, want)
	}
	if got.LastError == nil || *got.LastError != lastError {
		t.Fatalf("LastError = %#v, want %q", got.LastError, lastError)
	}
	if !got.UpdatedAt.Equal(now) {
		t.Fatalf("UpdatedAt = %s, want %s", got.UpdatedAt, now)
	}
}

func TestLogRepositoryInsertsAndListsNewestFirst(t *testing.T) {
	db := openTestDB(t)
	repo := NewLogRepository(db)
	firstAt := time.Date(2026, 5, 18, 8, 0, 0, 0, time.UTC)
	secondAt := firstAt.Add(time.Minute)
	soc := 61
	acLimit := 1500

	if err := repo.InsertPowerLog(context.Background(), domain.PowerLog{
		MeasuredAt:     firstAt,
		GridW:          200,
		ImportW:        200,
		ExportW:        0,
		TargetChargeW:  0,
		DecisionReason: "importing from grid, do not charge",
		Mode:           "mock",
		CommandSent:    false,
		CreatedAt:      firstAt,
	}); err != nil {
		t.Fatalf("InsertPowerLog first failed: %v", err)
	}
	if err := repo.InsertPowerLog(context.Background(), domain.PowerLog{
		MeasuredAt:     secondAt,
		GridW:          -900,
		ImportW:        0,
		ExportW:        900,
		BatterySoc:     &soc,
		ACChargeLimitW: &acLimit,
		TargetChargeW:  700,
		DecisionReason: "export power is above start threshold",
		Mode:           "mock",
		CommandSent:    false,
		CreatedAt:      secondAt,
	}); err != nil {
		t.Fatalf("InsertPowerLog second failed: %v", err)
	}

	logs, err := repo.ListPowerLogs(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListPowerLogs failed: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("len(logs) = %d, want 1", len(logs))
	}
	if logs[0].GridW != -900 || logs[0].BatterySoc == nil || *logs[0].BatterySoc != soc || logs[0].ACChargeLimitW == nil || *logs[0].ACChargeLimitW != acLimit {
		t.Fatalf("unexpected newest log: %+v", logs[0])
	}
}

func TestLogRepositoryListsSinceTimestamp(t *testing.T) {
	db := openTestDB(t)
	repo := NewLogRepository(db)
	firstAt := time.Date(2026, 5, 18, 8, 0, 0, 0, time.UTC)
	secondAt := firstAt.Add(2 * time.Hour)

	for _, log := range []domain.PowerLog{
		{
			MeasuredAt:     firstAt,
			GridW:          200,
			ImportW:        200,
			ExportW:        0,
			TargetChargeW:  0,
			DecisionReason: "importing from grid, do not charge",
			Mode:           "mock",
			CommandSent:    false,
			CreatedAt:      firstAt,
		},
		{
			MeasuredAt:     secondAt,
			GridW:          -900,
			ImportW:        0,
			ExportW:        900,
			TargetChargeW:  700,
			DecisionReason: "export power is above start threshold",
			Mode:           "mock",
			CommandSent:    false,
			CreatedAt:      secondAt,
		},
	} {
		if err := repo.InsertPowerLog(context.Background(), log); err != nil {
			t.Fatalf("InsertPowerLog failed: %v", err)
		}
	}

	logs, err := repo.ListPowerLogsSince(context.Background(), firstAt.Add(time.Hour), 0)
	if err != nil {
		t.Fatalf("ListPowerLogsSince failed: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("len(logs) = %d, want 1", len(logs))
	}
	if logs[0].MeasuredAt != secondAt {
		t.Fatalf("MeasuredAt = %s, want %s", logs[0].MeasuredAt, secondAt)
	}
}

func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Fatalf("db close failed: %v", err)
		}
	})
	if err := migrate(db); err != nil {
		t.Fatalf("migrate failed: %v", err)
	}
	return db
}
