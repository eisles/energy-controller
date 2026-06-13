package ecoflowdeveloper

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/eisles/energy-controller/backend/internal/ecoflow"
	"github.com/eisles/energy-controller/backend/internal/ecoflowprivate"
)

type Config struct {
	AccessKey      string
	SecretKey      string
	BaseURL        string
	PrivateAPIHost string
	Email          string
	Password       string
	DeviceSN       string
	MQTTClientID   string
	HTTPClient     *http.Client
	Timeout        time.Duration
	SessionCache   *ecoflowprivate.SessionCache
}

type sessionProvider interface {
	CachedSession(ctx context.Context) (ecoflowprivate.Session, bool, error)
	InvalidateCachedSession()
}

type quotaSubscriber interface {
	SubscribeOnce(ctx context.Context, topic string, timeout time.Duration) (ecoflowprivate.MQTTMessage, error)
	Subscribe(ctx context.Context, topic string, timeout time.Duration, onMessage func(ecoflowprivate.MQTTMessage)) error
	Disconnect()
}

type Client struct {
	cfg              Config
	sessionProvider  sessionProvider
	transportFactory func(ecoflowprivate.MQTTInfo) (quotaSubscriber, error)
}

func NewClient(cfg Config) *Client {
	normalized := cfg.normalized()
	privateCfg := ecoflowprivate.Config{
		PrivateAPIHost: normalized.PrivateAPIHost,
		Email:          normalized.Email,
		Password:       normalized.Password,
		DeviceSN:       normalized.DeviceSN,
		DeviceType:     "DEVELOPER_MQTT_QUOTA",
		MQTTClientID:   normalized.MQTTClientID,
		HTTPClient:     normalized.HTTPClient,
		Timeout:        normalized.Timeout,
		SessionCache:   normalized.SessionCache,
	}
	return &Client{
		cfg:             normalized,
		sessionProvider: ecoflowprivate.NewClient(privateCfg),
		transportFactory: func(info ecoflowprivate.MQTTInfo) (quotaSubscriber, error) {
			return ecoflowprivate.NewPahoTransport(info)
		},
	}
}

func (c *Client) ReadCycleStatus(ctx context.Context) (CycleStatus, error) {
	if missing := c.cfg.MissingReadCredentials(); len(missing) > 0 {
		return CycleStatus{}, fmt.Errorf("EcoFlow Developer MQTT quota missing required env: %v", missing)
	}
	info, fromCache, provider, err := c.mqttInfo(ctx)
	if err != nil {
		return CycleStatus{}, err
	}
	status, err := c.readCycleStatusWithMQTTInfo(ctx, info)
	if err != nil && fromCache && isSessionAuthError(err) {
		provider.InvalidateCachedSession()
		info, _, _, sessionErr := c.privateMQTTInfo(ctx, provider)
		if sessionErr != nil {
			return CycleStatus{}, sessionErr
		}
		return c.readCycleStatusWithMQTTInfo(ctx, info)
	}
	return status, err
}

func (c *Client) WatchQuota(ctx context.Context, onMessage func(QuotaMessage)) error {
	if missing := c.cfg.MissingReadCredentials(); len(missing) > 0 {
		return fmt.Errorf("EcoFlow Developer MQTT quota missing required env: %v", missing)
	}
	info, _, _, err := c.mqttInfo(ctx)
	if err != nil {
		return err
	}
	factory := c.transportFactory
	if factory == nil {
		factory = func(info ecoflowprivate.MQTTInfo) (quotaSubscriber, error) {
			return ecoflowprivate.NewPahoTransport(info)
		}
	}
	transport, err := factory(info)
	if err != nil {
		return err
	}
	defer transport.Disconnect()

	quotaTopic := QuotaTopic(info.Username, c.cfg.DeviceSN)
	statusTopic := StatusTopic(info.Username, c.cfg.DeviceSN)
	watchCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	errs := make(chan error, 2)
	for _, topic := range []string{quotaTopic, statusTopic} {
		topic := topic
		go func() {
			errs <- transport.Subscribe(watchCtx, topic, c.cfg.Timeout, func(msg ecoflowprivate.MQTTMessage) {
				if onMessage != nil {
					onMessage(QuotaMessageFromMQTT(msg, topic == quotaTopic))
				}
			})
		}()
	}
	select {
	case <-ctx.Done():
		cancel()
		<-errs
		<-errs
		return ctx.Err()
	case err := <-errs:
		cancel()
		<-errs
		return err
	}
}

func (c *Client) mqttInfo(ctx context.Context) (ecoflowprivate.MQTTInfo, bool, sessionProvider, error) {
	if c.cfg.useOfficialDeveloperAPI() {
		info, err := c.officialMQTTInfo(ctx)
		return info, false, nil, err
	}
	provider := c.sessionProvider
	if provider == nil {
		provider = NewClient(c.cfg).sessionProvider
	}
	return c.privateMQTTInfo(ctx, provider)
}

func (c *Client) privateMQTTInfo(ctx context.Context, provider sessionProvider) (ecoflowprivate.MQTTInfo, bool, sessionProvider, error) {
	session, fromCache, err := provider.CachedSession(ctx)
	if err != nil {
		return ecoflowprivate.MQTTInfo{}, false, provider, err
	}
	return session.MQTT, fromCache, provider, nil
}

func (c *Client) officialMQTTInfo(ctx context.Context) (ecoflowprivate.MQTTInfo, error) {
	client := ecoflow.NewSignedClient(ecoflow.Config{
		AccessKey:  c.cfg.AccessKey,
		SecretKey:  c.cfg.SecretKey,
		DeviceSN:   c.cfg.DeviceSN,
		BaseURL:    c.cfg.BaseURL,
		HTTPClient: c.cfg.HTTPClient,
	})
	cert, err := client.MQTTCertification(ctx)
	if err != nil {
		return ecoflowprivate.MQTTInfo{}, err
	}
	clientID := c.cfg.MQTTClientID
	if clientID == "" {
		clientID = "energy_controller_" + c.cfg.DeviceSN
	}
	return ecoflowprivate.MQTTInfo{
		URL:      cert.URL,
		Port:     cert.Port,
		Username: cert.CertificateAccount,
		Password: cert.CertificatePassword,
		ClientID: clientID,
	}, nil
}

func (c *Client) readCycleStatusWithMQTTInfo(ctx context.Context, info ecoflowprivate.MQTTInfo) (CycleStatus, error) {
	if info.Username == "" {
		return CycleStatus{}, fmt.Errorf("EcoFlow Developer MQTT certification account is empty")
	}
	factory := c.transportFactory
	if factory == nil {
		factory = func(info ecoflowprivate.MQTTInfo) (quotaSubscriber, error) {
			return ecoflowprivate.NewPahoTransport(info)
		}
	}
	transport, err := factory(info)
	if err != nil {
		return CycleStatus{}, err
	}
	defer transport.Disconnect()
	return readCycleStatusFromQuotaSubscription(ctx, transport, QuotaTopic(info.Username, c.cfg.DeviceSN), c.cfg.Timeout)
}

func readCycleStatusFromQuotaSubscription(ctx context.Context, transport quotaSubscriber, topic string, timeout time.Duration) (CycleStatus, error) {
	if timeout <= 0 {
		timeout = 20 * time.Second
	}
	watchCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	type result struct {
		status CycleStatus
		err    error
	}
	results := make(chan result, 8)
	errs := make(chan error, 1)
	go func() {
		errs <- transport.Subscribe(watchCtx, topic, timeout, func(msg ecoflowprivate.MQTTMessage) {
			status, err := CycleStatusFromQuotaPayload(msg.Payload)
			select {
			case results <- result{status: status, err: err}:
			case <-watchCtx.Done():
			}
		})
	}()

	var best CycleStatus
	received := false
	for {
		select {
		case <-ctx.Done():
			cancel()
			return best, ctx.Err()
		case res := <-results:
			if res.err != nil {
				continue
			}
			received = true
			best = res.status
			if best.CycleCount != nil {
				cancel()
				return best, nil
			}
		case err := <-errs:
			if err != nil && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
				return best, err
			}
			if received {
				return best, fmt.Errorf("EcoFlow MQTT subscribe timed out waiting for cycle count quota")
			}
			return best, fmt.Errorf("EcoFlow MQTT subscribe timed out waiting for quota")
		}
	}
}

func QuotaTopic(certificateAccount string, sn string) string {
	return fmt.Sprintf("/open/%s/%s/quota", strings.Trim(certificateAccount, "/"), strings.Trim(sn, "/"))
}

func StatusTopic(certificateAccount string, sn string) string {
	return fmt.Sprintf("/open/%s/%s/status", strings.Trim(certificateAccount, "/"), strings.Trim(sn, "/"))
}

func (c Config) normalized() Config {
	c.AccessKey = strings.TrimSpace(c.AccessKey)
	c.SecretKey = strings.TrimSpace(c.SecretKey)
	c.BaseURL = strings.TrimRight(strings.TrimSpace(c.BaseURL), "/")
	c.PrivateAPIHost = strings.TrimSpace(c.PrivateAPIHost)
	c.PrivateAPIHost = strings.TrimPrefix(strings.TrimPrefix(c.PrivateAPIHost, "https://"), "http://")
	c.PrivateAPIHost = strings.TrimRight(c.PrivateAPIHost, "/")
	if c.PrivateAPIHost == "" {
		c.PrivateAPIHost = ecoflowprivate.DefaultPrivateAPIHost
	}
	c.Email = strings.TrimSpace(c.Email)
	c.DeviceSN = strings.TrimSpace(c.DeviceSN)
	c.MQTTClientID = strings.TrimSpace(c.MQTTClientID)
	if c.Timeout <= 0 {
		c.Timeout = 20 * time.Second
	}
	return c
}

func (c Config) MissingReadCredentials() []string {
	c = c.normalized()
	missing := []string{}
	if c.useOfficialDeveloperAPI() {
		if c.AccessKey == "" {
			missing = append(missing, "ECOFLOW_ACCESS_KEY")
		}
		if c.SecretKey == "" {
			missing = append(missing, "ECOFLOW_SECRET_KEY")
		}
		if c.DeviceSN == "" {
			missing = append(missing, "deviceSn")
		}
		return missing
	}
	if c.Email == "" {
		missing = append(missing, "ECOFLOW_PRIVATE_EMAIL")
	}
	if c.Password == "" {
		missing = append(missing, "ECOFLOW_PRIVATE_PASSWORD")
	}
	if c.DeviceSN == "" {
		missing = append(missing, "deviceSn")
	}
	return missing
}

func (c Config) useOfficialDeveloperAPI() bool {
	return c.AccessKey != "" || c.SecretKey != "" || c.BaseURL != ""
}

func isSessionAuthError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "unauthorized") ||
		strings.Contains(message, "not authorized") ||
		strings.Contains(message, "forbidden") ||
		strings.Contains(message, "token expired") ||
		strings.Contains(message, "bad username or password")
}
