package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

func TestNightChargeSummarySeparatesPlannedSuccessfulCommandAndActualSOC(t *testing.T) {
	db := openTestDB(t)
	repo := NewNightChargeSummaryRepository(db)
	jst := time.FixedZone("JST", 9*60*60)
	sessionStart := time.Date(2026, 8, 3, 21, 0, 0, 0, jst)

	insertNightChargeSummaryLearningLog(t, db, sessionStart.Add(time.Hour), "NIGHT_PLAN_READY", 80, "plan=80", false, nil)
	insertNightChargeSummaryLearningLog(t, db, sessionStart.Add(2*time.Hour+10*time.Minute), "NIGHT_CHARGE_WINDOW", 75, "reserve=75", true, nil)
	insertNightChargeSummaryLearningLog(t, db, sessionStart.Add(3*time.Hour), "NIGHT_CHARGE_WINDOW", 65, "read-only-candidate=65", false, nil)
	latestSuccessfulAt := sessionStart.Add(4 * time.Hour)
	insertNightChargeSummaryLearningLog(t, db, latestSuccessfulAt, "NIGHT_CHARGE_WINDOW", 75, "reserve=75", true, nil)
	insertNightChargeSummaryLearningLog(t, db, sessionStart.Add(10*time.Hour), "NIGHT_RECOVER", 99, "reserve=99", true, nil)
	insertNightChargeSummaryLearningDensePowerSamples(
		t,
		db,
		sessionStart.Add(2*time.Hour),
		sessionStart.Add(10*time.Hour),
		72,
	)

	got, err := repo.buildSummary(context.Background(), sessionStart.Add(20*time.Hour), "2026-08-03")
	if err != nil {
		t.Fatalf("buildSummary failed: %v", err)
	}
	if got.PlannedTargetSoc == nil || *got.PlannedTargetSoc != 80 {
		t.Fatalf("PlannedTargetSoc = %v, want 80", got.PlannedTargetSoc)
	}
	if got.SuccessfulCommandTargetSoc == nil || *got.SuccessfulCommandTargetSoc != 75 {
		t.Fatalf("SuccessfulCommandTargetSoc = %v, want 75", got.SuccessfulCommandTargetSoc)
	}
	if got.SuccessfulCommandAt == nil || !got.SuccessfulCommandAt.Equal(latestSuccessfulAt) {
		t.Fatalf("SuccessfulCommandAt = %v, want %v", got.SuccessfulCommandAt, latestSuccessfulAt)
	}
	if got.SuccessfulCommandFingerprint != "reserve=75" {
		t.Fatalf("SuccessfulCommandFingerprint = %q, want reserve=75", got.SuccessfulCommandFingerprint)
	}
	if got.NightEndSoc == nil || *got.NightEndSoc != 72 {
		t.Fatalf("NightEndSoc = %v, want 72", got.NightEndSoc)
	}
	if got.MorningTargetSocGap == nil || *got.MorningTargetSocGap != -8 {
		t.Fatalf("MorningTargetSocGap = %v, want planned gap -8", got.MorningTargetSocGap)
	}
	if got.ExecutionTargetSocGap == nil || *got.ExecutionTargetSocGap != -3 {
		t.Fatalf("ExecutionTargetSocGap = %v, want execution gap -3", got.ExecutionTargetSocGap)
	}
	if got.NightCommandSentCount != 2 || got.NightCommandErrorCount != 0 {
		t.Fatalf("sent/error counts = %d/%d, want 2/0", got.NightCommandSentCount, got.NightCommandErrorCount)
	}
	if got.NightCommandFingerprintChanged {
		t.Fatal("NightCommandFingerprintChanged = true, want false because read-only and 07:00 logs are excluded")
	}
	if got.NightPowerSampleCoverageRatio != 1 || got.NightPowerSampleMaxGapSeconds != time.Hour.Seconds() {
		t.Fatalf(
			"power sample coverage/max gap = %v/%v, want 1/%v",
			got.NightPowerSampleCoverageRatio,
			got.NightPowerSampleMaxGapSeconds,
			time.Hour.Seconds(),
		)
	}
	if got.SettingsFingerprintVerified || got.DeviceFingerprintVerified {
		t.Fatalf(
			"settings/device fingerprint verified = %t/%t, want false/false",
			got.SettingsFingerprintVerified,
			got.DeviceFingerprintVerified,
		)
	}
	if got.TargetLearningEligible {
		t.Fatal("TargetLearningEligible = true, want fail-closed false while fingerprints are unverifiable")
	}
	for _, want := range []string{
		"settings fingerprint cannot be verified because it is not recorded",
		"device fingerprint cannot be verified because it is not recorded",
	} {
		if !strings.Contains(got.TargetLearningExclusionReason, want) {
			t.Fatalf("TargetLearningExclusionReason = %q, want contains %q", got.TargetLearningExclusionReason, want)
		}
	}
	if strings.Contains(got.TargetLearningExclusionReason, "execution target SOC gap") {
		t.Fatalf("TargetLearningExclusionReason = %q, gap -3 must remain within tolerance", got.TargetLearningExclusionReason)
	}
	if got.MorningStatus != nightSummaryUndercharged {
		t.Fatalf("MorningStatus = %q, want undercharged to remain in operational status", got.MorningStatus)
	}

	payload, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	for _, field := range []string{
		`"successfulCommandTargetSoc":75`,
		`"executionTargetSocGap":-3`,
		`"nightPowerSampleCoverageRatio":1`,
		`"nightPowerSampleMaxGapSeconds":3600`,
		`"settingsFingerprintVerified":false`,
		`"deviceFingerprintVerified":false`,
		`"targetLearningEligible":false`,
	} {
		if !strings.Contains(string(payload), field) {
			t.Fatalf("JSON = %s, want contains %s", payload, field)
		}
	}
}

func TestNightChargeSummaryExplainsTargetLearningExclusions(t *testing.T) {
	jst := time.FixedZone("JST", 9*60*60)
	tests := []struct {
		name                    string
		activities              []nightChargeSummaryLearningActivity
		insertActualSOC         bool
		wantSuccessfulTargetSoc *int
		wantSentCount           int
		wantErrorCount          int
		wantFingerprintChanged  bool
		wantReasonParts         []string
	}{
		{
			name: "no successful command",
			activities: []nightChargeSummaryLearningActivity{
				{offset: 2 * time.Hour, targetSoc: 75, fingerprint: "reserve=75"},
			},
			insertActualSOC: true,
			wantReasonParts: []string{"no fully successful night charge command"},
		},
		{
			name: "AC and TOU command does not prove target execution",
			activities: []nightChargeSummaryLearningActivity{
				{offset: 2 * time.Hour, targetSoc: 75, fingerprint: "tou=on|ac=1500", commandSent: true},
			},
			insertActualSOC: true,
			wantSentCount:   1,
			wantReasonParts: []string{"no fully successful night charge command with a matching reserve target"},
		},
		{
			name: "command error after a successful command",
			activities: []nightChargeSummaryLearningActivity{
				{offset: 2 * time.Hour, targetSoc: 75, fingerprint: "reserve=75", commandSent: true},
				{offset: 4 * time.Hour, targetSoc: 76, fingerprint: "reserve=75", commandSent: true, commandError: "set backup reserve failed"},
			},
			insertActualSOC:         true,
			wantSuccessfulTargetSoc: intTestPtr(75),
			wantSentCount:           2,
			wantErrorCount:          1,
			wantReasonParts:         []string{"1 night charge command error(s) occurred"},
		},
		{
			name: "command fingerprint changed",
			activities: []nightChargeSummaryLearningActivity{
				{offset: 2 * time.Hour, targetSoc: 75, fingerprint: "reserve=75", commandSent: true},
				{offset: 4 * time.Hour, targetSoc: 76, fingerprint: "reserve=76", commandSent: true},
			},
			insertActualSOC:         true,
			wantSuccessfulTargetSoc: intTestPtr(76),
			wantSentCount:           2,
			wantFingerprintChanged:  true,
			wantReasonParts:         []string{"night charge command fingerprint changed"},
		},
		{
			name: "07:00 SOC missing",
			activities: []nightChargeSummaryLearningActivity{
				{offset: 2 * time.Hour, targetSoc: 75, fingerprint: "reserve=75", commandSent: true},
			},
			wantSuccessfulTargetSoc: intTestPtr(75),
			wantSentCount:           1,
			wantReasonParts:         []string{"07:00 SOC is missing"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openTestDB(t)
			repo := NewNightChargeSummaryRepository(db)
			sessionStart := time.Date(2026, 8, 3, 21, 0, 0, 0, jst)
			insertNightChargeSummaryLearningLog(t, db, sessionStart.Add(time.Hour), "NIGHT_PLAN_READY", 80, "plan=80", false, nil)
			for _, activity := range tt.activities {
				var commandError *string
				if activity.commandError != "" {
					commandError = &activity.commandError
				}
				insertNightChargeSummaryLearningLog(
					t,
					db,
					sessionStart.Add(activity.offset),
					"NIGHT_CHARGE_WINDOW",
					activity.targetSoc,
					activity.fingerprint,
					activity.commandSent,
					commandError,
				)
			}
			if tt.insertActualSOC {
				insertNightChargeSummaryLearningSOC(t, db, sessionStart.Add(10*time.Hour), 72)
			}

			got, err := repo.buildSummary(context.Background(), sessionStart.Add(20*time.Hour), "2026-08-03")
			if err != nil {
				t.Fatalf("buildSummary failed: %v", err)
			}
			if got.TargetLearningEligible {
				t.Fatalf("TargetLearningEligible = true, want false: %#v", got)
			}
			if !intTestPtrEqual(got.SuccessfulCommandTargetSoc, tt.wantSuccessfulTargetSoc) {
				t.Fatalf("SuccessfulCommandTargetSoc = %v, want %v", got.SuccessfulCommandTargetSoc, tt.wantSuccessfulTargetSoc)
			}
			if got.NightCommandSentCount != tt.wantSentCount || got.NightCommandErrorCount != tt.wantErrorCount {
				t.Fatalf("sent/error counts = %d/%d, want %d/%d", got.NightCommandSentCount, got.NightCommandErrorCount, tt.wantSentCount, tt.wantErrorCount)
			}
			if got.NightCommandFingerprintChanged != tt.wantFingerprintChanged {
				t.Fatalf("NightCommandFingerprintChanged = %t, want %t", got.NightCommandFingerprintChanged, tt.wantFingerprintChanged)
			}
			for _, want := range tt.wantReasonParts {
				if !strings.Contains(got.TargetLearningExclusionReason, want) {
					t.Fatalf("TargetLearningExclusionReason = %q, want contains %q", got.TargetLearningExclusionReason, want)
				}
			}
		})
	}
}

func TestNightChargeSummaryExecutionGapEligibilityThreshold(t *testing.T) {
	tests := []struct {
		name         string
		gap          int
		wantEligible bool
	}{
		{name: "minus three is within tolerance", gap: -3, wantEligible: true},
		{name: "minus four is excluded", gap: -4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			targetSoc := 75
			actualSoc := targetSoc + tt.gap
			summary := domain.NightChargeDailySummary{
				SuccessfulCommandTargetSoc:    &targetSoc,
				NightEndSoc:                   &actualSoc,
				ExecutionTargetSocGap:         &tt.gap,
				NightPowerSampleCoverageRatio: 1,
				NightPowerSampleMaxGapSeconds: time.Hour.Seconds(),
				SettingsFingerprintVerified:   true,
				DeviceFingerprintVerified:     true,
			}

			applyNightSummaryTargetLearningEligibility(&summary)

			if summary.TargetLearningEligible != tt.wantEligible {
				t.Fatalf(
					"TargetLearningEligible = %t, want %t; reason=%q",
					summary.TargetLearningEligible,
					tt.wantEligible,
					summary.TargetLearningExclusionReason,
				)
			}
			hasGapReason := strings.Contains(summary.TargetLearningExclusionReason, "execution target SOC gap")
			if hasGapReason == tt.wantEligible {
				t.Fatalf(
					"TargetLearningExclusionReason = %q, gap reason presence want %t",
					summary.TargetLearningExclusionReason,
					!tt.wantEligible,
				)
			}
		})
	}
}

func TestNightChargeSummaryOnePowerSampleIsNotLearningEligible(t *testing.T) {
	db := openTestDB(t)
	repo := NewNightChargeSummaryRepository(db)
	jst := time.FixedZone("JST", 9*60*60)
	sessionStart := time.Date(2026, 8, 3, 21, 0, 0, 0, jst)

	insertNightChargeSummaryLearningLog(t, db, sessionStart.Add(time.Hour), "NIGHT_PLAN_READY", 80, "plan=80", false, nil)
	insertNightChargeSummaryLearningLog(t, db, sessionStart.Add(2*time.Hour), "NIGHT_CHARGE_WINDOW", 75, "reserve=75", true, nil)
	insertNightChargeSummaryLearningSOC(t, db, sessionStart.Add(10*time.Hour), 72)

	got, err := repo.buildSummary(context.Background(), sessionStart.Add(20*time.Hour), "2026-08-03")
	if err != nil {
		t.Fatalf("buildSummary failed: %v", err)
	}
	if got.NightPowerSampleCoverageRatio != 0 {
		t.Fatalf("NightPowerSampleCoverageRatio = %v, want 0", got.NightPowerSampleCoverageRatio)
	}
	if got.NightPowerSampleMaxGapSeconds != 8*time.Hour.Seconds() {
		t.Fatalf(
			"NightPowerSampleMaxGapSeconds = %v, want %v",
			got.NightPowerSampleMaxGapSeconds,
			8*time.Hour.Seconds(),
		)
	}
	if got.TargetLearningEligible {
		t.Fatal("TargetLearningEligible = true, want false for one power sample")
	}
	for _, want := range []string{
		"night power sample coverage",
		"night power sample maximum gap",
	} {
		if !strings.Contains(got.TargetLearningExclusionReason, want) {
			t.Fatalf("TargetLearningExclusionReason = %q, want contains %q", got.TargetLearningExclusionReason, want)
		}
	}
}

type nightChargeSummaryLearningActivity struct {
	offset       time.Duration
	targetSoc    int
	fingerprint  string
	commandSent  bool
	commandError string
}

func insertNightChargeSummaryLearningLog(t *testing.T, db *sql.DB, measuredAt time.Time, strategyState string, targetSoc int, fingerprint string, commandSent bool, commandError *string) {
	t.Helper()
	status := domain.Status{
		BatterySoc: 50,
		UpdatedAt:  measuredAt,
		NightChargePlan: &domain.NightChargePlan{
			StrategyState:             strategyState,
			RecommendedMode:           "tou",
			RecommendedNightTargetSoc: targetSoc,
			RecommendedNightTargetKWh: float64(targetSoc) / 10,
			RequiredNightChargeKWh:    1,
			CommandFingerprint:        fingerprint,
			CommandSent:               commandSent,
			CommandError:              commandError,
			Reason:                    "night charge summary learning test",
		},
	}
	if err := NewNightChargePlanRepository(db).InsertNightChargePlanLog(context.Background(), status); err != nil {
		t.Fatalf("InsertNightChargePlanLog failed: %v", err)
	}
}

func insertNightChargeSummaryLearningSOC(t *testing.T, db *sql.DB, measuredAt time.Time, soc int) {
	t.Helper()
	_, err := db.Exec(`INSERT INTO power_logs (
		measured_at, grid_w, import_w, export_w, battery_soc,
		target_charge_w, decision_reason, mode, command_sent, created_at
	) VALUES (?, 0, 0, 0, ?, 0, 'night charge summary learning test', 'read-only', 0, ?)`,
		measuredAt.Format(time.RFC3339Nano),
		soc,
		measuredAt.Format(time.RFC3339Nano),
	)
	if err != nil {
		t.Fatalf("insert power log failed: %v", err)
	}
}

func insertNightChargeSummaryLearningDensePowerSamples(t *testing.T, db *sql.DB, start time.Time, end time.Time, soc int) {
	t.Helper()
	for measuredAt := start; !measuredAt.After(end); measuredAt = measuredAt.Add(time.Hour) {
		insertNightChargeSummaryLearningSOC(t, db, measuredAt, soc)
	}
}

func intTestPtr(value int) *int {
	return &value
}

func intTestPtrEqual(left, right *int) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
