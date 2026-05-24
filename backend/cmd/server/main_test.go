package main

import (
	"context"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eisles/energy-controller/backend/internal/api"
	"github.com/eisles/energy-controller/backend/internal/config"
	"github.com/eisles/energy-controller/backend/internal/domain"
	"github.com/eisles/energy-controller/backend/internal/store"
)

type stubStatusProvider struct {
	status         domain.Status
	actualCommandW *int
	commandSent    bool
}

type fixedMainClock struct {
	now time.Time
}

func (c fixedMainClock) Now() time.Time {
	return c.now
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

type stubDelta3StatusReader struct {
	status api.Delta3StatusResponse
}

func (r stubDelta3StatusReader) CurrentStatus(context.Context) api.Delta3StatusResponse {
	return r.status
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
		nil,
		nil,
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

func TestRecordStatusPersistsDelta3AuxSuppressionOnStatus(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "energy.db"))
	if err != nil {
		t.Fatalf("store.Open failed: %v", err)
	}
	defer db.Close()

	now := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	provider := stubStatusProvider{
		status: domain.Status{
			GridW:     -900,
			ImportW:   0,
			ExportW:   900,
			State:     "simulation",
			Mode:      "mock",
			UpdatedAt: now,
			SurplusPlan: &domain.SurplusPlan{
				StrategyState: "HOLD",
				Reason:        "DELTA Pro 3 priority is already satisfied",
			},
		},
	}
	currentLimitW := 100
	soc := 50
	maxSoc := 100
	statusRepository := store.NewStatusRepository(db)

	recordStatus(
		context.Background(),
		config.Config{
			MockMode:          true,
			SimulationMode:    true,
			EnableRealControl: false,
			Delta3ReadEnabled: true,
			Delta3Aux: config.Delta3AuxConfig{
				Enabled:                   true,
				MinChargeW:                100,
				MaxChargeW:                1500,
				SafetyMarginW:             50,
				MinCommandDiffW:           100,
				MaxIncreaseStepW:          300,
				MaxDecreaseStepW:          500,
				MinCommandInterval:        2 * time.Minute,
				StopImportThresholdW:      50,
				TargetMaxSocBufferPercent: 2,
			},
		},
		provider,
		statusRepository,
		store.NewLogRepository(db),
		nil,
		nil,
		store.NewDelta3AuxControlCommandRepository(db),
		nil,
		nil,
		nil,
		stubDelta3StatusReader{status: api.Delta3StatusResponse{
			Available:      true,
			SOC:            &soc,
			ACChargeLimitW: &currentLimitW,
			MaxChargeSoc:   &maxSoc,
		}},
		nil,
		nil,
		nil,
		slog.Default(),
	)

	got, err := statusRepository.CurrentStatus(context.Background())
	if err != nil {
		t.Fatalf("CurrentStatus failed: %v", err)
	}
	if got.Delta3AuxPlan == nil {
		t.Fatal("Delta3AuxPlan = nil, want persisted plan")
	}
	if got.Delta3AuxPlan.WouldWrite {
		t.Fatal("Delta3AuxPlan.WouldWrite = true, want false under mock/simulation guard")
	}
	if !strings.Contains(got.Delta3AuxPlan.SuppressedReason, "mock mode") {
		t.Fatalf("SuppressedReason = %q, want mock mode guard", got.Delta3AuxPlan.SuppressedReason)
	}
}

func TestRecordStatusPersistsNightOwnedSurplusSkipLogWithoutWriteCandidate(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "energy.db"))
	if err != nil {
		t.Fatalf("store.Open failed: %v", err)
	}
	defer db.Close()

	now := time.Date(2026, 5, 25, 0, 10, 0, 0, time.FixedZone("JST", 9*60*60))
	reserve := 48
	provider := stubStatusProvider{
		status: domain.Status{
			GridW:              2440,
			ImportW:            2440,
			ExportW:            0,
			BatterySoc:         38,
			BatteryInputW:      1419,
			BatteryOutputW:     1031,
			ACChargeLimitW:     1500,
			BackupReserveSoc:   &reserve,
			State:              "real-control",
			Mode:               "nature-cloud+ecoflow-read",
			LastDecisionReason: "importing from grid, do not charge",
			UpdatedAt:          now,
			NightChargePlan: &domain.NightChargePlan{
				StrategyState:             "NIGHT_CHARGE_WINDOW",
				RecommendedMode:           "tou",
				RecommendedNightTargetSoc: 48,
				ShouldChargeTonight:       true,
				ActionSummary:             "推奨modeはtou; 深夜目標SOCを48%へ設定",
				Reason:                    "target daytime solar forecast is strong; keep night charging modest",
				CommandBlockReason:        "night charge settings already match plan",
			},
		},
	}
	surplusRepository := store.NewSurplusControlCommandRepository(db)

	recordStatus(
		context.Background(),
		config.Config{Clock: fixedMainClock{now: now}},
		provider,
		store.NewStatusRepository(db),
		store.NewLogRepository(db),
		store.NewNightChargePlanRepository(db),
		surplusRepository,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		slog.Default(),
	)

	logs, total, err := surplusRepository.ListSurplusControlCommandLogsPage(context.Background(), 10, 0, store.SurplusControlCommandLogPageFilter{})
	if err != nil {
		t.Fatalf("ListSurplusControlCommandLogsPage failed: %v", err)
	}
	if total != 1 || len(logs) != 1 {
		t.Fatalf("surplus skip logs = %d/%d, want 1/1", len(logs), total)
	}
	log := logs[0]
	if log.WouldWrite || log.CommandSent || !log.DryRun {
		t.Fatalf("WouldWrite,CommandSent,DryRun = %v,%v,%v; want false,false,true", log.WouldWrite, log.CommandSent, log.DryRun)
	}
	if log.ErrorMessage != nil {
		t.Fatalf("ErrorMessage = %q, want nil so skip log is not a write candidate", *log.ErrorMessage)
	}
	if log.SuppressedReason != "night charge plan owns control" {
		t.Fatalf("SuppressedReason = %q", log.SuppressedReason)
	}
	if !strings.Contains(log.DecisionReason, "推奨modeはtou") {
		t.Fatalf("DecisionReason = %q, want night plan action summary", log.DecisionReason)
	}
	latestCandidate, err := surplusRepository.LatestSurplusControlWriteCandidateLog(context.Background())
	if err != nil {
		t.Fatalf("LatestSurplusControlWriteCandidateLog failed: %v", err)
	}
	if latestCandidate != nil {
		t.Fatalf("LatestSurplusControlWriteCandidateLog = %+v, want nil for skip-only log", latestCandidate)
	}
}

func TestRealControlTrialActiveRequiresFutureDeadline(t *testing.T) {
	now := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	if realControlTrialActive(config.Config{Clock: fixedMainClock{now: now}}) {
		t.Fatal("trial active without deadline")
	}
	if realControlTrialActive(config.Config{RealControlTrialUntil: now, Clock: fixedMainClock{now: now}}) {
		t.Fatal("trial active at exact deadline")
	}
	if !realControlTrialActive(config.Config{RealControlTrialUntil: now.Add(time.Minute), Clock: fixedMainClock{now: now}}) {
		t.Fatal("trial inactive before deadline")
	}
}

func TestRealControlTrialActiveUsesCurrentClockNotMeasuredAt(t *testing.T) {
	deadline := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	cfg := config.Config{
		RealControlTrialUntil: deadline,
		Clock:                 fixedMainClock{now: deadline.Add(time.Second)},
	}
	if realControlTrialActive(cfg) {
		t.Fatal("trial active after current clock passed deadline")
	}
}
