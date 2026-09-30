package decision

import "encoding/json"

type source struct {
	URI     string `json:"uri"`
	Version string `json:"version"`
}

type fact struct {
	Metric string `json:"metric"`
	Value  string `json:"value"`
	Unit   string `json:"unit"`
}

type evidence struct {
	ID         string   `json:"id"`
	Key        string   `json:"key"`
	Text       string   `json:"text"`
	Source     source   `json:"source"`
	ObservedAt string   `json:"observed_at"`
	ValidUntil string   `json:"valid_until,omitempty"`
	Supersedes []string `json:"supersedes,omitempty"`
	Fact       fact     `json:"fact"`
}

type option struct {
	ID      string              `json:"id"`
	Name    string              `json:"name"`
	Metrics map[string][]string `json:"metrics"`
}

type constraint struct {
	ID          string   `json:"id"`
	Metric      string   `json:"metric"`
	Op          string   `json:"op"`
	Limit       string   `json:"limit"`
	Unit        string   `json:"unit"`
	EvidenceIDs []string `json:"evidence_ids"`
}

type preference struct {
	Metric    string `json:"metric"`
	Unit      string `json:"unit"`
	Direction string `json:"direction"`
}

type request struct {
	AsOf        string       `json:"as_of"`
	Options     []option     `json:"options"`
	Constraints []constraint `json:"constraints"`
	Preferences []preference `json:"preferences"`
	Evidence    []evidence   `json:"evidence"`
}

type problem struct {
	Code string `json:"code"`
	ID   string `json:"id,omitempty"`
}

func (p *problem) Error() string { return p.Code }

type metricResult struct {
	State       string    `json:"state"`
	Value       string    `json:"value,omitempty"`
	Unit        string    `json:"unit,omitempty"`
	EvidenceIDs []string  `json:"evidence_ids"`
	Issues      []problem `json:"issues,omitempty"`
}

type check struct {
	ConstraintID  string        `json:"constraint_id"`
	Verdict       string        `json:"verdict"`
	Code          string        `json:"code"`
	EvidenceIDs   []string      `json:"evidence_ids"`
	ValueEvidence *metricResult `json:"value_evidence,omitempty"`
	LimitEvidence *metricResult `json:"limit_evidence,omitempty"`
}

type optionResult struct {
	ID      string                  `json:"id"`
	Name    string                  `json:"name"`
	Status  string                  `json:"status"`
	Metrics map[string]metricResult `json:"metrics"`
	Checks  []check                 `json:"constraint_checks"`
}

type citedEvidence struct {
	evidence
	State string `json:"state"`
}

type result struct {
	Status            string          `json:"status"`
	AsOf              string          `json:"as_of"`
	Advisory          bool            `json:"advisory"`
	Provenance        string          `json:"provenance"`
	SnapshotSHA256    string          `json:"snapshot_sha256"`
	SnapshotID        string          `json:"snapshot_id,omitempty"`
	ParentSnapshotID  string          `json:"parent_snapshot_id,omitempty"`
	SnapshotExpiresAt string          `json:"snapshot_expires_at,omitempty"`
	Options           []optionResult  `json:"options"`
	Constraints       []constraint    `json:"constraints"`
	Preferences       []preference    `json:"preferences"`
	ParetoFrontier    []string        `json:"pareto_frontier"`
	ComparisonState   string          `json:"comparison_state"`
	Evidence          []citedEvidence `json:"evidence"`
	RecommendedOption *string         `json:"recommended_option"`
}

func errorResult(err *problem) (string, error) {
	b, marshalErr := json.Marshal(struct {
		Status string   `json:"status"`
		Error  *problem `json:"error"`
	}{"invalid", err})
	return string(b), marshalErr
}
