package decisionstore

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"time"

	_ "modernc.org/sqlite"
)

const (
	TTL      = 7 * 24 * time.Hour
	Capacity = 256
)

var (
	ErrNotFound           = errors.New("decisionstore: snapshot not found")
	ErrExpired            = errors.New("decisionstore: snapshot expired")
	ErrCorrupt            = errors.New("decisionstore: snapshot corrupt")
	ErrCapacity           = errors.New("decisionstore: capacity reached")
	ErrOpen               = errors.New("decisionstore: open failed")
	ErrVersionUnsupported = errors.New("decisionstore: unsupported store version")
	ErrRead               = errors.New("decisionstore: read failed")
	ErrWrite              = errors.New("decisionstore: write failed")
	ErrID                 = errors.New("decisionstore: id generation failed")
)

// Record is one stored snapshot.
type Record struct {
	Payload []byte
	Parent  string
	Expires time.Time
}

// Store is a SQLite-backed snapshot repository at Path.
type Store struct {
	Path string
	Now  func() time.Time
	// Valid, when set, must accept a payload for its record to count as intact.
	Valid func([]byte) bool
}

type query interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func ValidID(id string) bool {
	if len(id) != 32 {
		return false
	}
	for _, c := range id {
		if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}

func checksum(id, parent string, expires int64, raw []byte) string {
	hash := sha256.Sum256(append(fmt.Appendf(nil, "%s\n%s\n%d\n", id, parent, expires), raw...))
	return hex.EncodeToString(hash[:])
}

func (s *Store) open(ctx context.Context, writable bool) (*sql.DB, error) {
	if writable {
		if err := prepareFile(s.Path); err != nil {
			return nil, ErrOpen
		}
	}
	abs, err := filepath.Abs(s.Path)
	if err != nil {
		return nil, ErrOpen
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
		return nil, ErrOpen
	}
	db.SetMaxOpenConns(1)
	if err := initialize(ctx, db, writable); err != nil {
		_ = db.Close()
		return nil, err
	}
	return db, nil
}

func prepareFile(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	return file.Close()
}

func initialize(ctx context.Context, db *sql.DB, writable bool) error {
	var version int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return ErrOpen
	}
	if version != 0 && version != 1 {
		return ErrVersionUnsupported
	}
	if !writable {
		if version != 1 {
			return ErrVersionUnsupported
		}
		return nil
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS decision_snapshots (
id TEXT PRIMARY KEY, parent TEXT NOT NULL, expires INTEGER NOT NULL,
request BLOB NOT NULL, digest TEXT NOT NULL, checksum TEXT NOT NULL,
UNIQUE(parent,digest))`); err != nil {
		return ErrOpen
	}
	if _, err := db.ExecContext(ctx, "PRAGMA user_version=1"); err != nil {
		return ErrOpen
	}
	return nil
}

func (s *Store) read(ctx context.Context, q query, id string, now time.Time) (Record, error) {
	var parent, digest, sum string
	var expires int64
	var raw []byte
	err := q.QueryRowContext(ctx, `SELECT parent,expires,CASE WHEN length(request)<=262144 THEN request ELSE NULL END,digest,checksum FROM decision_snapshots WHERE id=?`, id).Scan(&parent, &expires, &raw, &digest, &sum)
	if errors.Is(err, sql.ErrNoRows) {
		return Record{}, ErrNotFound
	}
	if err != nil {
		return Record{}, ErrRead
	}
	hash := sha256.Sum256(raw)
	if len(raw) == 0 || parent != "" && !ValidID(parent) || hex.EncodeToString(hash[:]) != digest || checksum(id, parent, expires, raw) != sum {
		return Record{}, ErrCorrupt
	}
	until := time.Unix(0, expires).UTC()
	if !now.Before(until) {
		return Record{}, ErrExpired
	}
	if s.Valid != nil && !s.Valid(raw) {
		return Record{}, ErrCorrupt
	}
	return Record{Payload: raw, Parent: parent, Expires: until}, nil
}

// Load returns the snapshot id without creating the store file.
func (s *Store) Load(ctx context.Context, id string) (Record, error) {
	if _, err := os.Stat(s.Path); errors.Is(err, os.ErrNotExist) {
		return Record{}, ErrNotFound
	}
	db, err := s.open(ctx, false)
	if err != nil {
		return Record{}, err
	}
	defer db.Close()
	return s.read(ctx, db, id, s.Now())
}

// Save stores payload under parent, returning the existing id for an identical
// (parent, payload) pair.
func (s *Store) Save(ctx context.Context, payload []byte, parent string) (string, time.Time, error) {
	db, err := s.open(ctx, true)
	if err != nil {
		return "", time.Time{}, err
	}
	defer db.Close()
	conn, err := db.Conn(ctx)
	if err != nil {
		return "", time.Time{}, ErrWrite
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return "", time.Time{}, ErrWrite
	}
	defer func() { _, _ = conn.ExecContext(context.Background(), "ROLLBACK") }()
	id, expires, err := s.insert(ctx, conn, payload, parent)
	if err != nil {
		return "", time.Time{}, err
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return "", time.Time{}, ErrWrite
	}
	return id, expires, nil
}

func (s *Store) insert(ctx context.Context, conn *sql.Conn, raw []byte, parent string) (string, time.Time, error) {
	now := s.Now().UTC()
	if parent != "" {
		if _, err := s.read(ctx, conn, parent, now); err != nil {
			return "", time.Time{}, err
		}
	}
	if _, err := conn.ExecContext(ctx, "DELETE FROM decision_snapshots WHERE expires<=?", now.UnixNano()); err != nil {
		return "", time.Time{}, ErrWrite
	}
	digest := sha256.Sum256(raw)
	hexdigest := hex.EncodeToString(digest[:])
	var existing string
	err := conn.QueryRowContext(ctx, "SELECT id FROM decision_snapshots WHERE parent=? AND digest=?", parent, hexdigest).Scan(&existing)
	if err == nil {
		row, err := s.read(ctx, conn, existing, now)
		return existing, row.Expires, err
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", time.Time{}, ErrRead
	}
	var count int
	if err := conn.QueryRowContext(ctx, "SELECT count(*) FROM decision_snapshots").Scan(&count); err != nil {
		return "", time.Time{}, ErrRead
	}
	if count >= Capacity {
		return "", time.Time{}, ErrCapacity
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", time.Time{}, ErrID
	}
	id, expires := hex.EncodeToString(nonce[:]), now.Add(TTL)
	if _, err := conn.ExecContext(ctx, "INSERT INTO decision_snapshots VALUES(?,?,?,?,?,?)", id, parent, expires.UnixNano(), raw, hexdigest, checksum(id, parent, expires.UnixNano(), raw)); err != nil {
		return "", time.Time{}, ErrWrite
	}
	return id, expires, nil
}
