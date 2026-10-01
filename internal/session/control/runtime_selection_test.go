package control

import "testing"

func TestRuntimeSelectionMatchesCanonicalModelAndEffort(t *testing.T) {
	ctrl := New(Options{ModelRef: " provider/model ", Effort: " HIGH "})

	if !ctrl.MatchesRuntimeSelection("provider/model", "high") {
		t.Fatal("canonical model and normalized effort should match the running selection")
	}
	if ctrl.MatchesRuntimeSelection("other/model", "high") {
		t.Fatal("a different model must not match the running selection")
	}
	if ctrl.MatchesRuntimeSelection("provider/model", "low") {
		t.Fatal("a different effort must not match the running selection")
	}
}

func TestRuntimeSelectionTreatsEmptyAndAutoEffortAlike(t *testing.T) {
	ctrl := New(Options{ModelRef: "provider/model"})

	if !ctrl.MatchesRuntimeSelection("provider/model", "auto") {
		t.Fatal("auto should match the default empty effort selection")
	}
	if !ctrl.MatchesRuntimeSelection("provider/model", "") {
		t.Fatal("empty effort should match the default auto selection")
	}
}
