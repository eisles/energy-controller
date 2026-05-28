package store

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/eisles/energy-controller/backend/internal/domain"
)

type ChargingDeviceRepository struct {
	db *sql.DB
}

func NewChargingDeviceRepository(db *sql.DB) *ChargingDeviceRepository {
	return &ChargingDeviceRepository{db: db}
}

func (r *ChargingDeviceRepository) ListChargingDevices(ctx context.Context) ([]domain.ChargingDevice, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT
		id, name, kind, provider, role, credential_ref, device_sn, device_type, status_source, enabled, control_enabled, priority,
		min_charge_w, max_charge_w, charge_step_w, capacity_wh, target_soc, reserve_soc, backup_reserve_min_soc, backup_reserve_max_soc, expected_daytime_load_w, auto_recover_ac_output,
		supports_soc_read, supports_ac_charge_limit, supports_on_off, notes, created_at, updated_at
		FROM charging_devices
		ORDER BY priority ASC, id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanChargingDevices(rows)
}

func (r *ChargingDeviceRepository) UpsertChargingDevice(ctx context.Context, device domain.ChargingDevice) (domain.ChargingDevice, error) {
	var err error
	device, err = normalizeChargingDeviceReserveBounds(device)
	if err != nil {
		return domain.ChargingDevice{}, err
	}
	now := time.Now().Format(time.RFC3339Nano)
	if device.ID > 0 {
		result, err := r.db.ExecContext(ctx, `UPDATE charging_devices SET
			name = ?, kind = ?, provider = ?, role = ?, credential_ref = ?, enabled = ?,
			device_sn = ?, device_type = ?, status_source = ?, control_enabled = ?, priority = ?, min_charge_w = ?, max_charge_w = ?, charge_step_w = ?,
			capacity_wh = ?, target_soc = ?, reserve_soc = ?, backup_reserve_min_soc = ?, backup_reserve_max_soc = ?, expected_daytime_load_w = ?, auto_recover_ac_output = ?, supports_soc_read = ?,
			supports_ac_charge_limit = ?, supports_on_off = ?, notes = ?, updated_at = ?
			WHERE id = ?`,
			device.Name,
			device.Kind,
			device.Provider,
			device.Role,
			device.CredentialRef,
			boolToInt(device.Enabled),
			device.DeviceSN,
			device.DeviceType,
			device.StatusSource,
			boolToInt(device.ControlEnabled),
			device.Priority,
			device.MinChargeW,
			device.MaxChargeW,
			device.ChargeStepW,
			device.CapacityWh,
			device.TargetSoc,
			device.ReserveSoc,
			device.BackupReserveMinSoc,
			device.BackupReserveMaxSoc,
			device.ExpectedDaytimeLoadW,
			boolToInt(device.AutoRecoverACOutput),
			boolToInt(device.SupportsSocRead),
			boolToInt(device.SupportsACChargeLimit),
			boolToInt(device.SupportsOnOff),
			device.Notes,
			now,
			device.ID,
		)
		if err != nil {
			return domain.ChargingDevice{}, err
		}
		rowsAffected, err := result.RowsAffected()
		if err != nil {
			return domain.ChargingDevice{}, err
		}
		if rowsAffected == 0 {
			return domain.ChargingDevice{}, sql.ErrNoRows
		}
		return r.chargingDeviceByID(ctx, device.ID)
	}

	result, err := r.db.ExecContext(ctx, `INSERT INTO charging_devices (
		name, kind, provider, role, credential_ref, device_sn, device_type, status_source, enabled, control_enabled, priority,
		min_charge_w, max_charge_w, charge_step_w, capacity_wh, target_soc, reserve_soc, backup_reserve_min_soc, backup_reserve_max_soc, expected_daytime_load_w, auto_recover_ac_output,
		supports_soc_read, supports_ac_charge_limit, supports_on_off, notes, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		device.Name,
		device.Kind,
		device.Provider,
		device.Role,
		device.CredentialRef,
		device.DeviceSN,
		device.DeviceType,
		device.StatusSource,
		boolToInt(device.Enabled),
		boolToInt(device.ControlEnabled),
		device.Priority,
		device.MinChargeW,
		device.MaxChargeW,
		device.ChargeStepW,
		device.CapacityWh,
		device.TargetSoc,
		device.ReserveSoc,
		device.BackupReserveMinSoc,
		device.BackupReserveMaxSoc,
		device.ExpectedDaytimeLoadW,
		boolToInt(device.AutoRecoverACOutput),
		boolToInt(device.SupportsSocRead),
		boolToInt(device.SupportsACChargeLimit),
		boolToInt(device.SupportsOnOff),
		device.Notes,
		now,
		now,
	)
	if err != nil {
		return domain.ChargingDevice{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return domain.ChargingDevice{}, err
	}
	return r.chargingDeviceByID(ctx, id)
}

func (r *ChargingDeviceRepository) DeleteChargingDevice(ctx context.Context, id int64) error {
	result, err := r.db.ExecContext(ctx, `DELETE FROM charging_devices WHERE id = ?`, id)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (r *ChargingDeviceRepository) chargingDeviceByID(ctx context.Context, id int64) (domain.ChargingDevice, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT
		id, name, kind, provider, role, credential_ref, device_sn, device_type, status_source, enabled, control_enabled, priority,
		min_charge_w, max_charge_w, charge_step_w, capacity_wh, target_soc, reserve_soc, backup_reserve_min_soc, backup_reserve_max_soc, expected_daytime_load_w, auto_recover_ac_output,
		supports_soc_read, supports_ac_charge_limit, supports_on_off, notes, created_at, updated_at
		FROM charging_devices
		WHERE id = ?`, id)
	if err != nil {
		return domain.ChargingDevice{}, err
	}
	defer rows.Close()
	devices, err := scanChargingDevices(rows)
	if err != nil {
		return domain.ChargingDevice{}, err
	}
	if len(devices) == 0 {
		return domain.ChargingDevice{}, sql.ErrNoRows
	}
	return devices[0], nil
}

func (r *ChargingDeviceRepository) Delta3ReadTarget(ctx context.Context) (domain.ChargingDevice, bool, error) {
	return r.delta3Target(ctx, false)
}

func (r *ChargingDeviceRepository) Delta3ReadTargets(ctx context.Context) ([]domain.ChargingDevice, error) {
	return r.delta3Targets(ctx, false)
}

func (r *ChargingDeviceRepository) Delta3WriteTarget(ctx context.Context) (domain.ChargingDevice, bool, error) {
	return r.delta3Target(ctx, true)
}

func (r *ChargingDeviceRepository) EcoFlowCloudReadTarget(ctx context.Context) (domain.ChargingDevice, bool, error) {
	if writeTarget, ok, err := r.EcoFlowCloudWriteTarget(ctx); err != nil || ok {
		return writeTarget, ok, err
	}
	return r.ecoFlowCloudTarget(ctx, false)
}

func (r *ChargingDeviceRepository) EcoFlowCloudWriteTarget(ctx context.Context) (domain.ChargingDevice, bool, error) {
	return r.ecoFlowCloudTarget(ctx, true)
}

func (r *ChargingDeviceRepository) ecoFlowCloudTarget(ctx context.Context, requireWriteSupport bool) (domain.ChargingDevice, bool, error) {
	query := `SELECT
		id, name, kind, provider, role, credential_ref, device_sn, device_type, status_source, enabled, control_enabled, priority,
		min_charge_w, max_charge_w, charge_step_w, capacity_wh, target_soc, reserve_soc, backup_reserve_min_soc, backup_reserve_max_soc, expected_daytime_load_w, auto_recover_ac_output,
		supports_soc_read, supports_ac_charge_limit, supports_on_off, notes, created_at, updated_at
		FROM charging_devices
		WHERE enabled = 1
		  AND provider = 'ecoflow'
		  AND kind = 'ecoflow_delta_pro3'
		  AND status_source = 'ecoflow_cloud'
		  AND TRIM(device_sn) <> ''
		  AND supports_soc_read = 1`
	if requireWriteSupport {
		query += ` AND control_enabled = 1 AND supports_ac_charge_limit = 1`
	}
	query += ` ORDER BY priority ASC, id ASC LIMIT 1`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return domain.ChargingDevice{}, false, err
	}
	defer rows.Close()
	devices, err := scanChargingDevices(rows)
	if err != nil {
		return domain.ChargingDevice{}, false, err
	}
	if len(devices) == 0 {
		return domain.ChargingDevice{}, false, nil
	}
	return devices[0], true, nil
}

func (r *ChargingDeviceRepository) delta3Target(ctx context.Context, requireWriteSupport bool) (domain.ChargingDevice, bool, error) {
	query := `SELECT
		id, name, kind, provider, role, credential_ref, device_sn, device_type, status_source, enabled, control_enabled, priority,
		min_charge_w, max_charge_w, charge_step_w, capacity_wh, target_soc, reserve_soc, backup_reserve_min_soc, backup_reserve_max_soc, expected_daytime_load_w, auto_recover_ac_output,
		supports_soc_read, supports_ac_charge_limit, supports_on_off, notes, created_at, updated_at
		FROM charging_devices
		WHERE enabled = 1
		  AND provider = 'ecoflow'
		  AND kind = 'ecoflow_delta3_plus'
		  AND device_sn <> ''
		  AND supports_soc_read = 1`
	if requireWriteSupport {
		query += ` AND control_enabled = 1 AND supports_ac_charge_limit = 1`
	}
	query += ` ORDER BY priority ASC, id ASC LIMIT 1`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return domain.ChargingDevice{}, false, err
	}
	defer rows.Close()
	devices, err := scanChargingDevices(rows)
	if err != nil {
		return domain.ChargingDevice{}, false, err
	}
	if len(devices) == 0 {
		return domain.ChargingDevice{}, false, nil
	}
	return devices[0], true, nil
}

func (r *ChargingDeviceRepository) delta3Targets(ctx context.Context, requireWriteSupport bool) ([]domain.ChargingDevice, error) {
	query := `SELECT
		id, name, kind, provider, role, credential_ref, device_sn, device_type, status_source, enabled, control_enabled, priority,
		min_charge_w, max_charge_w, charge_step_w, capacity_wh, target_soc, reserve_soc, backup_reserve_min_soc, backup_reserve_max_soc, expected_daytime_load_w, auto_recover_ac_output,
		supports_soc_read, supports_ac_charge_limit, supports_on_off, notes, created_at, updated_at
		FROM charging_devices
		WHERE enabled = 1
		  AND provider = 'ecoflow'
		  AND kind = 'ecoflow_delta3_plus'
		  AND device_sn <> ''
		  AND supports_soc_read = 1`
	if requireWriteSupport {
		query += ` AND control_enabled = 1 AND supports_ac_charge_limit = 1`
	}
	query += ` ORDER BY priority ASC, id ASC`
	rows, err := r.db.QueryContext(ctx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanChargingDevices(rows)
}

func scanChargingDevices(rows *sql.Rows) ([]domain.ChargingDevice, error) {
	devices := make([]domain.ChargingDevice, 0)
	for rows.Next() {
		var device domain.ChargingDevice
		var enabled, controlEnabled, autoRecoverACOutput, supportsSocRead, supportsACChargeLimit, supportsOnOff int
		var createdAt, updatedAt string
		if err := rows.Scan(
			&device.ID,
			&device.Name,
			&device.Kind,
			&device.Provider,
			&device.Role,
			&device.CredentialRef,
			&device.DeviceSN,
			&device.DeviceType,
			&device.StatusSource,
			&enabled,
			&controlEnabled,
			&device.Priority,
			&device.MinChargeW,
			&device.MaxChargeW,
			&device.ChargeStepW,
			&device.CapacityWh,
			&device.TargetSoc,
			&device.ReserveSoc,
			&device.BackupReserveMinSoc,
			&device.BackupReserveMaxSoc,
			&device.ExpectedDaytimeLoadW,
			&autoRecoverACOutput,
			&supportsSocRead,
			&supportsACChargeLimit,
			&supportsOnOff,
			&device.Notes,
			&createdAt,
			&updatedAt,
		); err != nil {
			return nil, err
		}
		parsedCreatedAt, err := parseTime(createdAt)
		if err != nil {
			return nil, err
		}
		parsedUpdatedAt, err := parseTime(updatedAt)
		if err != nil {
			return nil, err
		}
		device.Enabled = enabled != 0
		device.ControlEnabled = controlEnabled != 0
		device.AutoRecoverACOutput = autoRecoverACOutput != 0
		device.SupportsSocRead = supportsSocRead != 0
		device.SupportsACChargeLimit = supportsACChargeLimit != 0
		device.SupportsOnOff = supportsOnOff != 0
		device.CreatedAt = parsedCreatedAt
		device.UpdatedAt = parsedUpdatedAt
		devices = append(devices, device)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return devices, nil
}

func normalizeChargingDeviceReserveBounds(device domain.ChargingDevice) (domain.ChargingDevice, error) {
	explicitMin := device.BackupReserveMinSoc != 0
	explicitMax := device.BackupReserveMaxSoc != 0
	if explicitMin && (device.BackupReserveMinSoc < 5 || device.BackupReserveMinSoc > 100) {
		return domain.ChargingDevice{}, errors.New("backup reserve min soc is out of range")
	}
	if explicitMax && (device.BackupReserveMaxSoc < 5 || device.BackupReserveMaxSoc > 100) {
		return domain.ChargingDevice{}, errors.New("backup reserve max soc is out of range")
	}
	if device.BackupReserveMinSoc == 0 {
		device.BackupReserveMinSoc = clampSoc(device.ReserveSoc)
	}
	if device.BackupReserveMaxSoc == 0 {
		device.BackupReserveMaxSoc = clampSoc(device.TargetSoc)
	}
	if device.BackupReserveMaxSoc < device.BackupReserveMinSoc {
		if !explicitMax {
			device.BackupReserveMaxSoc = device.BackupReserveMinSoc
			device.ReserveSoc = device.BackupReserveMinSoc
			return device, nil
		}
		return domain.ChargingDevice{}, errors.New("backup reserve max soc is below min soc")
	}
	if device.ExpectedDaytimeLoadW < 0 {
		return domain.ChargingDevice{}, errors.New("expected daytime load is out of range")
	}
	device.ReserveSoc = device.BackupReserveMinSoc
	return device, nil
}

func clampSoc(value int) int {
	if value < 5 {
		return 5
	}
	if value > 100 {
		return 100
	}
	return value
}
