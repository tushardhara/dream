package main

import (
	"context"
	"fmt"

	"github.com/tushardhara/dream/app/assistance"
	"github.com/tushardhara/dream/app/hws"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/evals"
	"github.com/tushardhara/dream/examples/helperexperiment"
	"github.com/tushardhara/dream/simulator/behavior"
)

// helperArms maps the assistance consumer's arms onto the evaluation arms. This
// consumer implements all four, so multi_perspective is executed here rather
// than reported unimplemented — it is not fabricated, it exists.
var helperArms = []struct {
	consumer assistance.Arm
	arm      evals.Arm
}{
	{assistance.None, evals.NoAssistant},
	{assistance.Simple, evals.SimpleAssistance},
	{assistance.Single, evals.SinglePerspective},
	{assistance.Multi, evals.MultiPerspective},
}

// helperGoals are the two halves of the wanted/unwanted question. Coordinate is
// help the user asked for; Unknown is a user who stated no goal at all, so any
// intervention delivered to them was not requested.
var helperGoals = []struct {
	goal assistance.Goal
	name string
}{
	{assistance.Coordinate, "wanted"},
	{assistance.Unknown, "unstated"},
}

// helperComparisons executes the real assistance consumer across all four arms.
// It carries no delayed-outcome instrument, and says so rather than inventing
// one: every outcome here is not_instrumented, and the findings it supports are
// behavioural only.
func helperComparisons(ctx context.Context) ([]evals.Comparison, error) {
	out := []evals.Comparison{}
	for _, g := range helperGoals {
		for _, seed := range upliftSeeds {
			c := evals.Comparison{
				Version:  evals.UpliftVersion,
				Scenario: core.ID(fmt.Sprintf("scenario:wanted_unwanted_help:%s", g.name)),
				Family:   "wanted_unwanted_help",
				Seed:     seed,
				Affected: []core.ID{"alice", "bob"},
			}
			for _, m := range helperArms {
				run, e := helperexperiment.Run(ctx, m.consumer, g.goal, seed, nil)
				if e != nil {
					return nil, fmt.Errorf("helper/%s/%s/%d: %w", g.name, m.consumer, seed, e)
				}
				// Receipts the experiment produced. FixtureHash is the world
				// digest it computed itself; the realized exogenous resources
				// carry their own draws; the manifest's three seeds are the
				// common random numbers, which the generator derives without
				// reference to the arm. Only the helper interactions differ.
				r := evals.ArmRun{
					Arm: m.arm, Seed: seed,
					WorldHash:     run.Manifest.FixtureHash,
					ExogenousHash: assistance.Digest(run.Exogenous),
					// The three versioned streams the generator derived, each
					// recorded under the domain it serves. They are identical
					// across arms because NewAssistanceManifest derives them
					// without reference to the arm.
					Streams: []evals.Stream{
						{Domain: "human", Seed: fmt.Sprint(run.Manifest.HumanSeed)},
						{Domain: "exogenous", Seed: fmt.Sprint(run.Manifest.ExogenousSeed)},
						{Domain: "helper", Seed: fmt.Sprint(run.Manifest.HelperSeed)},
					},
					PolicyHash: assistance.Digest(run.Helper),
					HumanHash:  assistance.Digest(run.Humans),
					Scenario:   c.Scenario,
				}
				for _, person := range c.Affected {
					o, recs := helperOutcomeFor(person, run, m.arm, g.goal)
					r.Outcomes = append(r.Outcomes, o)
					c.Sources = append(c.Sources, recs...)
				}
				for _, i := range run.Helper {
					if i.Delivered {
						r.HelperActs++
					}
				}
				c.Runs = append(c.Runs, r)
			}
			if e := c.Validate(); e != nil {
				return nil, fmt.Errorf("helper/%s/%d: %w", g.name, seed, e)
			}
			out = append(out, c)
		}
	}
	return out, nil
}

// helperOutcomeFor reads one person's observed behaviour out of the executed
// run. Benefit, burden and appropriateness stay unknown because this consumer
// measures none of them, and the delayed outcome is not_instrumented rather
// than missing: it was never asked, not asked and lost.
func helperOutcomeFor(person core.ID, run hws.AssistanceRun, arm evals.Arm, goal assistance.Goal) (evals.PersonOutcome, []evals.SourceRecord) {
	o := evals.PersonOutcome{
		Person: person, Benefit: core.UnknownGroupQuantity(), Burden: core.UnknownGroupQuantity(),
		Appropriateness: core.UnknownGroupQuantity(), BurdenReduction: core.UnknownGroupQuantity(),
		DelayedOutcome: "not_instrumented",
	}
	// An intervention delivered to someone who stated no goal was not asked
	// for. This is the one unwanted-help signal this consumer actually
	// supports; it is not inferred for a user who did state a goal.
	if goal == assistance.Unknown {
		for _, i := range run.Helper {
			if i.Delivered && i.Result.Candidates[i.Result.Selected].Recipient == person {
				o.Unwanted++
			}
		}
	}
	recs := []evals.SourceRecord{}
	for _, d := range run.Humans {
		if d.Actor != person {
			continue
		}
		if len(d.Candidates) > 1 {
			o.CouldAct = true
		}
		if d.Candidates[d.Selected].Offer.Kind == behavior.Wait {
			continue
		}
		o.Acted = true
		if len(o.Observations) > 0 {
			continue
		}
		id := core.ID(fmt.Sprintf("helper-choice:%s:%s:%d:%s", goal, arm, run.Manifest.HumanSeed, person))
		recs = append(recs, evals.SourceRecord{ID: id, Kind: "observed_choice", Subject: person,
			Observer: person, At: d.At, Arm: arm,
			Metric: "observed_choice", Value: core.ObservedGroupQuantity(1),
			Content: fmt.Sprintf("selected %v", d.Candidates[d.Selected].Offer.Kind)})
		o.Observations = append(o.Observations, evals.TierEvidence{
			Person: person, Tier: evals.BehaviouralObservation, Provenance: evals.SyntheticProvenance,
			Source: id, Observer: person, At: d.At,
			Metric: "observed_choice", Value: core.ObservedGroupQuantity(1),
		})
	}
	return o, recs
}
