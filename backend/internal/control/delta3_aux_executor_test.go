package control

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

type fakeDelta3AuxWriter struct {
	targets        []int
	reserves       []int
	reserveEnabled []bool
	err            error
	reserveErr     error
}

func (w *fakeDelta3AuxWriter) SetACChargePower(_ context.Context, watts int) error {
	w.targets = append(w.targets, watts)
	return w.err
}

func (w *fakeDelta3AuxWriter) SetEnergyBackupEnabled(_ context.Context, enabled bool, startSoc int) error {
	w.reserveEnabled = append(w.reserveEnabled, enabled)
	w.reserves = append(w.reserves, startSoc)
	if w.reserveErr != nil {
		return w.reserveErr
	}
	return w.err
}

func TestEvaluateDelta3AuxCommandGuardRequiresEveryRealWriteGate(t *testing.T) {
	now := time.Date(2026, 5, 24, 10, 0, 0, 0, time.UTC)
	current := 100
	status := domain.Status{
		ExportW:   900,
		UpdatedAt: now,
		Delta3AuxPlan: &domain.Delta3AuxPlan{
			StrategyState:             "READY",
			RecommendedACChargeLimitW: 300,
			CurrentACChargeLimitW:     &current,
			ResidualExportW:           900,
			ShouldAdjustACChargeLimit: true,
			Reason:                    "test",
		},
	}
	base := Delta3AuxCommandGuardInput{
		Status:                 status,
		MockMode:               false,
		SimulationMode:         false,
		EnableRealControl:      true,
		AutoControl:            true,
		ConfirmEcoFlowWrite:    confirmEcoFlowWriteValue,
		RealControlTrialActive: true,
		Delta3ReadEnabled:      true,
		Delta3ControlEnabled:   true,
		AllowAutoControlWrite:  true,
		Execute:                true,
		AllowPrivateAPIWrite:   true,
	}
	tests := []struct {
		name    string
		mutate  func(*Delta3AuxCommandGuardInput)
		wantErr string
	}{
		{name: "mock", mutate: func(input *Delta3AuxCommandGuardInput) { input.MockMode = true }, wantErr: "mock mode"},
		{name: "simulation", mutate: func(input *Delta3AuxCommandGuardInput) { input.SimulationMode = true }, wantErr: "simulation mode"},
		{name: "real control", mutate: func(input *Delta3AuxCommandGuardInput) { input.EnableRealControl = false }, wantErr: "ENABLE_REAL_CONTROL"},
		{name: "auto control", mutate: func(input *Delta3AuxCommandGuardInput) { input.AutoControl = false }, wantErr: "auto control disabled"},
		{name: "confirmation", mutate: func(input *Delta3AuxCommandGuardInput) { input.ConfirmEcoFlowWrite = "" }, wantErr: "CONFIRM_ECOFLOW_WRITE"},
		{name: "trial", mutate: func(input *Delta3AuxCommandGuardInput) { input.RealControlTrialActive = false }, wantErr: "trial window"},
		{name: "read", mutate: func(input *Delta3AuxCommandGuardInput) { input.Delta3ReadEnabled = false }, wantErr: "ECOFLOW_DELTA3_READ_ENABLED"},
		{name: "master control", mutate: func(input *Delta3AuxCommandGuardInput) { input.Delta3ControlEnabled = false }, wantErr: "master write target"},
		{name: "auto write", mutate: func(input *Delta3AuxCommandGuardInput) { input.AllowAutoControlWrite = false }, wantErr: "ECOFLOW_DELTA3_ALLOW_WRITE_WITH_AUTO_CONTROL"},
		{name: "execute", mutate: func(input *Delta3AuxCommandGuardInput) { input.Execute = false }, wantErr: "ECOFLOW_DELTA3_EXECUTE"},
		{name: "private", mutate: func(input *Delta3AuxCommandGuardInput) { input.AllowPrivateAPIWrite = false }, wantErr: "ECOFLOW_DELTA3_ALLOW_PRIVATE_API_WRITE"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := base
			tt.mutate(&input)
			log := EvaluateDelta3AuxCommandGuard(input, Delta3AuxSettings{Enabled: true, MinCommandDiffW: 100})
			if log.WouldWrite {
				t.Fatal("WouldWrite = true, want false")
			}
			if !strings.Contains(log.SuppressedReason, tt.wantErr) {
				t.Fatalf("SuppressedReason = %q, want contains %q", log.SuppressedReason, tt.wantErr)
			}
		})
	}
}

func TestExecuteDelta3AuxCommandSendsTargetWhenAllowed(t *testing.T) {
	target := 300
	log := domain.Delta3AuxControlCommandLog{
		WouldWrite:                true,
		DryRun:                    true,
		ShouldAdjustACChargeLimit: true,
		TargetACChargeLimitW:      &target,
	}
	writer := &fakeDelta3AuxWriter{}

	got := ExecuteDelta3AuxCommand(context.Background(), log, writer)

	if !got.CommandSent || got.DryRun || got.ErrorMessage != nil {
		t.Fatalf("unexpected log after execute: %+v", got)
	}
	if len(writer.targets) != 1 || writer.targets[0] != target {
		t.Fatalf("targets = %v, want [%d]", writer.targets, target)
	}
}

func TestExecuteDelta3AuxCommandClearsWouldWriteOnFailure(t *testing.T) {
	target := 300
	log := domain.Delta3AuxControlCommandLog{
		WouldWrite:                true,
		DryRun:                    true,
		ShouldAdjustACChargeLimit: true,
		TargetACChargeLimitW:      &target,
	}
	writer := &fakeDelta3AuxWriter{err: errors.New("temporary failure")}

	got := ExecuteDelta3AuxCommand(context.Background(), log, writer)

	if got.WouldWrite {
		t.Fatal("WouldWrite = true, want false after failed write")
	}
	if got.CommandSent {
		t.Fatal("CommandSent = true, want false after failed write")
	}
	if got.DryRun {
		t.Fatal("DryRun = true, want false after attempted write")
	}
	if got.ErrorMessage == nil || !strings.Contains(*got.ErrorMessage, "temporary failure") {
		t.Fatalf("ErrorMessage = %v, want temporary failure", got.ErrorMessage)
	}
}

func TestExecuteDelta3AuxCommandSendsBackupReserveWhenAllowed(t *testing.T) {
	target := 40
	log := domain.Delta3AuxControlCommandLog{
		WouldWrite:             true,
		DryRun:                 true,
		ShouldSetBackupReserve: true,
		TargetBackupReserveSoc: &target,
	}
	writer := &fakeDelta3AuxWriter{}

	got := ExecuteDelta3AuxCommand(context.Background(), log, writer)

	if !got.CommandSent || got.DryRun || got.ErrorMessage != nil {
		t.Fatalf("unexpected log after execute: %+v", got)
	}
	if len(writer.reserves) != 1 || writer.reserves[0] != target {
		t.Fatalf("reserves = %v, want [%d]", writer.reserves, target)
	}
	if len(writer.reserveEnabled) != 1 || !writer.reserveEnabled[0] {
		t.Fatalf("reserveEnabled = %v, want [true]", writer.reserveEnabled)
	}
}

func TestExecuteDelta3AuxCommandDisablesBackupReserveWhenAllowed(t *testing.T) {
	target := 40
	log := domain.Delta3AuxControlCommandLog{
		WouldWrite:                 true,
		DryRun:                     true,
		ShouldDisableBackupReserve: true,
		TargetBackupReserveSoc:     &target,
	}
	writer := &fakeDelta3AuxWriter{}

	got := ExecuteDelta3AuxCommand(context.Background(), log, writer)

	if !got.CommandSent || got.DryRun || got.ErrorMessage != nil {
		t.Fatalf("unexpected log after execute: %+v", got)
	}
	if len(writer.reserves) != 1 || writer.reserves[0] != target {
		t.Fatalf("reserves = %v, want [%d]", writer.reserves, target)
	}
	if len(writer.reserveEnabled) != 1 || writer.reserveEnabled[0] {
		t.Fatalf("reserveEnabled = %v, want [false]", writer.reserveEnabled)
	}
}

func TestExecuteDelta3AuxCommandPreservesPartialWriteWhenBackupReserveFails(t *testing.T) {
	acTarget := 400
	reserveTarget := 40
	log := domain.Delta3AuxControlCommandLog{
		WouldWrite:                true,
		DryRun:                    true,
		ShouldAdjustACChargeLimit: true,
		TargetACChargeLimitW:      &acTarget,
		ShouldSetBackupReserve:    true,
		TargetBackupReserveSoc:    &reserveTarget,
	}
	writer := &fakeDelta3AuxWriter{reserveErr: errors.New("reserve failure")}

	got := ExecuteDelta3AuxCommand(context.Background(), log, writer)

	if !got.CommandSent {
		t.Fatal("CommandSent = false, want true because AC charge write succeeded before reserve failure")
	}
	if got.WouldWrite {
		t.Fatal("WouldWrite = true, want false after attempted write")
	}
	if got.DryRun {
		t.Fatal("DryRun = true, want false after attempted write")
	}
	if got.ErrorMessage == nil || !strings.Contains(*got.ErrorMessage, "reserve failure") {
		t.Fatalf("ErrorMessage = %v, want reserve failure", got.ErrorMessage)
	}
	if len(writer.targets) != 1 || writer.targets[0] != acTarget {
		t.Fatalf("targets = %v, want [%d]", writer.targets, acTarget)
	}
}

func TestEvaluateDelta3AuxCommandGuardRetriesPartialWriteAfterErrorInterval(t *testing.T) {
	now := time.Date(2026, 5, 24, 10, 10, 0, 0, time.UTC)
	currentLimit := 400
	targetLimit := 400
	targetReserve := 45
	message := "reserve failure"
	previous := &domain.Delta3AuxControlCommandLog{
		MeasuredAt:                now.Add(-10 * time.Minute),
		CommandSent:               true,
		WouldWrite:                false,
		StrategyState:             "READY",
		ShouldAdjustACChargeLimit: true,
		TargetACChargeLimitW:      &targetLimit,
		ShouldSetBackupReserve:    true,
		TargetBackupReserveSoc:    &targetReserve,
		CommandFingerprint:        "delta3_aux;state=READY;ac=400;reserve=45;adjust_ac=true;set_reserve=true;disable_reserve=false",
		ErrorMessage:              &message,
	}
	status := domain.Status{
		ExportW:   900,
		UpdatedAt: now,
		Delta3AuxPlan: &domain.Delta3AuxPlan{
			StrategyState:               "READY",
			RecommendedACChargeLimitW:   targetLimit,
			CurrentACChargeLimitW:       &currentLimit,
			RecommendedBackupReserveSoc: &targetReserve,
			CurrentBackupReserveSoc:     &targetReserve,
			ShouldAdjustACChargeLimit:   true,
			ShouldSetBackupReserve:      true,
			Reason:                      "test retry",
		},
	}

	log := EvaluateDelta3AuxCommandGuard(delta3AuxRealWriteGuardInput(status, previous), Delta3AuxSettings{
		Enabled:            true,
		MinCommandDiffW:    100,
		MinCommandInterval: 5 * time.Minute,
	})

	if !log.WouldWrite {
		t.Fatalf("WouldWrite = false, want retry after partial error interval; suppressed=%q", log.SuppressedReason)
	}
	if log.SuppressedReason == "duplicate command candidate" {
		t.Fatal("SuppressedReason = duplicate command candidate, want partial error to use retry interval")
	}
}

func TestEvaluateDelta3AuxCommandGuardSuppressesPartialWriteRetryWithinInterval(t *testing.T) {
	now := time.Date(2026, 5, 24, 10, 10, 0, 0, time.UTC)
	currentLimit := 400
	targetLimit := 400
	targetReserve := 45
	message := "reserve failure"
	previous := &domain.Delta3AuxControlCommandLog{
		MeasuredAt:                now.Add(-1 * time.Minute),
		CommandSent:               true,
		WouldWrite:                false,
		StrategyState:             "READY",
		ShouldAdjustACChargeLimit: true,
		TargetACChargeLimitW:      &targetLimit,
		ShouldSetBackupReserve:    true,
		TargetBackupReserveSoc:    &targetReserve,
		CommandFingerprint:        "delta3_aux;state=READY;ac=400;reserve=45;adjust_ac=true;set_reserve=true;disable_reserve=false",
		ErrorMessage:              &message,
	}
	status := domain.Status{
		ExportW:   900,
		UpdatedAt: now,
		Delta3AuxPlan: &domain.Delta3AuxPlan{
			StrategyState:               "READY",
			RecommendedACChargeLimitW:   targetLimit,
			CurrentACChargeLimitW:       &currentLimit,
			RecommendedBackupReserveSoc: &targetReserve,
			CurrentBackupReserveSoc:     &targetReserve,
			ShouldAdjustACChargeLimit:   true,
			ShouldSetBackupReserve:      true,
			Reason:                      "test retry",
		},
	}

	log := EvaluateDelta3AuxCommandGuard(delta3AuxRealWriteGuardInput(status, previous), Delta3AuxSettings{
		Enabled:            true,
		MinCommandDiffW:    100,
		MinCommandInterval: 5 * time.Minute,
	})

	if log.WouldWrite {
		t.Fatal("WouldWrite = true, want retry suppressed within interval")
	}
	if log.SuppressedReason != "command retry suppressed after previous error" {
		t.Fatalf("SuppressedReason = %q, want retry suppression", log.SuppressedReason)
	}
}

func TestEvaluateDelta3AuxCommandGuardSuppressesBackupReserveWhenCurrentReserveUnavailable(t *testing.T) {
	now := time.Date(2026, 5, 24, 10, 10, 0, 0, time.UTC)
	currentLimit := 400
	targetReserve := 45
	status := domain.Status{
		ExportW:   900,
		UpdatedAt: now,
		Delta3AuxPlan: &domain.Delta3AuxPlan{
			StrategyState:               "READY",
			RecommendedACChargeLimitW:   currentLimit,
			CurrentACChargeLimitW:       &currentLimit,
			RecommendedBackupReserveSoc: &targetReserve,
			ShouldSetBackupReserve:      true,
			Reason:                      "test missing reserve",
		},
	}

	log := EvaluateDelta3AuxCommandGuard(delta3AuxRealWriteGuardInput(status, nil), Delta3AuxSettings{
		Enabled:         true,
		MinCommandDiffW: 100,
	})

	if log.WouldWrite {
		t.Fatal("WouldWrite = true, want false when current backup reserve is unavailable")
	}
	if log.SuppressedReason != "DELTA 3 Plus current backup reserve unavailable" {
		t.Fatalf("SuppressedReason = %q, want current backup reserve unavailable", log.SuppressedReason)
	}
}

func delta3AuxRealWriteGuardInput(status domain.Status, previous *domain.Delta3AuxControlCommandLog) Delta3AuxCommandGuardInput {
	return Delta3AuxCommandGuardInput{
		Status:                 status,
		MockMode:               false,
		SimulationMode:         false,
		EnableRealControl:      true,
		AutoControl:            true,
		ConfirmEcoFlowWrite:    confirmEcoFlowWriteValue,
		RealControlTrialActive: true,
		Delta3ReadEnabled:      true,
		Delta3ControlEnabled:   true,
		AllowAutoControlWrite:  true,
		Execute:                true,
		AllowPrivateAPIWrite:   true,
		Previous:               previous,
	}
}
