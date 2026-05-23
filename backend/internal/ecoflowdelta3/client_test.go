package ecoflowdelta3

import (
	"context"
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
