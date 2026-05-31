package ecoflowprivate

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
)

type Client struct {
	cfg        Config
	auth       sessionAuthenticator
	transport  MQTTTransport
	sessionKey string
	cache      *SessionCache
	sessionMu  sync.Mutex
	session    Session
	hasSession bool
}

type sessionAuthenticator interface {
	Login(ctx context.Context) (Session, error)
}

func NewClient(cfg Config) *Client {
	useDefaultSessionCache := cfg.SessionCache == nil && cfg.HTTPClient == nil
	cfg = cfg.normalized()
	if useDefaultSessionCache {
		cfg.SessionCache = defaultSessionCache
	}
	return &Client{cfg: cfg, auth: NewAuthClient(cfg), sessionKey: privateSessionCacheKey(cfg), cache: cfg.SessionCache}
}

func NewClientWithTransport(cfg Config, transport MQTTTransport) *Client {
	useDefaultSessionCache := cfg.SessionCache == nil && cfg.HTTPClient == nil
	cfg = cfg.normalized()
	if useDefaultSessionCache {
		cfg.SessionCache = defaultSessionCache
	}
	return &Client{cfg: cfg, auth: NewAuthClient(cfg), transport: transport, sessionKey: privateSessionCacheKey(cfg), cache: cfg.SessionCache}
}

func (c *Client) Probe(ctx context.Context) (Status, error) {
	if missing := c.cfg.MissingReadCredentials(); len(missing) > 0 {
		return Status{DeviceType: c.cfg.DeviceType, DeviceSN: c.cfg.DeviceSN}, fmt.Errorf("EcoFlow private probe missing required env: %v", missing)
	}
	session, fromCache, err := c.cachedSession(ctx)
	if err != nil {
		return Status{}, err
	}
	status, err := c.probeWithSession(ctx, session)
	if err != nil && fromCache && isSessionAuthError(err) {
		c.invalidateSession()
		session, _, sessionErr := c.cachedSession(ctx)
		if sessionErr != nil {
			return Status{}, sessionErr
		}
		return c.probeWithSession(ctx, session)
	}
	return status, err
}

func (c *Client) ProbeRaw(ctx context.Context) (Status, []MQTTMessage, error) {
	if missing := c.cfg.MissingReadCredentials(); len(missing) > 0 {
		return Status{DeviceType: c.cfg.DeviceType, DeviceSN: c.cfg.DeviceSN}, nil, fmt.Errorf("EcoFlow private probe missing required env: %v", missing)
	}
	session, fromCache, err := c.cachedSession(ctx)
	if err != nil {
		return Status{}, nil, err
	}
	status, replies, err := c.probeRawWithSession(ctx, session)
	if err != nil && fromCache && isSessionAuthError(err) {
		c.invalidateSession()
		session, _, sessionErr := c.cachedSession(ctx)
		if sessionErr != nil {
			return Status{}, nil, sessionErr
		}
		return c.probeRawWithSession(ctx, session)
	}
	return status, replies, err
}

func (c *Client) probeWithSession(ctx context.Context, session Session) (Status, error) {
	status, _, err := c.probeRawWithSession(ctx, session)
	return status, err
}

func (c *Client) probeRawWithSession(ctx context.Context, session Session) (Status, []MQTTMessage, error) {
	transport := c.transport
	if transport == nil {
		paho, err := NewPahoTransport(session.MQTT)
		if err != nil {
			return Status{}, nil, err
		}
		transport = paho
		defer transport.Disconnect()
	}
	topics := BuildTopics(session.UserID, c.cfg.DeviceSN)
	replies, err := transport.Request(ctx, topics.Get, BuildGetSnapshotPayload(NextSeq()), []string{topics.GetReply, topics.Data}, c.cfg.Timeout)
	if err != nil {
		return Status{}, replies, err
	}
	status := Status{DeviceType: c.cfg.DeviceType, DeviceSN: c.cfg.DeviceSN}
	for _, reply := range replies {
		part, err := DecodeSnapshot(c.cfg.DeviceType, c.cfg.DeviceSN, reply.Payload)
		if err != nil {
			status.UnsupportedMessages++
			continue
		}
		status.merge(part)
	}
	return status, replies, nil
}

func (c *Client) BuildDryRunACChargePower(watts int) (CommandPayload, error) {
	if err := c.validateDryRunTarget(); err != nil {
		return CommandPayload{}, err
	}
	if err := ValidateACChargePower(c.cfg.DeviceType, watts); err != nil {
		return CommandPayload{}, err
	}
	payload, err := BuildSetACChargePowerPayload(c.cfg.DeviceSN, watts, 1)
	if err != nil {
		return CommandPayload{}, err
	}
	return CommandPayload{
		Command: "set_ac_charge_power",
		Topic:   BuildTopics("USER_ID", c.cfg.DeviceSN).Set,
		Bytes:   payload,
		Hex:     hex.EncodeToString(payload),
	}, nil
}

func (c *Client) BuildDryRunBackupReserve(percent int) (CommandPayload, error) {
	if err := c.validateDryRunTarget(); err != nil {
		return CommandPayload{}, err
	}
	payload, err := BuildSetBackupReservePayload(c.cfg.DeviceSN, percent, 1)
	if err != nil {
		return CommandPayload{}, err
	}
	return CommandPayload{
		Command: "set_backup_reserve_soc",
		Topic:   BuildTopics("USER_ID", c.cfg.DeviceSN).Set,
		Bytes:   payload,
		Hex:     hex.EncodeToString(payload),
	}, nil
}

func (c *Client) BuildDryRunGridBypassDisabled(disabled bool) (CommandPayload, error) {
	if err := c.validateDryRunTarget(); err != nil {
		return CommandPayload{}, err
	}
	payload, err := BuildSetGridBypassDisabledPayload(c.cfg.DeviceSN, disabled, 1)
	if err != nil {
		return CommandPayload{}, err
	}
	return CommandPayload{
		Command: "set_grid_bypass_disabled",
		Topic:   BuildTopics("USER_ID", c.cfg.DeviceSN).Set,
		Bytes:   payload,
		Hex:     hex.EncodeToString(payload),
	}, nil
}

func (c *Client) BuildDryRunMinDischargeSoc(percent int) (CommandPayload, error) {
	if err := c.validateDryRunTarget(); err != nil {
		return CommandPayload{}, err
	}
	payload, err := BuildSetMinDischargeSocPayload(c.cfg.DeviceSN, percent, 1)
	if err != nil {
		return CommandPayload{}, err
	}
	return CommandPayload{
		Command: "set_min_discharge_soc",
		Topic:   BuildTopics("USER_ID", c.cfg.DeviceSN).Set,
		Bytes:   payload,
		Hex:     hex.EncodeToString(payload),
	}, nil
}

func (c *Client) BuildDryRunMaxChargeSoc(percent int) (CommandPayload, error) {
	if err := c.validateDryRunTarget(); err != nil {
		return CommandPayload{}, err
	}
	payload, err := BuildSetMaxChargeSocPayload(c.cfg.DeviceSN, percent, 1)
	if err != nil {
		return CommandPayload{}, err
	}
	return CommandPayload{
		Command: "set_max_charge_soc",
		Topic:   BuildTopics("USER_ID", c.cfg.DeviceSN).Set,
		Bytes:   payload,
		Hex:     hex.EncodeToString(payload),
	}, nil
}

func (c *Client) BuildDryRunEnergyBackupEnabled(enabled bool, startSoc int) (CommandPayload, error) {
	if err := c.validateDryRunTarget(); err != nil {
		return CommandPayload{}, err
	}
	payload, err := BuildSetEnergyBackupEnabledPayload(c.cfg.DeviceSN, enabled, startSoc, 1)
	if err != nil {
		return CommandPayload{}, err
	}
	return CommandPayload{
		Command: "set_energy_backup_enabled",
		Topic:   BuildTopics("USER_ID", c.cfg.DeviceSN).Set,
		Bytes:   payload,
		Hex:     hex.EncodeToString(payload),
	}, nil
}

func (c *Client) validateDryRunTarget() error {
	if c.cfg.DeviceSN == "" {
		return fmt.Errorf("EcoFlow private dry-run requires ECOFLOW_DELTA3_DEVICE_SN or --sn")
	}
	if _, ok := RangeForDeviceType(c.cfg.DeviceType); !ok {
		return fmt.Errorf("unsupported device type for EcoFlow private dry-run: %s", c.cfg.DeviceType)
	}
	return nil
}

func (c *Client) ExecuteACChargePower(ctx context.Context, watts int, guards WriteGuards) (Status, error) {
	if err := ValidateACChargePower(c.cfg.DeviceType, watts); err != nil {
		return Status{}, err
	}
	guards.Command = "set_ac_charge_power"
	guards.DeviceType = c.cfg.DeviceType
	if err := guards.Validate(); err != nil {
		return Status{}, err
	}
	return c.executeSet(ctx, func(seq int) ([]byte, error) {
		return BuildSetACChargePowerPayload(c.cfg.DeviceSN, watts, seq)
	})
}

func (c *Client) ExecuteBackupReserve(ctx context.Context, percent int, guards WriteGuards) (Status, error) {
	if err := ValidateBackupReserveSoc(percent); err != nil {
		return Status{}, err
	}
	guards.Command = "set_backup_reserve_soc"
	guards.DeviceType = c.cfg.DeviceType
	if err := guards.Validate(); err != nil {
		return Status{}, err
	}
	return c.executeSet(ctx, func(seq int) ([]byte, error) {
		return BuildSetBackupReservePayload(c.cfg.DeviceSN, percent, seq)
	})
}

func (c *Client) ExecuteGridBypassDisabled(ctx context.Context, disabled bool, guards WriteGuards) (Status, error) {
	guards.Command = "set_grid_bypass_disabled"
	guards.DeviceType = c.cfg.DeviceType
	if err := guards.Validate(); err != nil {
		return Status{}, err
	}
	return c.executeSet(ctx, func(seq int) ([]byte, error) {
		return BuildSetGridBypassDisabledPayload(c.cfg.DeviceSN, disabled, seq)
	})
}

func (c *Client) ExecuteMinDischargeSoc(ctx context.Context, percent int, guards WriteGuards) (Status, error) {
	if err := ValidateMinDischargeSoc(percent); err != nil {
		return Status{}, err
	}
	guards.Command = "set_min_discharge_soc"
	guards.DeviceType = c.cfg.DeviceType
	if err := guards.Validate(); err != nil {
		return Status{}, err
	}
	return c.executeSet(ctx, func(seq int) ([]byte, error) {
		return BuildSetMinDischargeSocPayload(c.cfg.DeviceSN, percent, seq)
	})
}

func (c *Client) ExecuteMaxChargeSoc(ctx context.Context, percent int, guards WriteGuards) (Status, error) {
	if err := ValidateMaxChargeSoc(percent); err != nil {
		return Status{}, err
	}
	guards.Command = "set_max_charge_soc"
	guards.DeviceType = c.cfg.DeviceType
	if err := guards.Validate(); err != nil {
		return Status{}, err
	}
	return c.executeSet(ctx, func(seq int) ([]byte, error) {
		return BuildSetMaxChargeSocPayload(c.cfg.DeviceSN, percent, seq)
	})
}

func (c *Client) ExecuteEnergyBackupEnabled(ctx context.Context, enabled bool, startSoc int, guards WriteGuards) (Status, error) {
	if err := ValidateBackupReserveSoc(startSoc); err != nil {
		return Status{}, err
	}
	guards.Command = "set_energy_backup_enabled"
	guards.DeviceType = c.cfg.DeviceType
	if err := guards.Validate(); err != nil {
		return Status{}, err
	}
	return c.executeSet(ctx, func(seq int) ([]byte, error) {
		return BuildSetEnergyBackupEnabledPayload(c.cfg.DeviceSN, enabled, startSoc, seq)
	})
}

func (c *Client) executeSet(ctx context.Context, build func(seq int) ([]byte, error)) (Status, error) {
	if missing := c.cfg.MissingReadCredentials(); len(missing) > 0 {
		return Status{DeviceType: c.cfg.DeviceType, DeviceSN: c.cfg.DeviceSN}, fmt.Errorf("EcoFlow private write missing required env: %v", missing)
	}
	session, fromCache, err := c.cachedSession(ctx)
	if err != nil {
		return Status{}, err
	}
	status, err := c.executeSetWithSession(ctx, session, build)
	if err != nil && fromCache && isSessionAuthError(err) {
		c.invalidateSession()
		session, _, sessionErr := c.cachedSession(ctx)
		if sessionErr != nil {
			return Status{}, sessionErr
		}
		return c.executeSetWithSession(ctx, session, build)
	}
	return status, err
}

func (c *Client) executeSetWithSession(ctx context.Context, session Session, build func(seq int) ([]byte, error)) (Status, error) {
	transport := c.transport
	if transport == nil {
		paho, err := NewPahoTransport(session.MQTT)
		if err != nil {
			return Status{}, err
		}
		transport = paho
		defer transport.Disconnect()
	}
	seq := NextSeq()
	payload, err := build(seq)
	if err != nil {
		return Status{}, err
	}
	topics := BuildTopics(session.UserID, c.cfg.DeviceSN)
	replies, err := transport.Request(ctx, topics.Set, payload, []string{topics.SetReply, topics.Data}, c.cfg.Timeout)
	if err != nil {
		return Status{}, err
	}
	status := Status{DeviceType: c.cfg.DeviceType, DeviceSN: c.cfg.DeviceSN}
	matchedSetReply := false
	for _, reply := range replies {
		part, err := DecodeSnapshot(c.cfg.DeviceType, c.cfg.DeviceSN, reply.Payload)
		if err != nil {
			status.UnsupportedMessages++
			continue
		}
		if part.LastSetReplyConfigOK != nil {
			if part.LastSetReplySeq == nil || *part.LastSetReplySeq != seq {
				continue
			}
			matchedSetReply = true
		}
		status.merge(part)
	}
	if !matchedSetReply || status.LastSetReplyConfigOK == nil {
		return status, fmt.Errorf("EcoFlow private write did not return set acknowledgement")
	}
	if !*status.LastSetReplyConfigOK {
		return status, fmt.Errorf("EcoFlow private write was rejected by device")
	}
	return status, nil
}

func (c *Client) cachedSession(ctx context.Context) (Session, bool, error) {
	c.sessionMu.Lock()
	defer c.sessionMu.Unlock()
	if c.hasSession {
		return c.session, true, nil
	}
	session, fromCache, err := c.cache.getOrLogin(ctx, c.sessionKey, c.auth)
	if err != nil {
		return Session{}, false, err
	}
	session, err = c.sessionForClient(session, fromCache)
	if err != nil {
		return Session{}, false, err
	}
	c.session = session
	c.hasSession = true
	return session, fromCache, nil
}

func (c *Client) sessionForClient(session Session, fromSharedCache bool) (Session, error) {
	if !fromSharedCache || c.cfg.MQTTClientID != "" {
		return session, nil
	}
	clientID, err := newPrivateMQTTClientID(session.UserID)
	if err != nil {
		return Session{}, err
	}
	session.MQTT.ClientID = clientID
	return session, nil
}

func (c *Client) invalidateSession() {
	c.sessionMu.Lock()
	defer c.sessionMu.Unlock()
	c.session = Session{}
	c.hasSession = false
	c.cache.invalidate(c.sessionKey)
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
