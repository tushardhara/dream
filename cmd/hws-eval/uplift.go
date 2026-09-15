package main

import (
	"context"
	"fmt"

	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/evals"
	"github.com/tushardhara/dream/examples/ordinaryexperiment"
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
				// The control arm must not be credited with helper activity.
				if m.arm == evals.NoAssistant {
					run.HelperActs = 0
				}
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
		Appropriateness: core.UnknownGroupQuantity(), DelayedOutcome: "missing",
	}
	for _, t := range r.Traces {
		if t.Actor == person {
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
		o.DelayedOutcome = "resolved"
		o.Benefit = e.Benefit
		o.Burden = e.BurdenReduction
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
