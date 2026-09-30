package decision

import (
	"bytes"
	"encoding/json"
	"sort"
	"strings"
)

type answer struct {
	Options           map[string]string `json:"options"`
	Frontier          []string          `json:"frontier"`
	ComparisonState   string            `json:"comparison_state"`
	RecommendedOption json.RawMessage   `json:"recommended_option"`
}

type delivery struct {
	Status          string    `json:"status"`
	ValidationScope string    `json:"validation_scope"`
	Reasons         []problem `json:"reasons"`
	Decision        answer    `json:"decision"`
	Basis           result    `json:"basis"`
}

func canonicalAnswer(basis result) answer {
	out := answer{Options: map[string]string{}, Frontier: basis.ParetoFrontier, ComparisonState: basis.ComparisonState, RecommendedOption: json.RawMessage("null")}
	for _, option := range basis.Options {
		out.Options[option.ID] = option.Status
	}
	return out
}

func deliver(basis result, candidate string) delivery {
	expected := canonicalAnswer(basis)
	out := delivery{Status: "answer_accepted", ValidationScope: "structured_consistency_with_supplied_request_only", Reasons: checkAnswer(candidate, expected), Decision: expected, Basis: basis}
	if len(out.Reasons) != 0 {
		out.Status = "computed_fallback"
	}
	return out
}

func checkAnswer(candidate string, expected answer) []problem {
	if len(candidate) > 64*1024 {
		return []problem{{Code: "answer.input_limit"}}
	}
	var actual answer
	if err := strictJSON([]byte(candidate), &actual); err != nil {
		return []problem{{Code: "answer." + strings.TrimPrefix(err.Code, "decision."), ID: err.ID}}
	}
	if actual.Options == nil || actual.Frontier == nil || actual.ComparisonState == "" || len(actual.RecommendedOption) == 0 {
		return []problem{{Code: "answer.required_field_missing"}}
	}
	reasons := []problem{}
	for _, id := range sortedKeys(expected.Options) {
		status, exists := actual.Options[id]
		if !exists {
			reasons = append(reasons, problem{Code: "answer.option_missing", ID: id})
		} else if status != expected.Options[id] {
			reasons = append(reasons, problem{Code: "answer.option_status_mismatch", ID: id})
		}
	}
	for _, id := range sortedKeys(actual.Options) {
		if _, exists := expected.Options[id]; !exists {
			reasons = append(reasons, problem{Code: "answer.option_unknown", ID: id})
		}
	}
	if !sameFrontier(actual.Frontier, expected.Frontier) {
		reasons = append(reasons, problem{Code: "answer.frontier_mismatch"})
	}
	if actual.ComparisonState != expected.ComparisonState {
		reasons = append(reasons, problem{Code: "answer.comparison_state_mismatch"})
	}
	if !bytes.Equal(bytes.TrimSpace(actual.RecommendedOption), []byte("null")) {
		reasons = append(reasons, problem{Code: "answer.recommendation_forbidden"})
	}
	return reasons
}

func sortedKeys(rows map[string]string) []string {
	keys := make([]string, 0, len(rows))
	for key := range rows {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func sameFrontier(actual, expected []string) bool {
	if len(actual) != len(expected) {
		return false
	}
	left, right := append([]string{}, actual...), append([]string{}, expected...)
	sort.Strings(left)
	sort.Strings(right)
	for i, id := range left {
		if id != right[i] || i > 0 && id == left[i-1] {
			return false
		}
	}
	return true
}
