package decision

import (
	"context"
	"encoding/json"
	"time"

	"reasonix/internal/contract/tool"
)

type runtimeUpdate struct{ snapshots snapshotRepository }
type metricUpdate struct {
	OptionID    string   `json:"option_id"`
	Metric      string   `json:"metric"`
	EvidenceIDs []string `json:"evidence_ids"`
}
type updateInput struct {
	SnapshotID string         `json:"snapshot_id"`
	AsOf       string         `json:"as_of"`
	Evidence   []evidence     `json:"evidence"`
	Metrics    []metricUpdate `json:"metric_updates"`
}

var evidenceUpdateSchema = buildUpdateSchema()

func (runtimeUpdate) Name() string            { return "decision_update" }
func (t runtimeUpdate) ReadOnly() bool        { return !t.snapshots.persistent() }
func (runtimeUpdate) PlanModeSafe() bool      { return true }
func (runtimeUpdate) SnipHint() tool.SnipHint { return tool.SnipHint{Head: 1, HeadChars: 256 * 1024} }
func (runtimeUpdate) Schema() json.RawMessage { return evidenceUpdateSchema }
func (runtimeUpdate) Description() string {
	return "Create a new snapshot version from a saved comparison: append new sourced evidence with new IDs, advance or retain as_of, and explicitly update option metric references. Existing evidence, option identities, user constraints and preferences stay unchanged. Use supersedes links for source revisions; never silently replace old evidence. Returns new snapshot_id and parent_snapshot_id. Old snapshot answers still refer to the old version. No source fetching or model calls."
}

func buildUpdateSchema() json.RawMessage {
	var source map[string]any
	if err := json.Unmarshal(schema, &source); err != nil {
		panic(err)
	}
	encoded, err := json.Marshal(map[string]any{
		"type": "object", "additionalProperties": false, "$defs": source["$defs"], "required": []string{"snapshot_id", "as_of", "evidence", "metric_updates"},
		"properties": map[string]any{
			"snapshot_id":    map[string]any{"type": "string", "pattern": "^[0-9a-f]{32}$"},
			"as_of":          map[string]any{"type": "string", "format": "date-time"},
			"evidence":       map[string]any{"type": "array", "minItems": 1, "maxItems": 32, "items": map[string]any{"$ref": "#/$defs/evidence"}},
			"metric_updates": map[string]any{"type": "array", "maxItems": 32, "items": map[string]any{"type": "object", "additionalProperties": false, "required": []string{"option_id", "metric", "evidence_ids"}, "properties": map[string]any{"option_id": map[string]any{"$ref": "#/$defs/id"}, "metric": map[string]any{"$ref": "#/$defs/metric"}, "evidence_ids": map[string]any{"$ref": "#/$defs/refs"}}}},
		},
	})
	if err != nil {
		panic(err)
	}
	return encoded
}

func applyUpdate(row savedRequest, input updateInput) (request, *problem) {
	req := row.Request
	current, err := time.Parse(time.RFC3339Nano, input.AsOf)
	prior, _ := time.Parse(time.RFC3339Nano, req.AsOf)
	if err != nil || current.Before(prior) {
		return req, &problem{Code: "decision.update_time_invalid"}
	}
	if len(input.Evidence) < 1 || len(input.Evidence) > 32 || input.Metrics == nil || len(input.Metrics) > 32 {
		return req, &problem{Code: "decision.update_arguments_invalid"}
	}
	req.AsOf = input.AsOf
	req.Evidence = append(req.Evidence, input.Evidence...)
	seen := map[[2]string]bool{}
	for _, update := range input.Metrics {
		key := [2]string{update.OptionID, update.Metric}
		if seen[key] {
			return req, &problem{Code: "decision.metric_update_duplicate", ID: update.OptionID}
		}
		seen[key] = true
		found := false
		for i, option := range req.Options {
			if option.ID == update.OptionID {
				req.Options[i].Metrics[update.Metric] = update.EvidenceIDs
				found = true
				break
			}
		}
		if !found {
			return req, &problem{Code: "decision.option_unknown", ID: update.OptionID}
		}
	}
	return req, validate(req)
}

func (t runtimeUpdate) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	var input updateInput
	if invalid := strictJSON(args, &input); invalid != nil {
		return errorResult(invalid)
	}
	if !validSnapshotID(input.SnapshotID) {
		return errorResult(&problem{Code: "decision.snapshot_arguments_invalid"})
	}
	row, invalid := t.snapshots.load(ctx, input.SnapshotID)
	if invalid != nil {
		return errorResult(invalid)
	}
	req, invalid := applyUpdate(row, input)
	if invalid != nil {
		return errorResult(invalid)
	}
	basis := compute(req)
	id, expires, invalid := t.snapshots.save(ctx, req, input.SnapshotID)
	if invalid != nil {
		return errorResult(invalid)
	}
	basis.SnapshotID, basis.SnapshotExpiresAt, basis.ParentSnapshotID = id, expires.Format(time.RFC3339Nano), input.SnapshotID
	data, err := json.Marshal(basis)
	return string(data), err
}
