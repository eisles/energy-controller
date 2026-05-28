package ecoflowprivate

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestExecuteACChargePowerRequiresAcceptedSetReply(t *testing.T) {
	tests := []struct {
		name    string
		replies []MQTTMessage
		wantErr string
	}{
		{
			name:    "missing ack",
			replies: []MQTTMessage{{Topic: "/app/device/property/SN123", Payload: BuildGetSnapshotPayload(1)}},
			wantErr: "did not return set acknowledgement",
		},
		{
			name:    "rejected ack",
			replies: nil,
			wantErr: "rejected",
		},
		{
			name:    "stale ack",
			replies: []MQTTMessage{{Topic: "/app/user-1/SN123/thing/property/set_reply", Payload: setReplyPayload(true, 1)}},
			wantErr: "did not return set acknowledgement",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := fakeTransport{replies: tt.replies}
			if tt.name == "rejected ack" {
				transport.buildReplies = func(payload []byte) []MQTTMessage {
					return []MQTTMessage{{Topic: "/app/user-1/SN123/thing/property/set_reply", Payload: setReplyPayload(false, mustPayloadSeq(t, payload))}}
				}
			}

			client := NewClientWithTransport(Config{
				PrivateAPIHost: "api.test",
				Email:          "user@example.com",
				Password:       "secret",
				DeviceSN:       "SN123",
				DeviceType:     "DELTA_3",
				HTTPClient:     newPrivateHTTPClient(t),
				Timeout:        time.Second,
			}, transport)
			_, err := client.ExecuteACChargePower(context.Background(), 100, validWriteGuards())
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ExecuteACChargePower error = %v, want %q", err, tt.wantErr)
			}
		})
	}
}

func TestExecuteACChargePowerAcceptsConfigOKSetReplyAfterDataMessage(t *testing.T) {
	client := NewClientWithTransport(Config{
		PrivateAPIHost: "api.test",
		Email:          "user@example.com",
		Password:       "secret",
		DeviceSN:       "SN123",
		DeviceType:     "DELTA_3",
		HTTPClient:     newPrivateHTTPClient(t),
		Timeout:        time.Second,
	}, fakeTransport{buildReplies: func(payload []byte) []MQTTMessage {
		seq := mustPayloadSeq(t, payload)
		return []MQTTMessage{
			{Topic: "/app/device/property/SN123", Payload: BuildGetSnapshotPayload(1)},
			{Topic: "/app/user-1/SN123/thing/property/set_reply", Payload: setReplyPayload(true, seq)},
		}
	}})
	status, err := client.ExecuteACChargePower(context.Background(), 100, validWriteGuards())
	if err != nil {
		t.Fatal(err)
	}
	if status.LastSetReplyConfigOK == nil || !*status.LastSetReplyConfigOK {
		t.Fatalf("LastSetReplyConfigOK = %v, want true", status.LastSetReplyConfigOK)
	}
}

func TestExecuteRemainingDelta3CommandsAcceptSetReply(t *testing.T) {
	tests := []struct {
		name string
		call func(*Client, context.Context, WriteGuards) (Status, error)
	}{
		{name: "min discharge", call: func(c *Client, ctx context.Context, guards WriteGuards) (Status, error) {
			return c.ExecuteMinDischargeSoc(ctx, 10, guards)
		}},
		{name: "max charge", call: func(c *Client, ctx context.Context, guards WriteGuards) (Status, error) {
			return c.ExecuteMaxChargeSoc(ctx, 95, guards)
		}},
		{name: "energy backup enabled", call: func(c *Client, ctx context.Context, guards WriteGuards) (Status, error) {
			return c.ExecuteEnergyBackupEnabled(ctx, false, 25, guards)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestClientWithTransport(t, fakeTransport{buildReplies: func(payload []byte) []MQTTMessage {
				return []MQTTMessage{{Topic: "/app/user-1/SN123/thing/property/set_reply", Payload: setReplyPayload(true, mustPayloadSeq(t, payload))}}
			}})
			status, err := tt.call(client, context.Background(), validWriteGuards())
			if err != nil {
				t.Fatal(err)
			}
			if status.LastSetReplyConfigOK == nil || !*status.LastSetReplyConfigOK {
				t.Fatalf("LastSetReplyConfigOK = %v, want true", status.LastSetReplyConfigOK)
			}
		})
	}
}

func TestExecuteRemainingDelta3CommandsRequireWriteGate(t *testing.T) {
	client := newTestClientWithTransport(t, fakeTransport{})
	guards := validWriteGuards()
	guards.Execute = false
	tests := []struct {
		name string
		call func(*Client, context.Context, WriteGuards) (Status, error)
	}{
		{name: "min discharge", call: func(c *Client, ctx context.Context, guards WriteGuards) (Status, error) {
			return c.ExecuteMinDischargeSoc(ctx, 10, guards)
		}},
		{name: "max charge", call: func(c *Client, ctx context.Context, guards WriteGuards) (Status, error) {
			return c.ExecuteMaxChargeSoc(ctx, 95, guards)
		}},
		{name: "energy backup enabled", call: func(c *Client, ctx context.Context, guards WriteGuards) (Status, error) {
			return c.ExecuteEnergyBackupEnabled(ctx, false, 25, guards)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.call(client, context.Background(), guards)
			if err == nil || !strings.Contains(err.Error(), "--execute") {
				t.Fatalf("error = %v, want write gate error", err)
			}
		})
	}
}

func TestExecuteRemainingDelta3CommandsRequireSetReply(t *testing.T) {
	tests := []struct {
		name string
		call func(*Client, context.Context, WriteGuards) (Status, error)
	}{
		{name: "min discharge", call: func(c *Client, ctx context.Context, guards WriteGuards) (Status, error) {
			return c.ExecuteMinDischargeSoc(ctx, 10, guards)
		}},
		{name: "max charge", call: func(c *Client, ctx context.Context, guards WriteGuards) (Status, error) {
			return c.ExecuteMaxChargeSoc(ctx, 95, guards)
		}},
		{name: "energy backup enabled", call: func(c *Client, ctx context.Context, guards WriteGuards) (Status, error) {
			return c.ExecuteEnergyBackupEnabled(ctx, false, 25, guards)
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := newTestClientWithTransport(t, fakeTransport{replies: []MQTTMessage{{Topic: "/app/device/property/SN123", Payload: BuildGetSnapshotPayload(1)}}})
			_, err := tt.call(client, context.Background(), validWriteGuards())
			if err == nil || !strings.Contains(err.Error(), "did not return set acknowledgement") {
				t.Fatalf("error = %v, want missing acknowledgement error", err)
			}
		})
	}
}

func TestProbeWaitsForFullGetReplyBeforeTelemetry(t *testing.T) {
	var gotReplyTopics []string
	client := NewClientWithTransport(Config{
		PrivateAPIHost: "api.test",
		Email:          "user@example.com",
		Password:       "secret",
		DeviceSN:       "SN123",
		DeviceType:     "DELTA_3",
		HTTPClient:     newPrivateHTTPClient(t),
		Timeout:        time.Second,
	}, fakeTransport{
		onRequest: func(_ string, _ []byte, replyTopics []string) {
			gotReplyTopics = append([]string(nil), replyTopics...)
		},
		replies: []MQTTMessage{
			{Topic: "/app/user-1/SN123/thing/property/get_reply", Payload: displayPayload(t, 30, true)},
		},
	})
	status, err := client.Probe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(gotReplyTopics) != 2 || !strings.HasSuffix(gotReplyTopics[0], "/thing/property/get_reply") {
		t.Fatalf("reply topics = %#v, want get_reply first", gotReplyTopics)
	}
	assertIntPtr(t, "BackupReserveSoc", status.BackupReserveSoc, 30)
	if status.BackupReserveEnabled == nil || !*status.BackupReserveEnabled {
		t.Fatalf("BackupReserveEnabled = %v, want true", status.BackupReserveEnabled)
	}
}

func TestProbeReusesPrivateSession(t *testing.T) {
	counts := &privateHTTPCallCounts{}
	client := NewClientWithTransport(Config{
		PrivateAPIHost: "api.test",
		Email:          "user@example.com",
		Password:       "secret",
		DeviceSN:       "SN123",
		DeviceType:     "DELTA_3",
		HTTPClient:     newCountingPrivateHTTPClientWithPort(t, `8883`, counts),
		Timeout:        time.Second,
	}, fakeTransport{
		replies: []MQTTMessage{
			{Topic: "/app/user-1/SN123/thing/property/get_reply", Payload: displayPayload(t, 30, true)},
		},
	})

	if _, err := client.Probe(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Probe(context.Background()); err != nil {
		t.Fatal(err)
	}

	if counts.login != 1 || counts.certification != 1 {
		t.Fatalf("login calls = %d certification calls = %d, want 1 each", counts.login, counts.certification)
	}
}

func TestProbeRefreshesPrivateSessionAfterAuthFailure(t *testing.T) {
	counts := &privateHTTPCallCounts{}
	transport := &authRetryTransport{
		failOnCall: 2,
		replies: []MQTTMessage{
			{Topic: "/app/user-1/SN123/thing/property/get_reply", Payload: displayPayload(t, 30, true)},
		},
	}
	client := NewClientWithTransport(Config{
		PrivateAPIHost: "api.test",
		Email:          "user@example.com",
		Password:       "secret",
		DeviceSN:       "SN123",
		DeviceType:     "DELTA_3",
		HTTPClient:     newCountingPrivateHTTPClientWithPort(t, `8883`, counts),
		Timeout:        time.Second,
	}, transport)

	if _, err := client.Probe(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Probe(context.Background()); err != nil {
		t.Fatal(err)
	}

	if counts.login != 2 || counts.certification != 2 {
		t.Fatalf("login calls = %d certification calls = %d, want 2 each after refresh", counts.login, counts.certification)
	}
	if transport.calls != 3 {
		t.Fatalf("transport calls = %d, want initial success, auth failure, retry success", transport.calls)
	}
}

func newTestClientWithTransport(t *testing.T, transport fakeTransport) *Client {
	t.Helper()
	return NewClientWithTransport(Config{
		PrivateAPIHost: "api.test",
		Email:          "user@example.com",
		Password:       "secret",
		DeviceSN:       "SN123",
		DeviceType:     "DELTA_3",
		HTTPClient:     newPrivateHTTPClient(t),
		Timeout:        time.Second,
	}, transport)
}

func displayPayload(t *testing.T, backupReserveSoc int, backupReserveEnabled bool) []byte {
	t.Helper()
	enabled := 0
	if backupReserveEnabled {
		enabled = 1
	}
	display := appendIntField(nil, 7, enabled)
	display = appendIntField(display, 8, backupReserveSoc)
	return encodeHeaderMessage(delta3Header{PData: display, Src: 2, CmdFunc: 254, CmdID: 21, Seq: 1})
}

func validWriteGuards() WriteGuards {
	return WriteGuards{
		MockMode:             false,
		SimulationMode:       false,
		EnableRealControl:    true,
		AutoControlEnabled:   false,
		ConfirmEcoFlowWrite:  ConfirmWriteValue,
		Execute:              true,
		AllowPrivateAPIWrite: true,
	}
}

func setReplyPayload(configOK bool, seq int) []byte {
	value := 0
	if configOK {
		value = 1
	}
	reply := []byte{}
	reply = append(reply, encodeTag(2, wireVarint)...)
	reply = append(reply, encodeVarint(uint64(value))...)
	reply = appendIntField(reply, 54, 100)
	return encodeHeaderMessage(delta3Header{PData: reply, Src: 2, CmdFunc: 254, CmdID: 18, Seq: seq})
}

type fakeTransport struct {
	replies      []MQTTMessage
	buildReplies func(payload []byte) []MQTTMessage
	onRequest    func(publishTopic string, payload []byte, replyTopics []string)
}

func (f fakeTransport) Request(_ context.Context, publishTopic string, payload []byte, replyTopics []string, _ time.Duration) ([]MQTTMessage, error) {
	if f.onRequest != nil {
		f.onRequest(publishTopic, payload, replyTopics)
	}
	if f.buildReplies != nil {
		return f.buildReplies(payload), nil
	}
	return f.replies, nil
}

func (f fakeTransport) Publish(context.Context, string, []byte, time.Duration) error {
	return nil
}

func (f fakeTransport) Disconnect() {}

type authRetryTransport struct {
	replies    []MQTTMessage
	failOnCall int
	calls      int
}

func (t *authRetryTransport) Request(context.Context, string, []byte, []string, time.Duration) ([]MQTTMessage, error) {
	t.calls++
	if t.calls == t.failOnCall {
		return nil, fmt.Errorf("EcoFlow MQTT request failed: not authorized")
	}
	return t.replies, nil
}

func (t *authRetryTransport) Publish(context.Context, string, []byte, time.Duration) error {
	return nil
}

func (t *authRetryTransport) Disconnect() {}

func mustPayloadSeq(t *testing.T, payload []byte) int {
	t.Helper()
	headers, err := decodeHeaderMessage(payload)
	if err != nil {
		t.Fatal(err)
	}
	if len(headers) == 0 {
		t.Fatal("payload has no headers")
	}
	return headers[0].Seq
}
