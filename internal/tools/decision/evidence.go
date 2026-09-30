package decision

import "time"

type evidenceIndex struct {
	rows   map[string]evidence
	states map[string]string
	used   map[string]bool
}

func indexEvidence(req request) *evidenceIndex {
	index := &evidenceIndex{rows: map[string]evidence{}, states: map[string]string{}, used: map[string]bool{}}
	asOf, _ := time.Parse(time.RFC3339Nano, req.AsOf)
	superseded := map[string]bool{}
	for _, e := range req.Evidence {
		observed, _ := time.Parse(time.RFC3339Nano, e.ObservedAt)
		if !observed.After(asOf) {
			for _, id := range e.Supersedes {
				superseded[id] = true
			}
		}
	}
	groups := map[string][]evidence{}
	for _, e := range req.Evidence {
		index.rows[e.ID] = e
		state := "current"
		observed, _ := time.Parse(time.RFC3339Nano, e.ObservedAt)
		until, _ := time.Parse(time.RFC3339Nano, e.ValidUntil)
		switch {
		case observed.After(asOf):
			state = "future"
		case superseded[e.ID]:
			state = "superseded"
		case e.ValidUntil != "" && !until.After(asOf):
			state = "expired"
		}
		index.states[e.ID] = state
		if state == "current" {
			groups[e.Key] = append(groups[e.Key], e)
		}
	}
	for _, rows := range groups {
		first := rows[0].Fact
		value, _ := numeric(first.Value)
		conflict := false
		for _, row := range rows[1:] {
			other, _ := numeric(row.Fact.Value)
			conflict = conflict || first.Metric != row.Fact.Metric || first.Unit != row.Fact.Unit || value.Cmp(other) != 0
		}
		if conflict {
			for _, row := range rows {
				index.states[row.ID] = "conflict"
			}
		}
	}
	return index
}

func (index *evidenceIndex) resolve(metric string, refs []string) metricResult {
	out := metricResult{State: "unknown", EvidenceIDs: refs}
	var first *fact
	for _, id := range refs {
		index.used[id] = true
		row, exists := index.rows[id]
		if !exists {
			out.Issues = append(out.Issues, problem{Code: "evidence.missing", ID: id})
			continue
		}
		if index.states[id] != "current" {
			out.Issues = append(out.Issues, problem{Code: "evidence." + index.states[id], ID: id})
			continue
		}
		if row.Fact.Metric != metric {
			out.Issues = append(out.Issues, problem{Code: "evidence.metric_mismatch", ID: id})
			continue
		}
		if first == nil {
			f := row.Fact
			first = &f
			continue
		}
		lhs, _ := numeric(first.Value)
		rhs, _ := numeric(row.Fact.Value)
		if first.Unit != row.Fact.Unit || lhs.Cmp(rhs) != 0 {
			out.Issues = append(out.Issues, problem{Code: "evidence.value_conflict", ID: id})
		}
	}
	if first != nil && len(out.Issues) == 0 {
		out.State, out.Value, out.Unit = "known", first.Value, first.Unit
	}
	return out
}
