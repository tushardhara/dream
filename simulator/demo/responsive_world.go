package demo

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"time"

	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/behavior"
	"github.com/tushardhara/dream/simulator/drives"
	"github.com/tushardhara/dream/simulator/dynamics"
	rt "github.com/tushardhara/dream/simulator/runtime"
	"github.com/tushardhara/dream/simulator/scenario"
)

func responseFocus(owner, peer core.ID) core.RelationshipFocus {
	return core.RelationshipFocus{Version: core.RelationshipFocusVersion, Domain: core.PracticalCoordination, RoleContext: "everyday", Account: core.ID("response-frame:" + string(owner) + ":" + string(peer))}
}

// One canonical draw per actor/period stays within the runtime's existing draw
// budget. Independent named subdraws bind every appraisal to the recorded draw,
// action identity and phase. This derivation is part of backend-demo.v3.
func responseDraw(draw uint64, action core.ID, phase string) uint64 {
	b, _ := json.Marshal(struct {
		Draw   uint64
		Action core.ID
		Phase  string
	}{draw, action, phase})
	h := sha256.Sum256(b)
	return binary.BigEndian.Uint64(h[:8])
}
func ownOutcomes(w World, owner core.ID) []core.OutcomeObservation {
	out := []core.OutcomeObservation{}
	for _, o := range w.OutcomeObservations {
		if o.Meta.Observer == owner {
			out = append(out, o)
		}
	}
	return out
}
func appendResponse(w *World, o core.OutcomeObservation) error {
	if _, e := core.AppendOutcome(ownOutcomes(*w, o.Meta.Observer), o); e != nil {
		return e
	}
	w.OutcomeObservations = append(w.OutcomeObservations, o)
	return nil
}
func responseBoundary(scope core.InteractionScope, principal core.ID, p ResponsePreference, source core.ID, actor behavior.ScopedActor) core.Boundary {
	other := scope.Target
	if principal == scope.Target {
		other = scope.Initiator
	}
	willingness := core.Declined
	if scope.Class == core.Discussion && p.Discussion || scope.Class == core.Coordination && p.Coordination {
		willingness = core.Willing
	}
	at := core.LogicalTime(0)
	for _, c := range actor.Contacts {
		if c.With == other && (c.Topic == scope.Topic || c.Topic == core.AllTopics) && c.State != "engaged" {
			willingness = core.Declined
			source = c.Evidence
			at = c.At
		}
	}
	id := core.ID("preference:" + string(principal) + ":" + string(other) + ":" + string(scope.Class))
	return core.Boundary{Version: core.BoundaryVersion, Meta: core.Metadata{ID: id, Observer: principal, Source: principal, Sensitivity: core.Restricted, RecordedAt: time.Unix(1, 0).UTC(), Confidence: 1, Supporting: []core.ID{source}, Rights: core.Rights{Resource: id}}, Principal: principal, With: other, Topic: scope.Topic, Class: scope.Class, Decision: willingness, Basis: "self_report", OccurredAt: at, LearnedAt: at}
}
func responseSituation(v scenario.ActorView, peer core.ID, at core.LogicalTime, event dynamics.Perceived, present []core.ID, resources map[core.ID]int64, horizon core.LogicalTime) (behavior.ActionSituation, error) {
	s := behavior.ActionSituation{Observation: drives.Observation{Event: event}, Sources: []core.ID{event.Event}, Present: present, Resources: resources, Horizon: at + 1000000000}
	if s.Horizon > horizon {
		s.Horizon = horizon
	}
	for i := range s.Observation.Context {
		unknown := event
		unknown.Confidence = 0
		unknown.Signals = dynamics.Signals{}
		s.Observation.Context[i] = drives.Cue{Evidence: unknown}
	}
	for _, r := range v.Contexts {
		if r.Other != peer {
			continue
		}
		for _, id := range append(r.Sources(), r.Account) {
			found := false
			for _, f := range v.Facts {
				if f.ID == id && permittedFacts(v)[id] {
					found = true
					s.Sources = append(s.Sources, id)
					s.RelationshipEvidence = append(s.RelationshipEvidence, perceived(v.Actor, id, 0, f.Confidence, dynamics.Signals{}))
				}
			}
			if !found {
				return s, fmt.Errorf("missing own relationship evidence")
			}
		}
		selected, e := core.SelectRelationship([]core.RelationshipContext{r}, responseFocus(v.Actor, peer), v.Actor, peer, at)
		if e != nil || selected.Status != "selected" {
			return s, fmt.Errorf("missing explicit response domain account")
		}
		return behavior.ApplyDomainRelationship(s, *selected.Account, v.Actor, at)
	}
	return s, nil // no peer context is unknown; never invent an expectation
}
func responseInput(s behavior.ActionSituation, d Delivery, owner core.ID, at core.LogicalTime, availability, phase string) behavior.RecipientInput {
	context := behavior.DisclosureContext{Observer: owner, Recipient: d.Sender}
	for _, c := range s.Contexts {
		if c.Recipient == d.Sender {
			context = c
		}
	}
	evidence := perceived(owner, core.ID("delivery-observation:"+string(owner)+":"+string(d.Decision)+":"+phase), at, .8, dynamics.Signals{})
	return behavior.RecipientInput{Delivery: behavior.ReceivedAction{Interaction: core.ID("interaction:" + string(d.Decision)), Decision: d.Decision, Sender: d.Sender, Recipient: owner, Kind: core.ID(d.Kind), EffectAt: d.At, Source: evidence}, Focus: responseFocus(owner, d.Sender), Context: context, Current: s.RelationshipEvidence, Availability: availability, Phase: phase}
}

// This version keeps a bounded inbox and observer ledgers. Only an actor's own
// appraisal can train its private memory; neither delivery nor a friendly reply
// supplies the sender with another person's private experience.
func reconstructResponsive(sc scenario.Scenario, through int, domain, common core.ID, coupled bool, record *rt.Random) (World, error) {
	n := len(sc.Public.Humans)
	if sc.Validate() != nil || (n != 2 && n != 4 && n != 5 && n != 24) || through < 0 || through > len(sc.Future) || len(sc.Future) > 24 {
		return World{}, fmt.Errorf("invalid responsive projection bound")
	}
	w := World{Version: ResponsiveVersion, Resources: map[core.ID]int64{}, Pending: []Delivery{}}
	views, e := sc.ActorViews()
	if e != nil {
		return w, e
	}
	viewMap := map[core.ID]scenario.ActorView{}
	prefs := map[core.ID]ResponsePreference{}
	prefSources := map[core.ID]core.ID{}
	indices := map[core.ID]int{}
	for _, v := range views {
		viewMap[v.Actor] = v
	}
	for _, r := range sc.Public.Resources {
		w.Resources[r.ID] = r.Available
	}
	for i, h := range sc.Public.Humans {
		v, ok := viewMap[h.ID]
		if !ok {
			return w, fmt.Errorf("missing response actor")
		}
		p, source, e := responsePreference(v)
		if e != nil {
			return w, e
		}
		prefs[h.ID] = p
		prefSources[h.ID] = source
		indices[h.ID] = i
		a, e := behavior.NewScopedActor(h.ID, 0)
		if e != nil {
			return w, e
		}
		w.ScopedActors = append(w.ScopedActors, a)
		w.ActionActors = append(w.ActionActors, a.Human)
	}
	// Each own evidence set has at most 24 delivery + 48 outcome records and
	// genesis sources. It is never sent wholesale into the 16-source choice port.
	proofs := map[core.ID][]dynamics.Perceived{}
	later := map[core.ID]Delivery{}
	rng := rt.NewScopedRandom(sc.World.Seed, nil, domain, common, coupled)
	for p := 0; p < through; p++ {
		event := sc.Future[p]
		var period Period
		if json.Unmarshal([]byte(event.Text), &period) != nil || period.Version != ResponsiveVersion || period.Index != p+1 {
			return w, fmt.Errorf("invalid responsive period")
		}
		sig, e := signal(period.Theme)
		if e != nil {
			return w, e
		}
		members := []core.ID{}
		for _, g := range sc.Public.Groups {
			if g.ID == period.Group {
				members = g.Members
			}
		}
		if len(members) == 0 {
			return w, fmt.Errorf("unknown response venue")
		}
		pending := []Delivery{}
		consumed := map[core.ID]bool{}
		for ai, stored := range w.ScopedActors {
			owner := stored.Human.Drives.Actor
			v := viewMap[owner]
			present := []core.ID{owner}
			signals := dynamics.Signals{}
			for _, id := range members {
				if id == owner {
					present = append([]core.ID{}, members...)
					signals = sig
				}
			}
			var incoming *Delivery
			for i := range w.Pending {
				d := w.Pending[i]
				if d.Recipient == owner && d.At <= event.At {
					copy := d
					incoming = &copy
					consumed[d.Decision] = true
					break
				}
			}
			peer := core.ID("")
			if incoming != nil {
				peer = incoming.Sender
				found := false
				for _, id := range present {
					found = found || id == peer
				}
				if !found {
					present = append(present, peer)
				}
			}
			if peer == "" {
				for j := 0; j < len(v.Contexts); j++ {
					r := v.Contexts[(p+j)%len(v.Contexts)]
					for _, id := range present {
						if id == r.Other {
							peer = id
							break
						}
					}
					if peer != "" {
						break
					}
				}
			}
			// No visible peer still permits only a native WAIT, using a known relationship
			// as the scoped target. Presence filtering prevents an invented delivery.
			if peer == "" && len(v.Contexts) > 0 {
				peer = v.Contexts[p%len(v.Contexts)].Other
			}
			if peer == "" {
				return w, fmt.Errorf("responsive fixture has no relationship")
			}
			ev := perceived(owner, core.ID(fmt.Sprintf("responsive-observation:%d:%s", p+1, owner)), event.At, .6, signals)
			s, e := responseSituation(v, peer, event.At, ev, present, w.Resources, sc.World.Horizon)
			if e != nil {
				return w, e
			}
			current := append([]dynamics.Perceived{}, proofs[owner]...)
			for _, f := range v.Facts {
				if permittedFacts(v)[f.ID] {
					current = append(current, perceived(owner, f.ID, 0, f.Confidence, dynamics.Signals{}))
				}
			}
			stored.Human.Memory, e = behavior.OutcomeLearning(ownOutcomes(w, owner), owner, responseFocus(owner, peer), event.At, current)
			if e != nil {
				return w, e
			}
			stored.Human.Beliefs = nil
			for _, m := range stored.Human.Memory {
				for _, id := range m.Evidence {
					for _, proof := range proofs[owner] {
						if proof.Event == id {
							s.Sources = append(s.Sources, id)
							s.RelationshipEvidence = append(s.RelationshipEvidence, proof)
						}
					}
				}
			}
			class := core.Discussion
			if p%2 == 0 {
				class = core.Coordination
			}
			kinds := []behavior.Kind{behavior.Say, behavior.Support, behavior.Decline, behavior.Challenge, behavior.Argue, behavior.Apologize, behavior.Ignore, behavior.Reconnect}
			if class == core.Coordination {
				kinds = []behavior.Kind{behavior.Invite, behavior.Coordinate, behavior.Help, behavior.Promise}
			}
			for _, kind := range kinds {
				o := behavior.ActionOffer{Kind: kind, Recipient: peer, Evidence: []core.ID{ev.Event}, Duration: 1}
				if kind == behavior.Help || kind == behavior.Promise {
					o.Resource = "shared-time"
					o.Units = 1
				}
				if kind == behavior.Promise {
					o.Commitment = core.ID(fmt.Sprintf("commitment:%d:%s", p+1, owner))
					o.Due = s.Horizon
				}
				s.Offers = append(s.Offers, o)
			}
			s.Offers = append(s.Offers, behavior.ActionOffer{Kind: behavior.Withdraw, Duration: 1}, behavior.ActionOffer{Kind: behavior.Leave, Duration: 1})
			for _, c := range w.Commitments {
				if c.Actor == owner && c.Recipient == peer && c.Status == "pending" {
					s.Commitments = append(s.Commitments, c)
					if class == core.Discussion {
						s.Offers = append(s.Offers, behavior.ActionOffer{Kind: behavior.BreakPromise, Recipient: peer, Commitment: c.ID, Evidence: []core.ID{ev.Event}, Duration: 1})
					}
				}
			}
			scope := core.InteractionScope{Version: core.InteractionScopeVersion, Initiator: owner, Target: peer, Topic: "everyday", Class: class}
			boundaries := []core.Boundary{responseBoundary(scope, owner, prefs[owner], prefSources[owner], stored), responseBoundary(scope, peer, prefs[peer], prefSources[peer], w.ScopedActors[indices[peer]])}
			stream := core.ID("responsive-choice:" + string(owner))
			draw, e := rng.Draw(stream)
			if e != nil {
				return w, e
			}
			if record != nil && p == through-1 {
				actual, e := record.Draw(stream)
				if e != nil || actual != draw {
					return w, fmt.Errorf("responsive RNG/history mismatch")
				}
			}
			next, decision, e := behavior.ChooseScopedAction(stored, behavior.ScopedSituation{Scope: scope, Situation: s, Boundaries: boundaries}, event.At, draw)
			if e != nil {
				return w, fmt.Errorf("responsive choice %s/%d: %w", owner, p+1, e)
			}
			d := decision.Human
			d.Stages[11] = "done"
			decision.Human = d
			selected := d.Candidates[d.Selected].Offer
			o, e := behavior.TrackAction(d, sc.World.Horizon)
			if e != nil {
				return w, e
			}
			w.ActionDecisions = append(w.ActionDecisions, d)
			w.ScopedDecisions = append(w.ScopedDecisions, decision)
			w.ActionOutcomes = append(w.ActionOutcomes, o)
			if selected.Kind == behavior.Help {
				w.Resources[selected.Resource] -= selected.Units
				if w.Resources[selected.Resource] < 0 {
					return w, fmt.Errorf("responsive overspend")
				}
			}
			if selected.Kind == behavior.Promise {
				w.Commitments = append(w.Commitments, behavior.Commitment{ID: selected.Commitment, Actor: owner, Recipient: peer, Resource: selected.Resource, Units: selected.Units, Due: selected.Due, Status: "pending"})
			}
			if selected.Kind == behavior.BreakPromise {
				for i := range w.Commitments {
					if w.Commitments[i].ID == selected.Commitment {
						w.Commitments[i].Status = "broken"
					}
				}
			}
			// A later appraisal is a distinct observation. It may disagree with the
			// immediate account; projection replaces its contribution, not its history.
			if old, ok := later[owner]; ok {
				ls, e := responseSituation(v, old.Sender, event.At, ev, present, w.Resources, sc.World.Horizon)
				if e != nil {
					return w, e
				}
				availability := prefs[owner].Observation
				if !prefs[owner].Followup {
					availability = "missing_followup"
				}
				input := responseInput(ls, old, owner, event.At, availability, "later")
				rd, e := behavior.RespondToAction(next.Human, input, event.At, responseDraw(draw, old.Decision, "later"))
				if e != nil {
					return w, e
				}
				if e = appendResponse(&w, rd.Observation); e != nil {
					return w, e
				}
				w.RecipientDecisions = append(w.RecipientDecisions, rd)
				proofs[owner] = append(proofs[owner], input.Delivery.Source, perceived(owner, rd.Observation.Meta.ID, event.At, rd.Observation.Meta.Confidence, dynamics.Signals{}))
				delete(later, owner)
			}
			if incoming != nil {
				input := responseInput(s, *incoming, owner, event.At, prefs[owner].Observation, "immediate")
				rd, e := behavior.RespondToAction(next.Human, input, event.At, responseDraw(draw, incoming.Decision, "immediate"))
				if e != nil {
					return w, e
				}
				if selected.Delivered() && selected.Recipient == incoming.Sender {
					rd.Observation.Reply = d.ID
				}
				if e = appendResponse(&w, rd.Observation); e != nil {
					return w, e
				}
				w.RecipientDecisions = append(w.RecipientDecisions, rd)
				proofs[owner] = append(proofs[owner], input.Delivery.Source, perceived(owner, rd.Observation.Meta.ID, event.At, rd.Observation.Meta.Confidence, dynamics.Signals{}))
				later[owner] = *incoming
			}
			if selected.Delivered() {
				reply := core.ID("")
				if incoming != nil && incoming.Sender == selected.Recipient {
					reply = incoming.Decision
				}
				pending = append(pending, Delivery{Decision: d.ID, ReplyTo: reply, Sender: owner, Recipient: selected.Recipient, Kind: selected.Kind, At: event.At + selected.Duration})
				// The sender can expect usefulness while the recipient privately experiences
				// pressure. This is an expectation, explicitly excluded from outcome learning.
				id := core.ID("sender-expectation:" + string(d.ID))
				appraisal := "unresolved"
				var benefit, burden *float64
				if next.Human.Private.Appraisal[1] > next.Human.Private.Appraisal[0] {
					appraisal = "supportive"
					b := .4
					c := .1
					benefit = &b
					burden = &c
				}
				expected := core.OutcomeObservation{Version: core.OutcomeObservationVersion, Meta: core.Metadata{ID: id, Observer: owner, Source: owner, Sensitivity: core.Restricted, Confidence: .4, RecordedAt: time.Unix(1, 0).UTC(), Supporting: []core.ID{ev.Event}, Rights: core.Rights{Resource: id, Grants: []core.Grant{{Actor: owner, Recipient: owner, Purpose: "simulation", Operation: core.Derive}, {Actor: owner, Recipient: owner, Purpose: "simulation", Operation: core.Read}}}}, Interaction: core.ID("interaction:" + string(d.ID)), Action: d.ID, Reply: reply, Participant: owner, Other: selected.Recipient, Focus: responseFocus(owner, selected.Recipient), Kind: "expected", Position: "sender", Phase: "immediate", Basis: "self_report", OccurredAt: event.At, LearnedAt: event.At, Status: core.Observed, Appraisal: appraisal, Benefit: benefit, Burden: burden}
				if e = appendResponse(&w, expected); e != nil {
					return w, e
				}
			}
			// Memory is projected for the exact focus at the next choice. Retaining an
			// unscoped copy here would leak learning into a different relationship frame.
			next.Human.Memory = nil
			next.Human.Beliefs = nil
			w.ScopedActors[ai] = next
			w.ActionActors[ai] = next.Human
		}
		carried := []Delivery{}
		for _, d := range w.Pending {
			if !consumed[d.Decision] {
				carried = append(carried, d)
			}
		}
		w.Pending = append(carried, pending...)
		if len(w.Pending) > 24*24 {
			return w, fmt.Errorf("responsive inbox bound")
		}
		w.Period = p + 1
		w.At = event.At
	}
	return w, nil
}
