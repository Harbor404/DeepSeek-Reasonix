package serve

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"reasonix/internal/base/testenv"
	"reasonix/internal/contract/config"
	"reasonix/internal/session/control"
	"reasonix/internal/tools/decision"
)

func decisionServer(t *testing.T, reporter control.DecisionReporter) *httptest.Server {
	t.Helper()
	t.Setenv("REASONIX_HOME", testenv.TempDir(t))
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Runner: fakeRunner{}, Sink: bc, DecisionReporter: reporter})
	t.Cleanup(ctrl.Close)
	srv := httptest.NewServer(operatorHandler(New(ctrl, bc, config.ServeConfig{})))
	t.Cleanup(srv.Close)
	return srv
}

func postDecision(t *testing.T, srv *httptest.Server, body string) (int, map[string]any) {
	t.Helper()
	response, err := http.Post(srv.URL+"/decision/deliver", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var out map[string]any
	if err := json.NewDecoder(response.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if response.Header.Get("Cache-Control") != "no-store" {
		t.Fatal("decision response is cacheable")
	}
	return response.StatusCode, out
}

func decisionFixtureSnapshot(t *testing.T, path string) string {
	t.Helper()
	tools := decision.NewPersistentRuntime(path)
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "benchmarks", "decision-engine", "native-request.json"))
	if err != nil {
		t.Fatal(err)
	}
	output, err := tools[0].Execute(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	var saved struct {
		ID string `json:"snapshot_id"`
	}
	if err := json.Unmarshal([]byte(output), &saved); err != nil || saved.ID == "" {
		t.Fatalf("save failed: %s %v", output, err)
	}
	return saved.ID
}

func TestDecisionHTTPRestoresAndCorrectsCandidate(t *testing.T) {
	path := filepath.Join(testenv.TempDir(t), "state.sqlite")
	id := decisionFixtureSnapshot(t, path)
	tools := decision.NewPersistentRuntime(path)
	srv := decisionServer(t, tools[3].(control.DecisionReporter))
	raw, _ := json.Marshal(map[string]any{"snapshot_id": id, "candidate_json": "BUY B"})
	status, out := postDecision(t, srv, string(raw))
	if status != http.StatusOK || out["status"] != "computed_fallback" || out["snapshot_id"] != id {
		t.Fatalf("incorrect delivery: %d %v", status, out)
	}
	report := out["report"].(map[string]any)
	if report["media_type"] != "text/plain" || strings.Contains(report["text"].(string), "BUY B") {
		t.Fatal("candidate leaked into report")
	}
	if out["decision"].(map[string]any)["options"].(map[string]any)["a"] != "feasible" {
		t.Fatal("wrong canonical answer")
	}
}

func TestDecisionHTTPFailureIdentity(t *testing.T) {
	path := filepath.Join(testenv.TempDir(t), "state.sqlite")
	tools := decision.NewPersistentRuntime(path)
	srv := decisionServer(t, tools[3].(control.DecisionReporter))
	for _, test := range []struct {
		body, code string
		status     int
	}{
		{"not JSON", "decision.json_invalid", 400},
		{`{"snapshot_id":"bad","candidate_json":"{}"}`, "decision.snapshot_arguments_invalid", 400},
		{`{"snapshot_id":"bad","snapshot_id":"bad","candidate_json":"{}"}`, "decision.duplicate_field", 400},
		{`{"snapshot_id":"00000000000000000000000000000000","candidate_json":"{}"}`, "decision.snapshot_not_found", 404},
		{strings.Repeat("x", 256*1024+1), "decision.report_input_limit", 413},
	} {
		status, out := postDecision(t, srv, test.body)
		if status != test.status || out["code"] != test.code {
			t.Fatalf("expected %d %s: %d %v", test.status, test.code, status, out)
		}
		if _, exists := out["report"]; exists {
			t.Fatal("failure fabricated report")
		}
	}
	if err := os.WriteFile(path, []byte("invalid database"), 0600); err != nil {
		t.Fatal(err)
	}
	status, out := postDecision(t, srv, `{"snapshot_id":"00000000000000000000000000000000","candidate_json":"{}"}`)
	if status != 503 || out["code"] != "decision.store_open_failed" {
		t.Fatalf("storage failure blamed on input: %d %v", status, out)
	}
	status, out = postDecision(t, decisionServer(t, nil), `{}`)
	if status != 503 || out["code"] != "decision.unavailable" {
		t.Fatal("disabled reporting did not fail closed")
	}
}

func TestDecisionHTTPRequiresLaunchCredential(t *testing.T) {
	bc := NewBroadcaster()
	ctrl := control.New(control.Options{Sink: bc})
	t.Cleanup(ctrl.Close)
	s := New(ctrl, bc, config.ServeConfig{AuthMode: "none"})
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	status, code := postLaunchJSON(t, srv.URL+"/decision/deliver", `{}`, nil)
	if status != 403 || code != codeLaunchTokenRequired {
		t.Fatalf("unauthenticated request reached reporting: %d %s", status, code)
	}
}
