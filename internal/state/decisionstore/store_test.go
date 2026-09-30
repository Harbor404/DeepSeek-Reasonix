package decisionstore

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func newStore(t *testing.T, now func() time.Time) *Store {
	t.Helper()
	return &Store{Path: filepath.Join(t.TempDir(), "state.sqlite"), Now: now}
}

func TestCorruptionDetected(t *testing.T) {
	for _, column := range []string{"request", "expires", "parent", "digest", "checksum"} {
		t.Run(column, func(t *testing.T) {
			s := newStore(t, time.Now)
			id, _, err := s.Save(context.Background(), []byte(`{"a":1}`), "")
			if err != nil {
				t.Fatal(err)
			}
			db, err := s.open(context.Background(), true)
			if err != nil {
				t.Fatal(err)
			}
			_, err = db.Exec("UPDATE decision_snapshots SET "+column+"=? WHERE id=?", "corrupt", id)
			_ = db.Close()
			if err != nil {
				t.Fatal(err)
			}
			if _, err = s.Load(context.Background(), id); !errors.Is(err, ErrCorrupt) && !errors.Is(err, ErrRead) {
				t.Fatalf("corruption not rejected: %v", err)
			}
		})
	}
}

func TestConcurrentDedupAndExpiry(t *testing.T) {
	now := time.Now().UTC()
	clock := func() time.Time { return now }
	s := newStore(t, clock)
	payload := []byte(`{"a":1}`)
	id, expires, err := s.Save(context.Background(), payload, "")
	if err != nil || expires.Sub(now) != TTL {
		t.Fatal("incorrect retention")
	}
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			other := &Store{Path: s.Path, Now: clock}
			got, until, err := other.Save(context.Background(), payload, "")
			if err != nil || got != id || !until.Equal(expires) {
				t.Errorf("concurrent retry changed identity: %s %v", got, err)
			}
		})
	}
	wg.Wait()
	now = expires
	if _, err := s.Load(context.Background(), id); !errors.Is(err, ErrExpired) {
		t.Fatal("expired snapshot accepted")
	}
	newID, _, err := s.Save(context.Background(), payload, "")
	if err != nil || newID == id {
		t.Fatal("expired snapshot not replaced")
	}
}

func TestParentLinkAndMissingParent(t *testing.T) {
	s := newStore(t, time.Now)
	ctx := context.Background()
	parent, _, err := s.Save(ctx, []byte(`{"v":1}`), "")
	if err != nil {
		t.Fatal(err)
	}
	child, _, err := s.Save(ctx, []byte(`{"v":1}`), parent)
	if err != nil || child == parent {
		t.Fatalf("same payload under a new parent must be a new version: %v", err)
	}
	if rec, err := s.Load(ctx, child); err != nil || rec.Parent != parent {
		t.Fatalf("parent link lost: %v", err)
	}
	if _, _, err := s.Save(ctx, []byte(`{"v":2}`), "00000000000000000000000000000000"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing parent accepted: %v", err)
	}
}

func TestLoadDoesNotCreateFile(t *testing.T) {
	s := newStore(t, time.Now)
	if _, err := s.Load(context.Background(), "00000000000000000000000000000000"); !errors.Is(err, ErrNotFound) {
		t.Fatal(err)
	}
}

func TestCapacityAndUnsupportedVersion(t *testing.T) {
	s := newStore(t, time.Now)
	ctx := context.Background()
	for i := range Capacity {
		if _, _, err := s.Save(ctx, fmt.Appendf(nil, `{"n":%d}`, i), ""); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := s.Save(ctx, []byte(`{"n":"x"}`), ""); !errors.Is(err, ErrCapacity) {
		t.Fatal("capacity ignored")
	}
	db, err := s.open(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec("PRAGMA user_version=99")
	_ = db.Close()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Save(ctx, []byte(`{"n":"y"}`), ""); !errors.Is(err, ErrVersionUnsupported) {
		t.Fatal("unsupported store overwritten")
	}
}
