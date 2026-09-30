package decision

import (
	"context"
	"encoding/json"
	"time"

	"reasonix/internal/contract/tool"
)

type runtimeCompare struct {
	compareTool
	snapshots snapshotRepository
}
type runtimeCheck struct {
	checkTool
	snapshots snapshotRepository
}

var snapshotCheckSchema = json.RawMessage(`{"type":"object","additionalProperties":false,"required":["snapshot_id","candidate_json"],"properties":{"snapshot_id":{"type":"string","pattern":"^[0-9a-f]{32}$"},"candidate_json":{"type":"string","maxLength":65536,"description":"Unmodified structured model answer, including malformed text"}}}`)

// NewRuntime creates tools sharing a bounded, private in-memory snapshot store.
func NewRuntime() []tool.Tool {
	store := memoryRepository{newSnapshots()}
	return runtimeTools(store)
}

// NewPersistentRuntime uses a host-selected database path without memory fallback.
func NewPersistentRuntime(path string) []tool.Tool {
	store := &diskRepository{path: path, now: time.Now}
	return runtimeTools(store)
}

func runtimeTools(store snapshotRepository) []tool.Tool {
	return []tool.Tool{runtimeCompare{snapshots: store}, runtimeCheck{snapshots: store}, runtimeUpdate{snapshots: store}, runtimeDeliver{snapshots: store}}
}

func (t runtimeCompare) ReadOnly() bool { return !t.snapshots.persistent() }
func (t runtimeCompare) Description() string {
	storage := "Private memory, 30-minute expiry, at most 64 snapshots."
	if t.snapshots.persistent() {
		storage = "Workspace-scoped persistent storage, 7-day expiry, at most 256 snapshots. Writes host-selected session storage; survives restart within expiry."
	}
	return "Compare sourced numeric options and explicit constraints using exact arithmetic. Saves the original request and returns snapshot_id and snapshot_expires_at. Check answers via tool:decision_check using the ID; update evidence via tool:decision_update to create a new version. " + storage + " Capacity failures are explicit. As_of remains the supplied reference time. Sources remain unverified, outputs advisory; no network or model calls."
}

func (t runtimeCompare) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	req, invalid := decode(args)
	if invalid != nil {
		return errorResult(invalid)
	}
	out := compute(req)
	if err := ctx.Err(); err != nil {
		return "", err
	}
	id, expires, invalid := t.snapshots.save(ctx, req, "")
	if invalid != nil {
		return errorResult(invalid)
	}
	out.SnapshotID, out.SnapshotExpiresAt = id, expires.Format(time.RFC3339Nano)
	data, err := json.Marshal(out)
	return string(data), err
}

func (runtimeCheck) Schema() json.RawMessage { return snapshotCheckSchema }
func (runtimeCheck) Description() string {
	return "Check a structured model answer against a saved original request in this storage scope. Submit only snapshot_id and candidate_json; request replacements are rejected. Returns canonical decision and sourced basis, with computed_fallback on answer errors. Missing, expired or corrupt snapshots return typed errors. Required answer fields: options (ID to status), frontier (ID array), comparison_state, recommended_option (null). Validates structured consistency only; no prose/source verification or automatic final-chat filtering. Storage is account-local, not a multi-tenant authorization boundary. No network or model calls."
}

func (t runtimeCheck) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	var input struct {
		SnapshotID    string  `json:"snapshot_id"`
		CandidateJSON *string `json:"candidate_json"`
	}
	if invalid := strictJSON(args, &input); invalid != nil {
		return errorResult(invalid)
	}
	if !validSnapshotID(input.SnapshotID) || input.CandidateJSON == nil {
		return errorResult(&problem{Code: "decision.snapshot_arguments_invalid"})
	}
	row, invalid := t.snapshots.load(ctx, input.SnapshotID)
	if invalid != nil {
		return errorResult(invalid)
	}
	basis := compute(row.Request)
	basis.SnapshotID, basis.SnapshotExpiresAt, basis.ParentSnapshotID = input.SnapshotID, row.Expires.Format(time.RFC3339Nano), row.Parent
	out := deliver(basis, *input.CandidateJSON)
	out.ValidationScope = "structured_consistency_with_host_snapshot_only"
	if err := ctx.Err(); err != nil {
		return "", err
	}
	data, err := json.Marshal(out)
	return string(data), err
}

func validSnapshotID(id string) bool {
	if len(id) != 32 {
		return false
	}
	for _, character := range id {
		if !(character >= '0' && character <= '9' || character >= 'a' && character <= 'f') {
			return false
		}
	}
	return true
}
