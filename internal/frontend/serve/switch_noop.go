package serve

import (
	"context"
	"strings"

	"reasonix/internal/contract/config"
	"reasonix/internal/session/control"
)

// switchModelRequested is the user-facing model switch. A request that already
// names the running model returns before the busy guard and before any rebuild;
// internal callers that must refresh the same model keep using switchModel.
func (s *Server) switchModelRequested(ctx context.Context, ref string) error {
	s.bindMu.Lock()
	defer s.bindMu.Unlock()
	if runtimeModelMatches(s.ctl(), ref) {
		return nil
	}
	return s.switchModelLocked(ctx, ref)
}

func runtimeModelMatches(cur control.SessionAPI, ref string) bool {
	if cur == nil {
		return false
	}
	current := currentModelRef(cur)
	ref = strings.TrimSpace(ref)
	if current == "" || ref == "" {
		return false
	}
	if ref == current {
		return true
	}
	if ctrl, ok := cur.(*control.Controller); ok && ctrl != nil {
		if cfg, err := config.LoadForRootReadOnly(ctrl.WorkspaceRoot()); err == nil {
			if entry, ok := cfg.ResolveModel(ref); ok {
				ref = entry.Name + "/" + entry.Model
			}
		}
	}
	return ref == current
}

func runtimeSelectionMatches(cur control.SessionAPI, modelRef, effort string) bool {
	matcher, ok := cur.(control.RuntimeSelectionMatcher)
	return ok && matcher.MatchesRuntimeSelection(modelRef, effort)
}
