package ecoflowdelta3

import (
	"context"
	"encoding/hex"
	"fmt"
)

type Client struct {
	cfg       Config
	auth      *AuthClient
	transport MQTTTransport
}

func NewClient(cfg Config) *Client {
	cfg = cfg.normalized()
	return &Client{cfg: cfg, auth: NewAuthClient(cfg)}
}

func NewClientWithTransport(cfg Config, transport MQTTTransport) *Client {
	cfg = cfg.normalized()
	return &Client{cfg: cfg, auth: NewAuthClient(cfg), transport: transport}
}

func (c *Client) Probe(ctx context.Context) (Status, error) {
	if missing := c.cfg.MissingReadCredentials(); len(missing) > 0 {
		return Status{DeviceType: c.cfg.DeviceType, DeviceSN: c.cfg.DeviceSN}, fmt.Errorf("EcoFlow DELTA_3 probe missing required env: %v", missing)
	}
	session, err := c.auth.Login(ctx)
	if err != nil {
		return Status{}, err
	}
	transport := c.transport
	if transport == nil {
		paho, err := NewPahoTransport(session.MQTT)
		if err != nil {
			return Status{}, err
		}
		transport = paho
		defer transport.Disconnect()
	}
	topics := BuildTopics(session.UserID, c.cfg.DeviceSN)
	replies, err := transport.Request(ctx, topics.Get, BuildGetSnapshotPayload(NextSeq()), []string{topics.GetReply, topics.Data}, c.cfg.Timeout)
	if err != nil {
		return Status{}, err
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
	return status, nil
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

func (c *Client) validateDryRunTarget() error {
	if c.cfg.DeviceSN == "" {
		return fmt.Errorf("DELTA_3 dry-run requires ECOFLOW_DELTA3_DEVICE_SN or --sn")
	}
	if _, ok := RangeForDeviceType(c.cfg.DeviceType); !ok {
		return fmt.Errorf("unsupported device type for DELTA_3 dry-run: %s", c.cfg.DeviceType)
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

func (c *Client) executeSet(ctx context.Context, build func(seq int) ([]byte, error)) (Status, error) {
	if missing := c.cfg.MissingReadCredentials(); len(missing) > 0 {
		return Status{DeviceType: c.cfg.DeviceType, DeviceSN: c.cfg.DeviceSN}, fmt.Errorf("EcoFlow DELTA_3 write missing required env: %v", missing)
	}
	session, err := c.auth.Login(ctx)
	if err != nil {
		return Status{}, err
	}
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
		return status, fmt.Errorf("DELTA_3 private write did not return set acknowledgement")
	}
	if !*status.LastSetReplyConfigOK {
		return status, fmt.Errorf("DELTA_3 private write was rejected by device")
	}
	return status, nil
}
