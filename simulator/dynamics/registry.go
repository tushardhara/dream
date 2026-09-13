// Package dynamics implements bounded synthetic research hypotheses. These are
// not psychological facts, diagnoses, fixed personality labels or core identity.
package dynamics

import "github.com/tushardhara/dream/core"

const Version = 1
const ModelVersion = "appraisal.v1"
const RegistryVersion = "ticket7-subset.v1"
const RegistryCompleteness = "LEGACY SUBSET: complete owner-supplied registry is simulator/drives hws-section6.v1"
const Count = 6
const Hour core.LogicalTime = 3600 * 1e9
const (
	Fatigue = iota
	ScarcityOpportunity
	Care
	Status
	Belonging
	SlowResidue
)

type Definition struct {
	ID       core.ID
	Name     string
	HalfLife core.LogicalTime
	Gain     float64
	Baseline float64
}

// Only the concepts explicitly named by #7. Array order is a wire contract;
// adding/reordering entries requires a registry AND codec version migration.
var definitions = [Count]Definition{
	{"fatigue", "fatigue", 8 * Hour, .25, .2},
	{"scarcity_opportunity", "scarcity/opportunity", 12 * Hour, .3, .3},
	{"care", "care", 24 * Hour, .25, .5},
	{"status", "status", 12 * Hour, .25, .3},
	{"belonging", "belonging", 24 * Hour, .25, .5},
	{"slow_residue", "slow residue", 168 * Hour, .08, .1},
}

func Registry() [Count]Definition { return definitions }
