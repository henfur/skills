package core

import "testing"

// A kind in the catalog without its behaviour would quietly act as a single-agent task.
func TestEveryKindHasItsBehaviour(t *testing.T) {
	for _, k := range Kinds {
		if _, ok := behaviours[k.ID]; !ok {
			t.Errorf("kind %q has no Behaviour in core/kinds.go", k.ID)
		}
	}
	if len(behaviours) != len(Kinds) {
		t.Errorf("%d behaviours for %d kinds in the catalog", len(behaviours), len(Kinds))
	}
}
