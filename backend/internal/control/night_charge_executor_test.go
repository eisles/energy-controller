package control

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

type recordingNightChargeWriter struct {
	commands []string
	err      error
}

func (w *recordingNightChargeWriter) SetACChargePower(_ context.Context, watts int) error {
	w.commands = append(w.commands, "ac")
	return w.err
}

func (w *recordingNightChargeWriter) SetBackupReserveSoc(_ context.Context, percent int) error {
	w.commands = append(w.commands, "reserve")
	return w.err
}

func (w *recordingNightChargeWriter) SetTOUMode(_ context.Context, enabled bool) error {
	if enabled {
		w.commands = append(w.commands, "tou-on")
	} else {
		w.commands = append(w.commands, "tou-off")
	}
	return w.err
}

func (w *recordingNightChargeWriter) SetSelfPoweredMode(_ context.Context, enabled bool) error {
	if enabled {
		w.commands = append(w.commands, "self-powered-on")
	} else {
		w.commands = append(w.commands, "self-powered-off")
	}
	return w.err
}

func TestGuardNightChargeCommandRequiresExplicitRealControlGuards(t *testing.T) {
	plan := GuardNightChargeCommand(NightChargeCommandGuardInput{
		Plan: domain.NightChargePlan{
			WouldWrite: true,
		},
		SimulationMode:         false,
		EnableRealControl:      true,
		AutoControl:            true,
		ConfirmEcoFlowWrite:    "",
		RealControlTrialActive: true,
	})

	if plan.WouldWrite {
		t.Fatal("WouldWrite = true, want false without write confirmation")
	}
	if plan.CommandBlockReason != "CONFIRM_ECOFLOW_WRITE is not I_UNDERSTAND" {
		t.Fatalf("CommandBlockReason = %q", plan.CommandBlockReason)
	}
}

func TestExecuteNightChargeCommandAppliesReserveBeforeSelfPoweredMode(t *testing.T) {
	reserve := 42
	writer := &recordingNightChargeWriter{}
	plan := ExecuteNightChargeCommand(context.Background(), domain.NightChargePlan{
		WouldWrite:                  true,
		ShouldSetBackupReserve:      true,
		RecommendedBackupReserveSoc: &reserve,
		ShouldEnableSelfPoweredMode: true,
	}, writer)

	if !plan.CommandSent {
		t.Fatal("CommandSent = false, want true")
	}
	want := []string{"reserve", "self-powered-on"}
	if len(writer.commands) != len(want) {
		t.Fatalf("commands = %v, want %v", writer.commands, want)
	}
	for i := range want {
		if writer.commands[i] != want[i] {
			t.Fatalf("commands = %v, want %v", writer.commands, want)
		}
	}
}

func TestExecuteNightChargeCommandDisablesModeBeforeChargeSettings(t *testing.T) {
	reserve := 90
	writer := &recordingNightChargeWriter{}
	plan := ExecuteNightChargeCommand(context.Background(), domain.NightChargePlan{
		WouldWrite:                  true,
		ShouldDisableEnergyModes:    true,
		ShouldSetACChargeLimit:      true,
		RecommendedACChargeLimitW:   1500,
		ShouldSetBackupReserve:      true,
		RecommendedBackupReserveSoc: &reserve,
	}, writer)

	if !plan.CommandSent {
		t.Fatal("CommandSent = false, want true")
	}
	want := []string{"tou-off", "ac", "reserve"}
	if len(writer.commands) != len(want) {
		t.Fatalf("commands = %v, want %v", writer.commands, want)
	}
	for i := range want {
		if writer.commands[i] != want[i] {
			t.Fatalf("commands = %v, want %v", writer.commands, want)
		}
	}
}

func TestGuardNightChargeCommandSuppressesDuplicateSentCommand(t *testing.T) {
	now := time.Date(2026, 5, 20, 3, 0, 0, 0, time.UTC)
	plan := GuardNightChargeCommand(NightChargeCommandGuardInput{
		Plan: domain.NightChargePlan{
			WouldWrite:         true,
			CommandFingerprint: "reserve=42|self-powered=on",
		},
		SimulationMode:         false,
		EnableRealControl:      true,
		AutoControl:            true,
		ConfirmEcoFlowWrite:    confirmEcoFlowWriteValue,
		RealControlTrialActive: true,
		Now:                    now,
		Settings:               DefaultSettings(),
		Previous: &domain.NightChargePlanLog{
			MeasuredAt:          now.Add(-30 * time.Second),
			CommandFingerprint:  "reserve=42|self-powered=on",
			CommandSent:         true,
			WouldWrite:          true,
			CommandBlockReason:  "",
			ShouldChargeTonight: false,
		},
	})

	if plan.WouldWrite {
		t.Fatal("WouldWrite = true, want false for duplicate command")
	}
	if plan.CommandBlockReason != "duplicate night charge command candidate" {
		t.Fatalf("CommandBlockReason = %q", plan.CommandBlockReason)
	}
}

func TestGuardNightChargeCommandAllowsSameFingerprintAfterInterval(t *testing.T) {
	now := time.Date(2026, 5, 20, 3, 0, 0, 0, time.UTC)
	plan := GuardNightChargeCommand(NightChargeCommandGuardInput{
		Plan: domain.NightChargePlan{
			WouldWrite:         true,
			CommandFingerprint: "reserve=42|self-powered=on",
		},
		SimulationMode:         false,
		EnableRealControl:      true,
		AutoControl:            true,
		ConfirmEcoFlowWrite:    confirmEcoFlowWriteValue,
		RealControlTrialActive: true,
		Now:                    now,
		Settings:               DefaultSettings(),
		Previous: &domain.NightChargePlanLog{
			MeasuredAt:         now.Add(-2 * time.Minute),
			CommandFingerprint: "reserve=42|self-powered=on",
			CommandSent:        true,
			WouldWrite:         true,
		},
	})

	if !plan.WouldWrite {
		t.Fatalf("WouldWrite = false after interval, block reason = %q", plan.CommandBlockReason)
	}
}

func TestGuardNightChargeCommandSuppressesRetryAfterErrorWithinInterval(t *testing.T) {
	now := time.Date(2026, 5, 20, 3, 0, 0, 0, time.UTC)
	message := "api down"
	plan := GuardNightChargeCommand(NightChargeCommandGuardInput{
		Plan: domain.NightChargePlan{
			WouldWrite:         true,
			CommandFingerprint: "ac=1500|reserve=90",
		},
		SimulationMode:         false,
		EnableRealControl:      true,
		AutoControl:            true,
		ConfirmEcoFlowWrite:    confirmEcoFlowWriteValue,
		RealControlTrialActive: true,
		Now:                    now,
		Settings:               DefaultSettings(),
		Previous: &domain.NightChargePlanLog{
			MeasuredAt:         now.Add(-30 * time.Second),
			CommandFingerprint: "ac=1500|reserve=90",
			CommandError:       &message,
		},
	})

	if plan.WouldWrite {
		t.Fatal("WouldWrite = true, want false after recent error")
	}
	if !plan.CommandSuppressed {
		t.Fatal("CommandSuppressed = false, want true")
	}
}

func TestExecuteNightChargeCommandRecordsError(t *testing.T) {
	writer := &recordingNightChargeWriter{err: errors.New("api down")}
	plan := ExecuteNightChargeCommand(context.Background(), domain.NightChargePlan{
		WouldWrite:                true,
		ShouldSetACChargeLimit:    true,
		RecommendedACChargeLimitW: 1500,
	}, writer)

	if plan.CommandError == nil {
		t.Fatal("CommandError = nil, want error")
	}
	if plan.WouldWrite {
		t.Fatal("WouldWrite = true, want false after error")
	}
}
