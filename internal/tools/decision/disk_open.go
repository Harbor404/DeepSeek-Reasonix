package decision

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"time"

	_ "modernc.org/sqlite"
)

const diskTTL = 7 * 24 * time.Hour
const diskCapacity = 256

type diskRepository struct {
	path string
	now  func() time.Time
}

func (r *diskRepository) persistent() bool { return true }

func (r *diskRepository) open(ctx context.Context, writable bool) (*sql.DB, *problem) {
	if writable {
		if err := prepareDiskFile(r.path); err != nil {
			return nil, err
		}
	}
	abs, err := filepath.Abs(r.path)
	if err != nil {
		return nil, &problem{Code: "decision.store_open_failed"}
	}
	slash := filepath.ToSlash(abs)
	if runtime.GOOS == "windows" && len(slash) >= 2 && slash[1] == ':' {
		slash = "/" + slash
	}
	u := url.URL{Scheme: "file", Path: slash}
	dsn := u.String() + "?_pragma=busy_timeout%285000%29&_pragma=synchronous%28FULL%29"
	if !writable {
		dsn += "&mode=ro"
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, &problem{Code: "decision.store_open_failed"}
	}
	db.SetMaxOpenConns(1)
	if invalid := initializeDisk(ctx, db, writable); invalid != nil {
		_ = db.Close()
		return nil, invalid
	}
	return db, nil
}

func prepareDiskFile(path string) *problem {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return &problem{Code: "decision.store_open_failed"}
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return &problem{Code: "decision.store_open_failed"}
	}
	if err := file.Close(); err != nil {
		return &problem{Code: "decision.store_open_failed"}
	}
	return nil
}

func initializeDisk(ctx context.Context, db *sql.DB, writable bool) *problem {
	var version int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return &problem{Code: "decision.store_open_failed"}
	}
	if version != 0 && version != 1 {
		return &problem{Code: "decision.store_version_unsupported"}
	}
	if !writable {
		if version != 1 {
			return &problem{Code: "decision.store_version_unsupported"}
		}
		return nil
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS decision_snapshots (
id TEXT PRIMARY KEY, parent TEXT NOT NULL, expires INTEGER NOT NULL,
request BLOB NOT NULL, digest TEXT NOT NULL, checksum TEXT NOT NULL,
UNIQUE(parent,digest))`); err != nil {
		return &problem{Code: "decision.store_open_failed"}
	}
	if _, err := db.ExecContext(ctx, "PRAGMA user_version=1"); err != nil {
		return &problem{Code: "decision.store_open_failed"}
	}
	return nil
}
