package main

import (
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eisles/energy-controller/backend/internal/config"
	"github.com/eisles/energy-controller/backend/internal/domain"
	"github.com/eisles/energy-controller/backend/internal/store"
)

type stubStatusProvider struct {
	status         domain.Status
	actualCommandW *int
	commandSent    bool
}

func (p stubStatusProvider) CurrentStatus(context.Context) (domain.Status, error) {
	return p.status, nil
}

func (p stubStatusProvider) LastCommandActualW() *int {
	return p.actualCommandW
}

func (p stubStatusProvider) LastCommandSent() bool {
	return p.commandSent
}

func TestRecordStatusPersistsWouldSendLogWithoutCommandSent(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "energy.db"))
	if err != nil {
		t.Fatalf("store.Open failed: %v", err)
	}
	defer db.Close()

	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	provider := stubStatusProvider{
		status: domain.Status{
			GridW:              -1600,
			ImportW:            0,
			ExportW:            1600,
			BatterySoc:         50,
			TargetChargeW:      1400,
			State:              "simulation",
			Mode:               "ecoflow-read",
			LastDecisionReason: "export power is above start threshold; EcoFlow mock write adapter recorded would-send command",
			UpdatedAt:          now,
		},
		commandSent: false,
	}

	recordStatus(
		context.Background(),
		config.Config{},
		provider,
		store.NewStatusRepository(db),
		store.NewLogRepository(db),
		nil,
		nil,
		nil,
		nil,
		slog.Default(),
	)

	logs, err := store.NewLogRepository(db).ListPowerLogs(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListPowerLogs failed: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("len(logs) = %d, want 1", len(logs))
	}
	if logs[0].CommandSent {
		t.Fatal("CommandSent = true, want false for would-send")
	}
	if logs[0].ActualCommandW != nil {
		t.Fatalf("ActualCommandW = %v, want nil for would-send", *logs[0].ActualCommandW)
	}
	if !strings.Contains(logs[0].DecisionReason, "would-send") {
		t.Fatalf("DecisionReason = %q, want would-send marker", logs[0].DecisionReason)
	}
}
