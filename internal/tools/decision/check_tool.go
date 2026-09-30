package decision

import (
	"context"
	"encoding/json"

	"reasonix/internal/contract/tool"
)

var answerSchema = buildAnswerSchema()

type checkTool struct{}

func NewCheck() tool.Tool      { return checkTool{} }
func (checkTool) Name() string { return "decision_check" }
func (checkTool) Description() string {
	return "Check a structured decision answer against a fresh computation of the supplied original request. Pass candidate_json as the model's unmodified answer text, even if malformed. The answer must have exactly options (ID to feasibility status), frontier (ID array), comparison_state and recommended_option (null). Returns canonical decision and sourced basis; any rejection uses computed_fallback with typed reasons. Use returned decision for structured presentation. Validates consistency only: no prose/source verification, input completeness guarantee, earlier-snapshot binding or automatic filtering of final chat text. No network or model calls."
}
func (checkTool) Schema() json.RawMessage { return answerSchema }
func (checkTool) ReadOnly() bool          { return true }
func (checkTool) PlanModeSafe() bool      { return true }
func (checkTool) SnipHint() tool.SnipHint { return tool.SnipHint{Head: 1, HeadChars: 256 * 1024} }

func buildAnswerSchema() json.RawMessage {
	var requestSchema map[string]any
	if err := json.Unmarshal(schema, &requestSchema); err != nil {
		panic(err)
	}
	defs := requestSchema["$defs"]
	delete(requestSchema, "$defs")
	out, err := json.Marshal(map[string]any{
		"type": "object", "additionalProperties": false, "required": []string{"request", "candidate_json"}, "$defs": defs,
		"properties": map[string]any{
			"request":        requestSchema,
			"candidate_json": map[string]any{"type": "string", "maxLength": 65536, "description": "Unmodified structured answer text; do not repair or silently truncate before checking"},
		},
	})
	if err != nil {
		panic(err)
	}
	return out
}

func (checkTool) Execute(ctx context.Context, args json.RawMessage) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	var input struct {
		Request       request `json:"request"`
		CandidateJSON *string `json:"candidate_json"`
	}
	if err := strictJSON(args, &input); err != nil {
		return errorResult(err)
	}
	if err := validate(input.Request); err != nil {
		return errorResult(err)
	}
	if input.CandidateJSON == nil {
		return errorResult(&problem{Code: "decision.candidate_missing"})
	}
	out := deliver(compute(input.Request), *input.CandidateJSON)
	if err := ctx.Err(); err != nil {
		return "", err
	}
	data, err := json.Marshal(out)
	return string(data), err
}
