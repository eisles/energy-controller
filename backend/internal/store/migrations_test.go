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

	for _, table := range []string{"settings", "current_status", "power_logs"} {
		var name string
		err := db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&name)
		if err != nil {
			t.Fatalf("table %s was not created: %v", table, err)
		}
	}
}
