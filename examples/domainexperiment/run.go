// Package domainexperiment composes the actual graph, helper and human domain
// consumers using only synthetic observations and explicit scoped willingness.
package domainexperiment

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/tushardhara/dream/app/assistance"
	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/examples/assistanceclient"
	"github.com/tushardhara/dream/examples/helperexperiment"
	"github.com/tushardhara/dream/simulator/behavior"
	"github.com/tushardhara/dream/simulator/dynamics"
)

const Version = "domain-experiment.v1"

type Trace struct {
	Version string
	Focus   core.RelationshipFocus
	// Arm is the assistant policy this run was executed under; see the boundary
	// experiment's Trace for why it is part of the trace identity.
	Arm    assistance.Arm
	Helper assistance.Interaction
	Human  behavior.DomainDecision
	Final  behavior.DomainActor
}

// Run holds words and identities constant across family/business frames. Numeric
// differences are explicitly authored observations. A maximal draw is a control
// for candidate eligibility, not an estimate of human behavior or helper benefit.
func Run(ctx context.Context, focus core.RelationshipFocus, arm assistance.Arm, recorded *Trace) (Trace, error) {
	if focus.Validate() != nil || !arm.Valid() || recorded != nil && (recorded.Version != Version || recorded.Arm != arm || assistance.Digest(recorded.Focus) != assistance.Digest(focus)) {
		return Trace{}, fmt.Errorf("invalid domain experiment/replay")
	}
	profiles := []core.RelationshipContext{}
	for _, owner := range []core.ID{"alice", "bob"} {
		other := core.ID("alice")
		if owner == "alice" {
			other = "bob"
		}
		profiles = append(profiles, assistanceclient.DomainProfile(owner, other, core.ID(string(owner)+"-family"), core.Childcare, "family", .8, .6), assistanceclient.DomainProfile(owner, other, core.ID(string(owner)+"-business"), core.Childcare, "business", -.7, -.6))
	}
	local, r, e := assistanceclient.DomainFixture(arm, focus, profiles)
	if e != nil {
		return Trace{}, e
	}
	facts := []core.Boundary{}
	for _, owner := range r.Participants {
		other := r.User
		if owner == r.User {
			other = r.Scope.Target
		}
		id := core.ID(string(owner) + "-willing")
		b := core.Boundary{Version: core.BoundaryVersion, Meta: core.Metadata{ID: id, Observer: owner, Source: owner, Sensitivity: core.Restricted, Confidence: 1, Supporting: []core.ID{"synthetic-choice"}, Valid: core.Interval{}, RecordedAt: time.Unix(1, 0).UTC(), Rights: core.Rights{Resource: id}}, Principal: owner, With: other, Topic: core.ID(focus.Domain), Class: core.Discussion, Decision: core.Willing, Basis: "self_report", OccurredAt: 1, LearnedAt: 1}
		facts = append(facts, b)
		if e = local.AppendBoundary(owner, b); e != nil {
			return Trace{}, e
		}
	}
	var previous *assistance.Interaction
	if recorded != nil {
		previous = &recorded.Helper
	}
	helper, e := local.Host(assistance.FakePlanner{}).Execute(ctx, r, previous)
	if e != nil {
		return Trace{}, e
	}
	// The human receives only records retrieved for Alice, with original rights,
	// observer, time and confidence. Bob's helper-visible accounts never enter it.
	//
	// This query is built from the request rather than borrowed from
	// r.Contexts[0]. Alice reads her OWN memory whether or not a helper proposed
	// anything, so making the human's retrieval depend on a helper context
	// proposal was both wrong and a panic on any arm that proposes none — which
	// is every no-assistant and simple-assistance run. The shape is unchanged
	// from the proposal it used to borrow; only its origin is.
	query := graph.MemoryQuery{
		Scope:        graph.MemoryScope{Owner: "alice", Namespace: "domain-synthetic"},
		Actor:        "alice",
		Purpose:      "simulation",
		Subject:      core.Subject{Principal: "alice"},
		ValidAt:      r.At,
		KnownAt:      r.At,
		RecordedAsOf: time.Unix(10, 0).UTC(),
		Limit:        16,
	}
	selected, e := (graph.MemoryService{Journal: local}).Retrieve(ctx, query)
	if e != nil {
		return Trace{}, e
	}
	frame := helperexperiment.World().Frames[0]
	input := behavior.DomainSituation{Focus: focus, Scoped: behavior.ScopedSituation{Scope: *r.Scope, Boundaries: facts, Situation: frame.Situation}}
	for _, pick := range selected {
		rec := pick.Record
		learned := core.LogicalTime(-1)
		for _, v := range rec.Content.Learned {
			if v.Actor == "alice" {
				learned = v.At
			}
		}
		input.Scoped.Situation.Sources = append(input.Scoped.Situation.Sources, rec.Event.Meta.ID)
		input.Scoped.Situation.RelationshipEvidence = append(input.Scoped.Situation.RelationshipEvidence, dynamics.Perceived{Event: rec.Event.Meta.ID, Actor: rec.Event.Meta.Observer, OccurredAt: rec.Event.OccurredAt, LearnedAt: learned, Confidence: rec.Event.Meta.Confidence, Rights: rec.Event.Meta.Rights})
		if rec.Content.Kind == graph.RelationshipMemory {
			v, err := graph.DecodeRelation(rec)
			if err != nil {
				return Trace{}, err
			}
			input.Accounts = append(input.Accounts, *v.Context)
		}
	}
	actor, e := behavior.NewDomainActor("alice", 0)
	if e != nil {
		return Trace{}, e
	}
	final, human, e := behavior.ChooseDomainAction(actor, input, frame.At, math.MaxUint64)
	if e != nil {
		return Trace{}, e
	}
	out := Trace{Version: Version, Focus: focus, Arm: arm, Helper: helper, Human: human, Final: final}
	if recorded != nil && assistance.Digest(out) != assistance.Digest(*recorded) {
		return Trace{}, fmt.Errorf("domain replay changed")
	}
	return out, nil
}
