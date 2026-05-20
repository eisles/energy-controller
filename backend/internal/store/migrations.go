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
			updated_at TEXT NOT NULL
		)`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			return err
		}
	}
	if err := addKnownColumnIfMissing(db, "power_logs", "ac_charge_limit_w"); err != nil {
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
		"surplus_plan_json",
		"night_charge_plan_json",
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
		"weather_forecast_enabled": "INTEGER NOT NULL DEFAULT 0",
		"weather_latitude":         "REAL",
		"weather_longitude":        "REAL",
		"weather_timezone":         "TEXT NOT NULL DEFAULT 'Asia/Tokyo'",
		"pv_capacity_kw":           "REAL NOT NULL DEFAULT 0",
		"pv_performance_ratio":     "REAL NOT NULL DEFAULT 0.75",
		"daily_base_load_kwh":      "REAL NOT NULL DEFAULT 0",
		"battery_capacity_kwh":     "REAL NOT NULL DEFAULT 4.096",
		"minimum_reserve_soc":      "INTEGER NOT NULL DEFAULT 30",
	},
	"power_logs": {
		"ac_charge_limit_w": "INTEGER",
	},
	"current_status": {
		"ac_charge_limit_w":      "INTEGER",
		"backup_reserve_soc":     "INTEGER",
		"energy_backup_enabled":  "INTEGER",
		"tou_mode_enabled":       "INTEGER",
		"self_powered_enabled":   "INTEGER",
		"scheduled_enabled":      "INTEGER",
		"intelligent_enabled":    "INTEGER",
		"battery_full_energy_wh": "INTEGER",
		"surplus_plan_json":      "TEXT",
		"night_charge_plan_json": "TEXT",
	},
	"tariff_plans": {
		"export_rate_yen": "REAL NOT NULL DEFAULT 7.0",
	},
	"night_charge_plan_logs": {
		"daily_estimated_pv_kwh":       "REAL NOT NULL DEFAULT 0",
		"pv_effective_start_at":        "TEXT NOT NULL DEFAULT ''",
		"pv_effective_end_at":          "TEXT NOT NULL DEFAULT ''",
		"pv_effective_window_source":   "TEXT NOT NULL DEFAULT ''",
		"morning_to_pv_start_load_kwh": "REAL NOT NULL DEFAULT 0",
		"forecast_daytime_deficit_kwh": "REAL NOT NULL DEFAULT 0",
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
	return err
}
