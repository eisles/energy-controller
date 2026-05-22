package notify

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

const ManualChargeAlertKind = "manual_charge_surplus"

type ManualChargeAlertSettings struct {
	Enabled          bool
	ExportThresholdW int
	SocThreshold     int
	ConsecutiveCount int
	Cooldown         time.Duration
	MaxChargeW       int
}

type ManualChargeAlertDecision struct {
	Candidate       bool
	ShouldNotify    bool
	NextConsecutive int
	Fingerprint     string
	Severity        string
	Message         string
	Reason          string
}

type ManualChargeAlertService struct {
	settings    ManualChargeAlertSettings
	notifier    Notifier
	consecutive int
}

func NewManualChargeAlertService(settings ManualChargeAlertSettings, notifier Notifier) *ManualChargeAlertService {
	if notifier == nil {
		notifier = NoopNotifier{}
	}
	settings = normalizeManualChargeAlertSettings(settings)
	return &ManualChargeAlertService{settings: settings, notifier: notifier}
}

func (s *ManualChargeAlertService) Fingerprint() string {
	return manualChargeAlertFingerprint(s.settings)
}

func (s *ManualChargeAlertService) EvaluateAndSend(ctx context.Context, status domain.Status, latest *domain.NotificationLog, now time.Time) (*domain.NotificationLog, error) {
	if now.IsZero() {
		now = time.Now()
	}
	decision := EvaluateManualChargeAlert(status, s.settings, latest, s.consecutive, now)
	s.consecutive = decision.NextConsecutive
	if !decision.ShouldNotify {
		return nil, nil
	}
	measuredAt := status.UpdatedAt
	if measuredAt.IsZero() {
		measuredAt = now
	}
	log := &domain.NotificationLog{
		MeasuredAt:      measuredAt,
		Kind:            ManualChargeAlertKind,
		Fingerprint:     decision.Fingerprint,
		Severity:        decision.Severity,
		Message:         decision.Message,
		Reason:          decision.Reason,
		ExportW:         status.ExportW,
		BatterySoc:      status.BatterySoc,
		ACChargeLimitW:  status.ACChargeLimitW,
		ConsecutiveHits: decision.NextConsecutive,
		CreatedAt:       now,
	}
	err := s.notifier.Send(ctx, Message{Text: decision.Message})
	if err != nil {
		message := err.Error()
		log.ErrorMessage = &message
		return log, err
	}
	log.Sent = true
	return log, nil
}

func EvaluateManualChargeAlert(status domain.Status, settings ManualChargeAlertSettings, latest *domain.NotificationLog, currentConsecutive int, now time.Time) ManualChargeAlertDecision {
	settings = normalizeManualChargeAlertSettings(settings)
	if !settings.Enabled {
		return ManualChargeAlertDecision{}
	}
	reason, ok := manualChargeAlertReason(status, settings)
	if !ok {
		return ManualChargeAlertDecision{}
	}
	nextConsecutive := currentConsecutive + 1
	decision := ManualChargeAlertDecision{
		Candidate:       true,
		NextConsecutive: nextConsecutive,
		Fingerprint:     manualChargeAlertFingerprint(settings),
		Severity:        "warning",
		Reason:          reason,
		Message:         manualChargeAlertMessage(status, reason),
	}
	if nextConsecutive < settings.ConsecutiveCount {
		return decision
	}
	if latest != nil && latest.Kind == ManualChargeAlertKind && latest.Fingerprint == decision.Fingerprint && settings.Cooldown > 0 && now.Sub(latest.CreatedAt) < settings.Cooldown {
		return decision
	}
	decision.ShouldNotify = true
	return decision
}

func manualChargeAlertReason(status domain.Status, settings ManualChargeAlertSettings) (string, bool) {
	if status.ExportW < settings.ExportThresholdW {
		return "", false
	}
	reasons := make([]string, 0, 3)
	if settings.MaxChargeW > 0 && status.ACChargeLimitW >= settings.MaxChargeW {
		reasons = append(reasons, fmt.Sprintf("AC充電上限が%dWで上限に到達", status.ACChargeLimitW))
	}
	if status.BatterySoc >= settings.SocThreshold {
		reasons = append(reasons, fmt.Sprintf("EcoFlow SOCが%d%%で高い", status.BatterySoc))
	}
	if status.SurplusPlan != nil && settings.MaxChargeW > 0 && status.SurplusPlan.RecommendedACChargeLimitW >= settings.MaxChargeW {
		reasons = append(reasons, fmt.Sprintf("余剰追従プランの推奨AC充電が%dWで上限に到達", status.SurplusPlan.RecommendedACChargeLimitW))
	}
	if len(reasons) == 0 {
		return "", false
	}
	return strings.Join(reasons, " / "), true
}

func manualChargeAlertMessage(status domain.Status, reason string) string {
	measuredAt := status.UpdatedAt
	timestamp := "unknown"
	if !measuredAt.IsZero() {
		timestamp = measuredAt.Format("2006-01-02 15:04:05")
	}
	return fmt.Sprintf("売電が多い状態です。別バッテリーの手動充電を検討してください。\n\n売電: %dW\nEcoFlow SOC: %d%%\nAC充電上限: %dW\n理由: %s\n時刻: %s", status.ExportW, status.BatterySoc, status.ACChargeLimitW, reason, timestamp)
}

func manualChargeAlertFingerprint(settings ManualChargeAlertSettings) string {
	return fmt.Sprintf("manual-charge|export>=%d|soc>=%d|max-ac=%d", settings.ExportThresholdW, settings.SocThreshold, settings.MaxChargeW)
}

func normalizeManualChargeAlertSettings(settings ManualChargeAlertSettings) ManualChargeAlertSettings {
	if settings.ExportThresholdW <= 0 {
		settings.ExportThresholdW = 700
	}
	if settings.SocThreshold <= 0 {
		settings.SocThreshold = 95
	}
	if settings.ConsecutiveCount <= 0 {
		settings.ConsecutiveCount = 3
	}
	if settings.Cooldown < 0 {
		settings.Cooldown = 0
	}
	return settings
}
