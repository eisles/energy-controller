package notify

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

type fakeNotifier struct {
	err   error
	sends int
	text  string
}

func (n *fakeNotifier) Send(_ context.Context, message Message) error {
	n.sends++
	n.text = message.Text
	return n.err
}

func TestEvaluateManualChargeAlertRequiresSurplusAndEcoFlowLimit(t *testing.T) {
	settings := testManualChargeAlertSettings()
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)

	if decision := EvaluateManualChargeAlert(domain.Status{ExportW: 699, BatterySoc: 99, ACChargeLimitW: 1500}, settings, nil, 2, now); decision.Candidate {
		t.Fatalf("Candidate = true for low export")
	}
	if decision := EvaluateManualChargeAlert(domain.Status{ExportW: 900, BatterySoc: 80, ACChargeLimitW: 1000}, settings, nil, 2, now); decision.Candidate {
		t.Fatalf("Candidate = true without SOC or AC limit")
	}
}

func TestEvaluateManualChargeAlertRequiresConsecutiveHits(t *testing.T) {
	settings := testManualChargeAlertSettings()
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)
	status := domain.Status{ExportW: 900, BatterySoc: 96, ACChargeLimitW: 1500, UpdatedAt: now}

	decision := EvaluateManualChargeAlert(status, settings, nil, 1, now)
	if !decision.Candidate {
		t.Fatal("Candidate = false, want true")
	}
	if decision.ShouldNotify {
		t.Fatal("ShouldNotify = true before consecutive threshold")
	}
	if decision.NextConsecutive != 2 {
		t.Fatalf("NextConsecutive = %d, want 2", decision.NextConsecutive)
	}

	decision = EvaluateManualChargeAlert(status, settings, nil, 2, now)
	if !decision.ShouldNotify {
		t.Fatal("ShouldNotify = false at consecutive threshold")
	}
}

func TestEvaluateManualChargeAlertAppliesCooldown(t *testing.T) {
	settings := testManualChargeAlertSettings()
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)
	status := domain.Status{ExportW: 900, BatterySoc: 96, ACChargeLimitW: 1500, UpdatedAt: now}
	latest := &domain.NotificationLog{
		Kind:        ManualChargeAlertKind,
		Fingerprint: manualChargeAlertFingerprint(settings),
		CreatedAt:   now.Add(-10 * time.Minute),
	}

	decision := EvaluateManualChargeAlert(status, settings, latest, 2, now)
	if decision.ShouldNotify {
		t.Fatal("ShouldNotify = true during cooldown")
	}
}

func TestManualChargeAlertServiceSendsAndRecordsFailure(t *testing.T) {
	settings := testManualChargeAlertSettings()
	now := time.Date(2026, 5, 23, 12, 0, 0, 0, time.UTC)
	status := domain.Status{ExportW: 900, BatterySoc: 96, ACChargeLimitW: 1500, UpdatedAt: now}
	notifier := &fakeNotifier{err: errors.New("slack unavailable")}
	service := NewManualChargeAlertService(settings, notifier)

	for i := 0; i < 2; i++ {
		log, err := service.EvaluateAndSend(context.Background(), status, nil, now)
		if err != nil || log != nil {
			t.Fatalf("round %d log,err = %v,%v; want nil,nil before threshold", i, log, err)
		}
	}
	log, err := service.EvaluateAndSend(context.Background(), status, nil, now)
	if err == nil {
		t.Fatal("err = nil, want send error")
	}
	if log == nil || log.ErrorMessage == nil || *log.ErrorMessage != "slack unavailable" {
		t.Fatalf("log error = %#v, want slack unavailable", log)
	}
	if notifier.sends != 1 {
		t.Fatalf("sends = %d, want 1", notifier.sends)
	}
}

func testManualChargeAlertSettings() ManualChargeAlertSettings {
	return ManualChargeAlertSettings{
		Enabled:          true,
		ExportThresholdW: 700,
		SocThreshold:     95,
		ConsecutiveCount: 3,
		Cooldown:         30 * time.Minute,
		MaxChargeW:       1500,
	}
}
