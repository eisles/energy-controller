package ecoflowdeveloper

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/eisles/energy-controller/backend/internal/ecoflowprivate"
)

func TestClientReadCycleStatusSubscribesDeveloperQuotaTopic(t *testing.T) {
	session := ecoflowprivate.Session{MQTT: ecoflowprivate.MQTTInfo{
		URL:      "mqtt.test",
		Port:     8883,
		Username: "acct",
		Password: "pass",
		ClientID: "client",
	}}
	subscriber := &fakeQuotaSubscriber{payload: []byte(`{"params":{"cycles":71}}`)}
	client := &Client{
		cfg: Config{
			Email:    "user@example.com",
			Password: "secret",
			DeviceSN: "SN123",
			Timeout:  time.Second,
		}.normalized(),
		sessionProvider: &fakeSessionProvider{session: session, fromCache: true},
		transportFactory: func(info ecoflowprivate.MQTTInfo) (quotaSubscriber, error) {
			if info.Username != "acct" {
				t.Fatalf("MQTT username = %q, want certificate account", info.Username)
			}
			return subscriber, nil
		},
	}

	status, err := client.ReadCycleStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.CycleCount == nil || *status.CycleCount != 71 || status.CycleCountSource != CycleCountSource {
		t.Fatalf("status = %+v, want cycle count from quota", status)
	}
	if !hasTopic(subscriber.topics, "/open/acct/SN123/quota") {
		t.Fatalf("topics = %v, want quota topic", subscriber.topics)
	}
	if !subscriber.disconnected {
		t.Fatal("subscriber was not disconnected")
	}
}

func TestClientReadCycleStatusInvalidatesCachedSessionBeforeAuthRetry(t *testing.T) {
	provider := &fakeSessionProvider{
		session: ecoflowprivate.Session{MQTT: ecoflowprivate.MQTTInfo{
			URL:      "mqtt.test",
			Port:     8883,
			Username: "old",
			Password: "pass",
			ClientID: "client",
		}},
		nextSession: ecoflowprivate.Session{MQTT: ecoflowprivate.MQTTInfo{
			URL:      "mqtt.test",
			Port:     8883,
			Username: "new",
			Password: "pass",
			ClientID: "client",
		}},
		fromCache: true,
	}
	client := &Client{
		cfg: Config{
			Email:    "user@example.com",
			Password: "secret",
			DeviceSN: "SN123",
			Timeout:  time.Second,
		}.normalized(),
		sessionProvider: provider,
		transportFactory: func(info ecoflowprivate.MQTTInfo) (quotaSubscriber, error) {
			if info.Username == "old" {
				return &fakeQuotaSubscriber{err: errNotAuthorized{}}, nil
			}
			return &fakeQuotaSubscriber{payload: []byte(`{"params":{"cycles":72}}`)}, nil
		},
	}

	status, err := client.ReadCycleStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if provider.invalidated != 1 {
		t.Fatalf("invalidated = %d, want 1", provider.invalidated)
	}
	if status.CycleCount == nil || *status.CycleCount != 72 {
		t.Fatalf("status = %+v, want retry cycle count", status)
	}
}

func TestClientReadCycleStatusWaitsForCycleQuota(t *testing.T) {
	session := ecoflowprivate.Session{MQTT: ecoflowprivate.MQTTInfo{
		URL:      "mqtt.test",
		Port:     8883,
		Username: "acct",
		Password: "pass",
		ClientID: "client",
	}}
	subscriber := &fakeQuotaSubscriber{payloads: [][]byte{
		[]byte(`{"params":{"soc":74}}`),
		[]byte(`{"params":{"cycles":74}}`),
	}}
	client := &Client{
		cfg: Config{
			Email:    "user@example.com",
			Password: "secret",
			DeviceSN: "SN123",
			Timeout:  time.Second,
		}.normalized(),
		sessionProvider: &fakeSessionProvider{session: session, fromCache: true},
		transportFactory: func(info ecoflowprivate.MQTTInfo) (quotaSubscriber, error) {
			return subscriber, nil
		},
	}

	status, err := client.ReadCycleStatus(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.CycleCount == nil || *status.CycleCount != 74 {
		t.Fatalf("status = %+v, want cycle count after non-cycle quota", status)
	}
}

func TestClientReadCycleStatusDoesNotSucceedWithoutCycleQuota(t *testing.T) {
	session := ecoflowprivate.Session{MQTT: ecoflowprivate.MQTTInfo{
		URL:      "mqtt.test",
		Port:     8883,
		Username: "acct",
		Password: "pass",
		ClientID: "client",
	}}
	subscriber := &fakeQuotaSubscriber{payload: []byte(`{"params":{"soc":74}}`)}
	client := &Client{
		cfg: Config{
			Email:    "user@example.com",
			Password: "secret",
			DeviceSN: "SN123",
			Timeout:  10 * time.Millisecond,
		}.normalized(),
		sessionProvider: &fakeSessionProvider{session: session, fromCache: true},
		transportFactory: func(info ecoflowprivate.MQTTInfo) (quotaSubscriber, error) {
			return subscriber, nil
		},
	}

	status, err := client.ReadCycleStatus(context.Background())
	if err == nil {
		t.Fatalf("ReadCycleStatus error = nil, status = %+v, want missing cycle quota error", status)
	}
	if status.CycleCount != nil {
		t.Fatalf("CycleCount = %v, want nil without cycle quota", *status.CycleCount)
	}
}

func TestClientWatchQuotaSubscribesQuotaAndStatusTopics(t *testing.T) {
	session := ecoflowprivate.Session{MQTT: ecoflowprivate.MQTTInfo{
		URL:      "mqtt.test",
		Port:     8883,
		Username: "acct",
		Password: "pass",
		ClientID: "client",
	}}
	subscriber := &fakeQuotaSubscriber{payload: []byte(`{"params":{"cycles":73}}`)}
	client := &Client{
		cfg: Config{
			Email:    "user@example.com",
			Password: "secret",
			DeviceSN: "SN123",
			Timeout:  time.Second,
		}.normalized(),
		sessionProvider: &fakeSessionProvider{session: session},
		transportFactory: func(info ecoflowprivate.MQTTInfo) (quotaSubscriber, error) {
			return subscriber, nil
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	var messagesMu sync.Mutex
	messages := []QuotaMessage{}
	err := client.WatchQuota(ctx, func(msg QuotaMessage) {
		messagesMu.Lock()
		defer messagesMu.Unlock()
		messages = append(messages, msg)
		if len(messages) == 2 {
			cancel()
		}
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("WatchQuota error = %v, want context canceled", err)
	}
	if len(subscriber.topics) != 2 || !hasTopic(subscriber.topics, "/open/acct/SN123/quota") || !hasTopic(subscriber.topics, "/open/acct/SN123/status") {
		t.Fatalf("topics = %v, want quota and status topics", subscriber.topics)
	}
	messagesMu.Lock()
	defer messagesMu.Unlock()
	if len(messages) != 2 || !hasCycleMessage(messages, 73) || !hasStatusMessage(messages) {
		t.Fatalf("messages = %+v, want quota cycle and status", messages)
	}
	if !subscriber.disconnected {
		t.Fatal("subscriber was not disconnected")
	}
}

type fakeSessionProvider struct {
	session     ecoflowprivate.Session
	nextSession ecoflowprivate.Session
	fromCache   bool
	err         error
	invalidated int
}

func (p *fakeSessionProvider) CachedSession(ctx context.Context) (ecoflowprivate.Session, bool, error) {
	if p.invalidated > 0 && p.nextSession.MQTT.Username != "" {
		return p.nextSession, false, p.err
	}
	return p.session, p.fromCache, p.err
}

func (p *fakeSessionProvider) InvalidateCachedSession() {
	p.invalidated++
}

type fakeQuotaSubscriber struct {
	mu           sync.Mutex
	topic        string
	topics       []string
	payload      []byte
	payloads     [][]byte
	err          error
	disconnected bool
}

func (s *fakeQuotaSubscriber) SubscribeOnce(_ context.Context, topic string, _ time.Duration) (ecoflowprivate.MQTTMessage, error) {
	s.topic = topic
	if s.err != nil {
		return ecoflowprivate.MQTTMessage{}, s.err
	}
	return ecoflowprivate.MQTTMessage{Topic: topic, Payload: s.payload}, nil
}

func (s *fakeQuotaSubscriber) Subscribe(ctx context.Context, topic string, _ time.Duration, onMessage func(ecoflowprivate.MQTTMessage)) error {
	s.mu.Lock()
	s.topics = append(s.topics, topic)
	s.mu.Unlock()
	if s.err != nil {
		return s.err
	}
	payload := s.payload
	if payload == nil {
		payload = []byte(`{"params":{"status":1}}`)
	}
	if len(s.payloads) > 0 {
		for _, payload := range s.payloads {
			onMessage(ecoflowprivate.MQTTMessage{Topic: topic, Payload: payload})
		}
	} else {
		onMessage(ecoflowprivate.MQTTMessage{Topic: topic, Payload: payload})
	}
	<-ctx.Done()
	return ctx.Err()
}

func (s *fakeQuotaSubscriber) Disconnect() {
	s.disconnected = true
}

type errNotAuthorized struct{}

func (errNotAuthorized) Error() string {
	return "not authorized"
}

func hasTopic(topics []string, want string) bool {
	for _, topic := range topics {
		if topic == want {
			return true
		}
	}
	return false
}

func hasCycleMessage(messages []QuotaMessage, want int) bool {
	for _, msg := range messages {
		if msg.CycleStatus.CycleCount != nil && *msg.CycleStatus.CycleCount == want {
			return true
		}
	}
	return false
}

func hasStatusMessage(messages []QuotaMessage) bool {
	for _, msg := range messages {
		if msg.TopicKind == "status" {
			return true
		}
	}
	return false
}
