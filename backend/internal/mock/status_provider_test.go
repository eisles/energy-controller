package mock

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/eisles/energy-controller/backend/internal/control"
	"github.com/eisles/energy-controller/backend/internal/domain"
	"github.com/eisles/energy-controller/backend/internal/ecoflow"
)

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time {
	return c.now
}

type mutableClock struct {
	now time.Time
}

func (c *mutableClock) Now() time.Time {
	return c.now
}

type staticGridReader struct {
	gridPower domain.GridPower
	updatedAt time.Time
}

func (r staticGridReader) CurrentGridPower(context.Context) (domain.GridPower, time.Time, error) {
	return r.gridPower, r.updatedAt, nil
}

type staticBatteryReader struct {
	status domain.BatteryStatus
}

func (r staticBatteryReader) GetBatteryStatus(context.Context) (domain.BatteryStatus, error) {
	return r.status, nil
}

type failingWriteClient struct{}

func (failingWriteClient) SetACChargePower(context.Context, int) error {
	return errors.New("write failed")
}

func (failingWriteClient) StopOrMinimizeCharging(context.Context) error {
	return errors.New("write failed")
}

func TestStatusProviderRecordsWouldSendWithoutMarkingCommandSent(t *testing.T) {
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	writer := ecoflow.NewMockWriteClient()
	provider := NewStatusProviderWithReaders(
		fixedClock{now: now},
		control.DefaultSettings(),
		false,
		false,
		true,
		true,
		staticGridReader{gridPower: domain.GridPower{GridW: -1600, ImportW: 0, ExportW: 1600}, updatedAt: now},
		staticBatteryReader{status: domain.BatteryStatus{Soc: 50, IsOnline: true}},
		writer,
		"ecoflow-read",
	)

	status, err := provider.CurrentStatus(context.Background())
	if err != nil {
		t.Fatalf("CurrentStatus failed: %v", err)
	}

	if provider.LastCommandSent() {
		t.Fatal("CommandSent = true, want false for mock write adapter")
	}
	if actualCommandW := provider.LastCommandActualW(); actualCommandW != nil {
		t.Fatalf("ActualCommandW = %v, want nil for mock write adapter", *actualCommandW)
	}
	if !strings.Contains(status.LastDecisionReason, "would-send") {
		t.Fatalf("LastDecisionReason = %q, want would-send marker", status.LastDecisionReason)
	}
	commands := writer.Snapshot()
	if len(commands) != 1 || commands[0].Name != "set_ac_charge_power" || commands[0].Watts != 1400 {
		t.Fatalf("recorded commands = %+v, want one set_ac_charge_power 1400", commands)
	}
}

func TestStatusProviderSuppressesRetryAfterWriteFailure(t *testing.T) {
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	clock := &mutableClock{now: now}
	provider := NewStatusProviderWithReaders(
		clock,
		control.DefaultSettings(),
		false,
		false,
		true,
		true,
		staticGridReader{gridPower: domain.GridPower{GridW: -1600, ImportW: 0, ExportW: 1600}, updatedAt: now},
		staticBatteryReader{status: domain.BatteryStatus{Soc: 50, IsOnline: true}},
		failingWriteClient{},
		"ecoflow-read",
	)

	first, err := provider.CurrentStatus(context.Background())
	if err != nil {
		t.Fatalf("first CurrentStatus failed: %v", err)
	}
	if first.LastError == nil || !strings.Contains(*first.LastError, "write failed") {
		t.Fatalf("first LastError = %v, want write failed", first.LastError)
	}

	clock.now = now.Add(30 * time.Second)
	second, err := provider.CurrentStatus(context.Background())
	if err != nil {
		t.Fatalf("second CurrentStatus failed: %v", err)
	}
	if second.LastError != nil {
		t.Fatalf("second LastError = %v, want nil because retry is suppressed", *second.LastError)
	}
	if !strings.Contains(second.LastDecisionReason, "command suppressed") {
		t.Fatalf("second LastDecisionReason = %q, want command suppressed", second.LastDecisionReason)
	}
}
