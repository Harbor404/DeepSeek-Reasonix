package control

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type decisionReportProbe struct{ calls int }

func (p *decisionReportProbe) Report(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
	p.calls++
	return raw, ctx.Err()
}

func TestDecisionReportControllerAdmission(t *testing.T) {
	probe := &decisionReportProbe{}
	ctrl := New(Options{DecisionReporter: probe})
	raw, err := ctrl.DeliverDecision(context.Background(), json.RawMessage(`{}`))
	if err != nil || string(raw) != "{}" || probe.calls != 1 {
		t.Fatal("controller did not forward explicit report request")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ctrl.DeliverDecision(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Fatal("cancelled request was admitted")
	}
	ctrl.Close()
	if _, err := ctrl.DeliverDecision(context.Background(), nil); !errors.Is(err, ErrDecisionUnavailable) || probe.calls != 1 {
		t.Fatal("closed controller admitted report")
	}
	without := New(Options{})
	defer without.Close()
	if _, err := without.DeliverDecision(context.Background(), nil); !errors.Is(err, ErrDecisionUnavailable) {
		t.Fatal("unconfigured report was admitted")
	}
}
