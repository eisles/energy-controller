package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Message struct {
	Text string
}

type Notifier interface {
	Send(ctx context.Context, message Message) error
}

type NoopNotifier struct{}

func (NoopNotifier) Send(context.Context, Message) error {
	return nil
}

type SlackWebhookNotifier struct {
	WebhookURL string
	Client     *http.Client
}

func (n SlackWebhookNotifier) Send(ctx context.Context, message Message) error {
	if n.WebhookURL == "" {
		return fmt.Errorf("slack webhook URL is not configured")
	}
	client := n.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	body, err := json.Marshal(map[string]string{"text": message.Text})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.WebhookURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return fmt.Errorf("slack webhook returned status %d", resp.StatusCode)
	}
	return nil
}
