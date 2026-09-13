package simulator

import "github.com/tushardhara/dream/core"

// EdgeContext is an observer-attributed, evidence-bound set of modifiers. It has
// no aggregate health, owner, action, or participation-to-care conversion.
// #11 consumes these named uncertain signals; #12 owns model-view policy.
type EdgeContext struct {
	Observer   core.ID
	Relation   core.ID
	Source     core.ID
	Types      []core.ID
	Dimensions []EdgeDimension
	Roles      []core.ID
	Patterns   []EdgePattern
	OpenLoops  []core.ID
}
type EdgeDimension struct {
	Name       core.ID
	Value      float64
	Confidence core.Confidence
}

type EdgePattern struct {
	Name       core.ID
	Confidence core.Confidence
	Evidence   []core.ID
}
