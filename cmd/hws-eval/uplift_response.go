package main

import (
	"context"
	"fmt"

	"github.com/tushardhara/dream/app/assistance"
	"github.com/tushardhara/dream/app/hws"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/evals"
	"github.com/tushardhara/dream/examples/responseexperiment"
	"github.com/tushardhara/dream/simulator/behavior"
)

// responseScenes are the recipient-response scenarios. The first two are the
// wanted/unwanted pair as the RECIPIENT experienced it — Expectation is Bob's
// own private expectation and is never supplied to the helper — and the rest
// are the ways a delayed outcome fails to arrive. They are included precisely
// because they do not resolve: an evaluation that only ran the clean scenes
// would never exercise a missing or censored outcome.
var responseScenes = []struct {
	name  string
	scene responseexperiment.Scene
}{
	{"welcome", responseexperiment.Scene{Expectation: 1, Observation: "observed", ShareReport: true, Followup: true}},
	{"unwanted", responseexperiment.Scene{Expectation: -1, Observation: "observed", ShareReport: true, Followup: true}},
	{"silence", responseexperiment.Scene{Expectation: -1, Observation: "silence", ShareReport: true, Followup: true}},
	{"declined_participation", responseexperiment.Scene{Expectation: -1, Observation: "declined_participation", ShareReport: true, Followup: true}},
	{"missing_followup", responseexperiment.Scene{Expectation: -1, Observation: "observed", ShareReport: true, Followup: false}},
}

// responseComparisons executes the recipient-response consumer across all four
// arms. Unlike the assistance consumer this one carries a real delayed-outcome
// instrument: recipients author later self-reports with measured benefit and
// burden, and the scenes include outcomes that never arrive.
func responseComparisons(ctx context.Context) ([]evals.Comparison, error) {
	out := []evals.Comparison{}
	for _, sc := range responseScenes {
		for _, seed := range upliftSeeds {
			c := evals.Comparison{
				Version:  evals.UpliftVersion,
				Scenario: core.ID(fmt.Sprintf("scenario:recipient_response:%s", sc.name)),
				Family:   "wanted_unwanted_help",
				Seed:     seed,
				Affected: []core.ID{"alice", "bob"},
			}
			for _, m := range helperArms {
				run, e := responseexperiment.Run(ctx, sc.scene, m.consumer, seed, nil)
				if e != nil {
					return nil, fmt.Errorf("response/%s/%s/%d: %w", sc.name, m.consumer, seed, e)
				}
				r := evals.ArmRun{
					Arm: m.arm, Seed: seed,
					WorldHash: run.Manifest.FixtureHash,
					// This consumer realizes no exogenous resources, so the
					// generator's exogenous stream receipt is what there is to
					// record. run.Actions is NOT used here: those are the human
					// actions chosen after advice and are arm-dependent, which
					// is the experiment working rather than an exogenous event.
					ExogenousHash: fmt.Sprint(run.Manifest.ExogenousSeed),
					Streams: []evals.Stream{
						{Domain: "human", Seed: fmt.Sprint(run.Manifest.HumanSeed)},
						{Domain: "exogenous", Seed: fmt.Sprint(run.Manifest.ExogenousSeed)},
						{Domain: "helper", Seed: fmt.Sprint(run.Manifest.HelperSeed)},
					},
					PolicyHash: assistance.Digest(run.Helper),
					HumanHash:  assistance.Digest(run.Humans),
					Scenario:   c.Scenario,
				}
				// A real, executed privacy check rather than a field nobody
				// writes. The same scene is re-run with ONLY the recipient's
				// private conditions changed; the helper never receives those,
				// so its output must be byte-identical. If it is not, private
				// recipient state reached the helper's output, and that is a
				// boundary violation attributed to whoever's state leaked.
				leak, e := privateStateLeaks(ctx, sc.scene, m.consumer, seed, run)
				if e != nil {
					return nil, fmt.Errorf("response/%s/%s/%d privacy probe: %w", sc.name, m.consumer, seed, e)
				}
				for _, person := range c.Affected {
					o, recs := responseOutcomeFor(person, run, m.arm, sc.name)
					if leak && person == privateStateOwner {
						o.BoundaryViolations++
					}
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
				return nil, fmt.Errorf("response/%s/%d: %w", sc.name, seed, e)
			}
			out = append(out, c)
		}
	}
	return out, nil
}

// responseOutcomeFor reads one person's outcome out of the executed run. The
// recipient's own later self-report is the only thing read as independently
// attributed later evidence: a sender's account of how their help landed is a
// report about someone else's experience, and the source contract says so —
// "a sender feeling useful is not recipient benefit".
func responseOutcomeFor(person core.ID, run hws.ResponsiveAssistanceRun, arm evals.Arm, scene string) (evals.PersonOutcome, []evals.SourceRecord) {
	o := evals.PersonOutcome{
		Person: person, Benefit: core.UnknownGroupQuantity(), Burden: core.UnknownGroupQuantity(),
		Appropriateness: core.UnknownGroupQuantity(), BurdenReduction: core.UnknownGroupQuantity(),
		DelayedOutcome: "unresolved",
	}
	for _, d := range run.Humans {
		h := d.Human.Human
		if h.Actor != person {
			continue
		}
		if len(h.Candidates) > 1 {
			o.CouldAct = true
		}
		if h.Candidates[h.Selected].Offer.Kind != behavior.Wait {
			o.Acted = true
		}
	}
	// The event a later report is about is this person's own IMMEDIATE account
	// of the same interaction, which the consumer records earlier in logical
	// time. The sender's immediate account is deliberately not eligible: it is
	// an expectation about someone else's experience, and this contract already
	// holds that a sender feeling useful is not recipient benefit.
	immediate := map[core.ID]core.OutcomeObservation{}
	for _, ob := range run.Outcomes {
		if ob.Phase == "immediate" && ob.Position == "recipient" && ob.Participant == person {
			if _, seen := immediate[ob.Interaction]; !seen {
				immediate[ob.Interaction] = ob
			}
		}
	}
	recs := []evals.SourceRecord{}
	first := true
	for _, ob := range run.Outcomes {
		// The Position check here is DEFENSIVE and is not exercised by this
		// consumer: it emits no later sender-position observations at all
		// (measured across every executed scene, arm and seed). It is kept so
		// a consumer that later does emit one cannot have it read as the
		// recipient's own account, and is marked rather than claimed as tested.
		if ob.Participant != person || ob.Phase != "later" || ob.Position != "recipient" {
			continue
		}
		// The DISPOSITION is read from every later recipient observation,
		// including the ones the recipient could not author. This separation
		// matters: an outcome that never arrived is recorded precisely because
		// nobody reported it, so gating the disposition on self_report would
		// erase every missing and censored result — which is what the scenes
		// exercising silence, lost observations, declined participation and
		// absent follow-ups exist to produce.
		if first {
			first = false
			switch {
			case ob.Status == core.Censored:
				o.DelayedOutcome = "censored"
			case ob.Missing != "":
				o.DelayedOutcome = "missing"
			case ob.Status == core.Observed && ob.Benefit != nil:
				o.DelayedOutcome = "resolved"
			default:
				// Reached but with nothing measurable is unresolved, not
				// resolved and not missing.
				o.DelayedOutcome = "unresolved"
			}
			o.Benefit = quantity(ob.Status, ob.Benefit)
			o.Burden = quantity(ob.Status, ob.Burden)
		}
		// A dismissive appraisal is an unwanted intervention, taken from the
		// recipient's own account rather than inferred from the scene name.
		if ob.Appraisal == "dismissive" {
			o.Unwanted++
		}
		// EVIDENCE, unlike the disposition, requires the subject's own report:
		// a later outcome the recipient did not author is not their self-report
		// and may not be cited as independently attributed later evidence.
		if ob.Basis != "self_report" {
			continue
		}
		// Only an observed value is a measurement. An unknown or censored
		// report is retained as the disposition above but never cited as one.
		if ob.Status != core.Observed || ob.Benefit == nil {
			continue
		}
		earlier, ok := immediate[ob.Interaction]
		// No earlier account of this interaction by this person means there is
		// nothing for the report to be later THAN. It is dropped rather than
		// bound to a surrogate time.
		if !ok || ob.LearnedAt <= earlier.OccurredAt {
			continue
		}
		event := recordID("interaction", scene, arm, run.Manifest.HumanSeed, earlier.Meta.ID)
		if !hasRecord(recs, event) {
			recs = append(recs, evals.SourceRecord{ID: event, Kind: "observed_choice", Subject: person,
				Observer: person, At: earlier.OccurredAt, Arm: arm,
				Metric: "observed_choice", Value: core.ObservedGroupQuantity(1),
				Content: fmt.Sprintf("immediate account of interaction %s", ob.Interaction)})
		}
		id := recordID("later", scene, arm, run.Manifest.HumanSeed, ob.Meta.ID)
		v := core.ObservedGroupQuantity(*ob.Benefit)
		recs = append(recs, evals.SourceRecord{ID: id, Kind: "later_self_report", Subject: person,
			Observer: person, At: ob.LearnedAt, About: event, Arm: arm,
			Metric: "reported_benefit", Value: v, Content: "recipient self-report"})
		o.Observations = append(o.Observations, evals.TierEvidence{
			Person: person, Tier: evals.AttributedLater, Provenance: evals.SyntheticProvenance,
			Source: id, Observer: person, At: ob.LearnedAt,
			Metric: "reported_benefit", Value: v,
		})
	}
	return o, recs
}

// quantity keeps an unobserved value unknown rather than reading a nil pointer
// as zero: "not measured" and "measured as zero" are different findings.
// The status half of this check is DEFENSIVE: this consumer never emits a
// value alongside a non-observed status, so only the nil check is exercised.
func quantity(status core.OutcomeStatus, v *float64) core.GroupQuantity {
	if status != core.Observed || v == nil {
		return core.UnknownGroupQuantity()
	}
	return core.ObservedGroupQuantity(*v)
}

// recordID namespaces a record by kind, scene, arm and seed and digests the
// consumer's own identifier into it, so distinct source records stay distinct
// while fitting the id contract.
func recordID(kind, scene string, arm evals.Arm, seed uint64, of core.ID) core.ID {
	return core.ID(fmt.Sprintf("%s:%s:%s:%d:%s", kind, scene, arm, seed,
		assistance.Digest(of)[:32]))
}

func hasRecord(in []evals.SourceRecord, id core.ID) bool {
	for _, r := range in {
		if r.ID == id {
			return true
		}
	}
	return false
}

// upliftChecks names the adversarial probes this run actually executes. A harm
// column that nothing writes to reports zero forever; naming the executed
// checks is what lets a reader tell that apart from a measured zero.
var upliftChecks = []string{
	"boundary/privacy: each recipient-response scene re-run at the same arm and seed with every private recipient condition changed and nothing else; a difference in the helper's interactions is recorded as a boundary violation. Bounded invariance over the conditions these scenes vary, not an exhaustive privacy proof.",
	"broken control: an arm in which every human waits is rejected, and a no-assistant arm that performed helper actions is rejected.",
	"identical arms: two arms whose policy output matches produced the same intervention and can never be read as uplift.",
	"engagement metrics: disqualified as benefit by contract at record validation, not by convention.",
}

// privateStateOwner is whose private conditions the scene varies. Scene's own
// documentation names the expectation as Bob's, and it is never supplied to the
// helper.
const privateStateOwner = core.ID("bob")

// privateStateLeaks re-runs the same scene, arm and seed with only the private
// recipient conditions changed and reports whether the helper's interactions
// differ. The helper is never given those conditions, so any difference means
// they reached its output. This is a bounded invariance check over the
// conditions this scene actually varies, not an exhaustive privacy proof.
func privateStateLeaks(ctx context.Context, sc responseexperiment.Scene, arm assistance.Arm, seed uint64, base hws.ResponsiveAssistanceRun) (bool, error) {
	variant, e := privateVariant(sc)
	if e != nil {
		return false, e
	}
	other, e := responseexperiment.Run(ctx, variant, arm, seed, nil)
	if e != nil {
		return false, e
	}
	return helperOutputDiffers(base, other), nil
}

// privateVariant changes EVERY private recipient condition the scene carries
// and nothing else. Varying only one of them would leave a probe that passes
// while the helper leaks the other, and a variant equal to its scene is a probe
// that proves nothing at all, so both are refused rather than run.
func privateVariant(sc responseexperiment.Scene) (responseexperiment.Scene, error) {
	variant := sc
	variant.Expectation = -sc.Expectation
	variant.PrivateFear = 1 - sc.PrivateFear
	if variant.Expectation == sc.Expectation || variant.PrivateFear == sc.PrivateFear {
		return sc, fmt.Errorf("private variant does not change every private condition, so this probe proves nothing")
	}
	return variant, nil
}

// helperOutputDiffers is separated so the detection itself can be exercised
// without needing a consumer that actually leaks.
func helperOutputDiffers(a, b hws.ResponsiveAssistanceRun) bool {
	return assistance.Digest(a.Helper) != assistance.Digest(b.Helper)
}
