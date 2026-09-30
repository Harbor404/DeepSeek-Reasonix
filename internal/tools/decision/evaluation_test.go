package decision

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"reflect"
	"testing"
)

func TestComparisonStatesIdentifyNextAction(t *testing.T) {
	cases := []struct {
		name, want string
		change     func(*request)
	}{
		{"all violate", "no_feasible_options", func(r *request) { r.Evidence[0].Fact.Value = "120"; r.Evidence[1].Fact.Value = "130" }},
		{"all unknown", "no_known_feasible_options", func(r *request) { r.Evidence[0].ValidUntil = r.AsOf; r.Evidence[1].ValidUntil = r.AsOf }},
		{"no preferences", "preferences_not_provided", func(r *request) { r.Preferences = []preference{} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := fixture()
			tc.change(&req)
			if got := executeFixture(t, req); got.ComparisonState != tc.want {
				t.Fatalf("got %q, want %q", got.ComparisonState, tc.want)
			}
		})
	}
}

func BenchmarkDecisionCompare32Options(b *testing.B) {
	req := fixture()
	req.Options = []option{}
	req.Evidence = req.Evidence[2:]
	req.Preferences = append(req.Preferences, preference{"speed", "points", "max"}, preference{"risk", "points", "min"})
	for i := range 32 {
		id := fmt.Sprintf("o%d", i)
		metrics := map[string][]string{}
		for k, pref := range req.Preferences {
			eid := id + "-" + pref.Metric
			e := record(eid, eid, fmt.Sprintf("%d", (i*3+k)%100))
			e.Fact.Metric, e.Fact.Unit = pref.Metric, pref.Unit
			req.Evidence = append(req.Evidence, e)
			metrics[pref.Metric] = []string{eid}
		}
		req.Options = append(req.Options, option{id, id, metrics})
	}
	args, _ := json.Marshal(req)
	tool := New()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := tool.Execute(context.Background(), args); err != nil {
			b.Fatal(err)
		}
	}
}

func TestLimitDiagnosticsPreserveCause(t *testing.T) {
	req := fixture()
	req.Constraints[0].Limit = "200"
	got := executeFixture(t, req)
	if len(got.Options[0].Checks[0].LimitEvidence.Issues) == 0 || got.Options[0].Checks[0].LimitEvidence.Issues[0].Code != "evidence.limit_mismatch" {
		t.Fatalf("missing cause: %+v", got.Options[0].Checks[0])
	}
}

func TestGeneratedDecisionsAgainstIntegerOracle(t *testing.T) {
	rng := rand.New(rand.NewPCG(21, 42))
	for caseID := range 1000 {
		req := fixture()
		req.Options, req.Evidence = []option{}, []evidence{}
		budget := rng.IntN(20001) - 10000
		budgetFact := record("budget", "budget", fmt.Sprintf("%d", budget))
		req.Evidence = append(req.Evidence, budgetFact)
		req.Constraints[0].Limit = budgetFact.Fact.Value
		op := []string{"lte", "gte", "eq"}[caseID%3]
		req.Constraints[0].Op = op
		direction := []string{"min", "max"}[caseID%2]
		req.Preferences[0].Direction = direction
		values := []int{}
		expected := []string{}
		best, found := 0, false
		for i := range 2 + rng.IntN(9) {
			value := rng.IntN(20001) - 10000
			values = append(values, value)
			id := fmt.Sprintf("o%d", i)
			e := record(id, id, fmt.Sprintf("%d", value))
			req.Evidence = append(req.Evidence, e)
			req.Options = append(req.Options, option{id, id, map[string][]string{"cost": {id}}})
			pass := op == "lte" && value <= budget || op == "gte" && value >= budget || op == "eq" && value == budget
			if pass && (!found || direction == "min" && value < best || direction == "max" && value > best) {
				best, found = value, true
			}
		}
		got := executeFixture(t, req)
		for i, value := range values {
			pass := op == "lte" && value <= budget || op == "gte" && value >= budget || op == "eq" && value == budget
			if (got.Options[i].Status == "feasible") != pass {
				t.Fatalf("case %d option %d: wrong constraint result", caseID, i)
			}
			if pass && value == best {
				expected = append(expected, req.Options[i].ID)
			}
		}
		if !reflect.DeepEqual(got.ParetoFrontier, expected) {
			t.Fatalf("case %d frontier: %v != %v", caseID, got.ParetoFrontier, expected)
		}
	}
}

func TestGeneratedTradeoffsAgainstIntegerOracle(t *testing.T) {
	rng := rand.New(rand.NewPCG(63, 19))
	for caseID := range 300 {
		req := fixture()
		req.Options, req.Evidence, req.Constraints = []option{}, []evidence{}, []constraint{}
		req.Preferences = []preference{{"cost", "points", "min"}, {"speed", "points", "max"}, {"risk", "points", "min"}}
		points := [][3]int{}
		for i := range 8 {
			point := [3]int{rng.IntN(21), rng.IntN(21), rng.IntN(21)}
			points = append(points, point)
			id := fmt.Sprintf("o%d", i)
			metrics := map[string][]string{}
			for k, pref := range req.Preferences {
				eid := id + "-" + pref.Metric
				e := record(eid, eid, fmt.Sprintf("%d", point[k]))
				e.Fact.Metric, e.Fact.Unit = pref.Metric, pref.Unit
				req.Evidence = append(req.Evidence, e)
				metrics[pref.Metric] = []string{eid}
			}
			req.Options = append(req.Options, option{id, id, metrics})
		}
		expected := []string{}
		for i, a := range points {
			dominated := false
			for _, b := range points {
				weak := b[0] <= a[0] && b[1] >= a[1] && b[2] <= a[2]
				strict := b[0] < a[0] || b[1] > a[1] || b[2] < a[2]
				dominated = dominated || weak && strict
			}
			if !dominated {
				expected = append(expected, req.Options[i].ID)
			}
		}
		if got := executeFixture(t, req); !reflect.DeepEqual(got.ParetoFrontier, expected) {
			t.Fatalf("case %d tradeoffs: %v != %v", caseID, got.ParetoFrontier, expected)
		}
	}
}
