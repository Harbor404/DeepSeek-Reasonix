package control

import (
	"context"
	"encoding/json"
	"errors"
)

var ErrDecisionUnavailable = errors.New("decision reporting is unavailable")

type DecisionReporter interface {
	Report(context.Context, json.RawMessage) (json.RawMessage, error)
}

func (c *Controller) DeliverDecision(ctx context.Context, args json.RawMessage) (json.RawMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	c.mu.Lock()
	unavailable := c.gate.closed
	draining := c.rejectDrainingGenerationLocked()
	c.mu.Unlock()
	if draining {
		return nil, ErrRuntimeDraining
	}
	if unavailable || c.decisionReporter == nil {
		return nil, ErrDecisionUnavailable
	}
	return c.decisionReporter.Report(ctx, args)
}
