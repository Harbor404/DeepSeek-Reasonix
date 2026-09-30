package decision

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func decodeResult(t *testing.T, raw []byte) result {
	t.Helper()
	var out result
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func priceUpdate(id string) updateInput {
	revised := fixture().Evidence[0]
	revised.ID = "revised-price"
	revised.Fact.Value = "150"
	revised.Source.Version = "v2"
	revised.ObservedAt = fixture().AsOf
	revised.Supersedes = []string{fixture().Evidence[0].ID}
	return updateInput{SnapshotID: id, AsOf: fixture().AsOf, Evidence: []evidence{revised}, Metrics: []metricUpdate{{OptionID: "a", Metric: "cost", EvidenceIDs: []string{revised.ID}}}}
}

func TestPersistentRestartAndEvidenceVersions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "snapshots.sqlite")
	first := NewPersistentRuntime(path)
	original := decodeResult(t, runtimeResult(t, first[0], fixture()))
	reopened := NewPersistentRuntime(path)
	retry := decodeResult(t, runtimeResult(t, reopened[0], fixture()))
	if retry.SnapshotID != original.SnapshotID || retry.SnapshotExpiresAt != original.SnapshotExpiresAt {
		t.Fatal("retry changed saved identity")
	}
	updated := decodeResult(t, runtimeResult(t, reopened[2], priceUpdate(original.SnapshotID)))
	if updated.SnapshotID == original.SnapshotID || updated.ParentSnapshotID != original.SnapshotID || updated.Options[0].Status != "infeasible" {
		t.Fatalf("bad update: %+v", updated)
	}
	duplicate := decodeResult(t, runtimeResult(t, reopened[2], priceUpdate(original.SnapshotID)))
	if duplicate.SnapshotID != updated.SnapshotID {
		t.Fatal("update retry duplicated version")
	}
	answer, _ := json.Marshal(canonicalAnswer(original))
	third := NewPersistentRuntime(path)
	for _, test := range []struct{ id, status string }{{original.SnapshotID, "answer_accepted"}, {updated.SnapshotID, "computed_fallback"}} {
		var got delivery
		_ = json.Unmarshal(runtimeResult(t, third[1], map[string]any{"snapshot_id": test.id, "candidate_json": string(answer)}), &got)
		if got.Status != test.status {
			t.Fatalf("version binding lost: %+v", got)
		}
	}
	repo := newDiskRepository(path, time.Now)
	old, invalid := repo.load(context.Background(), original.SnapshotID)
	if invalid != nil || !reflect.DeepEqual(old.Request, fixture()) {
		t.Fatal("old request changed")
	}
	newer, invalid := repo.load(context.Background(), updated.SnapshotID)
	if invalid != nil || !reflect.DeepEqual(newer.Request.Constraints, old.Request.Constraints) || !reflect.DeepEqual(newer.Request.Preferences, old.Request.Preferences) {
		t.Fatal("user constraints changed")
	}
	if first[0].ReadOnly() || first[2].ReadOnly() || !first[1].ReadOnly() {
		t.Fatal("incorrect mutation classification")
	}
}

func TestPersistentFailureAndIsolation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing.sqlite")
	tools := NewPersistentRuntime(path)
	args := map[string]any{"snapshot_id": "00000000000000000000000000000000", "candidate_json": "{}"}
	assertSnapshotError(t, runtimeResult(t, tools[1], args), "decision.snapshot_not_found")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("read created store")
	}
	if err := os.WriteFile(path, []byte("invalid database"), 0600); err != nil {
		t.Fatal(err)
	}
	assertSnapshotError(t, runtimeResult(t, tools[0], fixture()), "decision.store_open_failed")
}

func TestEvidenceUpdateRejectsImplicitReplacement(t *testing.T) {
	tools := NewRuntime()
	original := decodeResult(t, runtimeResult(t, tools[0], fixture()))
	for _, test := range []struct {
		name, code string
		alter      func(*updateInput)
	}{
		{"overwrite", "decision.evidence_identity_invalid", func(in *updateInput) { in.Evidence[0].ID = fixture().Evidence[0].ID }},
		{"unknown option", "decision.option_unknown", func(in *updateInput) { in.Metrics[0].OptionID = "unknown" }},
		{"duplicate metric", "decision.metric_update_duplicate", func(in *updateInput) { in.Metrics = append(in.Metrics, in.Metrics[0]) }},
		{"time backwards", "decision.update_time_invalid", func(in *updateInput) { in.AsOf = "2000-01-01T00:00:00Z" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := priceUpdate(original.SnapshotID)
			test.alter(&input)
			assertSnapshotError(t, runtimeResult(t, tools[2], input), test.code)
		})
	}
}

func TestPersistentStoreErrorsKeepDomainCodes(t *testing.T) {
	repo := newDiskRepository(filepath.Join(t.TempDir(), "state.sqlite"), time.Now)
	ctx := context.Background()
	id, _, invalid := repo.save(ctx, fixture(), "")
	if invalid != nil {
		t.Fatal(invalid)
	}
	missing := "00000000000000000000000000000000"
	if _, invalid := repo.load(ctx, missing); invalid == nil || invalid.Code != "decision.snapshot_not_found" || invalid.ID != missing {
		t.Fatalf("missing: %v", invalid)
	}
	if _, _, invalid := repo.save(ctx, fixture(), missing); invalid == nil || invalid.Code != "decision.snapshot_not_found" || invalid.ID != missing {
		t.Fatalf("missing parent: %v", invalid)
	}
	if got, invalid := repo.load(ctx, id); invalid != nil || !reflect.DeepEqual(got.Request, fixture()) {
		t.Fatal("round trip changed request")
	}
}
