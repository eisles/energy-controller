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
	"power_logs": {
		"ac_charge_limit_w": "INTEGER",
	},
	"current_status": {
		"ac_charge_limit_w": "INTEGER",
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

	_, err = db.Exec(
		`INSERT INTO current_status (
			id, grid_w, import_w, export_w, battery_soc, battery_input_w,
			battery_output_w, ac_charge_limit_w, target_charge_w, state, mode, last_decision_reason,
			last_error, updated_at
		) VALUES (1, 0, 0, 0, 60, 0, 0, 0, 0, 'simulation', 'mock', 'initialized in mock simulation mode', NULL, ?)
		ON CONFLICT(id) DO NOTHING`,
		now.Format(time.RFC3339),
	)
	return err
}
