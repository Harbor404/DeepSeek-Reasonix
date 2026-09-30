package boot

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"reasonix/internal/contract/event"
	"reasonix/internal/contract/provider"
)

func TestEffectDecisionReportFallbackReachesProvider(t *testing.T) {
	probe := &capabilityProbeProvider{calls: []string{
		`{"action":"inspect","capability_id":"tool:decision_deliver"}`, decisionCall(false), "",
	}}
	probe.callArgs = func(i int, req provider.Request) string {
		if i < 2 {
			return probe.calls[i]
		}
		raw, _ := json.Marshal(map[string]any{"action": "call", "capability_id": "tool:decision_deliver", "arguments": map[string]any{"snapshot_id": lastDecisionSnapshot(req), "candidate_json": "BUY B WITHOUT CHECKING"}})
		return string(raw)
	}
	runProbeWith(t, "boot-decision-report-effect", probe, event.Discard)
	results := probe.toolResults()
	if len(results) != 3 {
		t.Fatalf("missing results: %v", results)
	}
	var inspected struct {
		ID     string          `json:"id"`
		Schema json.RawMessage `json:"input_schema"`
	}
	if err := json.Unmarshal([]byte(results[0]), &inspected); err != nil {
		t.Fatal(err)
	}
	if inspected.ID != "tool:decision_deliver" || len(inspected.Schema) == 0 {
		t.Fatal("report capability not discoverable")
	}
	var got struct {
		Status     string `json:"status"`
		SnapshotID string `json:"snapshot_id"`
		Decision   struct {
			Options map[string]string `json:"options"`
		} `json:"decision"`
		Report struct {
			MediaType string `json:"media_type"`
			Text      string `json:"text"`
		} `json:"report"`
	}
	if err := json.Unmarshal([]byte(results[2]), &got); err != nil {
		t.Fatal(err)
	}
	if got.Status != "computed_fallback" || got.Decision.Options["a"] != "feasible" || got.Report.MediaType != "text/plain" || !strings.Contains(got.Report.Text, got.SnapshotID) || !strings.Contains(got.Report.Text, "100") || strings.Contains(got.Report.Text, "BUY B") {
		t.Fatal("checked report lost or candidate leaked")
	}
	if len(probe.reqs) != 4 {
		t.Fatal("unexpected model calls")
	}
	for _, req := range probe.reqs {
		if requestHasTool(req, "decision_deliver") || !reflect.DeepEqual(req.Tools, probe.reqs[0].Tools) {
			t.Fatal("provider prefix changed")
		}
	}
}
