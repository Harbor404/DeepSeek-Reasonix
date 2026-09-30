package decision

import (
	"context"
	_ "embed"
	"encoding/json"

	"reasonix/internal/contract/tool"
)

//go:embed schema.json
var schema json.RawMessage

type compareTool struct{}

func New() tool.Tool             { return compareTool{} }
func (compareTool) Name() string { return "decision_compare" }
func (compareTool) Description() string {
	return "Compare options against explicit sourced numeric constraints and preference directions without another model call. Submit decimal values as strings, source identities and observation times. Returns conditional feasibility, missing/stale/conflicting evidence and a Pareto frontier over known-feasible options. Never invent missing facts or user preferences; source contents remain unverified and outputs are advisory, not permission or a final choice. To check a structured answer against this same request, call tool:decision_check. No files, network or model calls."
}
func (compareTool) Schema() json.RawMessage { return schema }
func (compareTool) ReadOnly() bool          { return true }
func (compareTool) PlanModeSafe() bool      { return true }
func (compareTool) SnipHint() tool.SnipHint { return tool.SnipHint{Head: 1, HeadChars: 256 * 1024} }

func (compareTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	req, err := decode(args)
	if err != nil {
		return errorResult(err)
	}
	out := compute(req)
	if err := ctx.Err(); err != nil {
		return "", err
	}
	data, marshalErr := json.Marshal(out)
	return string(data), marshalErr
}
