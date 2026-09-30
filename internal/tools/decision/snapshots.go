package decision

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"
)

const snapshotTTL = 30 * time.Minute
const snapshotCapacity = 64

type snapshot struct {
	request []byte
	expires time.Time
	parent  string
}

type snapshots struct {
	mu   sync.Mutex
	rows map[string]snapshot
	now  func() time.Time
}

func newSnapshots() *snapshots {
	return &snapshots{rows: map[string]snapshot{}, now: time.Now}
}

func (s *snapshots) save(req request) (string, time.Time, *problem) {
	return s.saveVersion(req, "")
}

func (s *snapshots) saveVersion(req request, parent string) (string, time.Time, *problem) {
	raw, err := json.Marshal(req)
	if err != nil {
		return "", time.Time{}, &problem{Code: "decision.snapshot_encode_failed"}
	}
	if len(raw) > 256*1024 {
		return "", time.Time{}, &problem{Code: "decision.snapshot_input_limit"}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	if parent != "" {
		row, exists := s.rows[parent]
		if !exists {
			return "", time.Time{}, &problem{Code: "decision.snapshot_not_found", ID: parent}
		}
		if !now.Before(row.expires) {
			return "", time.Time{}, &problem{Code: "decision.snapshot_expired", ID: parent}
		}
	}
	for id, row := range s.rows {
		if !now.Before(row.expires) {
			delete(s.rows, id)
		}
	}
	for id, row := range s.rows {
		if row.parent == parent && bytes.Equal(raw, row.request) {
			return id, row.expires, nil
		}
	}
	if len(s.rows) >= snapshotCapacity {
		return "", time.Time{}, &problem{Code: "decision.snapshot_capacity"}
	}
	var nonce [16]byte
	for {
		if _, err := rand.Read(nonce[:]); err != nil {
			return "", time.Time{}, &problem{Code: "decision.snapshot_id_failed"}
		}
		id := hex.EncodeToString(nonce[:])
		if _, exists := s.rows[id]; exists {
			continue
		}
		expires := now.Add(snapshotTTL)
		s.rows[id] = snapshot{request: raw, expires: expires, parent: parent}
		return id, expires, nil
	}
}

func (s *snapshots) load(id string) (request, time.Time, *problem) {
	row, invalid := s.loadRecord(id)
	return row.Request, row.Expires, invalid
}

func (s *snapshots) loadRecord(id string) (savedRequest, *problem) {
	s.mu.Lock()
	defer s.mu.Unlock()
	row, exists := s.rows[id]
	if !exists {
		return savedRequest{}, &problem{Code: "decision.snapshot_not_found", ID: id}
	}
	if !s.now().Before(row.expires) {
		return savedRequest{}, &problem{Code: "decision.snapshot_expired", ID: id}
	}
	var req request
	if err := json.Unmarshal(row.request, &req); err != nil {
		return savedRequest{}, &problem{Code: "decision.snapshot_decode_failed", ID: id}
	}
	return savedRequest{Request: req, Expires: row.expires, Parent: row.parent}, nil
}
