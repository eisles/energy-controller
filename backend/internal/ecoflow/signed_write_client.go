package ecoflow

import (
	"context"
	"encoding/json"
	"fmt"
)

type WriteGuards struct {
	MockMode           bool
	SimulationMode     bool
	EnableRealControl  bool
	AutoControlEnabled bool
}

type SignedWriteClient struct {
	client *SignedClient
	guards WriteGuards
}

func NewSignedWriteClient(cfg Config, guards WriteGuards) *SignedWriteClient {
	return &SignedWriteClient{
		client: NewSignedClient(cfg),
		guards: guards,
	}
}

func (c *SignedWriteClient) SetACChargePower(ctx context.Context, watts int) error {
	if err := c.guard(); err != nil {
		return err
	}
	payload, err := buildSetACChargePowerPayload(c.client.deviceSN, watts)
	if err != nil {
		return err
	}
	return c.putCommand(ctx, payload)
}

func (c *SignedWriteClient) StopOrMinimizeCharging(context.Context) error {
	return fmt.Errorf("EcoFlow stop/minimize real write is not implemented")
}

func (c *SignedWriteClient) guard() error {
	switch {
	case c.guards.MockMode:
		return fmt.Errorf("EcoFlow real write disabled: MOCK_MODE=true")
	case c.guards.SimulationMode:
		return fmt.Errorf("EcoFlow real write disabled: SIMULATION_MODE=true")
	case !c.guards.EnableRealControl:
		return fmt.Errorf("EcoFlow real write disabled: ENABLE_REAL_CONTROL=false")
	case !c.guards.AutoControlEnabled:
		return fmt.Errorf("EcoFlow real write disabled: AUTO_CONTROL_ENABLED=false")
	case c.client.accessKey == "" || c.client.secretKey == "" || c.client.deviceSN == "":
		return fmt.Errorf("EcoFlow real write disabled: access key, secret key, or device SN is empty")
	default:
		return nil
	}
}

func (c *SignedWriteClient) putCommand(ctx context.Context, payload commandPayload) error {
	req, err := c.client.newSignedPUTRequest(ctx, "/iot-open/sign/device/quota", payload)
	if err != nil {
		return err
	}
	resp, err := c.client.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("EcoFlow write request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("EcoFlow write returned HTTP %d", resp.StatusCode)
	}
	var result writeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode EcoFlow write response: %w", err)
	}
	if result.Code != "0" {
		return fmt.Errorf("EcoFlow write returned code=%s message=%s", result.Code, result.Message)
	}
	return nil
}

type writeResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

var _ WriteClient = (*SignedWriteClient)(nil)
