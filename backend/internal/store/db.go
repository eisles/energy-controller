package store

import (
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

const sqliteBusyTimeoutMilliseconds = 5000

func Open(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	query := url.Values{}
	query.Add("_pragma", fmt.Sprintf("busy_timeout(%d)", sqliteBusyTimeoutMilliseconds))
	query.Add("_pragma", "journal_mode(WAL)")
	separator := "?"
	if strings.Contains(path, "?") {
		separator = "&"
	}
	db, err := sql.Open("sqlite", path+separator+query.Encode())
	if err != nil {
		return nil, err
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}
