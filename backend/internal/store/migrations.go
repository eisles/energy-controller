package store

import (
	"database/sql"
	"fmt"
	"time"
)

func migrate(db *sql.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS settings (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			auto_control_enabled INTEGER NOT NULL DEFAULT 0,
			simulation_mode INTEGER NOT NULL DEFAULT 1,
			start_export_threshold_w INTEGER NOT NULL DEFAULT 700,
			stop_export_threshold_w INTEGER NOT NULL DEFAULT 300,
			safety_margin_w INTEGER NOT NULL DEFAULT 150,
			min_charge_w INTEGER NOT NULL DEFAULT 400,
			max_charge_w INTEGER NOT NULL DEFAULT 1500,
			target_soc INTEGER NOT NULL DEFAULT 90,
			min_command_interval_sec INTEGER NOT NULL DEFAULT 60,
			min_command_diff_w INTEGER NOT NULL DEFAULT 100,
			require_consecutive_export_count INTEGER NOT NULL DEFAULT 2,
			require_consecutive_import_count INTEGER NOT NULL DEFAULT 2,
			weather_forecast_enabled INTEGER NOT NULL DEFAULT 0,
			weather_latitude REAL,
			weather_longitude REAL,
			weather_timezone TEXT NOT NULL DEFAULT 'Asia/Tokyo',
			pv_capacity_kw REAL NOT NULL DEFAULT 0,
			pv_performance_ratio REAL NOT NULL DEFAULT 0.75,
			daily_base_load_kwh REAL NOT NULL DEFAULT 0,
			battery_capacity_kwh REAL NOT NULL DEFAULT 4.096,
			minimum_reserve_soc INTEGER NOT NULL DEFAULT 30,
			pv_charge_correction_factor REAL NOT NULL DEFAULT 0.7,
			pv_charge_correction_manual INTEGER NOT NULL DEFAULT 0,
			pv_charge_correction_updated_at TEXT,
			pv_charge_correction_min_sample_days INTEGER NOT NULL DEFAULT 7,
			pv_charge_correction_min_factor REAL NOT NULL DEFAULT 0.2,
			pv_charge_correction_max_factor REAL NOT NULL DEFAULT 0.9,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS power_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			measured_at TEXT NOT NULL,
			grid_w INTEGER NOT NULL,
			import_w INTEGER NOT NULL,
			export_w INTEGER NOT NULL,
			battery_soc INTEGER,
			battery_input_w INTEGER,
			battery_output_w INTEGER,
			ac_charge_limit_w INTEGER,
			target_charge_w INTEGER NOT NULL DEFAULT 0,
			actual_command_w INTEGER,
			decision_reason TEXT NOT NULL,
			mode TEXT NOT NULL,
			command_sent INTEGER NOT NULL DEFAULT 0,
			error_message TEXT,
			ecoflow_diagnostics_json TEXT,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS energy_meter_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			measured_at TEXT NOT NULL,
			import_cumulative_kwh REAL NOT NULL,
			export_cumulative_kwh REAL NOT NULL,
			import_delta_kwh REAL,
			export_delta_kwh REAL,
			coefficient INTEGER NOT NULL,
			cumulative_unit REAL NOT NULL,
			raw_import_cumulative TEXT NOT NULL,
			raw_export_cumulative TEXT NOT NULL,
			import_value_updated_at TEXT NOT NULL,
			export_value_updated_at TEXT NOT NULL,
			created_at TEXT NOT NULL,
			UNIQUE(measured_at, raw_import_cumulative, raw_export_cumulative)
		)`,
		`CREATE TABLE IF NOT EXISTS night_charge_plan_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			measured_at TEXT NOT NULL,
			strategy_state TEXT NOT NULL,
			recommended_mode TEXT NOT NULL,
			recommended_night_target_soc INTEGER NOT NULL,
			recommended_night_target_kwh REAL NOT NULL,
			current_battery_energy_kwh REAL NOT NULL,
			required_night_charge_kwh REAL NOT NULL,
			daily_estimated_pv_kwh REAL NOT NULL DEFAULT 0,
			pv_charge_correction_factor REAL NOT NULL DEFAULT 0.7,
			pv_charge_correction_source TEXT NOT NULL DEFAULT 'default',
			corrected_estimated_pv_kwh REAL NOT NULL DEFAULT 0,
			corrected_estimated_pv_to_battery_kwh REAL NOT NULL DEFAULT 0,
			total_daytime_required_kwh REAL NOT NULL DEFAULT 0,
			total_available_kwh REAL NOT NULL DEFAULT 0,
			total_deficit_kwh REAL NOT NULL DEFAULT 0,
			pv_effective_start_at TEXT NOT NULL DEFAULT '',
			pv_effective_end_at TEXT NOT NULL DEFAULT '',
			pv_effective_window_source TEXT NOT NULL DEFAULT '',
			morning_to_pv_start_load_kwh REAL NOT NULL DEFAULT 0,
			forecast_daytime_deficit_kwh REAL NOT NULL DEFAULT 0,
			battery_soc INTEGER NOT NULL,
			battery_input_w INTEGER NOT NULL,
			battery_output_w INTEGER NOT NULL,
			grid_w INTEGER NOT NULL,
			import_w INTEGER NOT NULL,
			export_w INTEGER NOT NULL,
			should_charge_tonight INTEGER NOT NULL,
			would_write INTEGER NOT NULL,
			command_fingerprint TEXT NOT NULL DEFAULT 'none',
			command_sent INTEGER NOT NULL DEFAULT 0,
			command_error TEXT,
			command_block_reason TEXT NOT NULL,
			action_summary TEXT NOT NULL,
			reason TEXT NOT NULL,
			target_forecast_date TEXT,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS surplus_control_command_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			measured_at TEXT NOT NULL,
			strategy_state TEXT NOT NULL,
			command_kind TEXT NOT NULL DEFAULT 'none',
			command_fingerprint TEXT NOT NULL DEFAULT '',
			grid_w INTEGER NOT NULL,
			import_w INTEGER NOT NULL,
			export_w INTEGER NOT NULL,
			battery_soc INTEGER NOT NULL,
			battery_input_w INTEGER NOT NULL,
			battery_output_w INTEGER NOT NULL,
			previous_ac_charge_limit_w INTEGER,
			target_ac_charge_limit_w INTEGER,
			previous_backup_reserve_soc INTEGER,
			target_backup_reserve_soc INTEGER,
			command_sent INTEGER NOT NULL DEFAULT 0,
			dry_run INTEGER NOT NULL DEFAULT 1,
			would_write INTEGER NOT NULL DEFAULT 0,
			should_adjust_ac_charge_limit INTEGER NOT NULL DEFAULT 0,
			should_set_backup_reserve INTEGER NOT NULL DEFAULT 0,
			should_disable_energy_modes INTEGER NOT NULL DEFAULT 0,
			should_enable_tou_mode INTEGER NOT NULL DEFAULT 0,
			mode_guard_reason TEXT NOT NULL DEFAULT '',
			suppressed_reason TEXT NOT NULL DEFAULT '',
			decision_reason TEXT NOT NULL,
			error_message TEXT,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS notification_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			measured_at TEXT NOT NULL,
			kind TEXT NOT NULL,
			fingerprint TEXT NOT NULL,
			severity TEXT NOT NULL,
			message TEXT NOT NULL,
			reason TEXT NOT NULL,
			export_w INTEGER NOT NULL,
			battery_soc INTEGER NOT NULL,
			ac_charge_limit_w INTEGER NOT NULL,
			sent INTEGER NOT NULL DEFAULT 0,
			error_message TEXT,
			consecutive_hits INTEGER NOT NULL DEFAULT 0,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS tariff_settings (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			plan_name TEXT NOT NULL,
			day_rate_yen REAL NOT NULL,
			home_rate_yen REAL NOT NULL,
			night_rate_yen REAL NOT NULL,
			export_rate_yen REAL NOT NULL DEFAULT 7.0,
			timezone TEXT NOT NULL DEFAULT 'Asia/Tokyo',
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS tariff_plans (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			plan_name TEXT NOT NULL,
			day_rate_yen REAL NOT NULL,
			home_rate_yen REAL NOT NULL,
			night_rate_yen REAL NOT NULL,
			timezone TEXT NOT NULL DEFAULT 'Asia/Tokyo',
			effective_from TEXT NOT NULL,
			effective_to TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE(effective_from)
		)`,
		`CREATE TABLE IF NOT EXISTS current_status (
			id INTEGER PRIMARY KEY CHECK (id = 1),
			grid_w INTEGER NOT NULL DEFAULT 0,
			import_w INTEGER NOT NULL DEFAULT 0,
			export_w INTEGER NOT NULL DEFAULT 0,
			battery_soc INTEGER,
			battery_input_w INTEGER,
			battery_output_w INTEGER,
			ac_charge_limit_w INTEGER,
			target_charge_w INTEGER NOT NULL DEFAULT 0,
			state TEXT NOT NULL,
			mode TEXT NOT NULL,
			last_decision_reason TEXT NOT NULL,
			last_error TEXT,
			ecoflow_diagnostics_json TEXT,
			updated_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS delta3_aux_control_command_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			measured_at TEXT NOT NULL,
			strategy_state TEXT NOT NULL,
			command_fingerprint TEXT NOT NULL,
			grid_w INTEGER NOT NULL,
			import_w INTEGER NOT NULL,
			export_w INTEGER NOT NULL,
			residual_export_w INTEGER NOT NULL,
			delta3_soc INTEGER,
			previous_ac_charge_limit_w INTEGER,
			target_ac_charge_limit_w INTEGER,
			previous_backup_reserve_soc INTEGER,
			target_backup_reserve_soc INTEGER,
			command_sent INTEGER NOT NULL,
			dry_run INTEGER NOT NULL,
			would_write INTEGER NOT NULL,
			should_adjust_ac_charge_limit INTEGER NOT NULL,
			should_set_backup_reserve INTEGER NOT NULL DEFAULT 0,
			should_disable_backup_reserve INTEGER NOT NULL DEFAULT 0,
			suppressed_reason TEXT NOT NULL,
			decision_reason TEXT NOT NULL,
			error_message TEXT,
			created_at TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS charging_devices (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name TEXT NOT NULL,
			kind TEXT NOT NULL,
			provider TEXT NOT NULL,
			role TEXT NOT NULL,
			credential_ref TEXT NOT NULL,
			device_sn TEXT NOT NULL DEFAULT '',
			device_type TEXT NOT NULL DEFAULT '',
			status_source TEXT NOT NULL DEFAULT '',
			enabled INTEGER NOT NULL DEFAULT 1,
			control_enabled INTEGER NOT NULL DEFAULT 0,
			priority INTEGER NOT NULL DEFAULT 100,
			min_charge_w INTEGER NOT NULL DEFAULT 0,
			max_charge_w INTEGER NOT NULL DEFAULT 0,
			charge_step_w INTEGER NOT NULL DEFAULT 1,
			capacity_wh INTEGER NOT NULL DEFAULT 0,
			target_soc INTEGER NOT NULL DEFAULT 90,
			reserve_soc INTEGER NOT NULL DEFAULT 30,
			backup_reserve_min_soc INTEGER NOT NULL DEFAULT 0,
			backup_reserve_max_soc INTEGER NOT NULL DEFAULT 0,
			expected_daytime_load_w INTEGER NOT NULL DEFAULT 0,
			supports_soc_read INTEGER NOT NULL DEFAULT 0,
			supports_ac_charge_limit INTEGER NOT NULL DEFAULT 0,
			supports_on_off INTEGER NOT NULL DEFAULT 0,
			notes TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			UNIQUE(credential_ref)
		)`,
		`CREATE TABLE IF NOT EXISTS charging_device_power_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			device_id INTEGER NOT NULL,
			measured_at TEXT NOT NULL,
			soc INTEGER,
			ac_input_w INTEGER,
			ac_output_w INTEGER,
			ac_charge_limit_w INTEGER,
			backup_reserve_soc INTEGER,
			status_source TEXT NOT NULL DEFAULT '',
			available INTEGER NOT NULL DEFAULT 0,
			error_message TEXT,
			created_at TEXT NOT NULL,
			FOREIGN KEY(device_id) REFERENCES charging_devices(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS pv_charge_correction_daily_logs (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			date TEXT NOT NULL UNIQUE,
			forecast_pv_kwh REAL NOT NULL DEFAULT 0,
			forecast_pv_to_battery_kwh REAL NOT NULL DEFAULT 0,
			actual_battery_input_kwh REAL NOT NULL DEFAULT 0,
			actual_export_kwh REAL NOT NULL DEFAULT 0,
			actual_capture_factor REAL NOT NULL DEFAULT 0,
			weather_code INTEGER NOT NULL DEFAULT 0,
			cloud_cover_mean_percent INTEGER NOT NULL DEFAULT 0,
			sample_quality TEXT NOT NULL DEFAULT 'insufficient-data',
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			return err
		}
	}
	for _, column := range []string{"device_sn", "device_type", "status_source", "backup_reserve_min_soc", "backup_reserve_max_soc", "expected_daytime_load_w"} {
		if err := addKnownColumnIfMissing(db, "charging_devices", column); err != nil {
			return err
		}
	}
	if _, err := db.Exec(`UPDATE charging_devices
		SET status_source = CASE
			WHEN kind = 'ecoflow_delta_pro3' THEN 'ecoflow_cloud'
			WHEN kind = 'ecoflow_delta3_plus' THEN 'ecoflow_private_mqtt'
			WHEN kind = 'switchbot_plug' THEN 'switchbot_cloud'
			ELSE 'manual'
		END
		WHERE status_source = ''`); err != nil {
		return err
	}
	if _, err := db.Exec(`UPDATE charging_devices
		SET backup_reserve_min_soc = CASE
			WHEN reserve_soc < 5 THEN 5
			WHEN reserve_soc > 100 THEN 100
			ELSE reserve_soc
		END
		WHERE backup_reserve_min_soc = 0`); err != nil {
		return err
	}
	if _, err := db.Exec(`UPDATE charging_devices
		SET backup_reserve_max_soc = CASE
			WHEN target_soc < 5 THEN 5
			WHEN target_soc > 100 THEN 100
			ELSE target_soc
		END
		WHERE backup_reserve_max_soc = 0`); err != nil {
		return err
	}
	if _, err := db.Exec(`UPDATE charging_devices
		SET backup_reserve_max_soc = backup_reserve_min_soc
		WHERE backup_reserve_max_soc < backup_reserve_min_soc`); err != nil {
		return err
	}
	if _, err := db.Exec(`CREATE UNIQUE INDEX IF NOT EXISTS idx_charging_devices_device_sn_nonempty ON charging_devices(device_sn) WHERE device_sn <> ''`); err != nil {
		return err
	}
	if err := addKnownColumnIfMissing(db, "power_logs", "ac_charge_limit_w"); err != nil {
		return err
	}
	if err := addKnownColumnIfMissing(db, "power_logs", "ecoflow_diagnostics_json"); err != nil {
		return err
	}
	if err := addKnownColumnIfMissing(db, "current_status", "ac_charge_limit_w"); err != nil {
		return err
	}
	for _, column := range []string{
		"weather_forecast_enabled",
		"weather_latitude",
		"weather_longitude",
		"weather_timezone",
		"pv_capacity_kw",
		"pv_performance_ratio",
		"daily_base_load_kwh",
		"battery_capacity_kwh",
		"minimum_reserve_soc",
		"pv_charge_correction_factor",
		"pv_charge_correction_manual",
		"pv_charge_correction_updated_at",
		"pv_charge_correction_min_sample_days",
		"pv_charge_correction_min_factor",
		"pv_charge_correction_max_factor",
	} {
		if err := addKnownColumnIfMissing(db, "settings", column); err != nil {
			return err
		}
	}
	for _, column := range []string{
		"backup_reserve_soc",
		"energy_backup_enabled",
		"tou_mode_enabled",
		"self_powered_enabled",
		"scheduled_enabled",
		"intelligent_enabled",
		"battery_full_energy_wh",
		"ecoflow_diagnostics_json",
		"surplus_plan_json",
		"night_charge_plan_json",
		"delta3_aux_plan_json",
	} {
		if err := addKnownColumnIfMissing(db, "current_status", column); err != nil {
			return err
		}
	}
	if err := addKnownColumnIfMissing(db, "tariff_plans", "export_rate_yen"); err != nil {
		return err
	}
	for _, column := range []string{
		"daily_estimated_pv_kwh",
		"pv_effective_start_at",
		"pv_effective_end_at",
		"pv_effective_window_source",
		"morning_to_pv_start_load_kwh",
		"forecast_daytime_deficit_kwh",
		"pv_charge_correction_factor",
		"pv_charge_correction_source",
		"corrected_estimated_pv_kwh",
		"corrected_estimated_pv_to_battery_kwh",
		"total_daytime_required_kwh",
		"total_available_kwh",
		"total_deficit_kwh",
		"command_fingerprint",
		"command_sent",
		"command_error",
	} {
		if err := addKnownColumnIfMissing(db, "night_charge_plan_logs", column); err != nil {
			return err
		}
	}
	for _, column := range []string{
		"command_kind",
		"command_fingerprint",
		"should_adjust_ac_charge_limit",
		"should_set_backup_reserve",
		"should_disable_energy_modes",
		"should_enable_tou_mode",
		"mode_guard_reason",
	} {
		if err := addKnownColumnIfMissing(db, "surplus_control_command_logs", column); err != nil {
			return err
		}
	}
	for _, column := range []string{
		"previous_backup_reserve_soc",
		"target_backup_reserve_soc",
		"should_set_backup_reserve",
		"should_disable_backup_reserve",
	} {
		if err := addKnownColumnIfMissing(db, "delta3_aux_control_command_logs", column); err != nil {
			return err
		}
	}
	return seedDefaults(db, time.Now())
}

func addKnownColumnIfMissing(db *sql.DB, table string, column string) error {
	columnType, ok := knownMigrationColumns[table][column]
	if !ok {
		return fmt.Errorf("unsupported migration column: %s.%s", table, column)
	}
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull int
		var defaultValue any
		var primaryKey int
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			return err
		}
		if name == column {
			return rows.Err()
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = db.Exec(`ALTER TABLE ` + table + ` ADD COLUMN ` + column + ` ` + columnType)
	return err
}

var knownMigrationColumns = map[string]map[string]string{
	"settings": {
		"weather_forecast_enabled":             "INTEGER NOT NULL DEFAULT 0",
		"weather_latitude":                     "REAL",
		"weather_longitude":                    "REAL",
		"weather_timezone":                     "TEXT NOT NULL DEFAULT 'Asia/Tokyo'",
		"pv_capacity_kw":                       "REAL NOT NULL DEFAULT 0",
		"pv_performance_ratio":                 "REAL NOT NULL DEFAULT 0.75",
		"daily_base_load_kwh":                  "REAL NOT NULL DEFAULT 0",
		"battery_capacity_kwh":                 "REAL NOT NULL DEFAULT 4.096",
		"minimum_reserve_soc":                  "INTEGER NOT NULL DEFAULT 30",
		"pv_charge_correction_factor":          "REAL NOT NULL DEFAULT 0.7",
		"pv_charge_correction_manual":          "INTEGER NOT NULL DEFAULT 0",
		"pv_charge_correction_updated_at":      "TEXT",
		"pv_charge_correction_min_sample_days": "INTEGER NOT NULL DEFAULT 7",
		"pv_charge_correction_min_factor":      "REAL NOT NULL DEFAULT 0.2",
		"pv_charge_correction_max_factor":      "REAL NOT NULL DEFAULT 0.9",
	},
	"power_logs": {
		"ac_charge_limit_w":        "INTEGER",
		"ecoflow_diagnostics_json": "TEXT",
	},
	"current_status": {
		"ac_charge_limit_w":        "INTEGER",
		"backup_reserve_soc":       "INTEGER",
		"energy_backup_enabled":    "INTEGER",
		"tou_mode_enabled":         "INTEGER",
		"self_powered_enabled":     "INTEGER",
		"scheduled_enabled":        "INTEGER",
		"intelligent_enabled":      "INTEGER",
		"battery_full_energy_wh":   "INTEGER",
		"ecoflow_diagnostics_json": "TEXT",
		"surplus_plan_json":        "TEXT",
		"night_charge_plan_json":   "TEXT",
		"delta3_aux_plan_json":     "TEXT",
	},
	"tariff_plans": {
		"export_rate_yen": "REAL NOT NULL DEFAULT 7.0",
	},
	"night_charge_plan_logs": {
		"daily_estimated_pv_kwh":                "REAL NOT NULL DEFAULT 0",
		"pv_effective_start_at":                 "TEXT NOT NULL DEFAULT ''",
		"pv_effective_end_at":                   "TEXT NOT NULL DEFAULT ''",
		"pv_effective_window_source":            "TEXT NOT NULL DEFAULT ''",
		"morning_to_pv_start_load_kwh":          "REAL NOT NULL DEFAULT 0",
		"forecast_daytime_deficit_kwh":          "REAL NOT NULL DEFAULT 0",
		"pv_charge_correction_factor":           "REAL NOT NULL DEFAULT 0.7",
		"pv_charge_correction_source":           "TEXT NOT NULL DEFAULT 'default'",
		"corrected_estimated_pv_kwh":            "REAL NOT NULL DEFAULT 0",
		"corrected_estimated_pv_to_battery_kwh": "REAL NOT NULL DEFAULT 0",
		"total_daytime_required_kwh":            "REAL NOT NULL DEFAULT 0",
		"total_available_kwh":                   "REAL NOT NULL DEFAULT 0",
		"total_deficit_kwh":                     "REAL NOT NULL DEFAULT 0",
		"command_fingerprint":                   "TEXT NOT NULL DEFAULT 'none'",
		"command_sent":                          "INTEGER NOT NULL DEFAULT 0",
		"command_error":                         "TEXT",
	},
	"surplus_control_command_logs": {
		"command_kind":                  "TEXT NOT NULL DEFAULT 'none'",
		"command_fingerprint":           "TEXT NOT NULL DEFAULT ''",
		"should_adjust_ac_charge_limit": "INTEGER NOT NULL DEFAULT 0",
		"should_set_backup_reserve":     "INTEGER NOT NULL DEFAULT 0",
		"should_disable_energy_modes":   "INTEGER NOT NULL DEFAULT 0",
		"should_enable_tou_mode":        "INTEGER NOT NULL DEFAULT 0",
		"mode_guard_reason":             "TEXT NOT NULL DEFAULT ''",
	},
	"charging_devices": {
		"device_sn":               "TEXT NOT NULL DEFAULT ''",
		"device_type":             "TEXT NOT NULL DEFAULT ''",
		"status_source":           "TEXT NOT NULL DEFAULT ''",
		"backup_reserve_min_soc":  "INTEGER NOT NULL DEFAULT 0",
		"backup_reserve_max_soc":  "INTEGER NOT NULL DEFAULT 0",
		"expected_daytime_load_w": "INTEGER NOT NULL DEFAULT 0",
	},
	"delta3_aux_control_command_logs": {
		"previous_backup_reserve_soc":   "INTEGER",
		"target_backup_reserve_soc":     "INTEGER",
		"should_set_backup_reserve":     "INTEGER NOT NULL DEFAULT 0",
		"should_disable_backup_reserve": "INTEGER NOT NULL DEFAULT 0",
	},
}

func seedDefaults(db *sql.DB, now time.Time) error {
	_, err := db.Exec(
		`INSERT INTO settings (id, updated_at) VALUES (1, ?)
		 ON CONFLICT(id) DO NOTHING`,
		now.Format(time.RFC3339),
	)
	if err != nil {
		return err
	}

	if _, err := db.Exec(
		`INSERT INTO current_status (
			id, grid_w, import_w, export_w, battery_soc, battery_input_w,
			battery_output_w, ac_charge_limit_w, target_charge_w, state, mode, last_decision_reason,
			last_error, updated_at
		) VALUES (1, 0, 0, 0, 60, 0, 0, 0, 0, 'simulation', 'mock', 'initialized in mock simulation mode', NULL, ?)
		ON CONFLICT(id) DO NOTHING`,
		now.Format(time.RFC3339),
	); err != nil {
		return err
	}

	_, err = db.Exec(
		`INSERT INTO tariff_settings (
			id, plan_name, day_rate_yen, home_rate_yen, night_rate_yen, timezone, updated_at
		) VALUES (1, '中部電力 Eライフプラン（3時間帯別電灯）', 34.06, 26.00, 16.11, 'Asia/Tokyo', ?)
		ON CONFLICT(id) DO NOTHING`,
		now.Format(time.RFC3339),
	)
	if err != nil {
		return err
	}

	_, err = db.Exec(
		`INSERT INTO tariff_plans (
			plan_name, day_rate_yen, home_rate_yen, night_rate_yen, export_rate_yen, timezone,
			effective_from, effective_to, created_at, updated_at
		)
		SELECT
			plan_name, day_rate_yen, home_rate_yen, night_rate_yen, 7.0, timezone,
			'1970-01-01T00:00:00Z', NULL, ?, updated_at
		FROM tariff_settings
		WHERE id = 1
			AND NOT EXISTS (SELECT 1 FROM tariff_plans)`,
		now.Format(time.RFC3339),
	)
	if err != nil {
		return err
	}

	return seedChargingDevices(db, now)
}

func seedChargingDevices(db *sql.DB, now time.Time) error {
	timestamp := now.Format(time.RFC3339)
	_, err := db.Exec(
		`INSERT INTO charging_devices (
			name, kind, provider, role, credential_ref, device_sn, device_type, status_source, enabled, control_enabled, priority,
			min_charge_w, max_charge_w, charge_step_w, capacity_wh, target_soc, reserve_soc, backup_reserve_min_soc, backup_reserve_max_soc,
			expected_daytime_load_w, supports_soc_read, supports_ac_charge_limit, supports_on_off, notes, created_at, updated_at
		)
		SELECT name, kind, provider, role, credential_ref, device_sn, device_type, status_source, enabled, control_enabled, priority,
			min_charge_w, max_charge_w, charge_step_w, capacity_wh, target_soc, reserve_soc, backup_reserve_min_soc, backup_reserve_max_soc,
			expected_daytime_load_w, supports_soc_read, supports_ac_charge_limit, supports_on_off, notes, ?, ?
		FROM (
			SELECT 'DELTA Pro 3' AS name, 'ecoflow_delta_pro3' AS kind, 'ecoflow' AS provider, 'primary' AS role,
				'ecoflow_pro3_primary' AS credential_ref, '' AS device_sn, 'DELTA_PRO3' AS device_type, 'ecoflow_cloud' AS status_source,
				1 AS enabled, 0 AS control_enabled, 10 AS priority,
				400 AS min_charge_w, 1500 AS max_charge_w, 100 AS charge_step_w, 12288 AS capacity_wh,
				90 AS target_soc, 30 AS reserve_soc, 30 AS backup_reserve_min_soc, 90 AS backup_reserve_max_soc,
				0 AS expected_daytime_load_w,
				1 AS supports_soc_read, 1 AS supports_ac_charge_limit,
				0 AS supports_on_off, '既存の DELTA Pro 3 制御用。SN や認証情報は環境変数側で管理します。' AS notes
			UNION ALL
			SELECT 'DELTA 3 Plus', 'ecoflow_delta3_plus', 'ecoflow', 'auxiliary',
				'ecoflow_delta3_primary', '', 'DELTA_3', 'ecoflow_private_mqtt', 1, 0, 20,
				100, 1500, 100, 2048,
				90, 20, 20, 90, 400, 1, 1,
				1, 'DELTA 3 Plus 補助充電候補。追加台はこのマスタで増やします。'
			UNION ALL
			SELECT '手動補助バッテリー', 'manual', 'manual', 'manual_auxiliary',
				'manual_auxiliary', '', '', 'manual', 1, 0, 90,
				0, 0, 1, 0,
				90, 20, 20, 90, 0, 0, 0,
				0, 'API制御できない補助充電の運用メモ用です。'
		)
		WHERE NOT EXISTS (SELECT 1 FROM charging_devices)`,
		timestamp,
		timestamp,
	)
	return err
}
