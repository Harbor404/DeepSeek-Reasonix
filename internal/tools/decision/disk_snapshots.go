package decision

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"reasonix/internal/state/decisionstore"
)

type diskRepository struct {
	store *decisionstore.Store
}

func newDiskRepository(path string, now func() time.Time) *diskRepository {
	return &diskRepository{store: &decisionstore.Store{
		Path: path,
		Now:  now,
		Valid: func(raw []byte) bool {
			_, invalid := decode(raw)
			return invalid == nil
		},
	}}
}

func (r *diskRepository) persistent() bool { return true }

func storeProblem(err error, id string) *problem {
	codes := []struct {
		err  error
		code string
	}{
		{decisionstore.ErrNotFound, "decision.snapshot_not_found"},
		{decisionstore.ErrExpired, "decision.snapshot_expired"},
		{decisionstore.ErrCorrupt, "decision.snapshot_corrupt"},
		{decisionstore.ErrRead, "decision.store_read_failed"},
		{decisionstore.ErrCapacity, "decision.snapshot_capacity"},
		{decisionstore.ErrOpen, "decision.store_open_failed"},
		{decisionstore.ErrVersionUnsupported, "decision.store_version_unsupported"},
		{decisionstore.ErrID, "decision.snapshot_id_failed"},
	}
	for _, c := range codes {
		if errors.Is(err, c.err) {
			out := &problem{Code: c.code}
			if errors.Is(err, decisionstore.ErrNotFound) || errors.Is(err, decisionstore.ErrExpired) || errors.Is(err, decisionstore.ErrCorrupt) || errors.Is(err, decisionstore.ErrRead) {
				out.ID = id
			}
			return out
		}
	}
	return &problem{Code: "decision.store_write_failed"}
}

func (r *diskRepository) load(ctx context.Context, id string) (savedRequest, *problem) {
	rec, err := r.store.Load(ctx, id)
	if err != nil {
		return savedRequest{}, storeProblem(err, id)
	}
	req, invalid := decode(rec.Payload)
	if invalid != nil {
		return savedRequest{}, &problem{Code: "decision.snapshot_corrupt", ID: id}
	}
	return savedRequest{req, rec.Expires, rec.Parent}, nil
}

func (r *diskRepository) save(ctx context.Context, req request, parent string) (string, time.Time, *problem) {
	raw, err := json.Marshal(req)
	if err != nil {
		return "", time.Time{}, &problem{Code: "decision.snapshot_encode_failed"}
	}
	if len(raw) > 256*1024 {
		return "", time.Time{}, &problem{Code: "decision.snapshot_input_limit"}
	}
	id, expires, err := r.store.Save(ctx, raw, parent)
	if err != nil {
		return "", time.Time{}, storeProblem(err, parent)
	}
	return id, expires, nil
}
