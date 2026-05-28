package ecoflowprivate

import (
	"context"
	"crypto/tls"
	"fmt"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type MQTTTransport interface {
	Request(ctx context.Context, publishTopic string, payload []byte, replyTopics []string, timeout time.Duration) ([]MQTTMessage, error)
	Publish(ctx context.Context, topic string, payload []byte, timeout time.Duration) error
	Disconnect()
}

type MQTTMessage struct {
	Topic   string
	Payload []byte
}

type PahoTransport struct {
	client mqtt.Client
}

func NewPahoTransport(info MQTTInfo) (*PahoTransport, error) {
	if info.URL == "" || info.Port == 0 || info.Username == "" || info.Password == "" || info.ClientID == "" {
		return nil, fmt.Errorf("EcoFlow MQTT credentials are incomplete")
	}
	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("ssl://%s:%d", info.URL, info.Port))
	opts.SetClientID(info.ClientID)
	opts.SetUsername(info.Username)
	opts.SetPassword(info.Password)
	opts.SetCleanSession(true)
	opts.SetTLSConfig(&tls.Config{MinVersion: tls.VersionTLS12})
	opts.SetConnectTimeout(10 * time.Second)
	opts.SetAutoReconnect(false)
	client := mqtt.NewClient(opts)
	token := client.Connect()
	if !token.WaitTimeout(15 * time.Second) {
		return nil, fmt.Errorf("EcoFlow MQTT connect timed out")
	}
	if err := token.Error(); err != nil {
		return nil, fmt.Errorf("EcoFlow MQTT connect failed: %w", err)
	}
	return &PahoTransport{client: client}, nil
}

func (t *PahoTransport) Request(ctx context.Context, publishTopic string, payload []byte, replyTopics []string, timeout time.Duration) ([]MQTTMessage, error) {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	if len(replyTopics) == 0 {
		return nil, fmt.Errorf("EcoFlow MQTT request requires at least one reply topic")
	}
	targetTopic := replyTopics[0]
	messages := make(chan MQTTMessage, len(replyTopics)+4)
	for _, topic := range replyTopics {
		topic := topic
		token := t.client.Subscribe(topic, 1, func(_ mqtt.Client, msg mqtt.Message) {
			copied := append([]byte(nil), msg.Payload()...)
			select {
			case messages <- MQTTMessage{Topic: msg.Topic(), Payload: copied}:
			default:
			}
		})
		if !token.WaitTimeout(timeout) {
			return nil, fmt.Errorf("EcoFlow MQTT subscribe timed out: %s", topic)
		}
		if err := token.Error(); err != nil {
			return nil, fmt.Errorf("EcoFlow MQTT subscribe %s failed: %w", topic, err)
		}
		defer t.client.Unsubscribe(topic)
	}
	if err := t.Publish(ctx, publishTopic, payload, timeout); err != nil {
		return nil, err
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	collected := []MQTTMessage{}
	for {
		select {
		case <-ctx.Done():
			return collected, ctx.Err()
		case msg := <-messages:
			collected = append(collected, msg)
			if msg.Topic == targetTopic {
				return collected, nil
			}
		case <-timer.C:
			if len(collected) == 0 {
				return nil, fmt.Errorf("EcoFlow MQTT request timed out waiting for reply")
			}
			return collected, nil
		}
	}
}

func (t *PahoTransport) Publish(ctx context.Context, topic string, payload []byte, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	token := t.client.Publish(topic, 1, false, payload)
	done := make(chan struct{})
	go func() {
		token.Wait()
		close(done)
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(timeout):
		return fmt.Errorf("EcoFlow MQTT publish timed out: %s", topic)
	case <-done:
		if err := token.Error(); err != nil {
			return fmt.Errorf("EcoFlow MQTT publish %s failed: %w", topic, err)
		}
		return nil
	}
}

func (t *PahoTransport) Disconnect() {
	if t != nil && t.client != nil && t.client.IsConnected() {
		t.client.Disconnect(250)
	}
}
