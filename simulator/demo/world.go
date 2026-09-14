package demo

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/behavior"
	"github.com/tushardhara/dream/simulator/dynamics"
	rt "github.com/tushardhara/dream/simulator/runtime"
	"github.com/tushardhara/dream/simulator/scenario"
)

type Delivery struct {
	Decision  core.ID          `json:"decision,omitempty"`
	ReplyTo   core.ID          `json:"reply_to,omitempty"`
	Sender    core.ID          `json:"sender"`
	Recipient core.ID          `json:"recipient"`
	Kind      behavior.Kind    `json:"kind"`
	At        core.LogicalTime `json:"at"`
}
type World struct {
	ActionOutcomes  []behavior.ActionOutcome  `json:"action_outcomes,omitempty"`
	ActionActors    []behavior.ActionActor    `json:"action_actors,omitempty"`
	ActionDecisions []behavior.ActionDecision `json:"action_decisions,omitempty"`
	Version         string                    `json:"version"`
	Period          int                       `json:"period"`
	At              core.LogicalTime          `json:"at"`
	Actors          []behavior.Actor          `json:"actors"`
	Decisions       []behavior.Decision       `json:"decisions"`
	Resources       map[core.ID]int64         `json:"resources"`
	Pending         []Delivery                `json:"pending"`
}

func digest(v any) (string, error) {
	b, e := json.Marshal(v)
	if e != nil {
		return "", e
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}
func (w World) Hash() (string, error) { return digest(w) }

type Checkpoint struct {
	Version   string `json:"version"`
	Period    int    `json:"period"`
	WorldHash string `json:"world_hash"`
}

func signal(theme string) (dynamics.Signals, error) {
	switch theme {
	case "routine":
		return dynamics.Signals{}, nil
	case "joy":
		return dynamics.Signals{Support: .7, Inclusion: .8, Rest: .3}, nil
	case "scarcity":
		return dynamics.Signals{Scarcity: .8, Effort: .4}, nil
	case "support":
		return dynamics.Signals{OtherNeed: .7, Support: .4}, nil
	case "compound":
		return dynamics.Signals{Scarcity: .6, StatusThreat: .7, Exclusion: .6, Effort: .4}, nil
	case "work":
		return dynamics.Signals{Effort: .7, Opportunity: .4}, nil
	case "family":
		return dynamics.Signals{OtherNeed: .5, Inclusion: .5}, nil
	case "repair":
		return dynamics.Signals{Rest: .6, Support: .5, Inclusion: .4}, nil
	}
	return dynamics.Signals{}, fmt.Errorf("unknown demo theme")
}

// Reconstruct is a bounded projection over frozen demo inputs, not an alternative
// mutable database. Only actor views and already-reached periods enter behavior.
// Current-period randomness is also drawn through the canonical runtime recorder.
func reconstruct(sc scenario.Scenario, through int, domain, common core.ID, coupled bool, record *rt.Random) (World, error) {
	if demoVersion(sc) == RelationalVersion {
		return reconstructRelational(sc, through, domain, common, coupled, record)
	}
	if sc.Validate() != nil || through < 0 || through > len(sc.Future) || len(sc.Future) > 24 {
		return World{}, fmt.Errorf("invalid demo projection")
	}
	w := World{Version: Version, Resources: map[core.ID]int64{}, Actors: []behavior.Actor{}, Decisions: []behavior.Decision{}, Pending: []Delivery{}}
	for _, r := range sc.Public.Resources {
		w.Resources[r.ID] = r.Available
	}
	for i, h := range sc.Public.Humans {
		view, e := sc.View(h.ID)
		if e != nil {
			return World{}, e
		}
		state, e := dynamics.New(h.ID, 0, dynamics.DefaultSubstrate())
		if e != nil {
			return World{}, e
		}
		if len(view.Facts) == 0 {
			return World{}, fmt.Errorf("demo requires own initial evidence")
		}
		sources := map[core.ID]bool{}
		for _, fact := range view.Facts {
			rights := core.Rights{Resource: fact.ID, Grants: fact.Grants}
			sources[fact.ID] = rights.Allows(core.PermissionRequest{Resource: fact.ID, Context: core.Grant{Actor: h.ID, Recipient: h.ID, Purpose: "simulation", Operation: core.Derive}})
		}
		source := view.Facts[0].ID
		if !sources[source] {
			return World{}, fmt.Errorf("initial derivation not permitted")
		}
		state, e = dynamics.Intervene(state, dynamics.Fatigue, .1+float64(i%4)*.1, 0, source)
		if e != nil {
			return World{}, e
		}
		a := behavior.Actor{State: state}
		for _, edge := range view.Relationships {
			if !sources[edge.Evidence] {
				return World{}, fmt.Errorf("relationship derivation not permitted")
			}
			a.Memory = append(a.Memory, behavior.Memory{Other: edge.Other, Trust: .1, Disclosure: 0, Evidence: []core.ID{edge.Evidence}})
		}
		if a.Validate() != nil {
			return World{}, fmt.Errorf("invalid own demo state")
		}
		w.Actors = append(w.Actors, a)
	}
	rng := rt.NewScopedRandom(sc.World.Seed, nil, domain, common, coupled)
	for p := 0; p < through; p++ {
		event := sc.Future[p]
		var period Period
		if json.Unmarshal([]byte(event.Text), &period) != nil || period.Version != Version || period.Index != p+1 || event.Kind != "observation" {
			return World{}, fmt.Errorf("invalid monthly demo input")
		}
		signals, e := signal(period.Theme)
		if e != nil {
			return World{}, e
		}
		members := []core.ID{}
		for _, g := range sc.Public.Groups {
			if g.ID == period.Group {
				members = append(members, g.Members...)
			}
		}
		if len(members) == 0 {
			return World{}, fmt.Errorf("unknown demo venue")
		}
		pending := []Delivery{}
		for ai, actor := range w.Actors {
			owner := actor.State.Actor
			present := []core.ID{owner}
			visible := false
			for _, id := range members {
				visible = visible || id == owner
			}
			perceived := dynamics.Signals{}
			if visible {
				present = members
				perceived = signals
			}
			// A recipient can use a past typed delivery; neither other actors' private
			// notes nor the unseen group's event is supplied to this actor's choice.
			for _, delivery := range w.Pending {
				if delivery.Recipient != owner || delivery.At > event.At {
					continue
				}
				switch delivery.Kind {
				case behavior.Help:
					perceived.Support = .7
				case behavior.Ask:
					perceived.OtherNeed = .6
				case behavior.Invite:
					perceived.Inclusion = .6
				}
			}
			source := core.ID(fmt.Sprintf("demo-observation:%d:%s", p+1, owner))
			s := behavior.Situation{Perceived: dynamics.Perceived{Event: source, Actor: owner, OccurredAt: event.At, LearnedAt: event.At, Confidence: .6, Signals: perceived, Rights: core.Rights{Resource: source, Grants: []core.Grant{{Actor: owner, Recipient: owner, Purpose: "simulation", Operation: core.Read}, {Actor: owner, Recipient: owner, Purpose: "simulation", Operation: core.Derive}}}}, Present: present, Resources: w.Resources, Horizon: event.At + 1000000000, Offers: []behavior.Offer{{Kind: behavior.Observe, Duration: 1}}}
			if s.Horizon > sc.World.Horizon {
				s.Horizon = sc.World.Horizon
			}
			for _, m := range actor.Memory {
				s.Offers = append(s.Offers, behavior.Offer{Kind: behavior.Ask, Recipient: m.Other, Duration: 1}, behavior.Offer{Kind: behavior.Invite, Recipient: m.Other, Duration: 1}, behavior.Offer{Kind: behavior.Help, Recipient: m.Other, Resource: "shared-time", Units: 1, Duration: 1}, behavior.Offer{Kind: behavior.Decline, Recipient: m.Other, Duration: 1})
			}
			stream := core.ID("demo-choice:" + string(owner))
			draw, e := rng.Draw(stream)
			if e != nil {
				return World{}, e
			}
			if record != nil && p == through-1 {
				actual, e := record.Draw(stream)
				if e != nil || actual != draw {
					return World{}, fmt.Errorf("demo RNG differs from frozen history")
				}
			}
			next, decision, e := behavior.Choose(actor, s, event.At, draw)
			if e != nil {
				return World{}, e
			}
			w.Actors[ai] = next
			w.Decisions = append(w.Decisions, decision)
			selected := decision.Candidates[decision.Selected].Offer
			if selected.Kind == behavior.Help {
				if w.Resources[selected.Resource] < selected.Units {
					return World{}, fmt.Errorf("demo resource overspend")
				}
				w.Resources[selected.Resource] -= selected.Units
			}
			if selected.Recipient != "" {
				pending = append(pending, Delivery{Sender: owner, Recipient: selected.Recipient, Kind: selected.Kind, At: event.At + selected.Duration})
			}
		}
		w.Pending = pending
		w.Period = p + 1
		w.At = event.At
	}
	return w, nil
}

// Projection recomputes actor state rather than weakening the runtime's 4096-byte
// checkpoint cap or the cognitive service's existing small-fixture ledger limits.
func Projection(state rt.State) (World, error) {
	if state.Validate() != nil {
		return World{}, fmt.Errorf("invalid runtime state")
	}
	var sc scenario.Scenario
	if json.Unmarshal(state.Genesis.Payload, &sc) != nil {
		return World{}, fmt.Errorf("invalid genesis")
	}
	if demoVersion(sc) == Version {
		for _, a := range sc.Actors {
			if len(a.Contexts) > 0 {
				return World{}, fmt.Errorf("legacy demo cannot ignore relationship context")
			}
		}
	}
	if v := demoVersion(sc); v != Version && v != RelationalVersion {
		return World{}, fmt.Errorf("unsupported demo version")
	}
	var cp Checkpoint
	if state.Data != "" {
		if json.Unmarshal([]byte(state.Data), &cp) != nil || cp.Version != demoVersion(sc) || cp.Period < 1 || cp.Period > 24 {
			return World{}, fmt.Errorf("invalid demo checkpoint")
		}
	}
	if state.Data != "" && cp.Version == RelationalVersion {
		raw, _ := json.Marshal(cp)
		if string(raw) != state.Data {
			return World{}, fmt.Errorf("noncanonical relational checkpoint")
		}
	}
	world, e := reconstruct(sc, cp.Period, state.RandomDomain, state.ExogenousDomain, state.Coupled, nil)
	if e != nil {
		return World{}, e
	}
	h, e := world.Hash()
	if e != nil || state.Data != "" && h != cp.WorldHash {
		return World{}, fmt.Errorf("demo checkpoint/projection mismatch")
	}
	for id, value := range world.Resources {
		if state.Available[id] != value {
			return World{}, fmt.Errorf("demo resource checkpoint mismatch")
		}
	}
	return world, nil
}

type Handler struct{}

func (Handler) Transition(state rt.State, input rt.Input, clock rt.Clock, rng *rt.Random) (rt.Output, error) {
	previous, e := Projection(state)
	if e != nil {
		return rt.Output{}, e
	}
	var sc scenario.Scenario
	_ = json.Unmarshal(state.Genesis.Payload, &sc)
	if previous.Period >= len(sc.Future) {
		return rt.Output{}, fmt.Errorf("demo already complete")
	}
	expected := sc.Future[previous.Period]
	if input.ID != expected.ID || input.Text != expected.Text || input.At != expected.At || clock.Now() != input.At || input.Actor != expected.Actor || input.Kind != "observation" {
		return rt.Output{}, fmt.Errorf("unregistered demo event")
	}
	next, e := reconstruct(sc, previous.Period+1, state.RandomDomain, state.ExogenousDomain, state.Coupled, rng)
	if e != nil {
		return rt.Output{}, e
	}
	hash, e := next.Hash()
	if e != nil {
		return rt.Output{}, e
	}
	raw, e := json.Marshal(Checkpoint{Version: next.Version, Period: next.Period, WorldHash: hash})
	if e != nil {
		return rt.Output{}, e
	}
	out := rt.Output{Data: string(raw)}
	for _, r := range sc.Public.Resources {
		delta := previous.Resources[r.ID] - next.Resources[r.ID]
		if delta < 0 {
			return rt.Output{}, fmt.Errorf("implicit resource replenishment")
		}
		if delta > 0 {
			out.Consume = append(out.Consume, rt.Consumption{Resource: r.ID, Units: delta})
		}
	}
	return out, nil
}
