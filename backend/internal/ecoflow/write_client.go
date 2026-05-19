package ecoflow

import (
	"context"
	"fmt"
	"sync"
)

type WriteClient interface {
	SetACChargePower(ctx context.Context, watts int) error
	SetBackupReserveSoc(ctx context.Context, percent int) error
	SetTOUMode(ctx context.Context, enabled bool) error
	StopOrMinimizeCharging(ctx context.Context) error
}

type MockWriteClient struct {
	mu       sync.Mutex
	Commands []MockCommand
}

type MockCommand struct {
	Name  string
	Watts int
}

func NewMockWriteClient() *MockWriteClient {
	return &MockWriteClient{}
}

func (c *MockWriteClient) SetACChargePower(_ context.Context, watts int) error {
	if watts <= 0 {
		return fmt.Errorf("EcoFlow AC charge power must be positive: %d", watts)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Commands = append(c.Commands, MockCommand{Name: "set_ac_charge_power", Watts: watts})
	return nil
}

func (c *MockWriteClient) SetBackupReserveSoc(_ context.Context, percent int) error {
	if percent < 0 || percent > 100 {
		return fmt.Errorf("EcoFlow backup reserve SOC must be 0-100: %d", percent)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Commands = append(c.Commands, MockCommand{Name: "set_backup_reserve_soc", Watts: percent})
	return nil
}

func (c *MockWriteClient) SetTOUMode(_ context.Context, enabled bool) error {
	watts := 0
	if enabled {
		watts = 1
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Commands = append(c.Commands, MockCommand{Name: "set_tou_mode", Watts: watts})
	return nil
}

func (c *MockWriteClient) StopOrMinimizeCharging(_ context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.Commands = append(c.Commands, MockCommand{Name: "stop_or_minimize_charging", Watts: 0})
	return nil
}

func (c *MockWriteClient) Snapshot() []MockCommand {
	c.mu.Lock()
	defer c.mu.Unlock()
	commands := make([]MockCommand, len(c.Commands))
	copy(commands, c.Commands)
	return commands
}
