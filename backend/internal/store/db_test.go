package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenConfiguresSQLiteConcurrencyPragmas(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "energy.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	var journalMode string
	if err := db.QueryRow("PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("read journal mode: %v", err)
	}
	if journalMode != "wal" {
		t.Fatalf("journal mode = %q, want wal", journalMode)
	}

	ctx := context.Background()
	first, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("open first connection: %v", err)
	}
	defer first.Close()
	second, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("open second connection: %v", err)
	}
	defer second.Close()

	for index, connection := range []*sql.Conn{first, second} {
		var busyTimeout int
		if err := connection.QueryRowContext(ctx, "PRAGMA busy_timeout").Scan(&busyTimeout); err != nil {
			t.Fatalf("read busy timeout from connection %d: %v", index+1, err)
		}
		if busyTimeout != sqliteBusyTimeoutMilliseconds {
			t.Fatalf(
				"connection %d busy timeout = %d, want %d",
				index+1,
				busyTimeout,
				sqliteBusyTimeoutMilliseconds,
			)
		}
	}
}

func TestOpenAllowsWriteWhileAnotherConnectionIsReading(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "energy.db"))
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer db.Close()

	if _, err := db.Exec(`CREATE TABLE concurrency_test (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		value TEXT NOT NULL
	)`); err != nil {
		t.Fatalf("create concurrency table: %v", err)
	}
	if _, err := db.Exec("INSERT INTO concurrency_test (value) VALUES ('initial')"); err != nil {
		t.Fatalf("insert initial row: %v", err)
	}

	ctx := context.Background()
	reader, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("open reader connection: %v", err)
	}
	defer reader.Close()
	writer, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("open writer connection: %v", err)
	}
	defer writer.Close()

	rows, err := reader.QueryContext(ctx, "SELECT value FROM concurrency_test ORDER BY id")
	if err != nil {
		t.Fatalf("start read: %v", err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatal("expected initial row")
	}
	var value string
	if err := rows.Scan(&value); err != nil {
		t.Fatalf("scan initial row: %v", err)
	}

	writeCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	if _, err := writer.ExecContext(
		writeCtx,
		"INSERT INTO concurrency_test (value) VALUES ('during-read')",
	); err != nil {
		t.Fatalf("write while read cursor is open: %v", err)
	}
}

func TestOpenWaitsForConcurrentWriter(t *testing.T) {
	path := filepath.Join(t.TempDir(), "energy.db")
	firstDB, err := Open(path)
	if err != nil {
		t.Fatalf("open first database handle: %v", err)
	}
	defer firstDB.Close()
	secondDB, err := Open(path)
	if err != nil {
		t.Fatalf("open second database handle: %v", err)
	}
	defer secondDB.Close()

	if _, err := firstDB.Exec(`CREATE TABLE writer_test (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		value TEXT NOT NULL
	)`); err != nil {
		t.Fatalf("create writer table: %v", err)
	}

	ctx := context.Background()
	tx, err := firstDB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin first writer: %v", err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO writer_test (value) VALUES ('first')"); err != nil {
		tx.Rollback()
		t.Fatalf("hold first write lock: %v", err)
	}

	commitResult := make(chan error, 1)
	go func() {
		time.Sleep(150 * time.Millisecond)
		commitResult <- tx.Commit()
	}()

	startedAt := time.Now()
	writeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	_, writeErr := secondDB.ExecContext(
		writeCtx,
		"INSERT INTO writer_test (value) VALUES ('second')",
	)
	elapsed := time.Since(startedAt)
	if commitErr := <-commitResult; commitErr != nil {
		t.Fatalf("commit first writer: %v", commitErr)
	}
	if writeErr != nil {
		t.Fatalf("second writer did not wait for lock release: %v", writeErr)
	}
	if elapsed < 100*time.Millisecond {
		t.Fatalf("second writer returned after %s, want it to wait for the first writer", elapsed)
	}

	var count int
	if err := firstDB.QueryRow("SELECT COUNT(*) FROM writer_test").Scan(&count); err != nil {
		t.Fatalf("count writer rows: %v", err)
	}
	if count != 2 {
		t.Fatalf("writer row count = %d, want 2", count)
	}
}
