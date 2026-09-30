package decision

import (
	"context"
	"fmt"
	"testing"
)

func TestReportFailuresKeepProducerClassification(t *testing.T) {
	for _, test := range []struct {
		code string
		kind FailureKind
	}{
		{"decision.json_invalid", FailureInput},
		{"decision.snapshot_not_found", FailureMissing},
		{"decision.snapshot_expired", FailureExpired},
		{"decision.snapshot_corrupt", FailureStorage},
		{"decision.store_open_failed", FailureStorage},
		{"decision.store_read_failed", FailureStorage},
		{"decision.store_version_unsupported", FailureStorage},
		{"decision.report_output_limit", FailureLimit},
		{"decision.report_input_limit", FailureTooLarge},
	} {
		got, ok := ReportFailure(fmt.Errorf("wrapped: %w", &problem{Code: test.code}))
		if !ok || got.Code != test.code || got.Kind != test.kind {
			t.Fatalf("identity lost: %+v", got)
		}
	}
	if _, ok := ReportFailure(context.Canceled); ok {
		t.Fatal("context cancellation reclassified as input")
	}
}
