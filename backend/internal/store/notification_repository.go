package store

import (
	"context"
	"database/sql"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

type NotificationRepository struct {
	db *sql.DB
}

func NewNotificationRepository(db *sql.DB) *NotificationRepository {
	return &NotificationRepository{db: db}
}

func (r *NotificationRepository) InsertNotificationLog(ctx context.Context, log domain.NotificationLog) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO notification_logs (
		measured_at, kind, fingerprint, severity, message, reason,
		export_w, battery_soc, ac_charge_limit_w, sent, error_message,
		consecutive_hits, created_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		log.MeasuredAt.Format(time.RFC3339Nano),
		log.Kind,
		log.Fingerprint,
		log.Severity,
		log.Message,
		log.Reason,
		log.ExportW,
		log.BatterySoc,
		log.ACChargeLimitW,
		boolToInt(log.Sent),
		nullableString(log.ErrorMessage),
		log.ConsecutiveHits,
		log.CreatedAt.Format(time.RFC3339Nano),
	)
	return err
}

func (r *NotificationRepository) LatestNotificationLog(ctx context.Context, kind string, fingerprint string) (*domain.NotificationLog, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT
		id, measured_at, kind, fingerprint, severity, message, reason,
		export_w, battery_soc, ac_charge_limit_w, sent, error_message,
		consecutive_hits, created_at
		FROM notification_logs
		WHERE kind = ? AND fingerprint = ?
		ORDER BY created_at DESC, id DESC
		LIMIT 1`, kind, fingerprint)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	logs, err := scanNotificationLogs(rows, 1)
	if err != nil {
		return nil, err
	}
	if len(logs) == 0 {
		return nil, nil
	}
	return &logs[0], nil
}

func scanNotificationLogs(rows *sql.Rows, capacity int) ([]domain.NotificationLog, error) {
	logs := make([]domain.NotificationLog, 0, capacity)
	for rows.Next() {
		var log domain.NotificationLog
		var measuredAt, createdAt string
		var sent int
		var errorMessage sql.NullString
		if err := rows.Scan(
			&log.ID,
			&measuredAt,
			&log.Kind,
			&log.Fingerprint,
			&log.Severity,
			&log.Message,
			&log.Reason,
			&log.ExportW,
			&log.BatterySoc,
			&log.ACChargeLimitW,
			&sent,
			&errorMessage,
			&log.ConsecutiveHits,
			&createdAt,
		); err != nil {
			return nil, err
		}
		parsedMeasuredAt, err := parseTime(measuredAt)
		if err != nil {
			return nil, err
		}
		parsedCreatedAt, err := parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		log.MeasuredAt = parsedMeasuredAt
		log.CreatedAt = parsedCreatedAt
		log.Sent = sent != 0
		if errorMessage.Valid {
			log.ErrorMessage = &errorMessage.String
		}
		logs = append(logs, log)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return logs, nil
}
