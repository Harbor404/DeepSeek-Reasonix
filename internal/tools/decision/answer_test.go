package decision

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func checked(t *testing.T, req request, candidate string) delivery {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"request": req, "candidate_json": candidate})
	text, err := NewCheck().Execute(context.Background(), raw)
	if err != nil {
		t.Fatal(err)
	}
	var out delivery
	if err := json.Unmarshal([]byte(text), &out); err != nil {
		t.Fatal(err)
	}
	if out.Status != "answer_accepted" && out.Status != "computed_fallback" {
		t.Fatalf("unexpected delivery: %s", text)
	}
	return out
}

func TestAnswerRejectsAdversarialCandidatesAndKeepsBasis(t *testing.T) {
	req := fixture()
	expected := canonicalAnswer(compute(req))
	valid, _ := json.Marshal(expected)
	cases := []struct{ name, candidate, code string }{
		{"wrong status", strings.Replace(string(valid), `"a":"feasible"`, `"a":"unknown"`, 1), "answer.option_status_mismatch"},
		{"missing option", strings.Replace(string(valid), `"a":"feasible",`, "", 1), "answer.option_missing"},
		{"extra option", strings.Replace(string(valid), `"a":"feasible"`, `"a":"feasible","invented":"feasible"`, 1), "answer.option_unknown"},
		{"wrong frontier", strings.Replace(string(valid), `"frontier":["a"]`, `"frontier":["b"]`, 1), "answer.frontier_mismatch"},
		{"wrong scope", strings.Replace(string(valid), "known_feasible_options_only", "no_feasible_options", 1), "answer.comparison_state_mismatch"},
		{"invented winner", strings.Replace(string(valid), `"recommended_option":null`, `"recommended_option":"a"`, 1), "answer.recommendation_forbidden"},
		{"duplicate field", strings.Replace(string(valid), `"recommended_option":null`, `"recommended_option":"b","recommended_option":null`, 1), "answer.duplicate_field"},
		{"extra prose", strings.Replace(string(valid), `"recommended_option":null`, `"recommended_option":null,"summary":"buy B"`, 1), "answer.schema_invalid"},
		{"null array", strings.Replace(string(valid), `"frontier":["a"]`, `"frontier":null`, 1), "answer.required_field_missing"},
		{"truncated", string(valid)[:len(valid)-1], "answer.json_invalid"},
		{"large", strings.Repeat("x", 65537), "answer.input_limit"},
		{"trailing", string(valid) + `{}`, "answer.json_invalid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := checked(t, req, tc.candidate)
			if got.Status != "computed_fallback" || len(got.Reasons) == 0 || got.Reasons[0].Code != tc.code {
				t.Fatalf("wrong rejection: %+v", got)
			}
			if !reflect.DeepEqual(got.Decision, expected) || !reflect.DeepEqual(got.Basis, compute(req)) {
				t.Fatal("candidate corrupted fallback or basis")
			}
			if got.ValidationScope != "structured_consistency_with_supplied_request_only" {
				t.Fatal("validation scope lost")
			}
		})
	}
	if got := checked(t, req, string(valid)); got.Status != "answer_accepted" || len(got.Reasons) != 0 {
		t.Fatalf("valid answer rejected: %+v", got)
	}
}

func TestAnswerCannotTurnUnknownEvidenceIntoFeasible(t *testing.T) {
	req := fixture()
	req.Evidence[0].ValidUntil = req.AsOf
	wrong := canonicalAnswer(compute(fixture()))
	candidate, _ := json.Marshal(wrong)
	got := checked(t, req, string(candidate))
	if got.Status != "computed_fallback" || got.Decision.Options["a"] != "unknown" || !reflect.DeepEqual(got.Decision.Frontier, []string{"b"}) {
		t.Fatal("expired evidence promoted to a valid option")
	}
	if got.Basis.Options[0].Checks[0].ValueEvidence.Issues[0].Code != "evidence.expired" {
		t.Fatal("expiry cause lost")
	}
}

func TestAnswerFrontierOrderAndDuplicates(t *testing.T) {
	req := fixture()
	req.Evidence[1].Fact.Value = "80"
	expected := canonicalAnswer(compute(req))
	expected.Frontier = []string{"b", "a"}
	candidate, _ := json.Marshal(expected)
	if got := checked(t, req, string(candidate)); got.Status != "answer_accepted" {
		t.Fatal("equivalent frontier order rejected")
	}
	expected.Frontier = []string{"a", "a"}
	candidate, _ = json.Marshal(expected)
	if got := checked(t, req, string(candidate)); got.Status != "computed_fallback" {
		t.Fatal("duplicate frontier accepted")
	}
}

func TestCheckInputFailuresDoNotInventFallback(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{"request": fixture(), "candidate_json": nil})
	for _, args := range [][]byte{raw, []byte(`{"request":{},"candidate_json":"{}"}`)} {
		text, err := NewCheck().Execute(context.Background(), args)
		if err != nil {
			t.Fatal(err)
		}
		var invalid struct {
			Status string  `json:"status"`
			Error  problem `json:"error"`
		}
		if err := json.Unmarshal([]byte(text), &invalid); err != nil {
			t.Fatal(err)
		}
		if invalid.Status != "invalid" || invalid.Error.Code == "" {
			t.Fatal("invalid input received a computed fallback")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewCheck().Execute(ctx, raw); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestDecisionRuntimeSchemaStringEnums(t *testing.T) {
	for _, schema := range []json.RawMessage{New().Schema(), NewCheck().Schema()} {
		var node any
		if err := json.Unmarshal(schema, &node); err != nil {
			t.Fatal(err)
		}
		assertStringEnums(t, node)
	}
}

func assertStringEnums(t *testing.T, node any) {
	t.Helper()
	switch node := node.(type) {
	case map[string]any:
		if values, ok := node["enum"].([]any); ok {
			if node["type"] != "string" {
				t.Fatal("enum requires explicit string type")
			}
			for _, value := range values {
				if _, ok := value.(string); !ok {
					t.Fatal("non-string enum member")
				}
			}
		}
		for _, child := range node {
			assertStringEnums(t, child)
		}
	case []any:
		for _, child := range node {
			assertStringEnums(t, child)
		}
	}
}
