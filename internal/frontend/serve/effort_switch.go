package serve

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"reasonix/internal/contract/config"
)

// switchEffort persists a new reasoning-effort level for the active provider and
// rebuilds via switchModel (which serializes on bindMu).
func (s *Server) switchEffort(ctx context.Context, level string) error {
	cur := s.ctl()
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	ref := currentModelRef(cur)
	entry, ok := cfg.ResolveModel(ref)
	if !ok {
		return refusal(http.StatusConflict, "effort.no_provider",
			fmt.Errorf("cannot resolve current provider %q", ref), nil)
	}
	// Refusals, not failures: an endpoint with no effort vocabulary and a level
	// outside the one it has are both answers about this request. Reporting
	// them as 500 told a user their machine had broken instead of what to do.
	capability := config.EffortCapabilityForEntry(entry)
	if !capability.Supported {
		return refusal(http.StatusBadRequest, "effort.not_configurable",
			fmt.Errorf("%s declares no reasoning-effort levels; give it one with reasoning_protocol or supported_efforts in the provider's config block", entry.Name),
			map[string]any{"provider": entry.Name})
	}
	effort, err := config.NormalizeEffort(entry, level)
	if err != nil {
		return refusal(http.StatusBadRequest, "effort.unsupported_level", err,
			map[string]any{"provider": entry.Name, "level": level, "levels": strings.Join(capability.Levels, " | ")})
	}
	targetRef := entry.Name + "/" + entry.Model
	if runtimeSelectionMatches(cur, targetRef, effort) {
		return nil
	}
	if controllerHasActiveRuntimeWork(cur) {
		return busyErr("busy.change_effort", "cannot change effort while active work or background jobs are running")
	}
	editPath := config.UserConfigPath()
	if editPath == "" {
		return fmt.Errorf("no config file found")
	}
	// Lock only the load-modify-save cycle; switchModel below rebuilds the
	// controller and must not hold the config edit lock.
	if err := func() error {
		unlock := config.LockUserConfigEdits()
		defer unlock()
		edit := config.LoadForEdit(editPath)
		if err := applyEffortEdit(edit, entry, effort); err != nil {
			return err
		}
		if err := edit.SaveTo(editPath); err != nil {
			return fmt.Errorf("save config: %w", err)
		}
		return nil
	}(); err != nil {
		return err
	}
	return s.switchModel(ctx, targetRef)
}

// applyEffortEdit writes effort onto entry within edit, mirroring CLI/desktop
// SetEffort: upsert the provider when the user config has no block for it yet.
// It writes nothing else — which request fields an endpoint accepts is the
// provider contract's call, not a side effect of selecting a level.
func applyEffortEdit(edit *config.Config, entry *config.ProviderEntry, effort string) error {
	if _, ok := edit.Provider(entry.Name); !ok {
		if err := edit.UpsertProvider(*entry); err != nil {
			return err
		}
	}
	return edit.SetProviderEffort(entry.Name, effort)
}
