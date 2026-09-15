package demo

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/behavior"
	"github.com/tushardhara/dream/simulator/drives"
	"github.com/tushardhara/dream/simulator/dynamics"
	"github.com/tushardhara/dream/simulator/scenario"
)

const GroupVersion = "group-demo.v1"

type GroupNativeInput struct {
	Version  string
	Scenario scenario.Scenario
	History  core.GroupHistory
	Decision core.ID
	At       core.LogicalTime
	Theme    string
}
type GroupNativeTrace struct {
	Actor        core.ID
	DyadicHash   string
	OwnAgreement bool
	Group        core.GroupPerspective
	Signals      dynamics.Signals
	Present      []core.ID
	Decision     behavior.ScopedDecision
}
type GroupNativeRun struct {
	Version, HumanValidity string
	Theme                  string
	Traces                 []GroupNativeTrace
	Final                  []behavior.ScopedActor
	Responses              []behavior.RecipientDecision
}

func groupOwnGrants(actor core.ID) []core.Grant {
	return []core.Grant{{Actor: actor, Recipient: actor, Purpose: "simulation", Operation: core.Read}, {Actor: actor, Recipient: actor, Purpose: "simulation", Operation: core.Derive}}
}

// groupSignals deliberately replaces the theme's social inclusion/exclusion.
// Unknown effort/burden stays unknown in GroupPerspective and contributes no
// fabricated scalar observation. Only observed own evidence activates a cue.
func groupSignals(base dynamics.Signals, p core.GroupPerspective) dynamics.Signals {
	base.Inclusion, base.Exclusion = 0, 0
	if p.Inclusion.Status == core.Observed {
		base.Inclusion = *p.Inclusion.Value
	}
	if p.Exclusion.Status == core.Observed {
		base.Exclusion = *p.Exclusion.Value
	}
	if p.Effort.Status == core.Observed {
		base.Effort = *p.Effort.Value / 10
		if base.Effort > 1 {
			base.Effort = 1
		}
	}
	if p.Burden.Status == core.Observed {
		base.Scarcity = *p.Burden.Value / 10
		if base.Scarcity > 1 {
			base.Scarcity = 1
		}
	}
	return base
}
func groupSituation(v scenario.ActorView, peer core.ID, focus core.RelationshipFocus, ev dynamics.Perceived, present []core.ID, at core.LogicalTime, resources map[core.ID]int64) (behavior.ActionSituation, error) {
	s := behavior.ActionSituation{Observation: drives.Observation{Event: ev}, Sources: []core.ID{ev.Event}, Present: present, Resources: resources, Horizon: at + 100}
	for i := range s.Observation.Context {
		unknown := ev
		unknown.Confidence = 0
		unknown.Signals = dynamics.Signals{}
		s.Observation.Context[i] = drives.Cue{Evidence: unknown}
	}
	selected, e := core.SelectRelationship(v.Contexts, focus, v.Actor, peer, at)
	if e != nil {
		return s, e
	}
	if selected.Status != "selected" {
		return s, nil
	}
	r := *selected.Account
	for _, id := range append(r.Sources(), r.Account) {
		found := false
		for _, f := range v.Facts {
			if f.ID != id {
				continue
			}
			rights := core.Rights{Resource: id, Grants: f.Grants}
			allowed := true
			for _, g := range groupOwnGrants(v.Actor) {
				allowed = allowed && rights.Allows(core.PermissionRequest{Resource: id, Context: g})
			}
			if allowed {
				found = true
				s.Sources = append(s.Sources, id)
				s.RelationshipEvidence = append(s.RelationshipEvidence, perceived(v.Actor, id, 0, f.Confidence, dynamics.Signals{}))
			}
		}
		if !found {
			return s, fmt.Errorf("unpermitted group dyadic context")
		}
	}
	return behavior.ApplyDomainRelationship(s, r, v.Actor, at)
}

// RunGroups is an opt-in native-engine group demonstration. Legacy v1/v2/v3
// reconstruct/replay is unchanged. Each actor sees only their own permitted
// group accounts; private member state and research labels never enter a peer's
// appraisal. Group history influences the existing v2 human action machinery.
func RunGroups(in GroupNativeInput, recorded *GroupNativeRun) (GroupNativeRun, error) {
	out := GroupNativeRun{Version: GroupVersion, HumanValidity: "NOT_TESTED", Theme: in.Theme, Traces: []GroupNativeTrace{}, Final: []behavior.ScopedActor{}, Responses: []behavior.RecipientDecision{}}
	if in.Version != GroupVersion || in.Scenario.Validate() != nil || in.History.Validate() != nil || in.At < 0 || in.At > in.Scenario.World.Horizon-100 || len(in.Scenario.Public.Humans) != 5 && len(in.Scenario.Public.Humans) != 24 {
		return out, fmt.Errorf("invalid native group scenario")
	}
	var decision core.GroupDecision
	for _, d := range in.History.Decisions {
		if d.ID == in.Decision {
			decision = d
		}
	}
	if decision.ID == "" || decision.At > in.At {
		return out, fmt.Errorf("missing current group decision")
	}
	base, e := signal(in.Theme)
	if e != nil {
		return out, e
	}
	views, e := in.Scenario.ActorViews()
	if e != nil {
		return out, e
	}
	vm := map[core.ID]scenario.ActorView{}
	for _, v := range views {
		vm[v.Actor] = v
	}
	resources := map[core.ID]int64{}
	for _, r := range in.Scenario.Public.Resources {
		resources[r.ID] = r.Available
	}
	for _, person := range in.Scenario.Public.Humans {
		owner := person.ID
		v := vm[owner]
		for _, g := range groupOwnGrants(owner) {
			if !decision.Meta.Rights.Allows(core.PermissionRequest{Resource: decision.Meta.ID, Context: g}) {
				return out, fmt.Errorf("unpermitted published group decision")
			}
		}
		if decision.Meta.Valid.End != nil && in.At >= *decision.Meta.Valid.End {
			return out, fmt.Errorf("expired published group decision")
		}
		p, e := core.GroupPerspectiveFor(in.History, decision.Group, decision.Focus, owner, in.At, groupOwnGrants(owner))
		if e != nil {
			return out, e
		}
		sourceHash, _ := digest(p)
		source := core.ID("group-observation:" + sourceHash[:24])
		sig := groupSignals(base, p)
		confidence := core.Confidence(0)
		if len(p.Evidence) > 0 {
			confidence = .8
		}
		ev := perceived(owner, source, in.At, confidence, sig)
		present := append([]core.ID{}, p.CoPresent...)
		found := false
		for _, id := range present {
			found = found || id == owner
		}
		if !found {
			present = append(present, owner)
		}
		peer := core.ID("")
		for _, c := range v.Contexts {
			if c.Other != owner {
				peer = c.Other
				break
			}
		}
		if peer == "" {
			return out, fmt.Errorf("group fixture missing fixed dyadic peer")
		}
		s, e := groupSituation(v, peer, decision.Focus, ev, present, in.At, resources)
		if e != nil {
			return out, e
		}
		// A repeated practice is evidence, not a scripted group/theme label. Dispute
		// is the observer's explicit stance, never a vote or population ranking.
		cue := 0.0
		for _, pattern := range p.Patterns {
			if pattern.Kind == "repeated_participation" {
				cue = .6
			}
			if pattern.Stance == "disputes" {
				cue = .1
				break
			}
		}
		s.Observation.Context[drives.Setting] = drives.Cue{Evidence: ev, Value: cue}
		ownAgreement := false
		ownAccounts, e := core.CurrentGroupAccounts(in.History, decision.Group, decision.Focus, in.At, groupOwnGrants(owner))
		if e != nil {
			return out, e
		}
		for _, a := range ownAccounts {
			if a.Member == owner && a.Decision == decision.ID && a.Phase == "planning" && len(a.ApprovedOptions) > 0 && a.Capacity.Status == core.Observed {
				ownAgreement = true
			}
		}
		for _, kind := range []behavior.Kind{behavior.Coordinate, behavior.Invite, behavior.Decline} {
			if kind == behavior.Coordinate && !ownAgreement {
				continue
			}
			s.Offers = append(s.Offers, behavior.ActionOffer{Kind: kind, Recipient: peer, Duration: 1, Evidence: []core.ID{source}})
		}
		a, e := behavior.NewScopedActor(owner, 0)
		if e != nil {
			return out, e
		}
		scope := core.InteractionScope{Version: core.InteractionScopeVersion, Initiator: owner, Target: peer, Topic: core.ID(decision.Focus.Domain), Class: core.Coordination}
		pref, prefSource, e := responsePreference(v)
		if e != nil {
			return out, e
		}
		peerPref, peerSource, e := responsePreference(vm[peer])
		if e != nil {
			return out, e
		}
		boundaries := []core.Boundary{responseBoundary(scope, owner, pref, prefSource, a), responseBoundary(scope, peer, peerPref, peerSource, behavior.ScopedActor{})}
		// These are explicit fictional action-participation settings, held fixed
		// across group-history interventions; no attendance/role label grants consent.
		seed := sha256.Sum256([]byte(fmt.Sprintf("group-human.v1/%d/%s", in.Scenario.World.Seed, owner)))
		draw := binary.BigEndian.Uint64(seed[:8])
		next, d, e := behavior.ChooseScopedAction(a, behavior.ScopedSituation{Scope: scope, Situation: s, Boundaries: boundaries}, in.At, draw)
		if e != nil {
			return out, fmt.Errorf("group choice %s: %w", owner, e)
		}
		dyadicHash, _ := digest(v.Contexts)
		out.Traces = append(out.Traces, GroupNativeTrace{Actor: owner, DyadicHash: dyadicHash, OwnAgreement: ownAgreement, Group: p, Signals: sig, Present: present, Decision: d})
		out.Final = append(out.Final, next)
		// A separately observed group invitation can receive an independent native
		// recipient appraisal; no positive quota or success label is imposed.
		invited := false
		for _, id := range decision.Invited {
			invited = invited || id == owner
		}
		if owner == decision.Meta.Observer || !invited {
			continue
		}
		sender := decision.Meta.Observer
		rs, e := groupSituation(v, sender, decision.Focus, ev, present, in.At, resources)
		if e != nil {
			return out, e
		}
		c := behavior.DisclosureContext{Observer: owner, Recipient: sender}
		for _, x := range rs.Contexts {
			if x.Recipient == sender {
				c = x
			}
		}
		received := perceived(owner, core.ID("group-delivery:"+sourceHash[:24]), in.At+2, confidence, dynamics.Signals{})
		input := behavior.RecipientInput{Delivery: behavior.ReceivedAction{Interaction: decision.ID, Decision: decision.Meta.ID, Sender: sender, Recipient: owner, Kind: core.ID(behavior.Invite), EffectAt: decision.At, Source: received}, Focus: decision.Focus, Context: c, Current: rs.RelationshipEvidence, Availability: "observed", Phase: "immediate"}
		seen := true
		for _, g := range groupOwnGrants(owner) {
			seen = seen && decision.Meta.Rights.Allows(core.PermissionRequest{Resource: decision.Meta.ID, Context: g})
		}
		if !seen || p.Participation == "unknown" || p.Participation == "absent" {
			input.Availability = "lost_observation"
		}
		response, e := behavior.RespondToAction(next.Human, input, in.At+2, responseDraw(draw, decision.ID, "group"))
		if e != nil {
			return out, e
		}
		out.Responses = append(out.Responses, response)
	}
	if recorded != nil {
		a, _ := digest(out)
		b, _ := digest(*recorded)
		if a != b {
			return out, fmt.Errorf("group replay changed under current evidence/rights")
		}
	}
	return out, nil
}
