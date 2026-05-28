package notify

import (
	"context"
	"fmt"
	"time"

	"github.com/eisles/energy-controller/backend/internal/control"
	"github.com/eisles/energy-controller/backend/internal/domain"
)

const Pro3ACOutputOffAlertKind = "pro3_ac_output_off"

type Pro3ACOutputAlertSettings struct {
	Enabled  bool
	Cooldown time.Duration
}

type Pro3ACOutputAlertService struct {
	settings Pro3ACOutputAlertSettings
	notifier Notifier
}

func NewPro3ACOutputAlertService(settings Pro3ACOutputAlertSettings, notifier Notifier) *Pro3ACOutputAlertService {
	if notifier == nil {
		notifier = NoopNotifier{}
	}
	if settings.Cooldown <= 0 {
		settings.Cooldown = 30 * time.Minute
	}
	return &Pro3ACOutputAlertService{settings: settings, notifier: notifier}
}

func (s *Pro3ACOutputAlertService) Fingerprint() string {
	return "pro3-ac-output-off-memory"
}

func (s *Pro3ACOutputAlertService) EvaluateAndSend(ctx context.Context, event domain.Pro3ACOutputEvent, latest *domain.NotificationLog, now time.Time) (*domain.NotificationLog, error) {
	if !s.settings.Enabled || event.EventType != control.Pro3ACOutputOffEventType {
		return nil, nil
	}
	if now.IsZero() {
		now = time.Now()
	}
	if latest != nil && latest.Kind == Pro3ACOutputOffAlertKind && latest.Fingerprint == s.Fingerprint() && now.Sub(latest.CreatedAt) < s.settings.Cooldown {
		return nil, nil
	}
	log := &domain.NotificationLog{
		MeasuredAt:     event.MeasuredAt,
		Kind:           Pro3ACOutputOffAlertKind,
		Fingerprint:    s.Fingerprint(),
		Severity:       "critical",
		Message:        pro3ACOutputAlertMessage(event),
		Reason:         event.Message,
		ExportW:        event.ExportW,
		BatterySoc:     event.BatterySoc,
		ACChargeLimitW: event.ACChargeLimitW,
		CreatedAt:      now,
	}
	err := s.notifier.Send(ctx, Message{Text: log.Message})
	if err != nil {
		message := err.Error()
		log.ErrorMessage = &message
		return log, err
	}
	log.Sent = true
	return log, nil
}

func pro3ACOutputAlertMessage(event domain.Pro3ACOutputEvent) string {
	previousCommand := "なし"
	if event.PreviousCommandKind != "" {
		previousCommand = fmt.Sprintf("%s sent=%t wouldWrite=%t", event.PreviousCommandKind, event.PreviousCommandSent, event.PreviousCommandWouldWrite)
	}
	return fmt.Sprintf("DELTA Pro 3 のAC出力OFF履歴を検知しました。\n\n時刻: %s\nSOC: %d%%\nAC入力: %dW\nAC出力: %dW\nAC充電上限: %dW\n直前制御: %s\n理由: %s",
		event.MeasuredAt.Format("2006-01-02 15:04:05"),
		event.BatterySoc,
		event.BatteryInputW,
		event.BatteryOutputW,
		event.ACChargeLimitW,
		previousCommand,
		event.Message,
	)
}
