package control

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

func TestEvaluateSurplusCommandGuardAllowsDryRunCandidateWhenAllGuardsPass(t *testing.T) {
	now := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	status := surplusGuardStatus(now)

	log := EvaluateSurplusCommandGuard(SurplusCommandGuardInput{
		Status:                 status,
		MockMode:               false,
		SimulationMode:         false,
		EnableRealControl:      true,
		AutoControl:            true,
		ConfirmEcoFlowWrite:    confirmEcoFlowWriteValue,
		RealControlTrialActive: true,
	}, DefaultSettings())

	if !log.WouldWrite {
		t.Fatalf("WouldWrite = false, want true: %+v", log)
	}
	if !log.DryRun || log.CommandSent {
		t.Fatalf("DryRun,CommandSent = %v,%v; want true,false", log.DryRun, log.CommandSent)
	}
	if log.CommandKind != "mixed" {
		t.Fatalf("CommandKind = %q, want mixed", log.CommandKind)
	}
	if log.CommandFingerprint == "" {
		t.Fatal("CommandFingerprint is empty")
	}
	if log.ModeGuardReason != "mode status verified" {
		t.Fatalf("ModeGuardReason = %q, want verified", log.ModeGuardReason)
	}
}

func TestEvaluateSurplusCommandGuardBlocksUnsafeModes(t *testing.T) {
	now := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	base := surplusGuardStatus(now)
	tests := []struct {
		name   string
		mutate func(*SurplusCommandGuardInput)
		want   string
	}{
		{
			name: "mock",
			mutate: func(input *SurplusCommandGuardInput) {
				input.MockMode = true
			},
			want: "mock mode",
		},
		{
			name: "simulation",
			mutate: func(input *SurplusCommandGuardInput) {
				input.SimulationMode = true
			},
			want: "simulation mode",
		},
		{
			name: "real control disabled",
			mutate: func(input *SurplusCommandGuardInput) {
				input.EnableRealControl = false
			},
			want: "ENABLE_REAL_CONTROL=false",
		},
		{
			name: "auto disabled",
			mutate: func(input *SurplusCommandGuardInput) {
				input.AutoControl = false
			},
			want: "auto control disabled",
		},
		{
			name: "confirm missing",
			mutate: func(input *SurplusCommandGuardInput) {
				input.ConfirmEcoFlowWrite = ""
			},
			want: "CONFIRM_ECOFLOW_WRITE",
		},
		{
			name: "trial inactive",
			mutate: func(input *SurplusCommandGuardInput) {
				input.RealControlTrialActive = false
			},
			want: "real control trial window inactive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := SurplusCommandGuardInput{
				Status:                 base,
				EnableRealControl:      true,
				AutoControl:            true,
				ConfirmEcoFlowWrite:    confirmEcoFlowWriteValue,
				RealControlTrialActive: true,
			}
			tt.mutate(&input)

			log := EvaluateSurplusCommandGuard(input, DefaultSettings())
			if log.WouldWrite {
				t.Fatalf("WouldWrite = true, want false")
			}
			if !strings.Contains(log.SuppressedReason, tt.want) {
				t.Fatalf("SuppressedReason = %q, want contains %q", log.SuppressedReason, tt.want)
			}
		})
	}
}

func TestExecuteSurplusCommandSendsCommandsAndMarksLog(t *testing.T) {
	now := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	log := EvaluateSurplusCommandGuard(SurplusCommandGuardInput{
		Status:                 surplusGuardStatus(now),
		EnableRealControl:      true,
		AutoControl:            true,
		ConfirmEcoFlowWrite:    confirmEcoFlowWriteValue,
		RealControlTrialActive: true,
	}, DefaultSettings())
	writer := &stubSurplusWriteClient{}

	log = ExecuteSurplusCommand(context.Background(), log, writer)

	if !log.CommandSent || log.DryRun || log.ErrorMessage != nil {
		t.Fatalf("log = %+v, want command sent, non-dry-run, no error", log)
	}
	want := []string{"tou:false", "ac:800", "reserve:79"}
	if strings.Join(writer.commands, ",") != strings.Join(want, ",") {
		t.Fatalf("commands = %v, want %v", writer.commands, want)
	}
}

func TestExecuteSurplusCommandRecordsErrorAfterPartialSend(t *testing.T) {
	now := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	log := EvaluateSurplusCommandGuard(SurplusCommandGuardInput{
		Status:                 surplusGuardStatus(now),
		EnableRealControl:      true,
		AutoControl:            true,
		ConfirmEcoFlowWrite:    confirmEcoFlowWriteValue,
		RealControlTrialActive: true,
	}, DefaultSettings())
	writer := &stubSurplusWriteClient{reserveErr: errors.New("reserve rejected")}

	log = ExecuteSurplusCommand(context.Background(), log, writer)

	if !log.CommandSent {
		t.Fatal("CommandSent = false, want true because AC command was sent before reserve failure")
	}
	if log.DryRun {
		t.Fatal("DryRun = true, want false after execution attempt")
	}
	if log.WouldWrite {
		t.Fatal("WouldWrite = true, want false after execution error")
	}
	if log.ErrorMessage == nil || !strings.Contains(*log.ErrorMessage, "reserve rejected") {
		t.Fatalf("ErrorMessage = %v, want reserve rejected", log.ErrorMessage)
	}
}

func TestEvaluateSurplusCommandGuardAllowsRetryAfterErroredCandidateInterval(t *testing.T) {
	now := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	status := surplusGuardStatus(now)
	errorMessage := "set backup reserve SOC: transient"
	previous := domain.SurplusControlCommandLog{
		MeasuredAt:         now.Add(-30 * time.Second),
		CommandKind:        "mixed",
		CommandFingerprint: "kind=mixed;ac=800;reserve=79;adjust_ac=true;set_reserve=true;disable_modes=true;enable_tou=false",
		CommandSent:        true,
		WouldWrite:         false,
		ErrorMessage:       &errorMessage,
	}

	suppressed := EvaluateSurplusCommandGuard(SurplusCommandGuardInput{
		Status:                 status,
		EnableRealControl:      true,
		AutoControl:            true,
		ConfirmEcoFlowWrite:    confirmEcoFlowWriteValue,
		RealControlTrialActive: true,
		Previous:               &previous,
	}, DefaultSettings())
	if suppressed.WouldWrite || suppressed.SuppressedReason != "command retry suppressed after previous error" {
		t.Fatalf("suppressed = %+v, want retry interval suppression", suppressed)
	}

	previous.MeasuredAt = now.Add(-2 * time.Minute)
	retry := EvaluateSurplusCommandGuard(SurplusCommandGuardInput{
		Status:                 status,
		EnableRealControl:      true,
		AutoControl:            true,
		ConfirmEcoFlowWrite:    confirmEcoFlowWriteValue,
		RealControlTrialActive: true,
		Previous:               &previous,
	}, DefaultSettings())
	if !retry.WouldWrite {
		t.Fatalf("WouldWrite = false, want retry after interval: %+v", retry)
	}
}

func TestEvaluateSurplusCommandGuardRequiresModeStatusForModeActions(t *testing.T) {
	now := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	status := surplusGuardStatus(now)
	status.ScheduledEnabled = nil

	log := EvaluateSurplusCommandGuard(SurplusCommandGuardInput{
		Status:                 status,
		EnableRealControl:      true,
		AutoControl:            true,
		ConfirmEcoFlowWrite:    confirmEcoFlowWriteValue,
		RealControlTrialActive: true,
	}, DefaultSettings())

	if log.WouldWrite {
		t.Fatal("WouldWrite = true, want false")
	}
	if !strings.Contains(log.SuppressedReason, "mode status unavailable") || !strings.Contains(log.ModeGuardReason, "scheduled") {
		t.Fatalf("SuppressedReason=%q ModeGuardReason=%q, want missing scheduled", log.SuppressedReason, log.ModeGuardReason)
	}
}

func TestEvaluateSurplusCommandGuardRequiresExpectedModeState(t *testing.T) {
	now := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)

	t.Run("enable TOU when TOU is already enabled", func(t *testing.T) {
		status := surplusGuardStatus(now)
		tou := true
		status.TOUModeEnabled = &tou
		status.SelfPoweredEnabled = boolPtr(false)
		status.ScheduledEnabled = boolPtr(false)
		status.IntelligentEnabled = boolPtr(false)
		status.SurplusPlan = &domain.SurplusPlan{
			StrategyState:       "RECOVERING",
			ShouldEnableTOUMode: true,
			ActionSummary:       "TOUをONに戻す",
		}

		log := EvaluateSurplusCommandGuard(SurplusCommandGuardInput{
			Status:                 status,
			EnableRealControl:      true,
			AutoControl:            true,
			ConfirmEcoFlowWrite:    confirmEcoFlowWriteValue,
			RealControlTrialActive: true,
		}, DefaultSettings())
		if log.WouldWrite {
			t.Fatal("WouldWrite = true, want false")
		}
		if log.SuppressedReason != "TOU mode is already enabled" {
			t.Fatalf("SuppressedReason = %q, want TOU already enabled", log.SuppressedReason)
		}
	})

	t.Run("disable modes when already disabled", func(t *testing.T) {
		status := surplusGuardStatus(now)
		status.TOUModeEnabled = boolPtr(false)
		status.SelfPoweredEnabled = boolPtr(false)
		status.ScheduledEnabled = boolPtr(false)
		status.IntelligentEnabled = boolPtr(false)
		status.SurplusPlan.ShouldDisableEnergyModes = true

		log := EvaluateSurplusCommandGuard(SurplusCommandGuardInput{
			Status:                 status,
			EnableRealControl:      true,
			AutoControl:            true,
			ConfirmEcoFlowWrite:    confirmEcoFlowWriteValue,
			RealControlTrialActive: true,
		}, DefaultSettings())
		if log.WouldWrite {
			t.Fatal("WouldWrite = true, want false")
		}
		if log.SuppressedReason != "energy modes already disabled" {
			t.Fatalf("SuppressedReason = %q, want already disabled", log.SuppressedReason)
		}
	})
}

func TestEvaluateSurplusCommandGuardSuppressesDuplicateAndInterval(t *testing.T) {
	now := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	status := surplusGuardStatus(now)
	first := EvaluateSurplusCommandGuard(SurplusCommandGuardInput{
		Status:                 status,
		EnableRealControl:      true,
		AutoControl:            true,
		ConfirmEcoFlowWrite:    confirmEcoFlowWriteValue,
		RealControlTrialActive: true,
	}, DefaultSettings())

	duplicate := EvaluateSurplusCommandGuard(SurplusCommandGuardInput{
		Status:                 status,
		EnableRealControl:      true,
		AutoControl:            true,
		ConfirmEcoFlowWrite:    confirmEcoFlowWriteValue,
		RealControlTrialActive: true,
		Previous:               &first,
	}, DefaultSettings())
	if duplicate.WouldWrite || duplicate.SuppressedReason != "duplicate command candidate" {
		t.Fatalf("duplicate = %+v, want duplicate suppression", duplicate)
	}

	status.SurplusPlan.RecommendedACChargeLimitW = 900
	status.UpdatedAt = now.Add(30 * time.Second)
	interval := EvaluateSurplusCommandGuard(SurplusCommandGuardInput{
		Status:                 status,
		EnableRealControl:      true,
		AutoControl:            true,
		ConfirmEcoFlowWrite:    confirmEcoFlowWriteValue,
		RealControlTrialActive: true,
		Previous:               &first,
	}, DefaultSettings())
	if interval.WouldWrite || interval.SuppressedReason != "command suppressed by minimum interval" {
		t.Fatalf("interval = %+v, want interval suppression", interval)
	}
}

func TestEvaluateSurplusCommandGuardIgnoresPreviousNonWriteLogForSuppression(t *testing.T) {
	now := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	status := surplusGuardStatus(now)
	previous := domain.SurplusControlCommandLog{
		MeasuredAt:          now.Add(-30 * time.Second),
		CommandKind:         "none",
		CommandFingerprint:  "kind=none;ac=-;reserve=-;adjust_ac=false;set_reserve=false;disable_modes=false;enable_tou=false",
		SuppressedReason:    "no command candidate",
		WouldWrite:          false,
		ShouldEnableTOUMode: false,
	}

	log := EvaluateSurplusCommandGuard(SurplusCommandGuardInput{
		Status:                 status,
		EnableRealControl:      true,
		AutoControl:            true,
		ConfirmEcoFlowWrite:    confirmEcoFlowWriteValue,
		RealControlTrialActive: true,
		Previous:               &previous,
	}, DefaultSettings())
	if !log.WouldWrite {
		t.Fatalf("WouldWrite = false, want true when previous log was not a write candidate: %+v", log)
	}
}

func TestEvaluateSurplusCommandGuardKeepsSuppressingAfterSuppressedLogWhenPreviousCandidateIsUsed(t *testing.T) {
	now := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	status := surplusGuardStatus(now)
	first := EvaluateSurplusCommandGuard(SurplusCommandGuardInput{
		Status:                 status,
		EnableRealControl:      true,
		AutoControl:            true,
		ConfirmEcoFlowWrite:    confirmEcoFlowWriteValue,
		RealControlTrialActive: true,
	}, DefaultSettings())
	if !first.WouldWrite {
		t.Fatalf("first WouldWrite = false, want true: %+v", first)
	}

	status.UpdatedAt = now.Add(30 * time.Second)
	second := EvaluateSurplusCommandGuard(SurplusCommandGuardInput{
		Status:                 status,
		EnableRealControl:      true,
		AutoControl:            true,
		ConfirmEcoFlowWrite:    confirmEcoFlowWriteValue,
		RealControlTrialActive: true,
		Previous:               &first,
	}, DefaultSettings())
	if second.WouldWrite || second.SuppressedReason != "duplicate command candidate" {
		t.Fatalf("second = %+v, want duplicate suppression", second)
	}

	status.UpdatedAt = now.Add(60 * time.Second)
	third := EvaluateSurplusCommandGuard(SurplusCommandGuardInput{
		Status:                 status,
		EnableRealControl:      true,
		AutoControl:            true,
		ConfirmEcoFlowWrite:    confirmEcoFlowWriteValue,
		RealControlTrialActive: true,
		Previous:               &first,
	}, DefaultSettings())
	if third.WouldWrite || third.SuppressedReason != "duplicate command candidate" {
		t.Fatalf("third = %+v, want duplicate suppression based on latest write candidate, not suppressed log", third)
	}
}

func TestEvaluateSurplusCommandGuardFingerprintIncludesActionSet(t *testing.T) {
	now := time.Date(2026, 5, 20, 10, 0, 0, 0, time.UTC)
	status := surplusGuardStatus(now)
	acOnly := status
	acOnly.SurplusPlan = &domain.SurplusPlan{
		StrategyState:             "CHARGING",
		RecommendedACChargeLimitW: 800,
		ShouldAdjustACChargeLimit: true,
		ActionSummary:             "AC",
	}
	previous := EvaluateSurplusCommandGuard(SurplusCommandGuardInput{
		Status:                 acOnly,
		EnableRealControl:      true,
		AutoControl:            true,
		ConfirmEcoFlowWrite:    confirmEcoFlowWriteValue,
		RealControlTrialActive: true,
	}, DefaultSettings())

	mixed := EvaluateSurplusCommandGuard(SurplusCommandGuardInput{
		Status:                 status,
		EnableRealControl:      true,
		AutoControl:            true,
		ConfirmEcoFlowWrite:    confirmEcoFlowWriteValue,
		RealControlTrialActive: true,
		Previous: &domain.SurplusControlCommandLog{
			MeasuredAt:         now.Add(-2 * time.Minute),
			CommandKind:        previous.CommandKind,
			CommandFingerprint: previous.CommandFingerprint,
			WouldWrite:         true,
		},
	}, DefaultSettings())
	if !mixed.WouldWrite {
		t.Fatalf("WouldWrite = false, want true for different action set: %+v", mixed)
	}
	if mixed.CommandFingerprint == previous.CommandFingerprint {
		t.Fatalf("fingerprint did not include action set: %q", mixed.CommandFingerprint)
	}
}

func surplusGuardStatus(now time.Time) domain.Status {
	reserve := 79
	tou := false
	selfPowered := true
	scheduled := false
	intelligent := false
	return domain.Status{
		GridW:              -800,
		ImportW:            0,
		ExportW:            800,
		BatterySoc:         77,
		BatteryInputW:      740,
		BatteryOutputW:     250,
		ACChargeLimitW:     500,
		BackupReserveSoc:   &reserve,
		TOUModeEnabled:     &tou,
		SelfPoweredEnabled: &selfPowered,
		ScheduledEnabled:   &scheduled,
		IntelligentEnabled: &intelligent,
		UpdatedAt:          now,
		SurplusPlan: &domain.SurplusPlan{
			StrategyState:               "CHARGING",
			RecommendedACChargeLimitW:   800,
			RecommendedBackupReserveSoc: &reserve,
			ShouldAdjustACChargeLimit:   true,
			ShouldRaiseBackupReserve:    true,
			ShouldDisableEnergyModes:    true,
			ActionSummary:               "AC充電上限を800Wへ設定",
			Reason:                      "surplus tracking condition met",
		},
	}
}

func boolPtr(value bool) *bool {
	return &value
}

type stubSurplusWriteClient struct {
	commands   []string
	acErr      error
	reserveErr error
	touErr     error
}

func (c *stubSurplusWriteClient) SetACChargePower(_ context.Context, watts int) error {
	if c.acErr != nil {
		return c.acErr
	}
	c.commands = append(c.commands, "ac:"+formatInt(watts))
	return nil
}

func (c *stubSurplusWriteClient) SetBackupReserveSoc(_ context.Context, percent int) error {
	if c.reserveErr != nil {
		return c.reserveErr
	}
	c.commands = append(c.commands, "reserve:"+formatInt(percent))
	return nil
}

func (c *stubSurplusWriteClient) SetTOUMode(_ context.Context, enabled bool) error {
	if c.touErr != nil {
		return c.touErr
	}
	c.commands = append(c.commands, "tou:"+formatBool(enabled))
	return nil
}

func formatInt(value int) string {
	return fmt.Sprintf("%d", value)
}

func formatBool(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
