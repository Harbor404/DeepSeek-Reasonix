package decision

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"
)

type diskQuery interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func recordChecksum(id, parent string, expires int64, raw []byte) string {
	hash := sha256.Sum256(append(fmt.Appendf(nil, "%s\n%s\n%d\n", id, parent, expires), raw...))
	return hex.EncodeToString(hash[:])
}

func readDisk(ctx context.Context, query diskQuery, id string, now time.Time) (savedRequest, *problem) {
	var parent, digest, checksum string
	var expires int64
	var raw []byte
	err := query.QueryRowContext(ctx, `SELECT parent,expires,CASE WHEN length(request)<=262144 THEN request ELSE NULL END,digest,checksum FROM decision_snapshots WHERE id=?`, id).Scan(&parent, &expires, &raw, &digest, &checksum)
	if errors.Is(err, sql.ErrNoRows) {
		return savedRequest{}, &problem{Code: "decision.snapshot_not_found", ID: id}
	}
	if err != nil {
		return savedRequest{}, &problem{Code: "decision.store_read_failed", ID: id}
	}
	hash := sha256.Sum256(raw)
	if len(raw) == 0 || parent != "" && !validSnapshotID(parent) || hex.EncodeToString(hash[:]) != digest || recordChecksum(id, parent, expires, raw) != checksum {
		return savedRequest{}, &problem{Code: "decision.snapshot_corrupt", ID: id}
	}
	until := time.Unix(0, expires).UTC()
	if !now.Before(until) {
		return savedRequest{}, &problem{Code: "decision.snapshot_expired", ID: id}
	}
	req, invalid := decode(raw)
	if invalid != nil {
		return savedRequest{}, &problem{Code: "decision.snapshot_corrupt", ID: id}
	}
	return savedRequest{req, until, parent}, nil
}

func (r *diskRepository) load(ctx context.Context, id string) (savedRequest, *problem) {
	if _, err := os.Stat(r.path); errors.Is(err, os.ErrNotExist) {
		return savedRequest{}, &problem{Code: "decision.snapshot_not_found", ID: id}
	}
	db, invalid := r.open(ctx, false)
	if invalid != nil {
		return savedRequest{}, invalid
	}
	defer db.Close()
	return readDisk(ctx, db, id, r.now())
}

func (r *diskRepository) save(ctx context.Context, req request, parent string) (string, time.Time, *problem) {
	raw, err := json.Marshal(req)
	if err != nil {
		return "", time.Time{}, &problem{Code: "decision.snapshot_encode_failed"}
	}
	if len(raw) > 256*1024 {
		return "", time.Time{}, &problem{Code: "decision.snapshot_input_limit"}
	}
	db, invalid := r.open(ctx, true)
	if invalid != nil {
		return "", time.Time{}, invalid
	}
	defer db.Close()
	conn, err := db.Conn(ctx)
	if err != nil {
		return "", time.Time{}, &problem{Code: "decision.store_write_failed"}
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return "", time.Time{}, &problem{Code: "decision.store_write_failed"}
	}
	defer func() { _, _ = conn.ExecContext(context.Background(), "ROLLBACK") }()
	id, expires, invalid := r.insert(ctx, conn, raw, parent)
	if invalid != nil {
		return "", time.Time{}, invalid
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return "", time.Time{}, &problem{Code: "decision.store_write_failed"}
	}
	return id, expires, nil
}

func (r *diskRepository) insert(ctx context.Context, conn *sql.Conn, raw []byte, parent string) (string, time.Time, *problem) {
	now := r.now().UTC()
	if parent != "" {
		if _, invalid := readDisk(ctx, conn, parent, now); invalid != nil {
			return "", time.Time{}, invalid
		}
	}
	if _, err := conn.ExecContext(ctx, "DELETE FROM decision_snapshots WHERE expires<=?", now.UnixNano()); err != nil {
		return "", time.Time{}, &problem{Code: "decision.store_write_failed"}
	}
	digest := sha256.Sum256(raw)
	hexdigest := hex.EncodeToString(digest[:])
	var existing string
	err := conn.QueryRowContext(ctx, "SELECT id FROM decision_snapshots WHERE parent=? AND digest=?", parent, hexdigest).Scan(&existing)
	if err == nil {
		row, invalid := readDisk(ctx, conn, existing, now)
		return existing, row.Expires, invalid
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", time.Time{}, &problem{Code: "decision.store_read_failed"}
	}
	var count int
	if err := conn.QueryRowContext(ctx, "SELECT count(*) FROM decision_snapshots").Scan(&count); err != nil {
		return "", time.Time{}, &problem{Code: "decision.store_read_failed"}
	}
	if count >= diskCapacity {
		return "", time.Time{}, &problem{Code: "decision.snapshot_capacity"}
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", time.Time{}, &problem{Code: "decision.snapshot_id_failed"}
	}
	id, expires := hex.EncodeToString(nonce[:]), now.Add(diskTTL)
	_, err = conn.ExecContext(ctx, "INSERT INTO decision_snapshots VALUES(?,?,?,?,?,?)", id, parent, expires.UnixNano(), raw, hexdigest, recordChecksum(id, parent, expires.UnixNano(), raw))
	if err != nil {
		return "", time.Time{}, &problem{Code: "decision.store_write_failed"}
	}
	return id, expires, nil
}
