package store

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func TestMigrateCreatesPhaseOneTables(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	if err := migrate(db); err != nil {
		t.Fatalf("migrate failed: %v", err)
	}

	for _, table := range []string{"settings", "current_status", "power_logs", "night_charge_plan_logs", "notification_logs", "delta3_aux_control_command_logs", "charging_devices"} {
		var name string
		err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
		if err != nil {
			t.Fatalf("table %s was not created: %v", table, err)
		}
	}

	for _, column := range []string{"device_sn", "device_type", "status_source", "backup_reserve_min_soc", "backup_reserve_max_soc"} {
		if !tableHasColumn(t, db, "charging_devices", column) {
			t.Fatalf("charging_devices.%s was not created", column)
		}
	}

	var indexName string
	err = db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'index' AND name = 'idx_charging_devices_device_sn_nonempty'`).Scan(&indexName)
	if err != nil {
		t.Fatalf("device_sn partial unique index was not created: %v", err)
	}
}

func tableHasColumn(t *testing.T, db *sql.DB, table string, column string) bool {
	t.Helper()
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull int
		var defaultValue any
		var primaryKey int
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatal(err)
		}
		if name == column {
			return true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return false
}
