package decision

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
)

func compute(req request) result {
	index := indexEvidence(req)
	canonical, _ := json.Marshal(req)
	hash := sha256.Sum256(canonical)
	out := result{Status: "computed", AsOf: req.AsOf, Advisory: true, Provenance: "caller_supplied_not_independently_verified", SnapshotSHA256: hex.EncodeToString(hash[:]),
		Options: []optionResult{}, Constraints: req.Constraints, Preferences: req.Preferences, ParetoFrontier: []string{}, Evidence: []citedEvidence{}}
	limits := map[string]metricResult{}
	for _, c := range req.Constraints {
		limits[c.ID] = resolveLimit(index, c)
	}
	for _, o := range req.Options {
		item := optionResult{ID: o.ID, Name: o.Name, Status: "feasible", Metrics: map[string]metricResult{}, Checks: []check{}}
		for metric, refs := range o.Metrics {
			item.Metrics[metric] = index.resolve(metric, refs)
		}
		for _, c := range req.Constraints {
			value, exists := item.Metrics[c.Metric]
			if !exists {
				value = metricResult{State: "unknown", EvidenceIDs: []string{}, Issues: []problem{{Code: "evidence.metric_missing", ID: c.Metric}}}
			}
			support := limits[c.ID]
			verdict, code := compareConstraint(value, support, c)
			refs := append(append([]string{}, value.EvidenceIDs...), c.EvidenceIDs...)
			item.Checks = append(item.Checks, check{ConstraintID: c.ID, Verdict: verdict, Code: code, EvidenceIDs: refs, ValueEvidence: unresolved(value), LimitEvidence: unresolved(support)})
			if verdict == "fail" {
				item.Status = "infeasible"
			} else if verdict == "unknown" && item.Status != "infeasible" {
				item.Status = "unknown"
			}
		}
		out.Options = append(out.Options, item)
	}
	out.ComparisonState, out.ParetoFrontier = frontier(out.Options, req.Preferences)
	usedKeys := map[string]bool{}
	for id := range index.used {
		if row, ok := index.rows[id]; ok {
			usedKeys[row.Key] = true
		}
	}
	ids := []string{}
	for id, row := range index.rows {
		if usedKeys[row.Key] {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		out.Evidence = append(out.Evidence, citedEvidence{index.rows[id], index.states[id]})
	}
	return out
}

func resolveLimit(index *evidenceIndex, c constraint) metricResult {
	support := index.resolve(c.Metric, c.EvidenceIDs)
	if support.State != "known" {
		return support
	}
	value, _ := numeric(support.Value)
	limit, _ := numeric(c.Limit)
	code := ""
	if support.Unit != c.Unit {
		code = "evidence.unit_mismatch"
	} else if value.Cmp(limit) != 0 {
		code = "evidence.limit_mismatch"
	}
	if code != "" {
		support.State = "unknown"
		support.Issues = append(support.Issues, problem{Code: code, ID: c.ID})
	}
	return support
}

func unresolved(value metricResult) *metricResult {
	if value.State == "known" {
		return nil
	}
	return &value
}

func compareConstraint(value, support metricResult, c constraint) (string, string) {
	if support.State != "known" {
		return "unknown", "constraint.limit_evidence_unknown"
	}
	if value.State != "known" {
		return "unknown", "constraint.value_evidence_unknown"
	}
	if value.Unit != c.Unit {
		return "unknown", "constraint.unit_mismatch"
	}
	lhs, _ := numeric(value.Value)
	rhs, _ := numeric(c.Limit)
	cmp := lhs.Cmp(rhs)
	passed := c.Op == "lte" && cmp <= 0 || c.Op == "gte" && cmp >= 0 || c.Op == "eq" && cmp == 0
	if passed {
		return "pass", "constraint.satisfied"
	}
	return "fail", "constraint.violated"
}

func frontier(options []optionResult, preferences []preference) (string, []string) {
	eligible := []optionResult{}
	for _, o := range options {
		if o.Status == "feasible" {
			eligible = append(eligible, o)
		}
	}
	if len(eligible) == 0 {
		for _, o := range options {
			if o.Status == "unknown" {
				return "no_known_feasible_options", []string{}
			}
		}
		return "no_feasible_options", []string{}
	}
	if len(preferences) == 0 {
		return "preferences_not_provided", []string{}
	}
	for _, o := range eligible {
		for _, p := range preferences {
			v := o.Metrics[p.Metric]
			if v.State != "known" || v.Unit != p.Unit {
				return "insufficient_information", []string{}
			}
		}
	}
	ids := []string{}
	for _, o := range eligible {
		dominated := false
		for _, other := range eligible {
			if dominates(other, o, preferences) {
				dominated = true
				break
			}
		}
		if !dominated {
			ids = append(ids, o.ID)
		}
	}
	return "known_feasible_options_only", ids
}

func dominates(a, b optionResult, prefs []preference) bool {
	strict := false
	for _, p := range prefs {
		lhs, _ := numeric(a.Metrics[p.Metric].Value)
		rhs, _ := numeric(b.Metrics[p.Metric].Value)
		cmp := lhs.Cmp(rhs)
		if p.Direction == "max" {
			cmp = -cmp
		}
		if cmp > 0 {
			return false
		}
		strict = strict || cmp < 0
	}
	return strict
}
