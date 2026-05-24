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
	targets []int
	err     error
}

func (w *fakeDelta3AuxWriter) SetACChargePower(_ context.Context, watts int) error {
	w.targets = append(w.targets, watts)
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
