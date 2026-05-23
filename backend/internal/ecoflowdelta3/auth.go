package ecoflowdelta3

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
)

type AuthClient struct {
	cfg Config
}

func NewAuthClient(cfg Config) *AuthClient {
	return &AuthClient{cfg: cfg.normalized()}
}

type Session struct {
	Token    string
	UserID   string
	UserName string
	MQTT     MQTTInfo
}

type MQTTInfo struct {
	URL      string
	Port     int
	Username string
	Password string
	ClientID string
}

func (c *AuthClient) Login(ctx context.Context) (Session, error) {
	if c.cfg.Email == "" || c.cfg.Password == "" {
		return Session{}, fmt.Errorf("EcoFlow private login requires email and password")
	}
	body, err := buildLoginBody(c.cfg.Email, c.cfg.Password)
	if err != nil {
		return Session{}, err
	}
	reqURL := url.URL{Scheme: "https", Host: c.cfg.PrivateAPIHost, Path: "/auth/login"}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL.String(), bytes.NewReader(body))
	if err != nil {
		return Session{}, err
	}
	req.Header.Set("lang", "en_US")
	req.Header.Set("content-type", "application/json")

	resp, err := c.cfg.HTTPClient.Do(req)
	if err != nil {
		return Session{}, fmt.Errorf("EcoFlow private login request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Session{}, fmt.Errorf("EcoFlow private login returned HTTP %d", resp.StatusCode)
	}
	var payload privateLoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return Session{}, fmt.Errorf("decode EcoFlow private login response: %w", err)
	}
	if payload.Message != "Success" && payload.Message != "success" {
		return Session{}, fmt.Errorf("EcoFlow private login returned code=%s message=%s", payload.Code, payload.Message)
	}
	if payload.Data.Token == "" || payload.Data.User.UserID == "" {
		return Session{}, fmt.Errorf("EcoFlow private login response missing token or userId")
	}
	session := Session{
		Token:    payload.Data.Token,
		UserID:   payload.Data.User.UserID,
		UserName: payload.Data.User.Name,
	}
	mqttInfo, err := c.Certification(ctx, session.Token, session.UserID)
	if err != nil {
		return Session{}, err
	}
	session.MQTT = mqttInfo
	return session, nil
}

func (c *AuthClient) Certification(ctx context.Context, token string, userID string) (MQTTInfo, error) {
	reqURL := url.URL{Scheme: "https", Host: c.cfg.PrivateAPIHost, Path: "/iot-auth/app/certification"}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return MQTTInfo{}, err
	}
	req.Header.Set("lang", "en_US")
	req.Header.Set("authorization", "Bearer "+token)
	req.Header.Set("content-type", "application/json")
	resp, err := c.cfg.HTTPClient.Do(req)
	if err != nil {
		return MQTTInfo{}, fmt.Errorf("EcoFlow MQTT certification request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return MQTTInfo{}, fmt.Errorf("EcoFlow MQTT certification returned HTTP %d", resp.StatusCode)
	}
	var payload certificationResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return MQTTInfo{}, fmt.Errorf("decode EcoFlow MQTT certification response: %w", err)
	}
	if payload.Message != "Success" && payload.Message != "success" {
		return MQTTInfo{}, fmt.Errorf("EcoFlow MQTT certification returned code=%s message=%s", payload.Code, payload.Message)
	}
	if payload.Data.URL == "" || payload.Data.Port == 0 || payload.Data.CertificateAccount == "" || payload.Data.CertificatePassword == "" {
		return MQTTInfo{}, fmt.Errorf("EcoFlow MQTT certification response missing broker credentials")
	}
	clientID := c.cfg.MQTTClientID
	if clientID == "" {
		clientID = "EnergyController_" + userID
	}
	return MQTTInfo{
		URL:      payload.Data.URL,
		Port:     payload.Data.Port,
		Username: payload.Data.CertificateAccount,
		Password: payload.Data.CertificatePassword,
		ClientID: clientID,
	}, nil
}

func buildLoginBody(email string, password string) ([]byte, error) {
	return json.Marshal(map[string]any{
		"email":    email,
		"password": base64.StdEncoding.EncodeToString([]byte(password)),
		"scene":    "IOT_APP",
		"userType": "ECOFLOW",
	})
}

type privateLoginResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    struct {
		Token string `json:"token"`
		User  struct {
			UserID string `json:"userId"`
			Name   string `json:"name"`
		} `json:"user"`
	} `json:"data"`
}

type certificationResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Data    struct {
		URL                 string `json:"url"`
		Port                int    `json:"port"`
		CertificateAccount  string `json:"certificateAccount"`
		CertificatePassword string `json:"certificatePassword"`
	} `json:"data"`
}
