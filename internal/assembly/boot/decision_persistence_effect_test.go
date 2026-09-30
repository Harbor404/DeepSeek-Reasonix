package boot

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
)

func runDecisionRestartProbe(t *testing.T, probe *capabilityProbeProvider) {
	t.Helper()
	ctrl, err := Build(context.Background(), Options{Sink: event.Discard})
	if err != nil {
		t.Fatal(err)
	}
	defer ctrl.Close()
	if err := ctrl.Run(context.Background(), "compare saved evidence versions"); err != nil {
		t.Fatal(err)
	}
	if len(probe.reqs) != len(probe.calls)+1 {
		t.Fatal("unexpected inference count")
	}
	for _, req := range probe.reqs {
		if requestHasTool(req, "decision_update") || !reflect.DeepEqual(req.Tools, probe.reqs[0].Tools) {
			t.Fatal("unstable provider tool prefix")
		}
	}
}

func revisedPriceCall(id string) string {
	args := map[string]any{
		"snapshot_id": id, "as_of": "2026-09-30T12:00:00Z",
		"evidence": []any{map[string]any{
			"id": "price-v2", "key": "plan.price", "text": "Authored fixture revised price",
			"source":      map[string]any{"uri": "fixture://price", "version": "v2"},
			"observed_at": "2026-09-30T12:00:00Z", "supersedes": []string{"price"},
			"fact": map[string]any{"metric": "cost", "value": "150", "unit": "USD/month"},
		}},
		"metric_updates": []any{map[string]any{"option_id": "a", "metric": "cost", "evidence_ids": []string{"price-v2"}}},
	}
	raw, _ := json.Marshal(map[string]any{"action": "call", "capability_id": "tool:decision_update", "arguments": args})
	return string(raw)
}

func TestEffectDecisionPersistenceAndVersionBindingAcrossBuilds(t *testing.T) {
	isolateConfigHome(t)
	dir := robustTempDir(t)
	t.Chdir(dir)
	const kind = "boot-decision-restart-effect"
	probe := &capabilityProbeProvider{calls: []string{decisionCall(false)}}
	provider.Register(kind, func(provider.Config) (provider.Provider, error) { return probe, nil })
	writeFile(t, dir, "reasonix.toml", `default_model = "test-model"
[agent]
system_prompt = "BASE"
[[providers]]
name = "test-model"
kind = "`+kind+`"
model = "x"
`)
	approveWorkspace(t, dir)
	runDecisionRestartProbe(t, probe)
	id := lastDecisionSnapshot(probe.reqs[len(probe.reqs)-1])
	candidate := `{"options":{"a":"feasible"},"frontier":["a"],"comparison_state":"known_feasible_options_only","recommended_option":null}`
	probe = &capabilityProbeProvider{calls: []string{answerCheckCall(id, candidate), revisedPriceCall(id), ""}}
	probe.callArgs = func(i int, req provider.Request) string {
		if i < 2 {
			return probe.calls[i]
		}
		return answerCheckCall(lastDecisionSnapshot(req), candidate)
	}
	runDecisionRestartProbe(t, probe)
	results := probe.toolResults()
	if len(results) != 3 {
		t.Fatalf("missing version results: %v", results)
	}
	var accepted, rejected struct {
		Status string `json:"status"`
		Basis  struct {
			SnapshotID string `json:"snapshot_id"`
			Parent     string `json:"parent_snapshot_id"`
		} `json:"basis"`
	}
	if err := json.Unmarshal([]byte(results[0]), &accepted); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(results[2]), &rejected); err != nil {
		t.Fatal(err)
	}
	var updated struct {
		SnapshotID string `json:"snapshot_id"`
		Parent     string `json:"parent_snapshot_id"`
	}
	if err := json.NewDecoder(strings.NewReader(results[1])).Decode(&updated); err != nil {
		t.Fatal(err)
	}
	if accepted.Status != "answer_accepted" || accepted.Basis.SnapshotID != id || updated.Parent != id || updated.SnapshotID == id || rejected.Status != "computed_fallback" || rejected.Basis.SnapshotID != updated.SnapshotID || rejected.Basis.Parent != id {
		t.Fatalf("version binding failed: %v", results)
	}
	probe = &capabilityProbeProvider{calls: []string{answerCheckCall(id, candidate), answerCheckCall(updated.SnapshotID, candidate)}}
	runDecisionRestartProbe(t, probe)
	if !reflect.DeepEqual(probe.toolResults(), []string{results[0], results[2]}) {
		t.Fatal("restart changed saved versions")
	}
}
