package ecoflow

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

const defaultBaseURL = "https://api-e.ecoflow.com"

type Config struct {
	AccessKey  string
	SecretKey  string
	DeviceSN   string
	BaseURL    string
	HTTPClient *http.Client
}

type SignedClient struct {
	accessKey  string
	secretKey  string
	deviceSN   string
	baseURL    string
	httpClient *http.Client
	nonce      func() string
	now        func() time.Time
}

type MQTTCertification struct {
	URL                 string `json:"url"`
	Port                int    `json:"port"`
	CertificateAccount  string `json:"certificateAccount"`
	CertificatePassword string `json:"certificatePassword"`
}

func (c *MQTTCertification) UnmarshalJSON(raw []byte) error {
	type alias struct {
		URL                 string `json:"url"`
		Port                any    `json:"port"`
		CertificateAccount  string `json:"certificateAccount"`
		CertificatePassword string `json:"certificatePassword"`
	}
	var decoded alias
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return err
	}
	port, err := mqttPortNumber(decoded.Port)
	if err != nil {
		return err
	}
	c.URL = decoded.URL
	c.Port = port
	c.CertificateAccount = decoded.CertificateAccount
	c.CertificatePassword = decoded.CertificatePassword
	return nil
}

func NewSignedClient(cfg Config) *SignedClient {
	baseURL := strings.TrimRight(cfg.BaseURL, "/")
	if baseURL == "" {
		baseURL = defaultBaseURL
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &SignedClient{
		accessKey:  cfg.AccessKey,
		secretKey:  cfg.SecretKey,
		deviceSN:   cfg.DeviceSN,
		baseURL:    baseURL,
		httpClient: httpClient,
		nonce: func() string {
			return strconv.FormatInt(time.Now().UnixNano()%900000+100000, 10)
		},
		now: time.Now,
	}
}

func (c *SignedClient) GetBatteryStatus(ctx context.Context) (domain.BatteryStatus, error) {
	if c.accessKey == "" || c.secretKey == "" || c.deviceSN == "" {
		return domain.BatteryStatus{}, fmt.Errorf("EcoFlow access key, secret key, or device SN is empty")
	}
	quotas, err := c.getQuotaAll(ctx)
	if err != nil {
		return domain.BatteryStatus{}, err
	}
	return BatteryStatusFromQuotas(quotas)
}

type EnergyBackupStatus struct {
	StartSoc int
	Enabled  bool
}

func (c *SignedClient) GetEnergyBackupStatus(ctx context.Context) (EnergyBackupStatus, error) {
	if c.accessKey == "" || c.secretKey == "" || c.deviceSN == "" {
		return EnergyBackupStatus{}, fmt.Errorf("EcoFlow access key, secret key, or device SN is empty")
	}
	quotas, err := c.getQuotaAll(ctx)
	if err != nil {
		return EnergyBackupStatus{}, err
	}
	startSoc, ok := intFromQuota(quotas, "energyBackupStartSoc")
	if !ok {
		return EnergyBackupStatus{}, fmt.Errorf("EcoFlow quota does not include energyBackupStartSoc")
	}
	enabled, ok := boolFromQuota(quotas, "energyBackupEn")
	if !ok {
		return EnergyBackupStatus{}, fmt.Errorf("EcoFlow quota does not include energyBackupEn")
	}
	return EnergyBackupStatus{StartSoc: startSoc, Enabled: enabled}, nil
}

func (c *SignedClient) getQuotaAll(ctx context.Context) (map[string]any, error) {
	var payload quotaResponse
	if err := c.getJSON(ctx, "/iot-open/sign/device/quota/all", map[string]string{"sn": c.deviceSN}, &payload); err != nil {
		return nil, err
	}
	if payload.Code != "0" {
		return nil, fmt.Errorf("EcoFlow quota/all returned code=%s message=%s", payload.Code, payload.Message)
	}
	if payload.Data == nil {
		return nil, fmt.Errorf("EcoFlow quota/all response data is empty")
	}
	return payload.Data, nil
}

func (c *SignedClient) DeviceList(ctx context.Context) ([]Device, error) {
	var payload deviceListResponse
	if err := c.getJSON(ctx, "/iot-open/sign/device/list", nil, &payload); err != nil {
		return nil, err
	}
	if payload.Code != "0" {
		return nil, fmt.Errorf("EcoFlow device/list returned code=%s message=%s", payload.Code, payload.Message)
	}
	return payload.Data, nil
}

func (c *SignedClient) MQTTCertification(ctx context.Context) (MQTTCertification, error) {
	if c.accessKey == "" || c.secretKey == "" {
		return MQTTCertification{}, fmt.Errorf("EcoFlow access key or secret key is empty")
	}
	var payload mqttCertificationResponse
	if err := c.getJSON(ctx, "/iot-open/sign/certification", nil, &payload); err != nil {
		return MQTTCertification{}, err
	}
	if payload.Code != "0" {
		return MQTTCertification{}, fmt.Errorf("EcoFlow certification returned code=%s message=%s", payload.Code, payload.Message)
	}
	if payload.Data.URL == "" || payload.Data.Port == 0 || payload.Data.CertificateAccount == "" || payload.Data.CertificatePassword == "" {
		return MQTTCertification{}, fmt.Errorf("EcoFlow certification response data is incomplete")
	}
	return payload.Data, nil
}

func (c *SignedClient) getJSON(ctx context.Context, path string, params map[string]string, out any) error {
	endpoint, err := url.JoinPath(c.baseURL, path)
	if err != nil {
		return err
	}
	reqURL, err := url.Parse(endpoint)
	if err != nil {
		return err
	}
	query := reqURL.Query()
	for key, value := range params {
		query.Set(key, value)
	}
	reqURL.RawQuery = query.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return err
	}
	nonce := c.nonce()
	timestamp := strconv.FormatInt(c.now().UnixMilli(), 10)
	signature := sign(params, c.accessKey, c.secretKey, nonce, timestamp)
	req.Header.Set("accessKey", c.accessKey)
	req.Header.Set("nonce", nonce)
	req.Header.Set("timestamp", timestamp)
	req.Header.Set("sign", signature)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("EcoFlow request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("EcoFlow returned HTTP %d", resp.StatusCode)
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode EcoFlow response: %w", err)
	}
	return nil
}

func (c *SignedClient) newSignedPUTRequest(ctx context.Context, path string, payload any) (*http.Request, error) {
	endpoint, err := url.JoinPath(c.baseURL, path)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	nonce := c.nonce()
	timestamp := strconv.FormatInt(c.now().UnixMilli(), 10)
	signingParams, err := flattenSigningParams(payload)
	if err != nil {
		return nil, err
	}
	signature := sign(signingParams, c.accessKey, c.secretKey, nonce, timestamp)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("accessKey", c.accessKey)
	req.Header.Set("nonce", nonce)
	req.Header.Set("timestamp", timestamp)
	req.Header.Set("sign", signature)
	return req, nil
}

func sign(params map[string]string, accessKey, secretKey, nonce, timestamp string) string {
	pairs := make([]string, 0, len(params)+3)
	for key, value := range params {
		pairs = append(pairs, key+"="+value)
	}
	sort.Strings(pairs)
	pairs = append(pairs, "accessKey="+accessKey, "nonce="+nonce, "timestamp="+timestamp)
	mac := hmac.New(sha256.New, []byte(secretKey))
	_, _ = mac.Write([]byte(strings.Join(pairs, "&")))
	return hex.EncodeToString(mac.Sum(nil))
}

func flattenSigningParams(value any) (map[string]string, error) {
	body, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return nil, err
	}
	params := make(map[string]string)
	flattenSigningValue(params, "", decoded)
	return params, nil
}

func flattenSigningValue(params map[string]string, prefix string, value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			nextKey := key
			if prefix != "" {
				nextKey = prefix + "." + key
			}
			flattenSigningValue(params, nextKey, child)
		}
	case []any:
		for index, child := range typed {
			nextKey := prefix + "." + strconv.Itoa(index)
			flattenSigningValue(params, nextKey, child)
		}
	case json.Number:
		params[prefix] = typed.String()
	case bool:
		params[prefix] = strconv.FormatBool(typed)
	case string:
		params[prefix] = typed
	case nil:
		params[prefix] = ""
	default:
		params[prefix] = fmt.Sprint(typed)
	}
}

func mqttPortNumber(value any) (int, error) {
	switch typed := value.(type) {
	case float64:
		return int(typed), nil
	case string:
		port, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil {
			return 0, fmt.Errorf("parse EcoFlow MQTT certification port: %w", err)
		}
		return port, nil
	default:
		return 0, fmt.Errorf("EcoFlow MQTT certification port has unsupported type %T", value)
	}
}

type quotaResponse struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Data    map[string]any `json:"data"`
}

type deviceListResponse struct {
	Code    string   `json:"code"`
	Message string   `json:"message"`
	Data    []Device `json:"data"`
}

type mqttCertificationResponse struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Data    MQTTCertification `json:"data"`
}

type Device struct {
	SN string `json:"sn"`
}
