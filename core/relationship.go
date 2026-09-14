package core

import (
	"fmt"
	"math"
)

// RelationshipContext is one observer's account, not an edge-wide truth. Detail
// IDs reference separately attributed claims/memories; they do not grant access.
// An absent measure is unknown. Types never supply numerical defaults.
type RelationshipContext struct {
	Version  int                   `json:"version"`
	Observer ID                    `json:"observer"`
	Other    ID                    `json:"other"`
	Types    []ID                  `json:"types"`
	Valid    Interval              `json:"valid"`
	Details  []RelationshipDetail  `json:"details,omitempty"`
	Measures []RelationshipMeasure `json:"measures,omitempty"`
}
type RelationshipDetail struct {
	Kind    ID   `json:"kind"`
	Sources []ID `json:"sources"`
}
type RelationshipMeasure struct {
	Kind       ID         `json:"kind"`
	Value      float64    `json:"value"`
	Confidence Confidence `json:"confidence"`
	Source     ID         `json:"source"`
}

func (r RelationshipContext) Validate(observer, other ID) error {
	if r.Version != 1 || observer.Validate() != nil || other.Validate() != nil || observer == other || r.Observer != observer || r.Other != other || r.Valid.Validate() != nil || len(r.Types) < 1 || len(r.Types) > 8 || len(r.Details) > 14 || len(r.Measures) > 13 {
		return fmt.Errorf("invalid relationship context")
	}
	seen := map[ID]bool{}
	for _, t := range r.Types {
		if t.Validate() != nil || seen[t] {
			return fmt.Errorf("invalid relationship types")
		}
		seen[t] = true
	}
	seen = map[ID]bool{}
	for _, d := range r.Details {
		switch d.Kind {
		case "origin", "history", "view_of_relationship", "view_of_other", "communication", "positive_patterns", "friction_patterns", "significant_memories", "open_loops", "commitments", "recent_events", "current_state", "trajectory", "hypotheses":
		default:
			return fmt.Errorf("unknown relationship detail")
		}
		if seen[d.Kind] || len(d.Sources) < 1 || len(d.Sources) > 8 {
			return fmt.Errorf("relationship detail bound")
		}
		seen[d.Kind] = true
		ids := map[ID]bool{}
		for _, id := range d.Sources {
			if id.Validate() != nil || ids[id] {
				return fmt.Errorf("invalid relationship detail source")
			}
			ids[id] = true
		}
	}
	seen = map[ID]bool{}
	for _, m := range r.Measures {
		switch m.Kind {
		case "trust", "closeness", "expectation", "sensitivity", "fear", "pride", "shame", "expected_reaction", "protective_intent", "social_norm", "stress", "prior_outcome", "friction":
		default:
			return fmt.Errorf("unknown relationship measure")
		}
		if seen[m.Kind] || m.Source.Validate() != nil || m.Confidence.Validate() != nil || math.IsNaN(m.Value) || math.IsInf(m.Value, 0) || m.Value < -1 || m.Value > 1 {
			return fmt.Errorf("invalid relationship measure")
		}
		seen[m.Kind] = true
	}
	return nil
}

// Sources preserves references for the host's existing permission/lineage gate.
func (r RelationshipContext) Sources() []ID {
	out := []ID{}
	seen := map[ID]bool{}
	add := func(id ID) {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	for _, d := range r.Details {
		for _, id := range d.Sources {
			add(id)
		}
	}
	for _, m := range r.Measures {
		add(m.Source)
	}
	return out
}
