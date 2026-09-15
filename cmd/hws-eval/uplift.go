package main

import (
	"context"
	"fmt"

	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/evals"
	"github.com/tushardhara/dream/examples/ordinaryexperiment"
	"github.com/tushardhara/dream/simulator/behavior"
)

// realArms maps the arms the ordinary consumer actually implements onto the
// evaluation arms. No arm is invented: multi_perspective has no implementation
// in this consumer and is reported as not executed rather than fabricated.
var realArms = []struct {
	experiment string
	arm        evals.Arm
}{
	{"none", evals.NoAssistant},
	{"generic", evals.SimpleAssistance},
	{"permitted_context", evals.SinglePerspective},
}

// upliftSeeds are the bounded independent world units per family.
var upliftSeeds = []uint64{11, 23}

// realComparisons executes the actual ordinary consumer — real hosts, real
// boundaries, real native decisions — and derives evaluation records from the
// observed results. Generation stays in the experiment; this file only reads
// what it produced.
func realComparisons(ctx context.Context) ([]evals.Comparison, error) {
	out := []evals.Comparison{}
	for _, family := range ordinaryexperiment.Families {
		for _, seed := range upliftSeeds {
			c := evals.Comparison{
				Version:  evals.UpliftVersion,
				Scenario: core.ID(fmt.Sprintf("scenario:ordinary_joy:%s", family)),
				Family:   "ordinary_joy",
				Seed:     seed,
				Affected: []core.ID{"a", "b"},
			}
			world := fmt.Sprintf("ordinary/%s/%d", family, seed)
			for _, m := range realArms {
				report, e := ordinaryexperiment.Run(ctx, family, m.experiment, seed)
				if e != nil {
					return nil, fmt.Errorf("%s/%s/%d: %w", family, m.experiment, seed, e)
				}
				run := evals.ArmRun{
					Arm: m.arm, Seed: seed, WorldHash: world, ExogenousHash: world,
					RNGStream: fmt.Sprintf("%s/%s", world, m.arm),
					Scenario:  c.Scenario,
				}
				for _, person := range c.Affected {
					o, recs := outcomeFor(person, report, m.arm)
					run.Outcomes = append(run.Outcomes, o)
					c.Sources = append(c.Sources, recs...)
				}
				for _, o := range report.Opportunities {
					if o.Helper.Action != "WAIT" {
						run.HelperActs++
					}
				}
				// An observed violation is preserved, never zeroed: if the
				// control arm did act, the comparison must be rejected rather
				// than quietly corrected.
				c.Runs = append(c.Runs, run)
			}
			if e := c.Validate(); e != nil {
				return nil, fmt.Errorf("%s/%d: %w", family, seed, e)
			}
			out = append(out, c)
		}
	}
	return out, nil
}

// outcomeFor derives one person's result from what the run actually observed,
// together with the evaluator-owned source records that evidence cites. An
// absent later report stays missing, never a zero benefit, and no behavioural
// record is emitted unless a choice was actually observed.
func outcomeFor(person core.ID, r ordinaryexperiment.Report, arm evals.Arm) (evals.PersonOutcome, []evals.SourceRecord) {
	o := evals.PersonOutcome{
		Person: person, Benefit: core.UnknownGroupQuantity(), Burden: core.UnknownGroupQuantity(),
		Appropriateness: core.UnknownGroupQuantity(), BurdenReduction: core.UnknownGroupQuantity(),
		DelayedOutcome: "missing",
	}
	// Acted means this person actually chose to act. A trace that exists but
	// selected WAIT is not acting: reading it as action would bypass the
	// broken-control detector this evaluation depends on.
	for _, t := range r.Traces {
		if t.Actor == person && t.Decision.Human.Candidates[t.Decision.Human.Selected].Offer.Kind != behavior.Wait {
			o.Acted = true
		}
	}
	// A behavioural record exists only where this person actually selected an
	// action. The selected kind is the content; a placeholder zero is not an
	// observation.
	recs := []evals.SourceRecord{}
	eventID := core.ID("")
	firstReport := false
	seenOpportunity := map[core.ID]bool{}
	for _, t := range r.Traces {
		if t.Actor != person {
			continue
		}
		kind := t.Decision.Human.Candidates[t.Decision.Human.Selected].Offer.Kind
		if kind == behavior.Wait {
			continue
		}
		eventID = core.ID(fmt.Sprintf("choice:%s:%s:%d:%s", r.Family, arm, r.Seed, person))
		recs = append(recs, evals.SourceRecord{ID: eventID, Kind: "observed_choice", Subject: person,
			Observer: person, At: t.Decision.Human.At, Arm: arm,
			Metric: "observed_choice", Value: core.ObservedGroupQuantity(1),
			Content: fmt.Sprintf("selected %v", kind)})
		o.Observations = append(o.Observations, evals.TierEvidence{
			Person: person, Tier: evals.BehaviouralObservation, Provenance: evals.SyntheticProvenance,
			Source: eventID, Observer: person, At: t.Decision.Human.At,
			Metric: "observed_choice", Value: core.ObservedGroupQuantity(1),
		})
		break
	}
	for _, e := range r.Experiences {
		if e.Participant != person {
			continue
		}
		// Every later report is retained. The FIRST report sets the disposition,
		// and its participation is read before deciding it: a report whose
		// participation is unknown leaves the outcome unresolved rather than
		// silently resolved. A later positive report never erases an earlier
		// unknown or adverse one.
		if e.Participation == "unwelcome" {
			o.Unwanted++
		}
		if !firstReport {
			firstReport = true
			if e.Participation == "unknown" {
				o.DelayedOutcome = "unresolved"
			} else {
				o.DelayedOutcome = "resolved"
			}
			o.Benefit = e.Benefit
			// BurdenReduction is a REDUCTION in burden: opposite meaning and a
			// different scale. Burden itself was not observed and stays unknown.
			o.Burden = core.UnknownGroupQuantity()
			o.BurdenReduction = e.BurdenReduction
		}
		// A later self-report is about the OPPORTUNITY it actually reports, not
		// an unrelated earlier action that merely happened first. The event
		// record is emitted for that opportunity, in this arm.
		oppID, oppAt, found := opportunityFor(e.Opportunity, person, r, arm)
		if found && e.LearnedAt > oppAt {
			if !seenOpportunity[oppID] {
				seenOpportunity[oppID] = true
				recs = append(recs, evals.SourceRecord{ID: oppID, Kind: "observed_choice",
					Subject: person, Observer: person, At: oppAt, Arm: arm,
					Metric: "observed_choice", Value: core.ObservedGroupQuantity(1),
					Content: fmt.Sprintf("opportunity %s", e.Opportunity)})
			}
			// Records are namespaced by arm: the same experience identity recurs
			// in each arm's run and they are distinct observations.
			recID := core.ID(fmt.Sprintf("report:%s:%s:%d:%s", r.Family, arm, r.Seed, e.ID))
			recs = append(recs, evals.SourceRecord{ID: recID, Kind: "later_self_report", Subject: e.Participant,
				Observer: e.Observer, At: e.LearnedAt, About: oppID, Arm: arm,
				Metric: "reported_benefit", Value: e.Benefit, Content: "participant self-report"})
			o.Observations = append(o.Observations, evals.TierEvidence{
				Person: person, Tier: evals.AttributedLater, Provenance: evals.SyntheticProvenance,
				Source: recID, Observer: e.Observer, At: e.LearnedAt,
				Metric: "reported_benefit", Value: e.Benefit,
			})
		}
	}
	return o, recs
}

// opportunityFor resolves the actual opportunity a later report is about, and
// its logical time, within this run and arm.
// The record is per person: each participant's later report is about their own
// experience of that opportunity, so each carries its own attributable event.
func opportunityFor(opp core.ID, person core.ID, r ordinaryexperiment.Report, arm evals.Arm) (core.ID, core.LogicalTime, bool) {
	for _, o := range r.Opportunities {
		if o.Helper.ID == opp {
			return core.ID(fmt.Sprintf("opportunity:%s:%s:%d:%s:%s", r.Family, arm, r.Seed, opp, person)), o.Helper.At, true
		}
	}
	return "", 0, false
}

// firstActionTime is the LOGICAL TIME of this person's first observed action.
// A frame index is not a logical time: comparing a later report against a frame
// number compares incompatible units.
func firstActionTime(person core.ID, r ordinaryexperiment.Report) core.LogicalTime {
	for _, t := range r.Traces {
		if t.Actor == person && t.Decision.Human.Candidates[t.Decision.Human.Selected].Offer.Kind != behavior.Wait {
			return t.Decision.Human.At
		}
	}
	return 0
}
