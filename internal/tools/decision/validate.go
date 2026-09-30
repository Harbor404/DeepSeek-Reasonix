package decision

import (
	"math/big"
	"regexp"
	"strings"
	"time"
)

var decimalSyntax = regexp.MustCompile(`^[+-]?[0-9]+(?:\.[0-9]+)?(?:[eE][+-]?[0-9]{1,3})?$`)

func numeric(value string) (*big.Rat, bool) {
	if len(value) > 64 || !decimalSyntax.MatchString(value) {
		return nil, false
	}
	v, ok := new(big.Rat).SetString(value)
	return v, ok
}

func nonempty(value string, max int) bool {
	return len(value) <= max && strings.TrimSpace(value) != ""
}

func refsValid(refs []string) bool {
	if len(refs) < 1 || len(refs) > 8 {
		return false
	}
	seen := map[string]bool{}
	for _, id := range refs {
		if !nonempty(id, 128) || seen[id] {
			return false
		}
		seen[id] = true
	}
	return true
}

func validate(req request) *problem {
	if req.Options == nil || req.Constraints == nil || req.Preferences == nil || req.Evidence == nil {
		return &problem{Code: "decision.required_collection_missing"}
	}
	if _, err := time.Parse(time.RFC3339Nano, req.AsOf); err != nil {
		return &problem{Code: "decision.timestamp_invalid"}
	}
	if len(req.Options) < 1 || len(req.Options) > 32 || len(req.Constraints) > 32 || len(req.Preferences) > 16 || len(req.Evidence) > 128 {
		return &problem{Code: "decision.collection_limit"}
	}
	for _, validate := range []func(request) *problem{validateEvidence, validateOptions, validateConstraints, validatePreferences} {
		if err := validate(req); err != nil {
			return err
		}
	}
	return validateSupersession(req.Evidence)
}

func validateEvidence(req request) *problem {
	seen := map[string]bool{}
	for _, e := range req.Evidence {
		if !nonempty(e.ID, 128) || seen[e.ID] {
			return &problem{Code: "decision.evidence_identity_invalid", ID: e.ID}
		}
		seen[e.ID] = true
		if !nonempty(e.Key, 128) || !nonempty(e.Text, 1024) || !nonempty(e.Source.URI, 512) || !nonempty(e.Source.Version, 128) {
			return &problem{Code: "decision.source_invalid", ID: e.ID}
		}
		observed, err := time.Parse(time.RFC3339Nano, e.ObservedAt)
		if err != nil {
			return &problem{Code: "decision.timestamp_invalid", ID: e.ID}
		}
		if e.ValidUntil != "" {
			until, err := time.Parse(time.RFC3339Nano, e.ValidUntil)
			if err != nil || !until.After(observed) {
				return &problem{Code: "decision.validity_invalid", ID: e.ID}
			}
		}
		if _, ok := numeric(e.Fact.Value); !ok || !nonempty(e.Fact.Metric, 64) || !nonempty(e.Fact.Unit, 32) {
			return &problem{Code: "decision.fact_invalid", ID: e.ID}
		}
		if len(e.Supersedes) > 8 || len(e.Supersedes) > 0 && !refsValid(e.Supersedes) {
			return &problem{Code: "decision.supersession_invalid", ID: e.ID}
		}
	}
	return nil
}

func validateOptions(req request) *problem {
	seen := map[string]bool{}
	for _, o := range req.Options {
		if !nonempty(o.ID, 128) || seen[o.ID] || !nonempty(o.Name, 256) || len(o.Metrics) < 1 || len(o.Metrics) > 16 {
			return &problem{Code: "decision.option_invalid", ID: o.ID}
		}
		seen[o.ID] = true
		for metric, refs := range o.Metrics {
			if !nonempty(metric, 64) || !refsValid(refs) {
				return &problem{Code: "decision.metric_invalid", ID: o.ID}
			}
		}
	}
	return nil
}

func validateConstraints(req request) *problem {
	seen := map[string]bool{}
	for _, c := range req.Constraints {
		if !nonempty(c.ID, 128) || seen[c.ID] || !nonempty(c.Metric, 64) || !nonempty(c.Unit, 32) || !refsValid(c.EvidenceIDs) {
			return &problem{Code: "decision.constraint_invalid", ID: c.ID}
		}
		seen[c.ID] = true
		if _, ok := numeric(c.Limit); !ok {
			return &problem{Code: "decision.limit_invalid", ID: c.ID}
		}
		if c.Op != "lte" && c.Op != "gte" && c.Op != "eq" {
			return &problem{Code: "decision.operator_invalid", ID: c.ID}
		}
	}
	return nil
}

func validatePreferences(req request) *problem {
	seen := map[string]bool{}
	for _, p := range req.Preferences {
		if !nonempty(p.Metric, 64) || !nonempty(p.Unit, 32) || seen[p.Metric] || p.Direction != "min" && p.Direction != "max" {
			return &problem{Code: "decision.preference_invalid", ID: p.Metric}
		}
		seen[p.Metric] = true
	}
	return nil
}

func validateSupersession(records []evidence) *problem {
	rows := map[string]evidence{}
	for _, row := range records {
		rows[row.ID] = row
	}
	visited := map[string]int{}
	var visit func(string) *problem
	visit = func(id string) *problem {
		if visited[id] == 1 {
			return &problem{Code: "decision.supersession_cycle", ID: id}
		}
		if visited[id] == 2 {
			return nil
		}
		visited[id] = 1
		row := rows[id]
		for _, oldID := range row.Supersedes {
			old, exists := rows[oldID]
			if !exists || old.Key != row.Key {
				return &problem{Code: "decision.supersession_reference_invalid", ID: oldID}
			}
			prior, _ := time.Parse(time.RFC3339Nano, old.ObservedAt)
			current, _ := time.Parse(time.RFC3339Nano, row.ObservedAt)
			if prior.After(current) {
				return &problem{Code: "decision.supersession_time_invalid", ID: id}
			}
			if err := visit(oldID); err != nil {
				return err
			}
		}
		visited[id] = 2
		return nil
	}
	for _, row := range records {
		if err := visit(row.ID); err != nil {
			return err
		}
	}
	return nil
}
