package decision

import "errors"

type FailureKind uint8

const (
	FailureInput FailureKind = iota
	FailureMissing
	FailureExpired
	FailureStorage
	FailureLimit
	FailureTooLarge
)

type Failure struct {
	Code string
	Kind FailureKind
}

func ReportFailure(err error) (Failure, bool) {
	var invalid *problem
	if !errors.As(err, &invalid) {
		return Failure{}, false
	}
	out := Failure{Code: invalid.Code, Kind: FailureInput}
	switch invalid.Code {
	case "decision.snapshot_not_found":
		out.Kind = FailureMissing
	case "decision.snapshot_expired":
		out.Kind = FailureExpired
	case "decision.snapshot_corrupt", "decision.store_open_failed", "decision.store_read_failed", "decision.store_version_unsupported":
		out.Kind = FailureStorage
	case "decision.report_output_limit":
		out.Kind = FailureLimit
	case "decision.report_input_limit":
		out.Kind = FailureTooLarge
	}
	return out, true
}
