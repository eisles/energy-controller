package ecoflowprivate

import (
	"net/http"
	"strings"
	"time"
)

const (
	DefaultPrivateAPIHost = "api.ecoflow.com"
	ConfirmWriteValue     = "I_UNDERSTAND"
)

type Config struct {
	PrivateAPIHost string
	Email          string
	Password       string
	DeviceSN       string
	DeviceType     string
	MQTTClientID   string
	HTTPClient     *http.Client
	Timeout        time.Duration
	SessionCache   *SessionCache
}

func (c Config) normalized() Config {
	c.PrivateAPIHost = strings.TrimSpace(c.PrivateAPIHost)
	c.PrivateAPIHost = strings.TrimPrefix(strings.TrimPrefix(c.PrivateAPIHost, "https://"), "http://")
	c.PrivateAPIHost = strings.TrimRight(c.PrivateAPIHost, "/")
	if c.PrivateAPIHost == "" {
		c.PrivateAPIHost = DefaultPrivateAPIHost
	}
	c.Email = strings.TrimSpace(c.Email)
	c.DeviceSN = strings.TrimSpace(c.DeviceSN)
	c.DeviceType = strings.TrimSpace(c.DeviceType)
	c.MQTTClientID = strings.TrimSpace(c.MQTTClientID)
	if c.HTTPClient == nil {
		c.HTTPClient = &http.Client{Timeout: 15 * time.Second}
	}
	if c.Timeout <= 0 {
		c.Timeout = 20 * time.Second
	}
	return c
}

func (c Config) MissingReadCredentials() []string {
	c = c.normalized()
	missing := []string{}
	if c.Email == "" {
		missing = append(missing, "ECOFLOW_PRIVATE_EMAIL")
	}
	if c.Password == "" {
		missing = append(missing, "ECOFLOW_PRIVATE_PASSWORD")
	}
	if c.DeviceSN == "" {
		missing = append(missing, "ECOFLOW_DELTA3_DEVICE_SN")
	}
	if c.DeviceType == "" {
		missing = append(missing, "ECOFLOW_DELTA3_DEVICE_TYPE")
	}
	return missing
}
