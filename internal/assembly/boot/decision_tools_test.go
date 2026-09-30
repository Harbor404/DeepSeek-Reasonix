package boot

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"reasonix/internal/contract/tool"
)

func TestDecisionRuntimeToolsRespectEnabledList(t *testing.T) {
	for _, enabled := range [][]string{nil, {"decision_check"}, {"decision_update"}, {"decision_deliver"}, {"read_file"}} {
		reg := tool.NewRegistry()
		registerDecisionTools(reg, enabled)
		_, compare := reg.Get("decision_compare")
		_, check := reg.Get("decision_check")
		_, update := reg.Get("decision_update")
		_, deliver := reg.Get("decision_deliver")
		if (decisionReporter(reg) != nil) != deliver {
			t.Fatal("controller reporting bypassed enabled tools")
		}
		if compare != (len(enabled) == 0) || check != (len(enabled) == 0 || enabled[0] == "decision_check") || update != (len(enabled) == 0 || enabled[0] == "decision_update") || deliver != (len(enabled) == 0 || enabled[0] == "decision_deliver") {
			t.Fatalf("enabled %v: compare=%v check=%v", enabled, compare, check)
		}
	}
}

func TestDecisionRuntimeWorkspaceStorageScope(t *testing.T) {
	storage := t.TempDir()
	first := tool.NewRegistry()
	registerPersistentDecisionTools(first, nil, storage, "workspace-a")
	compare, _ := first.Get("decision_compare")
	var call struct {
		Arguments json.RawMessage `json:"arguments"`
	}
	_ = json.Unmarshal([]byte(decisionCall(false)), &call)
	raw, err := compare.Execute(context.Background(), call.Arguments)
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		SnapshotID string `json:"snapshot_id"`
	}
	if err := json.NewDecoder(strings.NewReader(raw)).Decode(&saved); err != nil {
		t.Fatal(err)
	}
	for _, root := range []string{"workspace-a", "workspace-b"} {
		reg := tool.NewRegistry()
		registerPersistentDecisionTools(reg, nil, storage, root)
		checker, _ := reg.Get("decision_check")
		args, _ := json.Marshal(map[string]any{"snapshot_id": saved.SnapshotID, "candidate_json": "{}"})
		raw, err := checker.Execute(context.Background(), args)
		if err != nil {
			t.Fatal(err)
		}
		var out struct {
			Status string `json:"status"`
			Error  struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(raw), &out); err != nil {
			t.Fatal(err)
		}
		if root == "workspace-a" && out.Status != "computed_fallback" || root == "workspace-b" && out.Error.Code != "decision.snapshot_not_found" {
			t.Fatalf("wrong storage scope: %s", raw)
		}
	}
}
