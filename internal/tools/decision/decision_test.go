package decision

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func fixture() request {
	return request{
		AsOf:        "2026-09-29T12:00:00Z",
		Options:     []option{{"a", "A", map[string][]string{"cost": {"a-cost"}}}, {"b", "B", map[string][]string{"cost": {"b-cost"}}}},
		Constraints: []constraint{{"budget", "cost", "lte", "100", "USD/month", []string{"budget"}}},
		Preferences: []preference{{"cost", "USD/month", "min"}},
		Evidence:    []evidence{record("a-cost", "a-cost", "80"), record("b-cost", "b-cost", "90"), record("budget", "budget", "100")},
	}
}

func record(id, key, value string) evidence {
	return evidence{ID: id, Key: key, Text: "Authored test fact", Source: source{"fixture://" + id, "v1"}, ObservedAt: "2026-09-28T12:00:00Z", Fact: fact{"cost", value, "USD/month"}}
}

func executeFixture(t *testing.T, req request) result {
	t.Helper()
	b, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	out, err := New().Execute(context.Background(), b)
	if err != nil {
		t.Fatal(err)
	}
	var got result
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	if got.Status != "computed" {
		t.Fatalf("unexpected result: %s", out)
	}
	return got
}

func TestExactConstraintsAndConditionalFrontier(t *testing.T) {
	req := fixture()
	req.Constraints[0].Limit = "0.1"
	req.Evidence[2].Fact.Value = "0.1"
	req.Evidence[0].Fact.Value = "0.1000000000000000000000000001"
	req.Evidence[1].Fact.Value = "0.10"
	got := executeFixture(t, req)
	if got.Options[0].Status != "infeasible" || got.Options[1].Status != "feasible" {
		t.Fatalf("precision lost: %+v", got.Options)
	}
	if !reflect.DeepEqual(got.ParetoFrontier, []string{"b"}) || got.RecommendedOption != nil || !got.Advisory {
		t.Fatalf("unexpected conclusion: %+v", got)
	}
	if !reflect.DeepEqual(got.Constraints, req.Constraints) || !reflect.DeepEqual(got.Preferences, req.Preferences) {
		t.Fatal("caller constraints changed")
	}
	if got.Provenance != "caller_supplied_not_independently_verified" || got.AsOf != req.AsOf {
		t.Fatal(got.Provenance)
	}
	if again := executeFixture(t, req); !reflect.DeepEqual(got, again) {
		t.Fatal("nondeterministic output")
	}
}

func TestUnusableEvidenceDoesNotPassConstraint(t *testing.T) {
	cases := []struct {
		name, code string
		change     func(*request)
	}{
		{"missing", "evidence.missing", func(r *request) { r.Options[0].Metrics["cost"] = []string{"absent"} }},
		{"expired", "evidence.expired", func(r *request) { r.Evidence[0].ValidUntil = r.AsOf }},
		{"future", "evidence.future", func(r *request) { r.Evidence[0].ObservedAt = "2026-09-30T12:00:00Z" }},
		{"conflict", "evidence.conflict", func(r *request) { r.Evidence = append(r.Evidence, record("counter", "a-cost", "120")) }},
		{"wrong metric", "evidence.metric_mismatch", func(r *request) { r.Evidence[0].Fact.Metric = "latency" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := fixture()
			tc.change(&req)
			got := executeFixture(t, req)
			metric := got.Options[0].Metrics["cost"]
			if got.Options[0].Status != "unknown" || metric.State != "unknown" || len(metric.Issues) != 1 || metric.Issues[0].Code != tc.code {
				t.Fatalf("unsafe evidence: %+v", got.Options[0])
			}
			if !reflect.DeepEqual(got.ParetoFrontier, []string{"b"}) || got.ComparisonState != "known_feasible_options_only" {
				t.Fatal("frontier must explicitly exclude unknown options")
			}
			if tc.name == "conflict" && len(got.Evidence) != 4 {
				t.Fatal("conflicting counterevidence omitted")
			}
		})
	}
}

func TestSupersessionAndEquivalentDecimals(t *testing.T) {
	req := fixture()
	newer := record("new", "a-cost", "8e1")
	req.Evidence = append(req.Evidence, newer)
	if got := executeFixture(t, req); got.Options[0].Status != "feasible" {
		t.Fatal("equivalent decimals treated as conflict")
	}
	req.Evidence[3].Fact.Value = "120"
	req.Evidence[3].Supersedes = []string{"a-cost"}
	got := executeFixture(t, req)
	if got.Options[0].Metrics["cost"].Issues[0].Code != "evidence.superseded" {
		t.Fatal("old value remained current")
	}
	req.Options[0].Metrics["cost"] = []string{"new"}
	if got := executeFixture(t, req); got.Options[0].Status != "infeasible" {
		t.Fatal("new value did not reach constraints")
	}
	req.Evidence[3].ObservedAt = "2026-09-30T12:00:00Z"
	req.Options[0].Metrics["cost"] = []string{"a-cost"}
	if got := executeFixture(t, req); got.Options[0].Status != "feasible" {
		t.Fatal("future update invalidated current value")
	}
}

func TestUnsupportedLimitAndUnits(t *testing.T) {
	for _, change := range []func(*request){
		func(r *request) { r.Constraints[0].Limit = "200" },
		func(r *request) { r.Constraints[0].EvidenceIDs = []string{"missing"} },
		func(r *request) { r.Evidence[0].Fact.Unit = "EUR/month" },
	} {
		req := fixture()
		change(&req)
		if got := executeFixture(t, req); got.Options[0].Status != "unknown" {
			t.Fatalf("unsupported comparison passed: %+v", got.Options[0])
		}
	}
}

func TestParetoTradeoffsAndMissingPreference(t *testing.T) {
	req := fixture()
	req.Preferences = append(req.Preferences, preference{"capacity", "users", "max"})
	for i, id := range []string{"a-cap", "b-cap"} {
		e := record(id, id, []string{"10", "20"}[i])
		e.Fact.Metric, e.Fact.Unit = "capacity", "users"
		req.Evidence = append(req.Evidence, e)
		req.Options[i].Metrics["capacity"] = []string{id}
	}
	if got := executeFixture(t, req); !reflect.DeepEqual(got.ParetoFrontier, []string{"a", "b"}) {
		t.Fatal("tradeoff collapsed into invented ranking")
	}
	delete(req.Options[0].Metrics, "capacity")
	if got := executeFixture(t, req); got.ComparisonState != "insufficient_information" || len(got.ParetoFrontier) != 0 {
		t.Fatal("missing preference evidence ranked")
	}
}

func TestInvalidInputsHaveTypedCodes(t *testing.T) {
	valid, _ := json.Marshal(fixture())
	cases := []struct{ input, code string }{
		{`{"as_of":"a","as_of":"b"}`, "decision.duplicate_field"},
		{string(valid) + `{}`, "decision.json_invalid"},
		{strings.Replace(string(valid), `"80"`, `80`, 1), "decision.schema_invalid"},
		{strings.Replace(string(valid), `"preferences":[{"metric":"cost","unit":"USD/month","direction":"min"}]`, `"preferences":null`, 1), "decision.required_collection_missing"},
		{strings.Repeat(" ", 256*1024+1), "decision.input_limit"},
		{strings.Repeat("[", 18) + "0" + strings.Repeat("]", 18), "decision.json_depth_limit"},
	}
	for _, tc := range cases {
		_, p := decode([]byte(tc.input))
		if p == nil || p.Code != tc.code {
			t.Fatalf("wanted %s, got %+v", tc.code, p)
		}
	}
	req := fixture()
	req.Evidence[0].Supersedes = []string{"a-cost"}
	b, _ := json.Marshal(req)
	if _, p := decode(b); p == nil || p.Code != "decision.supersession_cycle" {
		t.Fatalf("cycle accepted: %+v", p)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := New().Execute(ctx, valid); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func BenchmarkDecisionCompare(b *testing.B) {
	args, _ := json.Marshal(fixture())
	t := New()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := t.Execute(context.Background(), args); err != nil {
			b.Fatal(err)
		}
	}
}
