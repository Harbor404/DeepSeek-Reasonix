package boot

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"slices"

	"reasonix/internal/contract/tool"
	"reasonix/internal/session/control"
	"reasonix/internal/tools/decision"
)

func registerDecisionTools(reg *tool.Registry, enabled []string) {
	addDecisionTools(reg, enabled, decision.NewRuntime())
}

func decisionReporter(reg *tool.Registry) control.DecisionReporter {
	capability, ok := reg.Get("decision_deliver")
	if !ok {
		return nil
	}
	reporter, _ := capability.(control.DecisionReporter)
	return reporter
}

func registerPersistentDecisionTools(reg *tool.Registry, enabled []string, sessionDir, root string) {
	digest := sha256.Sum256([]byte(filepath.Clean(root)))
	path := filepath.Join(sessionDir, "decisions", hex.EncodeToString(digest[:]), "snapshots.sqlite")
	addDecisionTools(reg, enabled, decision.NewPersistentRuntime(path))
}

func addDecisionTools(reg *tool.Registry, enabled []string, tools []tool.Tool) {
	for _, capability := range tools {
		if len(enabled) == 0 || slices.Contains(enabled, capability.Name()) {
			reg.Add(capability)
		}
	}
}
