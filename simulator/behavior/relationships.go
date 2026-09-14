package behavior

import (
	"fmt"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/drives"
	"github.com/tushardhara/dream/simulator/dynamics"
)

// ApplyRelationship translates a currently permitted, observer-owned account
// into the common action engine's inputs. No role coefficient or permission is
// invented. For a focused interaction, it also supplies appraisal history and
// relationship cues. Multi-recipient callers select focus explicitly.
func ApplyRelationship(s ActionSituation, r core.RelationshipContext, actor core.ID, at core.LogicalTime, focus bool) (ActionSituation, error) {
	if r.Validate(actor, r.Other) != nil || at < r.Valid.Start || r.Valid.End != nil && at >= *r.Valid.End {
		return ActionSituation{}, fmt.Errorf("invalid or stale relationship perspective")
	}
	sources := map[core.ID]bool{}
	for _, id := range s.Sources {
		sources[id] = true
	}
	for _, id := range r.Sources() {
		if !sources[id] {
			return ActionSituation{}, fmt.Errorf("relationship source not permitted")
		}
	}
	for _, c := range s.Contexts {
		if c.Recipient == r.Other {
			return ActionSituation{}, fmt.Errorf("ambiguous relationship context")
		}
	}
	for _, m := range s.Relationships {
		if m.Other == r.Other {
			return ActionSituation{}, fmt.Errorf("ambiguous relationship memory")
		}
	}
	c := DisclosureContext{Observer: actor, Recipient: r.Other}
	measures := map[core.ID]ContextValue{}
	for _, m := range r.Measures {
		measures[m.Kind] = ContextValue{Value: m.Value, Confidence: m.Confidence, Evidence: m.Source}
	}
	c.Trust = measures["trust"]
	c.RoleExpectation = measures["expectation"]
	c.Sensitivity = measures["sensitivity"]
	c.Fear = measures["fear"]
	c.Pride = measures["pride"]
	c.Shame = measures["shame"]
	c.ExpectedReaction = measures["expected_reaction"]
	c.ProtectiveIntent = measures["protective_intent"]
	c.SocialNorm = measures["social_norm"]
	c.Stress = measures["stress"]
	c.PriorOutcome = measures["prior_outcome"]
	// Labels remain in the original account. The behavior input depends on the
	// attributed expectation, not a selected type when multiple roles coexist.
	s.Contexts = append(append([]DisclosureContext{}, s.Contexts...), c)
	refs := r.Sources()
	if len(refs) > 0 {
		trust := bounded(c.Trust.weighted() + .15*measures["closeness"].weighted() + .2*c.PriorOutcome.weighted() - .2*measures["friction"].weighted())
		s.Relationships = append(append([]Memory{}, s.Relationships...), Memory{Other: r.Other, Trust: trust, Disclosure: c.readiness(), Evidence: refs})
	}
	if focus {
		for _, pair := range []struct {
			slot int
			kind core.ID
		}{{drives.History, "prior_outcome"}, {drives.Relationships, "trust"}, {drives.Beliefs, "expected_reaction"}} {
			m := measures[pair.kind]
			if m.Evidence == "" {
				continue
			}
			p := s.Observation.Event
			p.Event = m.Evidence
			p.Confidence = m.Confidence
			p.Signals = dynamics.Signals{}
			p.Rights = core.Rights{Resource: m.Evidence, Grants: []core.Grant{{Actor: actor, Recipient: actor, Purpose: "simulation", Operation: core.Read}, {Actor: actor, Recipient: actor, Purpose: "simulation", Operation: core.Derive}}}
			// This is a trusted-host adapter over already permitted sources. It cannot
			// be used to authorize retrieval or disclosure of the original record.
			s.Observation.Context[pair.slot] = drives.Cue{Evidence: p, Value: (m.Value + 1) / 2}
		}
	}
	return s, nil
}
