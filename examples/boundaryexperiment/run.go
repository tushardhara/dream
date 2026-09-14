// Package boundaryexperiment composes the opt-in helper and human boundary paths
// using attributed synthetic policy facts. Nothing is sent outside the local host.
package boundaryexperiment

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/tushardhara/dream/app/assistance"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/examples/assistanceclient"
	"github.com/tushardhara/dream/examples/helperexperiment"
	"github.com/tushardhara/dream/simulator/behavior"
)

const Version = "boundary-experiment.v1"

type Trace struct {
	Version string
	Signal  string
	Helpers []assistance.Interaction
	Human   []behavior.ScopedDecision
	Final   behavior.ScopedActor
}

func fact(id, owner, with core.ID, decision core.Willingness) core.Boundary {
	return core.Boundary{Version: core.BoundaryVersion, Meta: core.Metadata{ID: id, Observer: owner, Source: owner, Sensitivity: core.Restricted, Supporting: []core.ID{"synthetic-policy-source"}, Confidence: .8, Valid: core.Interval{Start: 0}, RecordedAt: time.Unix(1, 0).UTC(), Rights: core.Rights{Resource: id}}, Principal: owner, With: with, Topic: "money", Class: core.Discussion, Decision: decision, Basis: "self_report", OccurredAt: 1, LearnedAt: 1}
}

// Run uses the same underlying v2 human engine with scoped eligibility added.
// The maximal draw makes each sole eligible non-WAIT offer observable; it is a
// deterministic control, not a behavioral estimate. Replay still rechecks policy.
func Run(ctx context.Context, signal string, recorded *Trace) (Trace, error) {
	if signal != "disagreement" && signal != "credible_pressure" {
		return Trace{}, fmt.Errorf("unsupported synthetic scenario")
	}
	if recorded != nil && (recorded.Version != Version || recorded.Signal != signal || len(recorded.Helpers) != 2) {
		return Trace{}, fmt.Errorf("replay scenario mismatch")
	}
	l := assistanceclient.New("helper", "alice", assistance.Coordinate)
	facts := []core.Boundary{}
	for _, pair := range [][2]core.ID{{"alice", "bob"}, {"bob", "alice"}, {"alice", "charlie"}, {"charlie", "alice"}} {
		facts = append(facts, fact(core.ID(string(pair[0])+"-"+string(pair[1])), pair[0], pair[1], core.Willing))
	}
	risk := fact("risk", "alice", "bob", core.PressureSignal)
	risk.Basis = "observed_signal"
	risk.Signal = signal
	facts = append(facts, risk)
	for _, b := range facts {
		if e := l.AppendBoundary(b.Meta.Observer, b); e != nil {
			return Trace{}, e
		}
	}
	actor, e := behavior.NewScopedActor("alice", 0)
	if e != nil {
		return Trace{}, e
	}
	out := Trace{Version: Version, Signal: signal}
	frames := helperexperiment.World().Frames
	for i, target := range []core.ID{"bob", "charlie"} {
		scope := core.InteractionScope{Version: core.InteractionScopeVersion, Initiator: "alice", Target: target, Topic: "money", Class: core.Discussion}
		frame := frames[i*2]
		r := assistance.Request{Version: assistance.ScopedVersion, ID: core.ID(fmt.Sprintf("scoped-%d", i)), Helper: "helper", User: "alice", Purpose: "help", Participants: []core.ID{"alice", target}, Scope: &scope, Arm: assistance.Single, Goal: assistance.Coordinate, At: frame.At, Seed: 7}
		if e := l.Register(r); e != nil {
			return Trace{}, e
		}
		var prior *assistance.Interaction
		if recorded != nil {
			prior = &recorded.Helpers[i]
		}
		help, e := l.Host(assistance.FakePlanner{}).Execute(ctx, r, prior)
		if e != nil {
			return Trace{}, e
		}
		out.Helpers = append(out.Helpers, help)
		// The person may leave Bob regardless of Bob's agreement, then speak to Charlie.
		kind := behavior.Leave
		if i == 1 {
			kind = behavior.Say
		}
		frame.Situation.Present = []core.ID{"alice", target}
		frame.Situation.Offers = []behavior.ActionOffer{{Kind: kind, Recipient: target, Duration: 1, Evidence: frame.Situation.Sources}}
		next, d, e := behavior.ChooseScopedAction(actor, behavior.ScopedSituation{Scope: scope, Situation: frame.Situation, Boundaries: facts}, frame.At, math.MaxUint64)
		if e != nil {
			return Trace{}, e
		}
		actor = next
		out.Human = append(out.Human, d)
	}
	out.Final = actor
	if recorded != nil && assistance.Digest(out) != assistance.Digest(*recorded) {
		return Trace{}, fmt.Errorf("replay trace mismatch")
	}
	return out, nil
}
