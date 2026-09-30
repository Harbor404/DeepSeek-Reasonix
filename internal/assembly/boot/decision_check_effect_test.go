package boot

import (
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"testing"

	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
)

func answerCheckCall(id, candidate string) string {
	raw, _ := json.Marshal(map[string]any{"action": "call", "capability_id": "tool:decision_check", "arguments": map[string]any{"snapshot_id": id, "candidate_json": candidate}})
	return string(raw)
}

func lastDecisionSnapshot(req provider.Request) string {
	for _, message := range slices.Backward(req.Messages) {
		if message.Role != provider.RoleTool {
			continue
		}
		var result struct {
			SnapshotID string `json:"snapshot_id"`
		}
		if json.NewDecoder(strings.NewReader(message.Content)).Decode(&result) == nil && result.SnapshotID != "" {
			return result.SnapshotID
		}
	}
	return "00000000000000000000000000000000"
}

func TestEffectDecisionAnswerFallbackReachesProvider(t *testing.T) {
	valid := `{"options":{"a":"feasible"},"frontier":["a"],"comparison_state":"known_feasible_options_only","recommended_option":null}`
	wrong := `{"options":{"a":"infeasible"},"frontier":[],"comparison_state":"no_feasible_options","recommended_option":"a"}`
	probe := &capabilityProbeProvider{calls: []string{
		`{"action":"inspect","capability_id":"tool:decision_check"}`, decisionCall(false), "", "", "",
	}}
	probe.callArgs = func(i int, req provider.Request) string {
		if i < 2 {
			return probe.calls[i]
		}
		return answerCheckCall(lastDecisionSnapshot(req), []string{valid, wrong, `{"options":`}[i-2])
	}
	runProbeWith(t, "boot-decision-check-effect", probe, event.Discard)
	results := probe.toolResults()
	if len(results) != 5 {
		t.Fatalf("missing results: %v", results)
	}
	var inspected struct {
		ID          string          `json:"id"`
		InputSchema json.RawMessage `json:"input_schema"`
	}
	if err := json.Unmarshal([]byte(results[0]), &inspected); err != nil {
		t.Fatal(err)
	}
	if inspected.ID != "tool:decision_check" || len(inspected.InputSchema) == 0 {
		t.Fatal("checker schema not discoverable")
	}
	for i, want := range []string{"answer_accepted", "computed_fallback", "computed_fallback"} {
		var got struct {
			Status          string `json:"status"`
			ValidationScope string `json:"validation_scope"`
			Reasons         []struct {
				Code string `json:"code"`
			} `json:"reasons"`
			Decision struct {
				Options     map[string]string `json:"options"`
				Frontier    []string          `json:"frontier"`
				Recommended json.RawMessage   `json:"recommended_option"`
			} `json:"decision"`
			Basis struct {
				Provenance string            `json:"provenance"`
				Evidence   []json.RawMessage `json:"evidence"`
			} `json:"basis"`
		}
		if err := json.Unmarshal([]byte(results[i+2]), &got); err != nil {
			t.Fatal(err)
		}
		if got.Status != want || got.Decision.Options["a"] != "feasible" || !reflect.DeepEqual(got.Decision.Frontier, []string{"a"}) || string(got.Decision.Recommended) != "null" {
			t.Fatalf("canonical response lost: %s", results[i+2])
		}
		if got.ValidationScope != "structured_consistency_with_host_snapshot_only" || got.Basis.Provenance != "caller_supplied_not_independently_verified" || len(got.Basis.Evidence) != 2 {
			t.Fatal("scope or sources lost at provider boundary")
		}
		if i > 0 && len(got.Reasons) == 0 {
			t.Fatal("rejection cause lost")
		}
		if i == 2 && got.Reasons[0].Code != "answer.json_invalid" {
			t.Fatal("malformed candidate did not reach native fallback")
		}
	}
	if len(probe.reqs) != 6 {
		t.Fatal("unexpected extra inference")
	}
	for _, req := range probe.reqs {
		if requestHasTool(req, "decision_check") || !reflect.DeepEqual(req.Tools, probe.reqs[0].Tools) {
			t.Fatal("provider schema surface changed")
		}
	}
}
