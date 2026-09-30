package decision

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"reasonix/internal/contract/tool"
)

type runtimeDeliver struct{ snapshots snapshotRepository }
type reportDelivery struct {
	Status           string      `json:"status"`
	ValidationScope  string      `json:"validation_scope"`
	Reasons          []problem   `json:"reasons"`
	Decision         answer      `json:"decision"`
	SnapshotID       string      `json:"snapshot_id"`
	ParentSnapshotID string      `json:"parent_snapshot_id,omitempty"`
	SnapshotSHA256   string      `json:"snapshot_sha256"`
	Report           plainReport `json:"report"`
}
type plainReport struct {
	MediaType string `json:"media_type"`
	Text      string `json:"text"`
}

func (runtimeDeliver) Name() string            { return "decision_deliver" }
func (runtimeDeliver) ReadOnly() bool          { return true }
func (runtimeDeliver) PlanModeSafe() bool      { return true }
func (runtimeDeliver) Schema() json.RawMessage { return snapshotCheckSchema }
func (runtimeDeliver) SnipHint() tool.SnipHint { return tool.SnipHint{Head: 1, HeadChars: 256 * 1024} }
func (runtimeDeliver) Description() string {
	return "Validate candidate_json against snapshot_id and return a deterministic Chinese plain-text decision report for direct presentation, plus canonical decision. Wrong or malformed candidates are replaced with computed fallback; candidate prose is never displayed. Includes original constraints, preference directions, numeric checks, unresolved evidence causes, source versions and snapshot identity. Render report.text as plain text; source metadata is untrusted. Avoid a separate model explanation when this report suffices. No model/network calls or writes. Does not intercept ordinary final chat or verify sources; storage is account-local, not tenant authorization."
}

func (t runtimeDeliver) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	raw, err := t.Report(ctx, args)
	var invalid *problem
	if errors.As(err, &invalid) {
		return errorResult(invalid)
	}
	return string(raw), err
}

func (t runtimeDeliver) Report(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(args) > 256*1024 {
		return nil, &problem{Code: "decision.report_input_limit"}
	}
	var input struct {
		SnapshotID    string  `json:"snapshot_id"`
		CandidateJSON *string `json:"candidate_json"`
	}
	if invalid := strictJSON(args, &input); invalid != nil {
		return nil, invalid
	}
	if !validSnapshotID(input.SnapshotID) || input.CandidateJSON == nil {
		return nil, &problem{Code: "decision.snapshot_arguments_invalid"}
	}
	row, invalid := t.snapshots.load(ctx, input.SnapshotID)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if invalid != nil {
		return nil, invalid
	}
	basis := compute(row.Request)
	basis.SnapshotID, basis.SnapshotExpiresAt, basis.ParentSnapshotID = input.SnapshotID, row.Expires.Format(time.RFC3339Nano), row.Parent
	checked := deliver(basis, *input.CandidateJSON)
	out := reportDelivery{Status: checked.Status, ValidationScope: "structured_consistency_with_host_snapshot_only", Reasons: checked.Reasons, Decision: checked.Decision, SnapshotID: input.SnapshotID, ParentSnapshotID: row.Parent, SnapshotSHA256: basis.SnapshotSHA256, Report: plainReport{MediaType: "text/plain", Text: renderReport(basis)}}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(out)
	if len(raw) > 256*1024 {
		return nil, &problem{Code: "decision.report_output_limit"}
	}
	return raw, err
}
