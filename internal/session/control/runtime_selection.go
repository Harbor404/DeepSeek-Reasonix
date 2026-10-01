package control

import "strings"

// RuntimeSelection is the model and session-visible effort selection a
// controller was built with. Both values are immutable for a controller
// generation; a different selection is what requires a rebuild today.
type RuntimeSelection struct {
	ModelRef string
	Effort   string
}

// RuntimeSelectionMatcher is the controller capability frontends use to skip a
// rebuild when a resolved target is already the running generation.
type RuntimeSelectionMatcher interface {
	MatchesRuntimeSelection(modelRef, effort string) bool
}

// RuntimeSelection returns the immutable model/effort identity of this
// controller generation.
func (c *Controller) RuntimeSelection() RuntimeSelection {
	if c == nil {
		return RuntimeSelection{}
	}
	return RuntimeSelection{
		ModelRef: strings.TrimSpace(c.modelRef),
		Effort:   normalizeRuntimeEffort(c.effort),
	}
}

// MatchesRuntimeSelection reports whether modelRef and effort name the running
// controller generation. The caller passes a canonical model ref and the value
// the provider capability resolved for effort; this method owns the shared
// no-op comparison used by ACP, serve, and the CLI path through serve.
func (c *Controller) MatchesRuntimeSelection(modelRef, effort string) bool {
	current := c.RuntimeSelection()
	if current.ModelRef == "" || strings.TrimSpace(modelRef) == "" {
		return false
	}
	return current.ModelRef == strings.TrimSpace(modelRef) &&
		current.Effort == normalizeRuntimeEffort(effort)
}

var _ RuntimeSelectionMatcher = (*Controller)(nil)

func normalizeRuntimeEffort(effort string) string {
	effort = strings.ToLower(strings.TrimSpace(effort))
	if effort == "" || effort == "auto" {
		return "auto"
	}
	return effort
}
