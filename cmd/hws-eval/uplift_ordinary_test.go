package main

import (
	"context"
	"testing"

	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/evals"
	"github.com/tushardhara/dream/examples/ordinaryexperiment"
)

// A helper that correctly stays silent produces no experience to measure. That
// is restraint working, not an outcome going astray, and scoring it as
// "missing" made it harm — penalising exactly the behaviour this evaluation
// exists to leave room for.
func TestCorrectRestraintIsNotAMissingOutcome(t *testing.T) {
	// quiet is a family in which the helper waits on every opportunity.
	run, e := ordinaryexperiment.Run(context.Background(), "quiet", "generic", 11)
	if e != nil {
		t.Fatal(e)
	}
	for _, o := range run.Opportunities {
		if o.Helper.Action != "WAIT" {
			t.Fatal("positive control: the helper acted, so this is not a restraint scenario")
		}
	}
	if len(run.Experiences) != 0 {
		t.Fatal("positive control: the restraint scenario produced experiences")
	}
	for _, person := range []core.ID{"a", "b"} {
		o, _ := outcomeFor(person, run, evals.SimpleAssistance)
		if o.DelayedOutcome != "no_intervention" {
			t.Fatalf("%s reports %q where no intervention occurred", person, o.DelayedOutcome)
		}
	}
}

// Where the helper DID act, a person with no experience record is genuinely
// missing an outcome, and that distinction must survive.
func TestAnAbsentOutcomeAfterAnInterventionIsStillMissing(t *testing.T) {
	run, e := ordinaryexperiment.Run(context.Background(), "daily", "generic", 11)
	if e != nil {
		t.Fatal(e)
	}
	acted := false
	for _, o := range run.Opportunities {
		acted = acted || o.Helper.Action != "WAIT"
	}
	if !acted {
		t.Fatal("positive control: the helper never acted in this scenario")
	}
	// Positive control: with its experiences intact this person resolves.
	if o, _ := outcomeFor("a", run, evals.SimpleAssistance); o.DelayedOutcome != "resolved" {
		t.Fatalf("positive control: expected resolved, got %q", o.DelayedOutcome)
	}
	// Drop only a's experiences. The intervention still happened, so a's
	// outcome is missing rather than untouched — and b's must be unaffected.
	dropped := run
	dropped.Experiences = nil
	for _, x := range run.Experiences {
		if x.Participant != "a" {
			dropped.Experiences = append(dropped.Experiences, x)
		}
	}
	if len(dropped.Experiences) == len(run.Experiences) {
		t.Fatal("positive control: this person had no experiences to drop")
	}
	if o, _ := outcomeFor("a", dropped, evals.SimpleAssistance); o.DelayedOutcome != "missing" {
		t.Fatalf("an absent outcome after a real intervention reports %q, not missing", o.DelayedOutcome)
	}
	if o, _ := outcomeFor("b", dropped, evals.SimpleAssistance); o.DelayedOutcome != "resolved" {
		t.Fatalf("dropping one person's outcome changed another's to %q", o.DelayedOutcome)
	}
}
