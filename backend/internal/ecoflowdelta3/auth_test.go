package ecoflowdelta3

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestBuildLoginBodyEncodesPassword(t *testing.T) {
	body, err := buildLoginBody("user@example.com", "plain-password")
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]string
	if err := json.Unmarshal(body, &got); err != nil {
		t.Fatal(err)
	}
	if got["password"] != base64.StdEncoding.EncodeToString([]byte("plain-password")) {
		t.Fatalf("password = %q", got["password"])
	}
	if got["scene"] != "IOT_APP" || got["userType"] != "ECOFLOW" {
		t.Fatalf("body = %#v", got)
	}
}

func TestLoginParsesTokenUserAndMQTTCredentials(t *testing.T) {
	client := NewAuthClient(Config{
		PrivateAPIHost: "api.test",
		Email:          "user@example.com",
		Password:       "secret",
		HTTPClient:     newPrivateHTTPClient(t),
	})
	session, err := client.Login(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if session.Token != "token-1" || session.UserID != "user-1" {
		t.Fatalf("session = %#v", session)
	}
	if session.MQTT.URL != "mqtt.ecoflow.com" || session.MQTT.Port != 8883 || session.MQTT.Username != "acct" || session.MQTT.Password != "pass" {
		t.Fatalf("mqtt = %#v", session.MQTT)
	}
	if session.MQTT.ClientID != "EnergyController_user-1" {
		t.Fatalf("client id = %q", session.MQTT.ClientID)
	}
}

func newPrivateHTTPClient(t *testing.T) *http.Client {
	t.Helper()
	return &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		switch r.URL.Path {
		case "/auth/login":
			if r.Method != http.MethodPost {
				t.Fatalf("login method = %s", r.Method)
			}
			var req map[string]string
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Fatal(err)
			}
			if req["password"] != base64.StdEncoding.EncodeToString([]byte("secret")) {
				t.Fatalf("encoded password = %q", req["password"])
			}
			return jsonResponse(`{"code":"0","message":"Success","data":{"token":"token-1","user":{"userId":"user-1","name":"Sato"}}}`), nil
		case "/iot-auth/app/certification":
			if got := r.Header.Get("authorization"); got != "Bearer token-1" {
				t.Fatalf("authorization = %q", got)
			}
			return jsonResponse(`{"code":"0","message":"Success","data":{"url":"mqtt.ecoflow.com","port":8883,"certificateAccount":"acct","certificatePassword":"pass"}}`), nil
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		return nil, nil
	})}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func jsonResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
