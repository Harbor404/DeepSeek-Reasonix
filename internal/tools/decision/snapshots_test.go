package decision

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"reasonix/internal/contract/tool"
)

func runtimeResult(t *testing.T, executor tool.Tool, args any) []byte {
	t.Helper()
	raw, _ := json.Marshal(args)
	text, err := executor.Execute(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	return []byte(text)
}

func TestRuntimeSnapshotBindingAndIsolation(t *testing.T) {
	tools := NewRuntime()
	req := fixture()
	var original result
	if err := json.Unmarshal(runtimeResult(t, tools[0], req), &original); err != nil {
		t.Fatal(err)
	}
	if len(original.SnapshotID) != 32 || original.SnapshotExpiresAt == "" {
		t.Fatal("missing snapshot identity")
	}
	var again result
	_ = json.Unmarshal(runtimeResult(t, tools[0], req), &again)
	if again.SnapshotID != original.SnapshotID || again.SnapshotExpiresAt != original.SnapshotExpiresAt {
		t.Fatal("retry duplicated or renewed snapshot")
	}
	answer, _ := json.Marshal(canonicalAnswer(original))
	args := map[string]any{"snapshot_id": original.SnapshotID, "candidate_json": string(answer)}
	var delivered delivery
	_ = json.Unmarshal(runtimeResult(t, tools[1], args), &delivered)
	if delivered.Status != "answer_accepted" || delivered.ValidationScope != "structured_consistency_with_host_snapshot_only" || delivered.Basis.SnapshotSHA256 != original.SnapshotSHA256 {
		t.Fatalf("not bound: %+v", delivered)
	}
	req.Evidence[0].Fact.Value = "200"
	args["request"] = req
	assertSnapshotError(t, runtimeResult(t, tools[1], args), "decision.schema_invalid")
	delete(args, "request")
	_ = json.Unmarshal(runtimeResult(t, tools[1], args), &delivered)
	if delivered.Decision.Options["a"] != "feasible" {
		t.Fatal("caller changed stored original")
	}
	other := NewRuntime()
	assertSnapshotError(t, runtimeResult(t, other[1], args), "decision.snapshot_not_found")
	args["snapshot_id"] = strings.Repeat("z", 32)
	assertSnapshotError(t, runtimeResult(t, tools[1], args), "decision.snapshot_arguments_invalid")
}

func assertSnapshotError(t *testing.T, raw []byte, code string) {
	t.Helper()
	var got struct {
		Status string  `json:"status"`
		Error  problem `json:"error"`
	}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Status != "invalid" || got.Error.Code != code {
		t.Fatalf("wanted %s, got %s", code, raw)
	}
}

func TestSnapshotExpiryCapacityAndImmutableCopies(t *testing.T) {
	store := newSnapshots()
	now := time.Now()
	store.now = func() time.Time { return now }
	req := fixture()
	first, expires, invalid := store.save(req)
	if invalid != nil {
		t.Fatal(invalid)
	}
	loaded, _, invalid := store.load(first)
	if invalid != nil {
		t.Fatal(invalid)
	}
	loaded.Options[0].Metrics["cost"][0] = "changed"
	loaded.Evidence[0].Fact.Value = "200"
	loaded, _, _ = store.load(first)
	if loaded.Options[0].Metrics["cost"][0] != "a-cost" || loaded.Evidence[0].Fact.Value != "80" {
		t.Fatal("stored snapshot was mutable")
	}
	for i := range snapshotCapacity - 1 {
		req.Options[0].Name = fmt.Sprintf("option-%d", i)
		if _, _, invalid := store.save(req); invalid != nil {
			t.Fatal(invalid)
		}
	}
	req.Options[0].Name = "overflow"
	if _, _, invalid := store.save(req); invalid == nil || invalid.Code != "decision.snapshot_capacity" {
		t.Fatal("capacity not enforced")
	}
	now = expires.Add(-time.Nanosecond)
	if _, _, invalid := store.load(first); invalid != nil {
		t.Fatal("expired early")
	}
	now = expires
	if _, _, invalid := store.load(first); invalid == nil || invalid.Code != "decision.snapshot_expired" {
		t.Fatal("expiry boundary ignored")
	}
	if _, _, invalid := store.save(req); invalid != nil || len(store.rows) != 1 {
		t.Fatal("expired capacity not reclaimed")
	}
}

func TestSnapshotCanonicalByteLimit(t *testing.T) {
	store := newSnapshots()
	req := fixture()
	for i := range 50 {
		e := record(fmt.Sprintf("extra%d", i), fmt.Sprintf("extra%d", i), "1")
		e.Text = strings.Repeat("<", 1024)
		req.Evidence = append(req.Evidence, e)
	}
	if invalid := validate(req); invalid != nil {
		t.Fatal(invalid)
	}
	if _, _, invalid := store.save(req); invalid == nil || invalid.Code != "decision.snapshot_input_limit" || len(store.rows) != 0 {
		t.Fatal("canonical byte bound not enforced")
	}
}

func TestSnapshotConcurrentCalls(t *testing.T) {
	store := newSnapshots()
	var wg sync.WaitGroup
	for i := range 32 {
		wg.Go(func() {
			req := fixture()
			req.Options[0].Name = fmt.Sprintf("concurrent-%d", i)
			id, _, invalid := store.save(req)
			if invalid != nil {
				t.Error(invalid)
				return
			}
			if _, _, invalid := store.load(id); invalid != nil {
				t.Error(invalid)
			}
		})
	}
	wg.Wait()
	if len(store.rows) != 32 {
		t.Fatal("concurrent snapshot lost")
	}
}
