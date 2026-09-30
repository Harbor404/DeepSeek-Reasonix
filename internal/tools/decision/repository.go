package decision

import (
	"context"
	"time"
)

type savedRequest struct {
	Request request
	Expires time.Time
	Parent  string
}

type snapshotRepository interface {
	save(context.Context, request, string) (string, time.Time, *problem)
	load(context.Context, string) (savedRequest, *problem)
	persistent() bool
}

type memoryRepository struct{ store *snapshots }

func (r memoryRepository) persistent() bool { return false }
func (r memoryRepository) save(_ context.Context, req request, parent string) (string, time.Time, *problem) {
	return r.store.saveVersion(req, parent)
}
func (r memoryRepository) load(_ context.Context, id string) (savedRequest, *problem) {
	return r.store.loadRecord(id)
}
