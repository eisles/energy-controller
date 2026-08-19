package main

import (
	"context"
	"log/slog"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/eisles/energy-controller/backend/internal/api"
	"github.com/eisles/energy-controller/backend/internal/config"
	"github.com/eisles/energy-controller/backend/internal/domain"
	"github.com/eisles/energy-controller/backend/internal/ecoflow"
	"github.com/eisles/energy-controller/backend/internal/store"
)

type stubStatusProvider struct {
	status         domain.Status
	actualCommandW *int
	commandSent    bool
}

type fixedMainClock struct {
	now time.Time
}

func (c fixedMainClock) Now() time.Time {
	return c.now
}

func (p stubStatusProvider) CurrentStatus(context.Context) (domain.Status, error) {
	return p.status, nil
}

func (p stubStatusProvider) LastCommandActualW() *int {
	return p.actualCommandW
}

func (p stubStatusProvider) LastCommandSent() bool {
	return p.commandSent
}

type stubDelta3StatusReader struct {
	status api.Delta3StatusResponse
}

func (r stubDelta3StatusReader) CurrentStatus(context.Context) api.Delta3StatusResponse {
	return r.status
}

type fakeEcoFlowCloudWriteTargetProvider struct {
	device domain.ChargingDevice
	ok     bool
	err    error
}

func (p fakeEcoFlowCloudWriteTargetProvider) EcoFlowCloudWriteTarget(context.Context) (domain.ChargingDevice, bool, error) {
	return p.device, p.ok, p.err
}

type fakeDelta3WriteTargetProvider struct {
	device domain.ChargingDevice
	ok     bool
	err    error
}

type fakeDelta3WriteTargetsProvider struct{ devices []domain.ChargingDevice }

func (p fakeDelta3WriteTargetsProvider) Delta3WriteTarget(context.Context) (domain.ChargingDevice, bool, error) {
	if len(p.devices) == 0 {
		return domain.ChargingDevice{}, false, nil
	}
	return p.devices[0], true, nil
}
func (p fakeDelta3WriteTargetsProvider) Delta3WriteTargets(context.Context) ([]domain.ChargingDevice, error) {
	return p.devices, nil
}

type configDelta3StatusReader struct {
	statuses map[string]api.Delta3StatusResponse
}

func (r configDelta3StatusReader) CurrentStatus(context.Context) api.Delta3StatusResponse {
	return api.Delta3StatusResponse{}
}
func (r configDelta3StatusReader) CurrentStatusForConfig(_ context.Context, cfg config.Config) api.Delta3StatusResponse {
	return r.statuses[cfg.Delta3DeviceSN]
}

type deviceBoundRecordingWriter struct{ writes []string }
type boundRecordingWriter struct {
	parent *deviceBoundRecordingWriter
	sn     string
}

func (w *deviceBoundRecordingWriter) ForDevice(device domain.ChargingDevice) delta3AuxWriteClient {
	return &boundRecordingWriter{parent: w, sn: device.DeviceSN}
}
func (w *deviceBoundRecordingWriter) SetACChargePower(context.Context, int) error { return nil }
func (w *deviceBoundRecordingWriter) SetEnergyBackupEnabled(context.Context, bool, int) error {
	return nil
}
func (w *boundRecordingWriter) SetACChargePower(_ context.Context, watts int) error {
	w.parent.writes = append(w.parent.writes, w.sn+":ac="+strconv.Itoa(watts))
	return nil
}
func (w *boundRecordingWriter) SetEnergyBackupEnabled(_ context.Context, _ bool, soc int) error {
	w.parent.writes = append(w.parent.writes, w.sn+":reserve="+strconv.Itoa(soc))
	return nil
}

type deviceCommandRepository struct {
	logs     []domain.Delta3AuxControlCommandLog
	previous map[int64]*domain.Delta3AuxControlCommandLog
}

func (r *deviceCommandRepository) InsertDelta3AuxControlCommandLog(_ context.Context, log domain.Delta3AuxControlCommandLog) error {
	r.logs = append(r.logs, log)
	return nil
}
func (r *deviceCommandRepository) LatestDelta3AuxControlCommandLog(context.Context) (*domain.Delta3AuxControlCommandLog, error) {
	return nil, nil
}
func (r *deviceCommandRepository) LatestDelta3AuxControlWriteCandidateLog(context.Context) (*domain.Delta3AuxControlCommandLog, error) {
	return nil, nil
}
func (r *deviceCommandRepository) LatestDelta3AuxReserveCommandLog(context.Context) (*domain.Delta3AuxControlCommandLog, error) {
	return nil, nil
}
func (r *deviceCommandRepository) LatestDelta3AuxControlWriteCandidateLogForDevice(_ context.Context, id int64) (*domain.Delta3AuxControlCommandLog, error) {
	if v := r.previous[id]; v != nil {
		return v, nil
	}
	return r.previous[0], nil
}
func (r *deviceCommandRepository) LatestDelta3AuxReserveCommandLogForDevice(context.Context, int64) (*domain.Delta3AuxControlCommandLog, error) {
	return nil, nil
}

func (p fakeDelta3WriteTargetProvider) Delta3WriteTarget(context.Context) (domain.ChargingDevice, bool, error) {
	return p.device, p.ok, p.err
}

type fakeChargingPriorityTargetProvider struct {
	pro3     domain.ChargingDevice
	pro3OK   bool
	delta3   domain.ChargingDevice
	delta3OK bool
}

func (p fakeChargingPriorityTargetProvider) EcoFlowCloudWriteTarget(context.Context) (domain.ChargingDevice, bool, error) {
	return p.pro3, p.pro3OK, nil
}

func (p fakeChargingPriorityTargetProvider) Delta3WriteTarget(context.Context) (domain.ChargingDevice, bool, error) {
	return p.delta3, p.delta3OK, nil
}

type recordingSurplusWriteClient struct {
	acChargePowerW   *int
	backupReserveSoc *int
}

type recordingDelta3AuxWriteClient struct {
	acChargePowerW   *int
	backupReserveSoc *int
}

type fakeDelta3AuxCommandRepository struct {
	previous *domain.Delta3AuxControlCommandLog
}

type fakePro3ACOutputEventRepository struct {
	latest *domain.Pro3ACOutputEvent
}

func (c *recordingSurplusWriteClient) SetACChargePower(_ context.Context, watts int) error {
	c.acChargePowerW = intPtr(watts)
	return nil
}

func (c *recordingDelta3AuxWriteClient) SetACChargePower(_ context.Context, watts int) error {
	c.acChargePowerW = intPtr(watts)
	return nil
}

func (c *recordingDelta3AuxWriteClient) SetEnergyBackupEnabled(_ context.Context, enabled bool, startSoc int) error {
	if enabled {
		c.backupReserveSoc = intPtr(startSoc)
	}
	return nil
}

func (r fakeDelta3AuxCommandRepository) InsertDelta3AuxControlCommandLog(context.Context, domain.Delta3AuxControlCommandLog) error {
	return nil
}

func (r fakeDelta3AuxCommandRepository) LatestDelta3AuxControlCommandLog(context.Context) (*domain.Delta3AuxControlCommandLog, error) {
	return r.previous, nil
}

func (r fakeDelta3AuxCommandRepository) LatestDelta3AuxControlWriteCandidateLog(context.Context) (*domain.Delta3AuxControlCommandLog, error) {
	return r.previous, nil
}

func (r fakeDelta3AuxCommandRepository) LatestDelta3AuxReserveCommandLog(context.Context) (*domain.Delta3AuxControlCommandLog, error) {
	if r.previous != nil && r.previous.TargetBackupReserveSoc != nil {
		return r.previous, nil
	}
	return nil, nil
}

func (r *fakePro3ACOutputEventRepository) InsertPro3ACOutputEvent(_ context.Context, event domain.Pro3ACOutputEvent) error {
	r.latest = &event
	return nil
}

func (r *fakePro3ACOutputEventRepository) LatestPro3ACOutputEvent(_ context.Context) (*domain.Pro3ACOutputEvent, error) {
	return r.latest, nil
}

func (r *fakePro3ACOutputEventRepository) LatestPro3ACOutputEventByType(_ context.Context, _ string) (*domain.Pro3ACOutputEvent, error) {
	return r.latest, nil
}

func (c *recordingSurplusWriteClient) SetBackupReserveSoc(_ context.Context, percent int) error {
	c.backupReserveSoc = intPtr(percent)
	return nil
}

func (c *recordingSurplusWriteClient) SetTOUMode(context.Context, bool) error {
	return nil
}

func (c *recordingSurplusWriteClient) SetSelfPoweredMode(context.Context, bool) error {
	return nil
}

func TestRecordStatusPersistsWouldSendLogWithoutCommandSent(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "energy.db"))
	if err != nil {
		t.Fatalf("store.Open failed: %v", err)
	}
	defer db.Close()

	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	provider := stubStatusProvider{
		status: domain.Status{
			GridW:              -1600,
			ImportW:            0,
			ExportW:            1600,
			BatterySoc:         50,
			TargetChargeW:      1400,
			State:              "simulation",
			Mode:               "ecoflow-read",
			LastDecisionReason: "export power is above start threshold; EcoFlow mock write adapter recorded would-send command",
			UpdatedAt:          now,
		},
		commandSent: false,
	}

	recordStatus(
		context.Background(),
		config.Config{},
		provider,
		store.NewStatusRepository(db),
		store.NewLogRepository(db),
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		slog.Default(),
	)

	logs, err := store.NewLogRepository(db).ListPowerLogs(context.Background(), 1)
	if err != nil {
		t.Fatalf("ListPowerLogs failed: %v", err)
	}
	if len(logs) != 1 {
		t.Fatalf("len(logs) = %d, want 1", len(logs))
	}
	if logs[0].CommandSent {
		t.Fatal("CommandSent = true, want false for would-send")
	}
	if logs[0].ActualCommandW != nil {
		t.Fatalf("ActualCommandW = %v, want nil for would-send", *logs[0].ActualCommandW)
	}
	if !strings.Contains(logs[0].DecisionReason, "would-send") {
		t.Fatalf("DecisionReason = %q, want would-send marker", logs[0].DecisionReason)
	}
}

func TestRecordPro3ACOutputEventClearsCurrentStatusWhenOffMemoryIsStateRestoreSettingOnly(t *testing.T) {
	now := time.Date(2026, 5, 28, 19, 20, 0, 0, time.UTC)
	repo := &fakePro3ACOutputEventRepository{
		latest: &domain.Pro3ACOutputEvent{
			MeasuredAt:           now.Add(-10 * time.Minute),
			EventType:            "ac_output_off_memory",
			OutputPowerOffMemory: true,
			Message:              "previous event",
			CreatedAt:            now.Add(-10 * time.Minute),
		},
	}
	status := domain.Status{
		UpdatedAt:          now,
		EcoFlowDiagnostics: map[string]any{"outputPowerOffMemory": true, "acOutFreq": 60},
		Pro3ACOutputEvent:  repo.latest,
		LastDecisionReason: "normal",
		State:              "running",
		Mode:               "test",
		BatterySoc:         50,
		BatteryInputW:      0,
		BatteryOutputW:     200,
		ACChargeLimitW:     400,
		TargetChargeW:      0,
	}

	recordPro3ACOutputEvent(
		context.Background(),
		config.Config{Clock: fixedMainClock{now: now}},
		&status,
		nil,
		repo,
		nil,
		nil,
		slog.Default(),
	)

	if status.Pro3ACOutputEvent != nil {
		t.Fatalf("Pro3ACOutputEvent = %+v, want nil when outputPowerOffMemory is only the state restore setting", status.Pro3ACOutputEvent)
	}
}

func TestEcoFlowCloudSurplusWriteClientRequiresMasterWriteTarget(t *testing.T) {
	factoryCalled := false
	client := ecoFlowCloudSurplusWriteClient{
		cfg: config.Config{
			EcoFlowAccessKey: "access",
			EcoFlowSecretKey: "secret",
			EcoFlowDeviceSN:  "ENV-SN",
		},
		targetProvider: fakeEcoFlowCloudWriteTargetProvider{ok: false},
		factory: func(ecoflow.Config, ecoflow.WriteGuards) surplusWriteClient {
			factoryCalled = true
			return &recordingSurplusWriteClient{}
		},
	}

	err := client.SetACChargePower(context.Background(), 900)
	if err == nil || !strings.Contains(err.Error(), "master write target is unavailable") {
		t.Fatalf("SetACChargePower error = %v, want unavailable master target", err)
	}
	if factoryCalled {
		t.Fatal("writer factory was called without a control-enabled master target")
	}
}

func TestEcoFlowCloudSurplusWriteClientUsesMasterSNAndGuards(t *testing.T) {
	recordingClient := &recordingSurplusWriteClient{}
	var gotConfig ecoflow.Config
	var gotGuards ecoflow.WriteGuards
	client := ecoFlowCloudSurplusWriteClient{
		cfg: config.Config{
			EcoFlowAccessKey:    "access",
			EcoFlowSecretKey:    "secret",
			EcoFlowDeviceSN:     "ENV-SN",
			EcoFlowBaseURL:      "https://api.test",
			MockMode:            false,
			SimulationMode:      false,
			EnableRealControl:   true,
			AutoControlEnabled:  true,
			ConfirmEcoFlowWrite: "I_UNDERSTAND_REAL_ECOFLOW_WRITE",
		},
		targetProvider: fakeEcoFlowCloudWriteTargetProvider{
			ok: true,
			device: domain.ChargingDevice{
				DeviceSN: "MASTER-SN",
			},
		},
		factory: func(cfg ecoflow.Config, guards ecoflow.WriteGuards) surplusWriteClient {
			gotConfig = cfg
			gotGuards = guards
			return recordingClient
		},
	}

	if err := client.SetACChargePower(context.Background(), 900); err != nil {
		t.Fatalf("SetACChargePower failed: %v", err)
	}
	if gotConfig.DeviceSN != "MASTER-SN" {
		t.Fatalf("DeviceSN = %q, want MASTER-SN", gotConfig.DeviceSN)
	}
	if gotConfig.AccessKey != "access" || gotConfig.SecretKey != "secret" || gotConfig.BaseURL != "https://api.test" {
		t.Fatalf("EcoFlow config = %+v, want env credentials with master SN", gotConfig)
	}
	if gotGuards.MockMode || gotGuards.SimulationMode || !gotGuards.EnableRealControl || !gotGuards.AutoControlEnabled || gotGuards.ManualOneShot {
		t.Fatalf("WriteGuards = %+v, want real-control auto guards from config", gotGuards)
	}
	if recordingClient.acChargePowerW == nil || *recordingClient.acChargePowerW != 900 {
		t.Fatalf("recorded AC charge W = %v, want 900", recordingClient.acChargePowerW)
	}
}

func TestChargingPriorityContextRequiresDelta3AuxControlReadiness(t *testing.T) {
	priority := chargingPriorityContext{
		pro3OK: true,
		pro3: domain.ChargingDevice{
			Name:     "DELTA Pro 3",
			Priority: 20,
		},
		delta3OK: true,
		delta3: domain.ChargingDevice{
			Name:     "DELTA 3 Plus",
			Priority: 10,
		},
	}

	if priority.ignorePro3WaitForDelta3(config.Config{}) {
		t.Fatal("ignorePro3WaitForDelta3 without aux/read = true, want false")
	}

	cfg := config.Config{
		Delta3ReadEnabled: true,
		Delta3Aux: config.Delta3AuxConfig{
			Enabled: true,
		},
	}
	if !priority.ignorePro3WaitForDelta3(cfg) {
		t.Fatal("ignorePro3WaitForDelta3 = false, want true")
	}
}

func TestDelta3AuxSettingsForDeviceUsesMasterReserveAsDischargeFloor(t *testing.T) {
	settings := delta3AuxSettingsForDevice(config.Config{
		Delta3Aux: config.Delta3AuxConfig{
			Enabled:              true,
			MinChargeW:           100,
			MaxChargeW:           1400,
			SafetyMarginW:        50,
			MinCommandDiffW:      100,
			StopImportThresholdW: 50,
		},
	}, domain.ChargingDevice{
		MinChargeW: 100,
		MaxChargeW: 1400,
		ReserveSoc: 25,
		TargetSoc:  80,
	})

	if settings.MinDischargeReserveSoc != 25 {
		t.Fatalf("MinDischargeReserveSoc = %d, want 25", settings.MinDischargeReserveSoc)
	}
	if settings.BackupReserveMinSoc != 25 {
		t.Fatalf("BackupReserveMinSoc = %d, want 25", settings.BackupReserveMinSoc)
	}
	if settings.BackupReserveMaxSoc != 80 {
		t.Fatalf("BackupReserveMaxSoc = %d, want 80", settings.BackupReserveMaxSoc)
	}
}

func TestDelta3AuxSettingsForDeviceUsesBackupReserveRange(t *testing.T) {
	settings := delta3AuxSettingsForDevice(config.Config{
		Delta3Aux: config.Delta3AuxConfig{
			Enabled:              true,
			MinChargeW:           100,
			MaxChargeW:           1400,
			SafetyMarginW:        50,
			MinCommandDiffW:      100,
			StopImportThresholdW: 50,
		},
	}, domain.ChargingDevice{
		MinChargeW:          100,
		MaxChargeW:          1400,
		ReserveSoc:          25,
		TargetSoc:           80,
		BackupReserveMinSoc: 30,
		BackupReserveMaxSoc: 75,
	})

	if settings.MinDischargeReserveSoc != 30 || settings.BackupReserveMinSoc != 30 {
		t.Fatalf("reserve min settings = MinDischargeReserveSoc:%d BackupReserveMinSoc:%d, want 30", settings.MinDischargeReserveSoc, settings.BackupReserveMinSoc)
	}
	if settings.BackupReserveMaxSoc != 75 {
		t.Fatalf("BackupReserveMaxSoc = %d, want 75", settings.BackupReserveMaxSoc)
	}
}

func TestApplyPro3MasterReserveToSurplusPlanLowersReserveToMasterFloorWhenImporting(t *testing.T) {
	reserve := 15
	tou := true
	status := domain.Status{
		GridW:              1500,
		ImportW:            1500,
		ExportW:            0,
		BatterySoc:         16,
		ACChargeLimitW:     400,
		BackupReserveSoc:   &reserve,
		TOUModeEnabled:     &tou,
		LastDecisionReason: "importing from grid, do not charge",
	}

	applyPro3MasterReserveToSurplusPlan(config.Config{
		MockMode:           false,
		SimulationMode:     false,
		EnableRealControl:  true,
		AutoControlEnabled: true,
	}, &status, chargingPriorityContext{
		pro3OK: true,
		pro3: domain.ChargingDevice{
			ReserveSoc:          30,
			BackupReserveMinSoc: 10,
			BackupReserveMaxSoc: 90,
		},
	})

	if status.SurplusPlan == nil {
		t.Fatal("SurplusPlan = nil, want recomputed plan")
	}
	if status.SurplusPlan.RecommendedBackupReserveSoc == nil || *status.SurplusPlan.RecommendedBackupReserveSoc != 10 {
		t.Fatalf("RecommendedBackupReserveSoc = %v, want 10", status.SurplusPlan.RecommendedBackupReserveSoc)
	}
	if !status.SurplusPlan.ShouldLowerBackupReserve || !status.SurplusPlan.WouldWrite {
		t.Fatalf("lower/write flags = %t/%t, want true/true", status.SurplusPlan.ShouldLowerBackupReserve, status.SurplusPlan.WouldWrite)
	}
	if !strings.Contains(status.LastDecisionReason, "バックアップリザーブを10%へ戻す") {
		t.Fatalf("LastDecisionReason = %q, want reserve action", status.LastDecisionReason)
	}
}

func TestHigherPriorityDelta3ChargeCandidateRequiresActionablePlan(t *testing.T) {
	now := time.Date(2026, 5, 26, 10, 0, 0, 0, time.UTC)
	priority := chargingPriorityContext{
		pro3OK: true,
		pro3: domain.ChargingDevice{
			Name:     "DELTA Pro 3",
			Priority: 20,
		},
		delta3OK: true,
		delta3: domain.ChargingDevice{
			Name:     "DELTA 3 Plus",
			Priority: 10,
		},
	}
	cfg := config.Config{
		MockMode:                false,
		SimulationMode:          false,
		EnableRealControl:       true,
		AutoControlEnabled:      true,
		ConfirmEcoFlowWrite:     "I_UNDERSTAND",
		RealControlTrialUntil:   now.Add(time.Hour),
		Clock:                   fixedMainClock{now: now},
		Delta3ReadEnabled:       true,
		Delta3AllowAutoWrite:    true,
		Delta3ExecuteWrite:      true,
		Delta3AllowPrivateWrite: true,
		Delta3Aux: config.Delta3AuxConfig{
			Enabled:                   true,
			MinChargeW:                100,
			MaxChargeW:                1500,
			SafetyMarginW:             50,
			MinCommandDiffW:           100,
			MaxIncreaseStepW:          300,
			MaxDecreaseStepW:          500,
			MinCommandInterval:        2 * time.Minute,
			StopImportThresholdW:      50,
			TargetMaxSocBufferPercent: 2,
		},
	}
	status := domain.Status{
		GridW:     -900,
		ImportW:   0,
		ExportW:   900,
		UpdatedAt: now,
	}
	currentLimitW := 100
	maxSoc := 100
	fullSoc := 99
	availableSoc := 50

	got := higherPriorityDelta3ChargeCandidateDevice(
		context.Background(),
		cfg,
		status,
		priority,
		stubDelta3StatusReader{status: api.Delta3StatusResponse{
			Available:      true,
			SOC:            &fullSoc,
			ACChargeLimitW: &currentLimitW,
			MaxChargeSoc:   &maxSoc,
		}},
		&recordingDelta3AuxWriteClient{},
		fakeDelta3AuxCommandRepository{},
		slog.Default(),
	)
	if got != "" {
		t.Fatalf("higherPriorityDelta3ChargeCandidateDevice near full = %q, want empty", got)
	}

	got = higherPriorityDelta3ChargeCandidateDevice(
		context.Background(),
		cfg,
		status,
		priority,
		stubDelta3StatusReader{status: api.Delta3StatusResponse{
			Available:      true,
			SOC:            &availableSoc,
			ACChargeLimitW: &currentLimitW,
			MaxChargeSoc:   &maxSoc,
		}},
		&recordingDelta3AuxWriteClient{},
		fakeDelta3AuxCommandRepository{previous: &domain.Delta3AuxControlCommandLog{
			MeasuredAt:           now.Add(-time.Minute),
			CommandFingerprint:   "delta3_aux;state=READY;ac=400;adjust_ac=true",
			WouldWrite:           true,
			TargetACChargeLimitW: intPtr(400),
		}},
		slog.Default(),
	)
	if got != "" {
		t.Fatalf("higherPriorityDelta3ChargeCandidateDevice during DELTA 3 guard interval = %q, want empty", got)
	}

	got = higherPriorityDelta3ChargeCandidateDevice(
		context.Background(),
		cfg,
		status,
		priority,
		stubDelta3StatusReader{status: api.Delta3StatusResponse{
			Available:      true,
			SOC:            &availableSoc,
			ACChargeLimitW: &currentLimitW,
			MaxChargeSoc:   &maxSoc,
		}},
		&recordingDelta3AuxWriteClient{},
		fakeDelta3AuxCommandRepository{},
		slog.Default(),
	)
	if got != "DELTA 3 Plus" {
		t.Fatalf("higherPriorityDelta3ChargeCandidateDevice = %q, want DELTA 3 Plus", got)
	}
}

func TestRecordStatusPersistsDelta3AuxSuppressionOnStatus(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "energy.db"))
	if err != nil {
		t.Fatalf("store.Open failed: %v", err)
	}
	defer db.Close()

	now := time.Date(2026, 5, 24, 12, 0, 0, 0, time.UTC)
	provider := stubStatusProvider{
		status: domain.Status{
			GridW:     -900,
			ImportW:   0,
			ExportW:   900,
			State:     "simulation",
			Mode:      "mock",
			UpdatedAt: now,
			SurplusPlan: &domain.SurplusPlan{
				StrategyState: "HOLD",
				Reason:        "DELTA Pro 3 priority is already satisfied",
			},
		},
	}
	currentLimitW := 100
	soc := 50
	maxSoc := 100
	delta3MaxPlus := domain.ChargingDevice{
		ID:                    42,
		Name:                  "DELTA 3 Max Plus",
		Kind:                  "ecoflow_delta3_plus",
		Provider:              "ecoflow",
		Role:                  "auxiliary",
		DeviceSN:              "MAXPLUSSN",
		DeviceType:            "DELTA_3_MAX_PLUS",
		StatusSource:          "ecoflow_private_mqtt",
		Enabled:               true,
		ControlEnabled:        true,
		Priority:              2,
		MinChargeW:            200,
		MaxChargeW:            1500,
		ChargeStepW:           100,
		BackupReserveMinSoc:   20,
		BackupReserveMaxSoc:   90,
		SupportsSocRead:       true,
		SupportsACChargeLimit: true,
	}
	statusRepository := store.NewStatusRepository(db)

	recordStatus(
		context.Background(),
		config.Config{
			MockMode:          true,
			SimulationMode:    true,
			EnableRealControl: false,
			Delta3ReadEnabled: true,
			Delta3Aux: config.Delta3AuxConfig{
				Enabled:                   true,
				MinChargeW:                100,
				MaxChargeW:                1500,
				SafetyMarginW:             50,
				MinCommandDiffW:           100,
				MaxIncreaseStepW:          300,
				MaxDecreaseStepW:          500,
				MinCommandInterval:        2 * time.Minute,
				StopImportThresholdW:      50,
				TargetMaxSocBufferPercent: 2,
			},
		},
		provider,
		statusRepository,
		store.NewLogRepository(db),
		nil,
		nil,
		store.NewDelta3AuxControlCommandRepository(db),
		nil,
		nil,
		nil,
		nil,
		nil,
		stubDelta3StatusReader{status: api.Delta3StatusResponse{
			Available:      true,
			SOC:            &soc,
			ACChargeLimitW: &currentLimitW,
			MaxChargeSoc:   &maxSoc,
		}},
		nil,
		fakeDelta3WriteTargetProvider{device: delta3MaxPlus, ok: true},
		nil,
		nil,
		slog.Default(),
	)

	got, err := statusRepository.CurrentStatus(context.Background())
	if err != nil {
		t.Fatalf("CurrentStatus failed: %v", err)
	}
	if got.Delta3AuxPlan == nil {
		t.Fatal("Delta3AuxPlan = nil, want persisted plan")
	}
	if got.Delta3AuxPlan.WouldWrite {
		t.Fatal("Delta3AuxPlan.WouldWrite = true, want false under mock/simulation guard")
	}
	if got.Delta3AuxPlan.DeviceID != delta3MaxPlus.ID || got.Delta3AuxPlan.DeviceName != "DELTA 3 Max Plus" || got.Delta3AuxPlan.DeviceType != "DELTA_3_MAX_PLUS" {
		t.Fatalf("Delta3AuxPlan device = %d/%q/%q, want %d/DELTA 3 Max Plus/DELTA_3_MAX_PLUS", got.Delta3AuxPlan.DeviceID, got.Delta3AuxPlan.DeviceName, got.Delta3AuxPlan.DeviceType, delta3MaxPlus.ID)
	}
	if !strings.Contains(got.Delta3AuxPlan.SuppressedReason, "mock mode") {
		t.Fatalf("SuppressedReason = %q, want mock mode guard", got.Delta3AuxPlan.SuppressedReason)
	}
}

func TestApplyDelta3AuxControlBindsWritesAndAllocatesResidualAcrossDevices(t *testing.T) {
	now := time.Now().UTC()
	devices := []domain.ChargingDevice{
		{ID: 11, Name: "2F South", DeviceSN: "SOUTH", DeviceType: "DELTA_3_MAX_PLUS", Enabled: true, ControlEnabled: true, MinChargeW: 100, MaxChargeW: 500, SupportsACChargeLimit: true},
		{ID: 12, Name: "2F North", DeviceSN: "NORTH", DeviceType: "DELTA_3_MAX_PLUS", Enabled: true, ControlEnabled: true, MinChargeW: 100, MaxChargeW: 500, SupportsACChargeLimit: true},
	}
	soc, limit, maxSoc := 50, 100, 100
	reader := configDelta3StatusReader{statuses: map[string]api.Delta3StatusResponse{
		"SOUTH": {Available: true, SOC: &soc, ACChargeLimitW: &limit, MaxChargeSoc: &maxSoc},
		"NORTH": {Available: true, SOC: &soc, ACChargeLimitW: &limit, MaxChargeSoc: &maxSoc},
	}}
	writer := &deviceBoundRecordingWriter{}
	repo := &deviceCommandRepository{previous: map[int64]*domain.Delta3AuxControlCommandLog{}}
	status := domain.Status{ExportW: 800, GridW: -800, UpdatedAt: now, SurplusPlan: &domain.SurplusPlan{StrategyState: "HOLD"}}
	applyDelta3AuxControl(context.Background(), realAuxTestConfig(now), &status, reader, writer, fakeDelta3WriteTargetsProvider{devices: devices}, nil, repo, true, slog.Default())
	if got, want := strings.Join(writer.writes, ","), "SOUTH:ac=400,NORTH:ac=400"; got != want {
		t.Fatalf("device-bound writes = %q, want %q", got, want)
	}
	if status.Delta3AuxPlan == nil || len(status.Delta3AuxPlan.DevicePlans) != 2 {
		t.Fatalf("device plans = %#v, want two", status.Delta3AuxPlan)
	}
	allocated := 0
	for _, plan := range status.Delta3AuxPlan.DevicePlans {
		allocated += plan.RecommendedACChargeLimitW - *plan.CurrentACChargeLimitW
	}
	if allocated > status.ExportW {
		t.Fatalf("allocated increment = %dW exceeds export %dW", allocated, status.ExportW)
	}
	if len(repo.logs) != 2 || repo.logs[0].DeviceID != 11 || repo.logs[1].DeviceID != 12 {
		t.Fatalf("device command logs = %#v", repo.logs)
	}
}

func TestApplyDelta3AuxControlSuppressesLegacyButNotOtherDeviceLog(t *testing.T) {
	now := time.Now().UTC()
	device := domain.ChargingDevice{ID: 11, DeviceSN: "SOUTH", DeviceType: "DELTA_3_MAX_PLUS", Enabled: true, ControlEnabled: true, MinChargeW: 100, MaxChargeW: 500, SupportsACChargeLimit: true}
	soc, limit, maxSoc := 50, 100, 100
	reader := configDelta3StatusReader{statuses: map[string]api.Delta3StatusResponse{"SOUTH": {Available: true, SOC: &soc, ACChargeLimitW: &limit, MaxChargeSoc: &maxSoc}}}
	legacy := &domain.Delta3AuxControlCommandLog{DeviceID: 0, MeasuredAt: now, CommandFingerprint: "different", WouldWrite: true}
	writer := &deviceBoundRecordingWriter{}
	repo := &deviceCommandRepository{previous: map[int64]*domain.Delta3AuxControlCommandLog{0: legacy}}
	status := domain.Status{ExportW: 800, GridW: -800, UpdatedAt: now, SurplusPlan: &domain.SurplusPlan{StrategyState: "HOLD"}}
	applyDelta3AuxControl(context.Background(), realAuxTestConfig(now), &status, reader, writer, fakeDelta3WriteTargetsProvider{devices: []domain.ChargingDevice{device}}, nil, repo, true, slog.Default())
	if len(writer.writes) != 0 {
		t.Fatalf("legacy recent log allowed writes: %#v", writer.writes)
	}
	if repo.logs[0].SuppressedReason != "command suppressed by minimum interval" {
		t.Fatalf("legacy suppressed reason = %q", repo.logs[0].SuppressedReason)
	}
	repo = &deviceCommandRepository{previous: map[int64]*domain.Delta3AuxControlCommandLog{11: legacy}}
	writer = &deviceBoundRecordingWriter{}
	status = domain.Status{ExportW: 800, GridW: -800, UpdatedAt: now, SurplusPlan: &domain.SurplusPlan{StrategyState: "HOLD"}}
	applyDelta3AuxControl(context.Background(), realAuxTestConfig(now), &status, reader, writer, fakeDelta3WriteTargetsProvider{devices: []domain.ChargingDevice{device}}, nil, repo, true, slog.Default())
	if len(writer.writes) != 0 {
		t.Fatalf("same device recent log allowed writes: %#v", writer.writes)
	}
	repo = &deviceCommandRepository{previous: map[int64]*domain.Delta3AuxControlCommandLog{999: legacy}}
	writer = &deviceBoundRecordingWriter{}
	status = domain.Status{ExportW: 800, GridW: -800, UpdatedAt: now, SurplusPlan: &domain.SurplusPlan{StrategyState: "HOLD"}}
	applyDelta3AuxControl(context.Background(), realAuxTestConfig(now), &status, reader, writer, fakeDelta3WriteTargetsProvider{devices: []domain.ChargingDevice{device}}, nil, repo, true, slog.Default())
	if len(writer.writes) == 0 {
		t.Fatal("other device log suppressed this device write")
	}
}

func TestApplyDelta3AuxControlDoesNotWriteUnavailableDevice(t *testing.T) {
	now := time.Now().UTC()
	device := domain.ChargingDevice{ID: 11, DeviceSN: "SOUTH", DeviceType: "DELTA_3_MAX_PLUS", Enabled: true, ControlEnabled: true, MinChargeW: 100, MaxChargeW: 500, SupportsACChargeLimit: true}
	writer := &deviceBoundRecordingWriter{}
	repo := &deviceCommandRepository{previous: map[int64]*domain.Delta3AuxControlCommandLog{}}
	status := domain.Status{ExportW: 800, GridW: -800, UpdatedAt: now, SurplusPlan: &domain.SurplusPlan{StrategyState: "HOLD"}}
	applyDelta3AuxControl(context.Background(), realAuxTestConfig(now), &status, configDelta3StatusReader{statuses: map[string]api.Delta3StatusResponse{"SOUTH": {Available: false, LastError: "timeout"}}}, writer, fakeDelta3WriteTargetsProvider{devices: []domain.ChargingDevice{device}}, nil, repo, true, slog.Default())
	if len(writer.writes) != 0 {
		t.Fatalf("unavailable device allowed writes: %#v", writer.writes)
	}
	if status.Delta3AuxPlan == nil || status.Delta3AuxPlan.StrategyState != "UNAVAILABLE" {
		t.Fatalf("plan = %#v, want unavailable", status.Delta3AuxPlan)
	}
}

func TestDelta3AuxControlDevicesKeepsOnlyPrimaryDeviceType(t *testing.T) {
	devices := []domain.ChargingDevice{
		{ID: 11, Name: "2F South", DeviceType: "DELTA_3_MAX_PLUS"},
		{ID: 12, Name: "2F North", DeviceType: "DELTA_3_MAX_PLUS"},
		{ID: 13, Name: "1F", DeviceType: "DELTA_3_PLUS"},
	}
	got, err := delta3AuxControlDevices(context.Background(), fakeDelta3WriteTargetsProvider{devices: devices})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != 11 || got[1].ID != 12 {
		t.Fatalf("daytime aux targets = %#v, want only Max Plus north/south", got)
	}
}

func realAuxTestConfig(now time.Time) config.Config {
	return config.Config{MockMode: false, SimulationMode: false, EnableRealControl: true, AutoControlEnabled: true, ConfirmEcoFlowWrite: "I_UNDERSTAND", RealControlTrialUntil: now.Add(time.Hour), Delta3ReadEnabled: true, Delta3AllowAutoWrite: true, Delta3ExecuteWrite: true, Delta3AllowPrivateWrite: true, Delta3Aux: config.Delta3AuxConfig{Enabled: true, MinChargeW: 100, MaxChargeW: 500, SafetyMarginW: 50, MinCommandDiffW: 100, MaxIncreaseStepW: 300, MaxDecreaseStepW: 500, MinCommandInterval: 2 * time.Minute, StopImportThresholdW: 50, TargetMaxSocBufferPercent: 2}}
}

func TestNightChargeDeviceInputsMarksDelta3MaxPlusWriteTarget(t *testing.T) {
	pro3 := domain.ChargingDevice{
		ID:                    1,
		Name:                  "DELTA Pro 3",
		Kind:                  "ecoflow_delta_pro3",
		DeviceType:            "DELTA_PRO3",
		SupportsACChargeLimit: true,
	}
	delta3Plus := domain.ChargingDevice{
		ID:                    2,
		Name:                  "DELTA 3 Plus",
		Kind:                  "ecoflow_delta3_plus",
		DeviceType:            "DELTA_3_PLUS",
		SupportsACChargeLimit: true,
	}
	delta3MaxPlus := domain.ChargingDevice{
		ID:                    3,
		Name:                  "DELTA 3 Max Plus",
		Kind:                  "ecoflow_delta3_plus",
		DeviceType:            "DELTA_3_MAX_PLUS",
		SupportsACChargeLimit: true,
	}
	statuses := []api.DeviceStatusResponse{
		{ID: pro3.ID, Name: pro3.Name, Kind: pro3.Kind, DeviceType: pro3.DeviceType, Enabled: true, ControlEnabled: true, Status: api.Delta3StatusResponse{Available: true}},
		{ID: delta3Plus.ID, Name: delta3Plus.Name, Kind: delta3Plus.Kind, DeviceType: delta3Plus.DeviceType, Enabled: true, ControlEnabled: true, Status: api.Delta3StatusResponse{Available: true}},
		{ID: delta3MaxPlus.ID, Name: delta3MaxPlus.Name, Kind: delta3MaxPlus.Kind, DeviceType: delta3MaxPlus.DeviceType, Enabled: true, ControlEnabled: true, Status: api.Delta3StatusResponse{Available: true}},
	}

	inputs := nightChargeDeviceInputs(
		context.Background(),
		fakeChargingPriorityTargetProvider{pro3: pro3, pro3OK: true, delta3: delta3MaxPlus, delta3OK: true},
		statuses,
		[]domain.ChargingDevice{pro3, delta3Plus, delta3MaxPlus},
		slog.Default(),
	)

	if len(inputs) != 3 {
		t.Fatalf("inputs len = %d, want 3", len(inputs))
	}
	if !inputs[0].WriteTarget {
		t.Fatal("DELTA Pro 3 WriteTarget = false, want true")
	}
	if inputs[1].WriteTarget {
		t.Fatal("DELTA 3 Plus WriteTarget = true, want false when Max Plus is selected")
	}
	if !inputs[2].WriteTarget {
		t.Fatal("DELTA 3 Max Plus WriteTarget = false, want true")
	}
	if inputs[2].DeviceType != "DELTA_3_MAX_PLUS" {
		t.Fatalf("DELTA 3 Max Plus DeviceType = %q, want DELTA_3_MAX_PLUS", inputs[2].DeviceType)
	}
}

func TestNightChargeDeviceInputsTreatsStaleCachedStatusAsUnavailable(t *testing.T) {
	device := domain.ChargingDevice{
		ID:                    2,
		Name:                  "DELTA 3 Plus",
		Kind:                  "ecoflow_delta3_plus",
		DeviceType:            "DELTA_3_PLUS",
		SupportsACChargeLimit: true,
	}
	statuses := []api.DeviceStatusResponse{
		{
			ID:             device.ID,
			Name:           device.Name,
			Kind:           device.Kind,
			DeviceType:     device.DeviceType,
			Enabled:        true,
			ControlEnabled: true,
			Status: api.Delta3StatusResponse{
				Available: true,
				Cached:    true,
				LastError: "refresh failed: context deadline exceeded",
			},
		},
	}

	inputs := nightChargeDeviceInputs(
		context.Background(),
		fakeChargingPriorityTargetProvider{delta3: device, delta3OK: true},
		statuses,
		[]domain.ChargingDevice{device},
		slog.Default(),
	)

	if len(inputs) != 1 {
		t.Fatalf("inputs len = %d, want 1", len(inputs))
	}
	if inputs[0].StatusAvailable {
		t.Fatal("StatusAvailable = true, want false for cached status in control input")
	}
	if !strings.Contains(inputs[0].StatusUnavailableReason, "refresh failed") {
		t.Fatalf("StatusUnavailableReason = %q, want refresh failure reason", inputs[0].StatusUnavailableReason)
	}
}

func TestNightChargeDeviceInputsKeepsSuccessfulCachedStatusAvailable(t *testing.T) {
	device := domain.ChargingDevice{
		ID:                    2,
		Name:                  "DELTA 3 Plus",
		Kind:                  "ecoflow_delta3_plus",
		DeviceType:            "DELTA_3_PLUS",
		SupportsACChargeLimit: true,
	}
	statuses := []api.DeviceStatusResponse{
		{
			ID:             device.ID,
			Name:           device.Name,
			Kind:           device.Kind,
			DeviceType:     device.DeviceType,
			Enabled:        true,
			ControlEnabled: true,
			Status: api.Delta3StatusResponse{
				Available: true,
				Cached:    true,
			},
		},
	}

	inputs := nightChargeDeviceInputs(
		context.Background(),
		fakeChargingPriorityTargetProvider{delta3: device, delta3OK: true},
		statuses,
		[]domain.ChargingDevice{device},
		slog.Default(),
	)

	if len(inputs) != 1 {
		t.Fatalf("inputs len = %d, want 1", len(inputs))
	}
	if !inputs[0].StatusAvailable {
		t.Fatal("StatusAvailable = false, want true for successful cached status")
	}
}

func TestDelta3AuxSettingsForDeviceClampsPrivateProfileRange(t *testing.T) {
	settings := delta3AuxSettingsForDevice(config.Config{
		Delta3Aux: config.Delta3AuxConfig{
			Enabled:    true,
			MinChargeW: 100,
			MaxChargeW: 1800,
		},
	}, domain.ChargingDevice{
		DeviceType: "DELTA_3_MAX_PLUS",
		MinChargeW: 100,
		MaxChargeW: 1800,
	})

	if settings.MinChargeW != 200 {
		t.Fatalf("MinChargeW = %d, want DELTA_3_MAX_PLUS profile minimum 200", settings.MinChargeW)
	}
	if settings.MaxChargeW != 1500 {
		t.Fatalf("MaxChargeW = %d, want DELTA_3_MAX_PLUS profile maximum 1500", settings.MaxChargeW)
	}
}

func TestRecordStatusPersistsNightOwnedSurplusSkipLogWithoutWriteCandidate(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "energy.db"))
	if err != nil {
		t.Fatalf("store.Open failed: %v", err)
	}
	defer db.Close()

	now := time.Date(2026, 5, 25, 0, 10, 0, 0, time.FixedZone("JST", 9*60*60))
	reserve := 48
	provider := stubStatusProvider{
		status: domain.Status{
			GridW:              2440,
			ImportW:            2440,
			ExportW:            0,
			BatterySoc:         38,
			BatteryInputW:      1419,
			BatteryOutputW:     1031,
			ACChargeLimitW:     1500,
			BackupReserveSoc:   &reserve,
			State:              "real-control",
			Mode:               "nature-cloud+ecoflow-read",
			LastDecisionReason: "importing from grid, do not charge",
			UpdatedAt:          now,
			NightChargePlan: &domain.NightChargePlan{
				StrategyState:             "NIGHT_CHARGE_WINDOW",
				RecommendedMode:           "tou",
				RecommendedNightTargetSoc: 48,
				ShouldChargeTonight:       true,
				ActionSummary:             "推奨modeはtou; 深夜目標SOCを48%へ設定",
				Reason:                    "target daytime solar forecast is strong; keep night charging modest",
				CommandBlockReason:        "night charge settings already match plan",
			},
		},
	}
	surplusRepository := store.NewSurplusControlCommandRepository(db)

	recordStatus(
		context.Background(),
		config.Config{Clock: fixedMainClock{now: now}},
		provider,
		store.NewStatusRepository(db),
		store.NewLogRepository(db),
		store.NewNightChargePlanRepository(db),
		surplusRepository,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		nil,
		slog.Default(),
	)

	logs, total, err := surplusRepository.ListSurplusControlCommandLogsPage(context.Background(), 10, 0, store.SurplusControlCommandLogPageFilter{})
	if err != nil {
		t.Fatalf("ListSurplusControlCommandLogsPage failed: %v", err)
	}
	if total != 1 || len(logs) != 1 {
		t.Fatalf("surplus skip logs = %d/%d, want 1/1", len(logs), total)
	}
	log := logs[0]
	if log.WouldWrite || log.CommandSent || !log.DryRun {
		t.Fatalf("WouldWrite,CommandSent,DryRun = %v,%v,%v; want false,false,true", log.WouldWrite, log.CommandSent, log.DryRun)
	}
	if log.ErrorMessage != nil {
		t.Fatalf("ErrorMessage = %q, want nil so skip log is not a write candidate", *log.ErrorMessage)
	}
	if log.SuppressedReason != "night charge plan owns control" {
		t.Fatalf("SuppressedReason = %q", log.SuppressedReason)
	}
	if !strings.Contains(log.DecisionReason, "推奨modeはtou") {
		t.Fatalf("DecisionReason = %q, want night plan action summary", log.DecisionReason)
	}
	latestCandidate, err := surplusRepository.LatestSurplusControlWriteCandidateLog(context.Background())
	if err != nil {
		t.Fatalf("LatestSurplusControlWriteCandidateLog failed: %v", err)
	}
	if latestCandidate != nil {
		t.Fatalf("LatestSurplusControlWriteCandidateLog = %+v, want nil for skip-only log", latestCandidate)
	}
}

func TestRealControlTrialActiveRequiresFutureDeadline(t *testing.T) {
	now := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	if realControlTrialActive(config.Config{Clock: fixedMainClock{now: now}}) {
		t.Fatal("trial active without deadline")
	}
	if realControlTrialActive(config.Config{RealControlTrialUntil: now, Clock: fixedMainClock{now: now}}) {
		t.Fatal("trial active at exact deadline")
	}
	if !realControlTrialActive(config.Config{RealControlTrialUntil: now.Add(time.Minute), Clock: fixedMainClock{now: now}}) {
		t.Fatal("trial inactive before deadline")
	}
}

func TestTariffLowPriceWindowActive(t *testing.T) {
	if tariffLowPriceWindowActive(nil) {
		t.Fatal("nil status should not activate tariff window")
	}
	status := &domain.Status{TariffControl: &domain.TariffControlContext{IsLowPrice: true}}
	if !tariffLowPriceWindowActive(status) {
		t.Fatal("low-price tariff context should activate tariff window")
	}
	status.TariffControl.IsLowPrice = false
	if tariffLowPriceWindowActive(status) {
		t.Fatal("non-low-price tariff context should not activate tariff window")
	}
}

func TestNightPlanOwnsEnergyControlUsesTariffLowPriceForRecover(t *testing.T) {
	measuredAt := time.Date(2026, 6, 8, 2, 0, 0, 0, time.UTC)
	plan := domain.NightChargePlan{StrategyState: "NIGHT_RECOVER"}
	highPrice := &domain.TariffControlContext{IsLowPrice: false, IsHighPrice: true}
	if nightPlanOwnsEnergyControl(plan, measuredAt, highPrice, "Asia/Tokyo") {
		t.Fatal("high-price tariff should not let night recovery own surplus control")
	}
	lowPrice := &domain.TariffControlContext{IsLowPrice: true}
	if !nightPlanOwnsEnergyControl(plan, measuredAt, lowPrice, "Asia/Tokyo") {
		t.Fatal("low-price tariff should let night recovery own surplus control")
	}
}

func TestNightPlanOwnsEnergyControlFallsBackToConfiguredTimezone(t *testing.T) {
	measuredAt := time.Date(2026, 6, 8, 2, 0, 0, 0, time.UTC)
	plan := domain.NightChargePlan{StrategyState: "NIGHT_RECOVER"}
	if nightPlanOwnsEnergyControl(plan, measuredAt, nil, "Asia/Tokyo") {
		t.Fatal("02:00 UTC should be treated as 11:00 JST and not own surplus control")
	}
	if !nightPlanOwnsEnergyControl(plan, measuredAt, nil, "UTC") {
		t.Fatal("02:00 UTC should own surplus control when UTC fallback is used")
	}
}

func TestNightPlanOwnsEnergyControlKeepsNightChargeWindowOwnership(t *testing.T) {
	plan := domain.NightChargePlan{StrategyState: "NIGHT_CHARGE_WINDOW"}
	highPrice := &domain.TariffControlContext{IsLowPrice: false, IsHighPrice: true}
	if !nightPlanOwnsEnergyControl(plan, time.Date(2026, 6, 8, 11, 0, 0, 0, time.UTC), highPrice, "Asia/Tokyo") {
		t.Fatal("night charge window should keep ownership because command execution is handled by its own guard")
	}
}

func TestNightPlanOwnsEnergyControlForSelfPoweredDischarge(t *testing.T) {
	plan := domain.NightChargePlan{StrategyState: "NIGHT_RECOVER", RecommendedMode: "self-powered-discharge"}
	highPrice := &domain.TariffControlContext{IsLowPrice: false, IsHighPrice: true}
	if !nightPlanOwnsEnergyControl(plan, time.Date(2026, 6, 8, 11, 0, 0, 0, time.UTC), highPrice, "Asia/Tokyo") {
		t.Fatal("self-powered discharge recovery should own control to avoid surplus mode-off competition")
	}
}

func TestRealControlTrialActiveUsesCurrentClockNotMeasuredAt(t *testing.T) {
	deadline := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	cfg := config.Config{
		RealControlTrialUntil: deadline,
		Clock:                 fixedMainClock{now: deadline.Add(time.Second)},
	}
	if realControlTrialActive(cfg) {
		t.Fatal("trial active after current clock passed deadline")
	}
}
