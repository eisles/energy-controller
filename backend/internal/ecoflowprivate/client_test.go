package ecoflowprivate

import (
	"context"
	"fmt"
	"net/http"
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

func TestProbeRawReturnsCapturedMQTTReplies(t *testing.T) {
	client := NewClientWithTransport(Config{
		PrivateAPIHost: "api.test",
		Email:          "user@example.com",
		Password:       "secret",
		DeviceSN:       "SN123",
		DeviceType:     "DELTA_3",
		HTTPClient:     newPrivateHTTPClient(t),
		Timeout:        time.Second,
	}, fakeTransport{
		replies: []MQTTMessage{
			{Topic: "/app/user-1/SN123/thing/property/get_reply", Payload: displayPayload(t, 30, true)},
		},
	})
	status, replies, err := client.ProbeRaw(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	assertIntPtr(t, "BackupReserveSoc", status.BackupReserveSoc, 30)
	if len(replies) != 1 || !strings.HasSuffix(replies[0].Topic, "/thing/property/get_reply") || len(replies[0].Payload) == 0 {
		t.Fatalf("replies = %#v, want captured get_reply payload", replies)
	}
}

func TestProbeAddsReadOnlyFieldSummaries(t *testing.T) {
	client := NewClientWithTransport(Config{
		PrivateAPIHost: "api.test",
		Email:          "user@example.com",
		Password:       "secret",
		DeviceSN:       "SN123",
		DeviceType:     "DELTA_3_MAX_PLUS",
		HTTPClient:     newPrivateHTTPClient(t),
		Timeout:        time.Second,
	}, fakeTransport{
		replies: []MQTTMessage{
			{Topic: "/app/user-1/SN123/thing/property/get_reply", Payload: displayPayload(t, 30, true)},
		},
	})

	status, err := client.Probe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.ReplyCount != 1 {
		t.Fatalf("ReplyCount = %d, want 1", status.ReplyCount)
	}
	if len(status.FieldSummaries) == 0 {
		t.Fatal("FieldSummaries = empty, want read-only payload field summaries")
	}
	first := status.FieldSummaries[0]
	if first.CmdFunc != 254 || first.CmdID != 21 || first.Field != 7 {
		t.Fatalf("first field summary = %+v, want cmdFunc 254 cmdId 21 field 7", first)
	}
}

func TestAppendTelemetryFieldSummariesTracksTotalWhenTruncated(t *testing.T) {
	fields := make([]SnapshotField, telemetryFieldSummaryLimit+3)
	for i := range fields {
		fields[i] = SnapshotField{MessageIndex: 0, CmdFunc: 254, CmdID: 21, Field: i + 1, Wire: 0, Value: "1"}
	}
	var status Status

	appendTelemetryFieldSummaries(&status, fields)

	if status.FieldCount != len(fields) {
		t.Fatalf("FieldCount = %d, want %d", status.FieldCount, len(fields))
	}
	if len(status.FieldSummaries) != telemetryFieldSummaryLimit {
		t.Fatalf("FieldSummaries length = %d, want %d", len(status.FieldSummaries), telemetryFieldSummaryLimit)
	}
	if !status.FieldSummaryTruncated {
		t.Fatal("FieldSummaryTruncated = false, want true")
	}
}

func TestProbeAddsReadOnlyInspectErrorDiagnostics(t *testing.T) {
	client := NewClientWithTransport(Config{
		PrivateAPIHost: "api.test",
		Email:          "user@example.com",
		Password:       "secret",
		DeviceSN:       "SN123",
		DeviceType:     "DELTA_3_MAX_PLUS",
		HTTPClient:     newPrivateHTTPClient(t),
		Timeout:        time.Second,
	}, fakeTransport{
		replies: []MQTTMessage{
			{Topic: "/app/user-1/SN123/thing/property/get_reply", Payload: []byte{0xff}},
		},
	})

	status, err := client.Probe(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if status.ReplyCount != 1 || status.InspectErrorCount != 1 || status.LastInspectError == "" {
		t.Fatalf("status diagnostics = %+v, want reply and inspect error diagnostics", status)
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

func TestProbeSharesPrivateSessionAcrossClients(t *testing.T) {
	counts := &privateHTTPCallCounts{}
	cache := newSessionCache(time.Hour, time.Minute, time.Now)
	cfg := Config{
		PrivateAPIHost: "api.test",
		Email:          "user@example.com",
		Password:       "secret",
		DeviceType:     "DELTA_3",
		HTTPClient:     newCountingPrivateHTTPClientWithPort(t, `8883`, counts),
		Timeout:        time.Second,
		SessionCache:   cache,
	}
	client1 := NewClientWithTransport(withDeviceSN(cfg, "SN123"), fakeTransport{
		replies: []MQTTMessage{{Topic: "/app/user-1/SN123/thing/property/get_reply", Payload: displayPayload(t, 30, true)}},
	})
	client2 := NewClientWithTransport(withDeviceSN(cfg, "SN456"), fakeTransport{
		replies: []MQTTMessage{{Topic: "/app/user-1/SN456/thing/property/get_reply", Payload: displayPayload(t, 40, true)}},
	})

	if _, err := client1.Probe(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := client2.Probe(context.Background()); err != nil {
		t.Fatal(err)
	}

	if counts.login != 1 || counts.certification != 1 {
		t.Fatalf("login calls = %d certification calls = %d, want 1 each across clients", counts.login, counts.certification)
	}
}

func TestProbeBacksOffBusyPrivateLoginAcrossClients(t *testing.T) {
	counts := &privateHTTPCallCounts{}
	cache := newSessionCache(time.Hour, time.Minute, time.Now)
	cfg := Config{
		PrivateAPIHost: "api.test",
		Email:          "user@example.com",
		Password:       "secret",
		DeviceType:     "DELTA_3",
		HTTPClient:     busyLoginHTTPClient(counts),
		Timeout:        time.Second,
		SessionCache:   cache,
	}
	client1 := NewClientWithTransport(withDeviceSN(cfg, "SN123"), fakeTransport{})
	client2 := NewClientWithTransport(withDeviceSN(cfg, "SN456"), fakeTransport{})

	if _, err := client1.Probe(context.Background()); err == nil || !strings.Contains(err.Error(), "Server is too busy") {
		t.Fatalf("first Probe error = %v, want busy login error", err)
	}
	if _, err := client2.Probe(context.Background()); err == nil || !strings.Contains(err.Error(), "login suppressed") {
		t.Fatalf("second Probe error = %v, want suppressed login error", err)
	}

	if counts.login != 1 {
		t.Fatalf("login calls = %d, want 1 after busy backoff", counts.login)
	}
}

func TestProbeInvalidatesSharedPrivateSessionAfterAuthFailure(t *testing.T) {
	counts := &privateHTTPCallCounts{}
	cache := newSessionCache(time.Hour, time.Minute, time.Now)
	cfg := Config{
		PrivateAPIHost: "api.test",
		Email:          "user@example.com",
		Password:       "secret",
		DeviceSN:       "SN123",
		DeviceType:     "DELTA_3",
		HTTPClient:     newCountingPrivateHTTPClientWithPort(t, `8883`, counts),
		Timeout:        time.Second,
		SessionCache:   cache,
	}
	client1 := NewClientWithTransport(cfg, fakeTransport{
		replies: []MQTTMessage{{Topic: "/app/user-1/SN123/thing/property/get_reply", Payload: displayPayload(t, 30, true)}},
	})
	if _, err := client1.Probe(context.Background()); err != nil {
		t.Fatal(err)
	}

	transport := &authRetryTransport{
		failOnCall: 1,
		replies: []MQTTMessage{
			{Topic: "/app/user-1/SN123/thing/property/get_reply", Payload: displayPayload(t, 30, true)},
		},
	}
	client2 := NewClientWithTransport(cfg, transport)
	if _, err := client2.Probe(context.Background()); err != nil {
		t.Fatal(err)
	}

	if counts.login != 2 || counts.certification != 2 {
		t.Fatalf("login calls = %d certification calls = %d, want 2 each after shared session refresh", counts.login, counts.certification)
	}
	if transport.calls != 2 {
		t.Fatalf("transport calls = %d, want auth failure and retry success", transport.calls)
	}
}

func TestSessionForClientRegeneratesAutoMQTTClientIDFromSharedCache(t *testing.T) {
	client := &Client{cfg: Config{}.normalized()}
	session := Session{
		UserID: "user-1",
		MQTT:   MQTTInfo{ClientID: "ANDROID_CACHED_user-1"},
	}

	got1, err := client.sessionForClient(session, true)
	if err != nil {
		t.Fatal(err)
	}
	got2, err := client.sessionForClient(session, true)
	if err != nil {
		t.Fatal(err)
	}

	if got1.MQTT.ClientID == session.MQTT.ClientID || got2.MQTT.ClientID == session.MQTT.ClientID {
		t.Fatalf("client IDs = %q, %q, want regenerated from shared cache", got1.MQTT.ClientID, got2.MQTT.ClientID)
	}
	if got1.MQTT.ClientID == got2.MQTT.ClientID {
		t.Fatalf("client ID reused = %q, want per-client generated ID", got1.MQTT.ClientID)
	}
	if !strings.HasPrefix(got1.MQTT.ClientID, "ANDROID_") || !strings.HasSuffix(got1.MQTT.ClientID, "_user-1") {
		t.Fatalf("client ID = %q, want EcoFlow Android style ID", got1.MQTT.ClientID)
	}
}

func TestSessionForClientPreservesConfiguredMQTTClientIDFromSharedCache(t *testing.T) {
	client := &Client{cfg: Config{MQTTClientID: "fixed-client-id"}.normalized()}
	session := Session{
		UserID: "user-1",
		MQTT:   MQTTInfo{ClientID: "fixed-client-id"},
	}

	got, err := client.sessionForClient(session, true)
	if err != nil {
		t.Fatal(err)
	}
	if got.MQTT.ClientID != "fixed-client-id" {
		t.Fatalf("client ID = %q, want configured ID preserved", got.MQTT.ClientID)
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

func withDeviceSN(cfg Config, sn string) Config {
	cfg.DeviceSN = sn
	return cfg
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

func busyLoginHTTPClient(counts *privateHTTPCallCounts) *http.Client {
	return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/auth/login":
			if counts != nil {
				counts.login++
			}
			return jsonResponse(`{"code":"1001","message":"Server is too busy","data":{}}`), nil
		case "/iot-auth/app/certification":
			if counts != nil {
				counts.certification++
			}
			return jsonResponse(`{"code":"0","message":"Success","data":{"url":"mqtt.ecoflow.com","port":8883,"certificateAccount":"acct","certificatePassword":"pass"}}`), nil
		default:
			return nil, fmt.Errorf("unexpected path %s", r.URL.Path)
		}
	})}
}

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
