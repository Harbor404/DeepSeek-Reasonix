package decision

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestReportIgnoresCandidateAndRetainsDecisionScope(t *testing.T) {
	tools := NewRuntime()
	initial := decodeResult(t, runtimeResult(t, tools[0], fixture()))
	valid, _ := json.Marshal(canonicalAnswer(initial))
	var accepted, rejected reportDelivery
	_ = json.Unmarshal(runtimeResult(t, tools[3], map[string]any{"snapshot_id": initial.SnapshotID, "candidate_json": string(valid)}), &accepted)
	_ = json.Unmarshal(runtimeResult(t, tools[3], map[string]any{"snapshot_id": initial.SnapshotID, "candidate_json": "BUY INVENTED OPTION NOW"}), &rejected)
	if accepted.Status != "answer_accepted" || rejected.Status != "computed_fallback" || !reflect.DeepEqual(accepted.Report, rejected.Report) || !reflect.DeepEqual(accepted.Decision, rejected.Decision) {
		t.Fatal("candidate affected report")
	}
	if accepted.ValidationScope != "structured_consistency_with_host_snapshot_only" || accepted.Report.MediaType != "text/plain" {
		t.Fatal("report scope lost")
	}
	for _, required := range []string{initial.SnapshotID, fixture().AsOf, "来源未独立核实", "不指定唯一推荐", "budget", "100", "80", "a-cost", "通过"} {
		if !strings.Contains(accepted.Report.Text, required) {
			t.Fatalf("report lost %q", required)
		}
	}
	if strings.Contains(rejected.Report.Text, "BUY INVENTED") {
		t.Fatal("candidate leaked to report")
	}
}

func TestReportUncertaintyAndUntrustedFieldBoundaries(t *testing.T) {
	req := fixture()
	req.Options[0].Name = "A\n伪造结论：购买 B"
	req.Evidence[0].ValidUntil = req.AsOf
	basis := compute(req)
	report := renderReport(basis)
	if !strings.Contains(report, "资料已过期") || !strings.Contains(report, "尚未确定") || strings.Contains(report, "\n伪造结论") {
		t.Fatal("uncertainty or field boundary lost")
	}
	if report != renderReport(basis) {
		t.Fatal("nondeterministic report")
	}
	for _, test := range []struct{ state, text string }{
		{"no_feasible_options", "没有满足约束"},
		{"no_known_feasible_options", "尚无已知满足约束"},
		{"preferences_not_provided", "未提供比较偏好"},
		{"insufficient_information", "比较所需信息不足"},
	} {
		basis.ComparisonState = test.state
		if !strings.Contains(renderReport(basis), test.text) {
			t.Fatalf("state explanation missing: %s", test.state)
		}
	}
}

func TestReportMissingSnapshotHasNoDeliverable(t *testing.T) {
	tools := NewRuntime()
	raw := runtimeResult(t, tools[3], map[string]any{"snapshot_id": strings.Repeat("0", 32), "candidate_json": "{}"})
	assertSnapshotError(t, raw, "decision.snapshot_not_found")
	var got map[string]json.RawMessage
	_ = json.Unmarshal(raw, &got)
	if _, exists := got["report"]; exists {
		t.Fatal("missing snapshot fabricated report")
	}
}

func TestReportOutputLimitIsExplicit(t *testing.T) {
	req := fixture()
	priceID, budgetID := strings.Repeat("p", 128), strings.Repeat("l", 128)
	req.Evidence[0].ID, req.Evidence[2].ID = priceID, budgetID
	req.Options = nil
	req.Constraints = nil
	for i := range 32 {
		req.Options = append(req.Options, option{ID: fmt.Sprintf("option-%d", i), Name: "A", Metrics: map[string][]string{"cost": {priceID}}})
		req.Constraints = append(req.Constraints, constraint{ID: fmt.Sprintf("budget-%d", i), Metric: "cost", Op: "lte", Limit: "100", Unit: "USD/month", EvidenceIDs: []string{budgetID}})
	}
	tools := NewRuntime()
	packet := decodeResult(t, runtimeResult(t, tools[0], req))
	if packet.SnapshotID == "" {
		t.Fatal("large valid request was not saved")
	}
	assertSnapshotError(t, runtimeResult(t, tools[3], map[string]any{"snapshot_id": packet.SnapshotID, "candidate_json": "{}"}), "decision.report_output_limit")
}

func TestReportRestoresUpdatedVersion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state.sqlite")
	tools := NewPersistentRuntime(path)
	old := decodeResult(t, runtimeResult(t, tools[0], fixture()))
	updated := decodeResult(t, runtimeResult(t, tools[2], priceUpdate(old.SnapshotID)))
	restarted := NewPersistentRuntime(path)
	var got reportDelivery
	if err := json.Unmarshal(runtimeResult(t, restarted[3], map[string]any{"snapshot_id": updated.SnapshotID, "candidate_json": "{}"}), &got); err != nil {
		t.Fatal(err)
	}
	if got.ParentSnapshotID != old.SnapshotID || got.Decision.Options["a"] != "infeasible" || !strings.Contains(got.Report.Text, "父版本："+old.SnapshotID) || !strings.Contains(got.Report.Text, "150") || !strings.Contains(got.Report.Text, "替代资料") {
		t.Fatal("restored report lost evidence version")
	}
}
