package boot

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"reasonix/internal/contract/event"
)

func decisionCall(expired bool) string {
	e := map[string]any{
		"id": "price", "key": "plan.price", "text": "Authored fixture price",
		"source":      map[string]any{"uri": "fixture://price", "version": "v1"},
		"observed_at": "2026-09-28T12:00:00Z",
		"fact":        map[string]any{"metric": "cost", "value": "80", "unit": "USD/month"},
	}
	if expired {
		e["valid_until"] = "2026-09-29T12:00:00Z"
	}
	args := map[string]any{
		"as_of":       "2026-09-29T12:00:00Z",
		"options":     []any{map[string]any{"id": "a", "name": "A", "metrics": map[string]any{"cost": []string{"price"}}}},
		"constraints": []any{map[string]any{"id": "budget", "metric": "cost", "op": "lte", "limit": "100", "unit": "USD/month", "evidence_ids": []string{"budget"}}},
		"preferences": []any{map[string]any{"metric": "cost", "unit": "USD/month", "direction": "min"}},
		"evidence": []any{e, map[string]any{
			"id": "budget", "key": "user.budget", "text": "Authored fixture budget",
			"source":      map[string]any{"uri": "fixture://user", "version": "v1"},
			"observed_at": "2026-09-28T12:00:00Z",
			"fact":        map[string]any{"metric": "cost", "value": "100", "unit": "USD/month"},
		}},
	}
	raw, _ := json.Marshal(map[string]any{"action": "call", "capability_id": "tool:decision_compare", "arguments": args})
	return string(raw)
}

func TestEffectDecisionCompareReachesProviderWithoutExtraInference(t *testing.T) {
	probe := &capabilityProbeProvider{calls: []string{
		`{"action":"inspect","capability_id":"tool:decision_compare"}`,
		decisionCall(false), decisionCall(true),
	}}
	runProbeWith(t, "boot-decision-compare-effect", probe, event.Discard)
	results := probe.toolResults()
	if len(results) != 3 {
		t.Fatalf("tool results: %v", results)
	}
	var inspected struct {
		ID          string          `json:"id"`
		InputSchema json.RawMessage `json:"input_schema"`
	}
	if err := json.Unmarshal([]byte(results[0]), &inspected); err != nil {
		t.Fatal(err)
	}
	if inspected.ID != "tool:decision_compare" || len(inspected.InputSchema) == 0 {
		t.Fatalf("not discoverable: %s", results[0])
	}
	assertDecisionBoundary(t, results[1], "feasible", "pass", []string{"a"})
	assertDecisionBoundary(t, results[2], "unknown", "unknown", []string{})
	probe.mu.Lock()
	defer probe.mu.Unlock()
	if len(probe.reqs) != 4 {
		t.Fatalf("extra inference requests: %d", len(probe.reqs))
	}
	for _, req := range probe.reqs {
		if requestHasTool(req, "decision_compare") || !requestHasTool(req, "use_capability") {
			t.Fatal("optional tool expanded the provider schema surface")
		}
		if !reflect.DeepEqual(req.Tools, probe.reqs[0].Tools) {
			t.Fatal("provider tool prefix changed")
		}
	}
}

func assertDecisionBoundary(t *testing.T, raw, status, verdict string, frontier []string) {
	t.Helper()
	var got struct {
		Status            string  `json:"status"`
		Advisory          bool    `json:"advisory"`
		Provenance        string  `json:"provenance"`
		RecommendedOption *string `json:"recommended_option"`
		Options           []struct {
			Status string `json:"status"`
			Checks []struct {
				Verdict       string `json:"verdict"`
				ValueEvidence struct {
					State  string `json:"state"`
					Issues []struct {
						Code string `json:"code"`
					} `json:"issues"`
				} `json:"value_evidence"`
				LimitEvidence *struct {
					State string `json:"state"`
				} `json:"limit_evidence"`
			} `json:"constraint_checks"`
		} `json:"options"`
		Frontier []string          `json:"pareto_frontier"`
		Evidence []json.RawMessage `json:"evidence"`
	}
	if err := json.NewDecoder(strings.NewReader(raw)).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.Status != "computed" || len(got.Options) != 1 || got.Options[0].Status != status || len(got.Options[0].Checks) != 1 || got.Options[0].Checks[0].Verdict != verdict {
		t.Fatalf("decision did not reach provider: %s", raw)
	}
	if !got.Advisory || got.RecommendedOption != nil || got.Provenance != "caller_supplied_not_independently_verified" || len(got.Evidence) != 2 || !reflect.DeepEqual(got.Frontier, frontier) {
		t.Fatalf("qualification or citations lost: %s", raw)
	}
	check := got.Options[0].Checks[0]
	if check.LimitEvidence != nil {
		t.Fatal("known constraint source diagnostics unnecessarily duplicated")
	}
	if status == "unknown" && (check.ValueEvidence.State != "unknown" || len(check.ValueEvidence.Issues) != 1 || check.ValueEvidence.Issues[0].Code != "evidence.expired") {
		t.Fatal("expiry cause lost at provider boundary")
	}
}
