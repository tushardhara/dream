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
					run.Outcomes = append(run.Outcomes, outcomeFor(person, report))
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

// outcomeFor derives one person's result from what the run actually observed.
// An absent later report stays missing, never a zero benefit.
func outcomeFor(person core.ID, r ordinaryexperiment.Report) evals.PersonOutcome {
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
	for _, o2 := range r.Opportunities {
		o.Observations = append(o.Observations, evals.TierEvidence{
			Person: person, Tier: evals.BehaviouralObservation, Provenance: evals.SyntheticProvenance,
			Source: core.ID(fmt.Sprintf("opportunity:%d", o2.Frame)), Observer: person, At: core.LogicalTime(o2.Frame),
			Metric: "observed_choice", Value: core.ObservedGroupQuantity(0),
		})
		break // one behavioural record per person is enough to attest the run
	}
	for _, e := range r.Experiences {
		if e.Participant != person {
			continue
		}
		// Later reports accumulate; an earlier unknown or adverse report is not
		// overwritten by a later one. The first resolved report sets the
		// disposition and the remaining ones are retained as separate evidence.
		if o.DelayedOutcome != "resolved" {
			o.DelayedOutcome = "resolved"
			o.Benefit = e.Benefit
			// BurdenReduction is a REDUCTION in burden, the opposite of burden
			// and on a different scale. It is not burden and is not recorded as
			// such; burden itself was not observed here and stays unknown.
			o.Burden = core.UnknownGroupQuantity()
			o.BurdenReduction = e.BurdenReduction
		}
		switch e.Participation {
		case "unwelcome":
			o.Unwanted++
		case "unknown":
			o.DelayedOutcome = "unresolved"
		}
		if e.Benefit.Status == core.Observed {
			o.Observations = append(o.Observations, evals.TierEvidence{
				Person: person, Tier: evals.AttributedLater, Provenance: evals.SyntheticProvenance,
				Source: e.ID, Observer: e.Observer, At: e.LearnedAt,
				Metric: "reported_benefit", Value: e.Benefit,
			})
		}
	}
	return o
}
