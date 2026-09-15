// Package ordinaryexperiment is an offline consumer of the production ordinary
// helper and the existing native scoped human policy. It is not a human study.
package ordinaryexperiment

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"github.com/tushardhara/dream/app/assistance"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/examples/ordinaryclient"
	"github.com/tushardhara/dream/simulator/behavior"
	"github.com/tushardhara/dream/simulator/drives"
	"github.com/tushardhara/dream/simulator/dynamics"
)

const Frames = 12

var Families = []string{"quiet", "daily", "unknown_benefit", "declined_ritual"}
var Arms = []string{"none", "generic", "permitted_context"}

type Trace struct {
	Frame                     int
	Stage                     string
	Actor                     core.ID
	Decision                  behavior.ScopedDecision
	MemoryBefore, MemoryAfter string
}
type Opportunity struct {
	Frame    int
	Helper   assistance.OrdinaryResponse
	Selected behavior.Kind
}
type Report struct {
	Version, Family, Arm, HumanValidity, GlobalWelfare string
	Seed                                               uint64
	Traces                                             []Trace
	Opportunities                                      []Opportunity
	Experiences                                        []core.OrdinaryExperience
	Reservations                                       []core.GroupReservation
}

func draw(seed uint64, frame int, actor core.ID, stage string) uint64 {
	h := sha256.Sum256([]byte(fmt.Sprintf("ordinary:%d:%d:%s:%s", seed, frame, actor, stage)))
	return binary.BigEndian.Uint64(h[:8])
}
func kindAt(family string, frame int) core.OrdinaryKind {
	if family == "quiet" {
		return core.OrdinaryQuiet
	}
	if family == "unknown_benefit" {
		return core.OrdinaryActivity
	}
	if family == "declined_ritual" && (frame == 2 || frame == 5) {
		return core.OrdinaryActivity
	}
	switch frame {
	case 2:
		return core.OrdinaryAppreciation
	case 5:
		return core.OrdinaryMemory
	case 8:
		return core.OrdinaryActivity
	case 11:
		return core.OrdinaryCoordination
	}
	return core.OrdinaryQuiet
}
func fixture(family string) (*ordinaryclient.Local, error) {
	h, b, bs := ordinaryclient.FixtureGroup(121, 128)
	for _, k := range []core.OrdinaryKind{core.OrdinaryAppreciation, core.OrdinaryMemory, core.OrdinaryActivity, core.OrdinaryCoordination} {
		bs = append(bs, ordinaryclient.Boundary("a", "b", core.ID(k), core.Willing, 0), ordinaryclient.Boundary("b", "a", core.ID(k), core.Willing, 0))
	}
	l, e := ordinaryclient.New("ordinary-native", "helper", []core.ID{"a", "b"}, h, b, nil, bs)
	if e != nil {
		return nil, e
	}
	end := core.LogicalTime(200)
	for _, k := range []core.OrdinaryKind{core.OrdinaryAppreciation, core.OrdinaryMemory, core.OrdinaryActivity, core.OrdinaryCoordination} {
		for _, person := range []core.ID{"a", "b"} {
			benefit := core.ObservedGroupQuantity(.6)
			if family == "unknown_benefit" {
				benefit = core.UnknownGroupQuantity()
			}
			p := core.OrdinaryPreference{Version: core.OrdinaryVersion, Source: core.ID(fmt.Sprintf("pref:%s:%s", person, k)), Person: person, Activity: core.ID(k), Kind: k, Window: core.Interval{End: &end}, Choice: "wanted", Availability: "available", ExpectedBenefit: benefit, MaxEffort: core.ObservedGroupQuantity(4)}
			if e = l.PutPreference(person, p, []core.ID{"a"}, 0); e != nil {
				return nil, e
			}
		}
		if k == core.OrdinaryAppreciation || k == core.OrdinaryMemory {
			words := "Thank you for making tea."
			if k == core.OrdinaryMemory {
				words = "We played chess after dinner."
			}
			s := core.OrdinaryStory{Version: core.OrdinaryVersion, Source: core.ID("story:" + string(k)), Author: "b", Activity: core.ID(k), Kind: k, Origin: "participant_authored", Words: words}
			if e = l.PutStory("b", s, core.Public, []core.ID{"a"}, nil, 0); e != nil {
				return nil, e
			}
		}
	}
	return l, nil
}

// nativeStep supplies only mechanical action opportunities and self-observed
// signals. It never gives another person's private preference or story to policy.
func nativeStep(a behavior.ScopedActor, other core.ID, frame int, stage string, at core.LogicalTime, kind behavior.Kind, seed uint64) (behavior.ScopedActor, Trace, error) {
	person := a.Human.Drives.Actor
	event := core.ID(fmt.Sprintf("ordinary-event:%s:%d:%s", person, frame, stage))
	topic := core.ID(stage)
	class := core.Discussion
	if kind == behavior.Invite || kind == behavior.Coordinate {
		class = core.Coordination
	}
	p := dynamics.Perceived{Event: event, Actor: person, OccurredAt: at, LearnedAt: at, Confidence: .8, Signals: dynamics.Signals{Rest: .2, Opportunity: .3, Support: .1}, Rights: core.Rights{Resource: event, Grants: []core.Grant{{Actor: person, Recipient: person, Purpose: "simulation", Operation: core.Read}, {Actor: person, Recipient: person, Purpose: "simulation", Operation: core.Derive}}}}
	s := behavior.ActionSituation{Observation: drives.Observation{Event: p}, Sources: []core.ID{event}, Present: []core.ID{person, other}, Horizon: at + 5}
	for i := range s.Observation.Context {
		q := p
		q.Confidence = 0
		q.Signals = dynamics.Signals{}
		s.Observation.Context[i] = drives.Cue{Evidence: q}
	}
	if kind != behavior.Wait {
		s.Offers = []behavior.ActionOffer{{Kind: kind, Recipient: other, Duration: 1, Evidence: []core.ID{event}}}
	}
	bs := []core.Boundary{ordinaryclient.Boundary(person, other, topic, core.Willing, 0), ordinaryclient.Boundary(other, person, topic, core.Willing, 0)}
	for i := range bs {
		bs[i].Class = class
	}
	in := behavior.ScopedSituation{Scope: core.InteractionScope{Version: core.InteractionScopeVersion, Initiator: person, Target: other, Topic: topic, Class: class}, Situation: s, Boundaries: bs}
	next, d, e := behavior.ChooseScopedAction(a, in, at, draw(seed, frame, person, stage))
	return next, Trace{Frame: frame, Stage: stage, Actor: person, Decision: d, MemoryBefore: assistance.Digest(a.Human.Memory), MemoryAfter: assistance.Digest(next.Human.Memory)}, e
}
func selected(t Trace) behavior.Kind {
	return t.Decision.Human.Candidates[t.Decision.Human.Selected].Offer.Kind
}

// Run retains null and adverse reports. Later self-reports are explicit fictional
// fixtures, independent of arm and wording; choosing or replying never generates
// a welcomed label. Missing self-reports remain absent, not zero benefit.
func Run(ctx context.Context, family, arm string, seed uint64) (Report, error) {
	out := Report{Version: core.OrdinaryVersion, Family: family, Arm: arm, Seed: seed, HumanValidity: "NOT_TESTED", GlobalWelfare: "NOT_AGGREGATED", Traces: []Trace{}, Opportunities: []Opportunity{}, Experiences: []core.OrdinaryExperience{}, Reservations: []core.GroupReservation{}}
	valid := false
	for _, f := range Families {
		valid = valid || f == family
	}
	if !valid || arm != "none" && arm != "generic" && arm != "permitted_context" {
		return out, fmt.Errorf("invalid ordinary experiment")
	}
	l, e := fixture(family)
	if e != nil {
		return out, e
	}
	actors := map[core.ID]behavior.ScopedActor{}
	for _, person := range []core.ID{"a", "b"} {
		a, err := behavior.NewScopedActor(person, 0)
		if err != nil {
			return out, err
		}
		other := core.ID("a")
		if person == "a" {
			other = "b"
		}
		a.Human.Memory = []behavior.Memory{{Other: other, Trust: .4, Disclosure: .1, Evidence: []core.ID{"steady-history"}}}
		actors[person] = a
	}
	for frame := 0; frame < Frames; frame++ {
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		at := core.LogicalTime(10 + 10*frame)
		for _, person := range []core.ID{"a", "b"} {
			other := core.ID("a")
			if person == "a" {
				other = "b"
			}
			a, tr, err := nativeStep(actors[person], other, frame, "daily-life", at-1, behavior.Say, seed)
			if err != nil {
				return out, fmt.Errorf("daily %d: %w", frame, err)
			}
			actors[person] = a
			out.Traces = append(out.Traces, tr)
		}
		k := kindAt(family, frame)
		end := at + 8
		r := assistance.OrdinaryRequest{ID: core.ID(fmt.Sprintf("opportunity:%d", frame)), Participants: []core.ID{"a", "b"}, Activity: core.ID(k), Kind: k, Arm: arm, At: at, Window: core.Interval{Start: at + 1, End: &end}, Effort: map[core.ID]int64{"a": 1, "b": 1}}
		author := core.ID("")
		if k == core.OrdinaryAppreciation || k == core.OrdinaryMemory {
			author = "b"
		}
		if k == core.OrdinaryCoordination {
			r.Requested = true
			r.PlanDecision = "ordinary-plan"
			r.PlanOption = "slot"
		}
		r, e = l.Request("a", r, author)
		if e != nil {
			return out, e
		}
		response, err := l.Host().Execute(ctx, r, nil)
		if err != nil {
			return out, err
		}
		offer := behavior.Wait
		// Unknown benefit must not acquire the native social-action scoring floor.
		if response.Action != "WAIT" {
			switch k {
			case core.OrdinaryActivity:
				offer = behavior.Invite
			case core.OrdinaryCoordination:
				offer = behavior.Coordinate
			default:
				offer = behavior.Say
			}
		}
		a, tr, err := nativeStep(actors["a"], "b", frame, "opportunity", at, offer, seed)
		if err != nil {
			return out, fmt.Errorf("opportunity %d: %w", frame, err)
		}
		actors["a"] = a
		out.Traces = append(out.Traces, tr)
		choice := selected(tr)
		out.Opportunities = append(out.Opportunities, Opportunity{frame, response, choice})
		if choice == behavior.Coordinate {
			if e = l.Reserve(ctx, "a", r.ID, "native-reservation"); e != nil {
				return out, e
			}
		}
		declined := false
		if family == "declined_ritual" && k == core.OrdinaryActivity && choice == behavior.Invite {
			b, decline, err := nativeStep(actors["b"], "a", frame, "decline-ritual", at+1, behavior.Decline, seed)
			if err != nil {
				return out, err
			}
			actors["b"] = b
			out.Traces = append(out.Traces, decline)
			if selected(decline) == behavior.Decline {
				declined = true
				if e = l.AppendBoundary("b", ordinaryclient.Boundary("b", "a", r.Activity, core.Declined, at+1)); e != nil {
					return out, e
				}
			}
		}
		// Authored later reports exist only for rare delivered opportunities. They
		// are not derived from the selected action; seed 0 gives an explicit adverse
		// report and seed 1 an unknown report even when the native action is sociable.
		if response.Action != "WAIT" {
			for _, person := range []core.ID{"a", "b"} {
				participation := "welcomed"
				benefit := core.ObservedGroupQuantity(.3)
				if seed%4 == 0 && person == "b" {
					participation = "unwelcome"
					benefit = core.ObservedGroupQuantity(-.3)
				}
				if seed%4 == 1 {
					participation = "unknown"
					benefit = core.UnknownGroupQuantity()
				}
				if declined && person == "b" {
					participation = "declined"
					benefit = core.UnknownGroupQuantity()
				}
				x := core.OrdinaryExperience{Version: core.OrdinaryVersion, ID: core.ID(fmt.Sprintf("later:%d:%s", frame, person)), Opportunity: r.ID, Participant: person, Observer: person, Source: person, OccurredAt: at + 1, LearnedAt: at + 1, Participation: participation, Benefit: benefit, BurdenReduction: core.UnknownGroupQuantity()}
				if e = l.Observe(person, x); e != nil {
					return out, e
				}
			}
		}
	}
	for _, person := range []core.ID{"a", "b"} {
		xs, err := l.Experiences(person)
		if err != nil {
			return out, err
		}
		out.Experiences = append(out.Experiences, xs...)
	}
	out.Reservations = append(out.Reservations, l.Reservations()...)
	return out, nil
}
