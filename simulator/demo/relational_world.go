package demo

import (
	"encoding/json"
	"fmt"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/behavior"
	"github.com/tushardhara/dream/simulator/drives"
	"github.com/tushardhara/dream/simulator/dynamics"
	rt "github.com/tushardhara/dream/simulator/runtime"
	"github.com/tushardhara/dream/simulator/scenario"
)

func demoVersion(sc scenario.Scenario) string {
	if len(sc.Future) == 0 {
		return ""
	}
	var p Period
	if json.Unmarshal([]byte(sc.Future[0].Text), &p) != nil {
		return ""
	}
	return p.Version
}
func perceived(owner, source core.ID, at core.LogicalTime, confidence core.Confidence, sig dynamics.Signals) dynamics.Perceived {
	return dynamics.Perceived{Actor: owner, Event: source, OccurredAt: at, LearnedAt: at, Confidence: confidence, Signals: sig, Rights: core.Rights{Resource: source, Grants: []core.Grant{{Actor: owner, Recipient: owner, Purpose: "simulation", Operation: core.Read}, {Actor: owner, Recipient: owner, Purpose: "simulation", Operation: core.Derive}}}}
}
func permittedFacts(v scenario.ActorView) map[core.ID]bool {
	out := map[core.ID]bool{}
	for _, f := range v.Facts {
		out[f.ID] = (core.Rights{Resource: f.ID, Grants: f.Grants}).Allows(core.PermissionRequest{Resource: f.ID, Context: core.Grant{Actor: v.Actor, Recipient: v.Actor, Purpose: "simulation", Operation: core.Derive}})
	}
	return out
}

// Reconstruct uses only each actor's view and observed deliveries. It is bounded
// to 24 people x 24 periods, preserving all receipts and decisions. The persisted
// hash/period checkpoint binds this derived history to immutable genesis + RNG.
func reconstructRelational(sc scenario.Scenario, through int, domain, common core.ID, coupled bool, record *rt.Random) (World, error) {
	n := len(sc.Public.Humans)
	if sc.Validate() != nil || (n != 2 && n != 4 && n != 5 && n != 24) || through < 0 || through > len(sc.Future) || len(sc.Future) > 24 {
		return World{}, fmt.Errorf("invalid relational projection bound")
	}
	w := World{Version: RelationalVersion, Resources: map[core.ID]int64{}, Pending: []Delivery{}}
	views := []scenario.ActorView{}
	allViews, e := sc.ActorViews()
	if e != nil {
		return World{}, e
	}
	viewMap := map[core.ID]scenario.ActorView{}
	for _, v := range allViews {
		viewMap[v.Actor] = v
	}
	for _, r := range sc.Public.Resources {
		w.Resources[r.ID] = r.Available
	}
	for _, h := range sc.Public.Humans {
		view, ok := viewMap[h.ID]
		if !ok {
			return World{}, fmt.Errorf("missing actor view")
		}
		if len(view.Facts) == 0 || !permittedFacts(view)[view.Facts[0].ID] {
			return World{}, fmt.Errorf("initial relationship derivation not permitted")
		}
		a, e := behavior.NewActionActor(h.ID, 0)
		if e != nil {
			return World{}, e
		}
		w.ActionActors = append(w.ActionActors, a)
		views = append(views, view)
	}
	rng := rt.NewScopedRandom(sc.World.Seed, nil, domain, common, coupled)
	for p := 0; p < through; p++ {
		event := sc.Future[p]
		var period Period
		if json.Unmarshal([]byte(event.Text), &period) != nil || period.Version != RelationalVersion || period.Index != p+1 || event.Kind != "observation" {
			return World{}, fmt.Errorf("invalid relational period")
		}
		sig, e := signal(period.Theme)
		if e != nil {
			return World{}, e
		}
		members := []core.ID{}
		for _, g := range sc.Public.Groups {
			if g.ID == period.Group {
				members = g.Members
			}
		}
		if len(members) == 0 {
			return World{}, fmt.Errorf("unknown relational venue")
		}
		pending := []Delivery{}
		for ai, actor := range w.ActionActors {
			owner := actor.Drives.Actor
			view := views[ai]
			known := permittedFacts(view)
			present := []core.ID{owner}
			signals := dynamics.Signals{}
			for _, id := range members {
				if id == owner {
					present = members
					signals = sig
				}
			}
			for _, d := range w.Pending {
				if d.Recipient == owner && d.At <= event.At {
					switch d.Kind {
					case behavior.Support, behavior.Help:
						signals.Support = .7
					case behavior.Ask:
						signals.OtherNeed = .6
					case behavior.Invite:
						signals.Inclusion = .6
					}
				}
			}
			source := core.ID(fmt.Sprintf("relational-observation:%d:%s", p+1, owner))
			ev := perceived(owner, source, event.At, .6, signals)
			s := behavior.ActionSituation{Observation: drives.Observation{Event: ev}, Sources: []core.ID{source}, Present: present, Resources: w.Resources, Horizon: event.At + 1000000000}
			if s.Horizon > sc.World.Horizon {
				s.Horizon = sc.World.Horizon
			}
			for i := range s.Observation.Context {
				unknown := ev
				unknown.Confidence = 0
				unknown.Signals = dynamics.Signals{}
				s.Observation.Context[i] = drives.Cue{Evidence: unknown}
			}
			for _, f := range view.Facts {
				if known[f.ID] {
					s.Sources = append(s.Sources, f.ID)
					s.RelationshipEvidence = append(s.RelationshipEvidence, perceived(owner, f.ID, 0, f.Confidence, dynamics.Signals{}))
				}
			}
			visible := map[core.ID]bool{}
			for _, id := range present {
				visible[id] = true
			}
			contexts := []core.RelationshipContext{}
			for _, r := range view.Contexts {
				if visible[r.Other] {
					contexts = append(contexts, r)
				}
			}
			focus := core.ID("")
			if len(contexts) > 0 {
				focus = contexts[p%len(contexts)].Other
			}
			for _, r := range contexts {
				s, e = behavior.ApplyRelationship(s, r, owner, event.At, r.Other == focus)
				if e != nil {
					return World{}, e
				}
				for _, kind := range []behavior.Kind{behavior.Ask, behavior.Support, behavior.Invite, behavior.Decline, behavior.Help} {
					offer := behavior.ActionOffer{Kind: kind, Recipient: r.Other, Evidence: []core.ID{source}, Duration: 1}
					if kind == behavior.Help {
						offer.Resource = "shared-time"
						offer.Units = 1
					}
					s.Offers = append(s.Offers, offer)
				}
			}
			// Overlapping group membership is perceived context only when this actor is
			// present; it never reveals the venue's event to an outsider.
			if len(present) > 1 {
				s.Observation.Context[drives.Setting] = drives.Cue{Evidence: ev, Value: .6}
			}
			if focus != "" {
				own := view.Facts[0]
				if own.Observer == owner && own.Subject == owner && (core.Rights{Resource: own.ID, Grants: own.Grants}).Allows(core.PermissionRequest{Resource: own.ID, Context: core.Grant{Actor: owner, Recipient: focus, Purpose: "simulation", Operation: core.Disclose}}) {
					s.Disclosure = &behavior.DisclosureGrant{Recipient: focus, Mode: behavior.Full, Sources: []core.ID{own.ID}}
					s.Offers = append(s.Offers, behavior.ActionOffer{Kind: behavior.Reveal, Recipient: focus, Evidence: []core.ID{own.ID}, Duration: 1, Mode: behavior.Full})
				}
			}
			// The sender must have observed the action before choosing its
			// explicitly linked reply. Same-period messages cannot be replies.
			for _, delivery := range w.Pending {
				if delivery.ReplyTo == "" || delivery.Recipient != owner || delivery.At > event.At {
					continue
				}
				if delivery.Kind != behavior.Support && delivery.Kind != behavior.Help && delivery.Kind != behavior.Invite {
					continue
				}
				for oi, outcome := range w.ActionOutcomes {
					if outcome.Outcome.Decision != delivery.ReplyTo || outcome.Outcome.Observer != owner || outcome.Outcome.Other != delivery.Sender || outcome.Outcome.Status != core.Unknown {
						continue
					}
					response := perceived(owner, core.ID("response:"+string(delivery.Decision)), event.At, .6, dynamics.Signals{Support: .7})
					response.OccurredAt = delivery.At
					actor, outcome, e = behavior.ResolveAction(actor, outcome, response, delivery.Sender, "supportive")
					if e != nil {
						return World{}, e
					}
					s.Sources = append(s.Sources, response.Event)
					w.ActionOutcomes[oi] = outcome
					break
				}
			}
			stream := core.ID("relational-choice:" + string(owner))
			draw, e := rng.Draw(stream)
			if e != nil {
				return World{}, e
			}
			if record != nil && p == through-1 {
				actual, e := record.Draw(stream)
				if e != nil || actual != draw {
					return World{}, fmt.Errorf("relational RNG/history mismatch")
				}
			}
			next, d, e := behavior.ChooseAction(actor, s, event.At, draw)
			if e != nil {
				return World{}, e
			}
			d.Stages[11] = "done"
			o, e := behavior.TrackAction(d, sc.World.Horizon)
			if e != nil {
				return World{}, e
			}
			w.ActionActors[ai] = next
			w.ActionDecisions = append(w.ActionDecisions, d)
			w.ActionOutcomes = append(w.ActionOutcomes, o)
			selected := d.Candidates[d.Selected].Offer
			if selected.Kind == behavior.Help {
				if w.Resources[selected.Resource] < selected.Units {
					return World{}, fmt.Errorf("relational overspend")
				}
				w.Resources[selected.Resource] -= selected.Units
			}
			if selected.Delivered() {
				kind := selected.Kind
				if kind == behavior.Lie {
					kind = behavior.Say
				}
				replyTo := core.ID("")
				if kind == behavior.Support || kind == behavior.Help || kind == behavior.Invite {
					for _, incoming := range w.Pending {
						if incoming.Recipient == owner && incoming.Sender == selected.Recipient && incoming.Decision != "" && incoming.At < event.At {
							replyTo = incoming.Decision
							break
						}
					}
				}
				pending = append(pending, Delivery{Decision: d.ID, ReplyTo: replyTo, Sender: owner, Recipient: selected.Recipient, Kind: kind, At: event.At + selected.Duration})
			}
		}
		w.Pending = pending
		w.Period = p + 1
		w.At = event.At
	}
	return w, nil
}
