package serve

import (
	"context"
	"errors"
	"io"
	"net/http"

	"reasonix/internal/session/control"
	"reasonix/internal/tools/decision"
)

func (s *Server) deliverDecision(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 256*1024))
	if err != nil {
		var large *http.MaxBytesError
		if errors.As(err, &large) {
			refuse(w, http.StatusRequestEntityTooLarge, "decision.report_input_limit", "decision request exceeds the size limit", nil)
		} else {
			refuse(w, http.StatusBadRequest, "decision.request_read_failed", "could not read decision request", nil)
		}
		return
	}
	s.bindMu.Lock()
	defer s.bindMu.Unlock()
	result, err := s.ctl().DeliverDecision(r.Context(), raw)
	if err == nil {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write(result)
		return
	}
	if r.Context().Err() != nil {
		return
	}
	if failure, ok := decision.ReportFailure(err); ok {
		refuse(w, decisionFailureStatus(failure.Kind), failure.Code, "decision report could not be generated", nil)
		return
	}
	switch {
	case errors.Is(err, control.ErrDecisionUnavailable):
		refuse(w, http.StatusServiceUnavailable, "decision.unavailable", "decision reporting is not enabled", nil)
	case errors.Is(err, control.ErrRuntimeDraining):
		refuse(w, http.StatusConflict, "decision.runtime_changed", "runtime changed; retry on the active session", nil)
	case errors.Is(err, context.DeadlineExceeded):
		refuse(w, http.StatusGatewayTimeout, "decision.timeout", "decision report timed out", nil)
	default:
		refuse(w, http.StatusInternalServerError, "decision.failed", "decision report failed", nil)
	}
}

func decisionFailureStatus(kind decision.FailureKind) int {
	switch kind {
	case decision.FailureMissing:
		return http.StatusNotFound
	case decision.FailureExpired:
		return http.StatusGone
	case decision.FailureStorage:
		return http.StatusServiceUnavailable
	case decision.FailureLimit:
		return http.StatusUnprocessableEntity
	case decision.FailureTooLarge:
		return http.StatusRequestEntityTooLarge
	default:
		return http.StatusBadRequest
	}
}
