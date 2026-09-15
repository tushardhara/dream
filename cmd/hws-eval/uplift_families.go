package main

import (
	"context"
	"fmt"

	"github.com/tushardhara/dream/app/assistance"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/evals"
	"github.com/tushardhara/dream/examples/boundaryexperiment"
	"github.com/tushardhara/dream/examples/domainexperiment"
	"github.com/tushardhara/dream/examples/groupexperiment"
	"github.com/tushardhara/dream/examples/listeningclient"
	"github.com/tushardhara/dream/examples/repairclient"
	"github.com/tushardhara/dream/examples/temporalexperiment"
	"github.com/tushardhara/dream/simulator/behavior"
)

// None of the four consumers below carries a delayed-outcome instrument: none
// of them asks anyone afterwards how it went. Every outcome they produce is
// therefore not_instrumented, and the comparisons they support are behavioural
// only. That is recorded rather than filled in.
const noOutcomeInstrument = "not_instrumented"

// unmeasured is the honest starting point for a consumer that measures no
// benefit, burden, appropriateness or burden reduction: every quantity unknown,
// and a delayed outcome that was never instrumented rather than one that went
// missing.
func unmeasured(person core.ID) evals.PersonOutcome {
	return evals.PersonOutcome{
		Person: person, Benefit: core.UnknownGroupQuantity(), Burden: core.UnknownGroupQuantity(),
		Appropriateness: core.UnknownGroupQuantity(), BurdenReduction: core.UnknownGroupQuantity(),
		DelayedOutcome: noOutcomeInstrument,
	}
}

// choiceRecord emits the behavioural observation for a person who actually
// chose to act, and nothing at all for a person who waited. A trace that exists
// but selected WAIT is not evidence that anyone acted.
func choiceRecord(o *evals.PersonOutcome, arm evals.Arm, scope string, at core.LogicalTime, d behavior.ActionDecision) []evals.SourceRecord {
	// Whether this person had anything to choose besides waiting. A decision
	// offering only WAIT is not restraint, it is an absence of options, and the
	// contract needs the two kept apart.
	if len(d.Candidates) > 1 {
		o.CouldAct = true
	}
	kind := d.Candidates[d.Selected].Offer.Kind
	if kind == behavior.Wait {
		return nil
	}
	o.Acted = true
	if len(o.Observations) > 0 {
		return nil
	}
	id := recordID("choice", scope, arm, 0, core.ID(fmt.Sprintf("%s-%d", o.Person, at)))
	o.Observations = append(o.Observations, evals.TierEvidence{
		Person: o.Person, Tier: evals.BehaviouralObservation, Provenance: evals.SyntheticProvenance,
		Source: id, Observer: o.Person, At: at,
		Metric: "observed_choice", Value: core.ObservedGroupQuantity(1),
	})
	return []evals.SourceRecord{{ID: id, Kind: "observed_choice", Subject: o.Person,
		Observer: o.Person, At: at, Arm: arm,
		Metric: "observed_choice", Value: core.ObservedGroupQuantity(1),
		Content: fmt.Sprintf("selected %v", kind)}}
}

// boundaryComparisons executes the scoped-boundary consumer across all four
// arms over both synthetic signals.
func boundaryComparisons(ctx context.Context) ([]evals.Comparison, error) {
	out := []evals.Comparison{}
	for _, signal := range []string{"disagreement", "credible_pressure"} {
		c := evals.Comparison{Version: evals.UpliftVersion,
			Scenario: core.ID("scenario:selective_boundaries:" + signal),
			Family:   "selective_boundaries", Seed: 7,
			Affected: []core.ID{"alice", "bob"}}
		for _, m := range helperArms {
			tr, e := boundaryexperiment.Run(ctx, signal, m.consumer, nil)
			if e != nil {
				return nil, fmt.Errorf("boundary/%s/%s: %w", signal, m.consumer, e)
			}
			r := evals.ArmRun{Arm: m.arm, Seed: 7,
				// This consumer is deterministic and takes no seed: its world
				// and exogenous events are the fixture itself, identical across
				// arms by construction rather than by draw. One stream is
				// recorded because it draws from one.
				WorldHash:     assistance.Digest("boundary-fixture/" + signal),
				ExogenousHash: assistance.Digest("boundary-frames/" + signal),
				Streams:       []evals.Stream{{Domain: "boundary", Seed: assistance.Digest("boundary/" + signal)}},
				PolicyHash:    assistance.Digest(tr.Helpers),
				HumanHash:     assistance.Digest(tr.Human),
				Scenario:      c.Scenario}
			for _, h := range tr.Helpers {
				if h.Delivered {
					r.HelperActs++
				}
			}
			// Alice is the only actor this consumer gives decisions for; Bob is
			// an affected third party whose outcome is accounted for as
			// unobserved rather than dropped from the comparison.
			alice, bob := unmeasured("alice"), unmeasured("bob")
			for i, d := range tr.Human {
				h := d.Human
				c.Sources = append(c.Sources, choiceRecord(&alice, m.arm, "boundary:"+signal,
					core.LogicalTime(i), h)...)
			}
			r.Outcomes = append(r.Outcomes, alice, bob)
			c.Runs = append(c.Runs, r)
		}
		if e := c.Validate(); e != nil {
			return nil, fmt.Errorf("boundary/%s: %w", signal, e)
		}
		out = append(out, c)
	}
	return out, nil
}

// domainComparisons executes the domain/role-trust consumer across all four
// arms over both role contexts.
func domainComparisons(ctx context.Context) ([]evals.Comparison, error) {
	out := []evals.Comparison{}
	for _, role := range []core.ID{"family", "business"} {
		focus := core.RelationshipFocus{Version: core.RelationshipFocusVersion, Domain: core.Childcare, RoleContext: role}
		c := evals.Comparison{Version: evals.UpliftVersion,
			Scenario: core.ID("scenario:role_domain_trust:" + string(role)),
			Family:   "role_domain_trust", Seed: 7,
			Affected: []core.ID{"alice", "bob"}}
		for _, m := range helperArms {
			tr, e := domainexperiment.Run(ctx, focus, m.consumer, nil)
			if e != nil {
				return nil, fmt.Errorf("domain/%s/%s: %w", role, m.consumer, e)
			}
			r := evals.ArmRun{Arm: m.arm, Seed: 7,
				WorldHash:     assistance.Digest("domain-profiles/" + string(role)),
				ExogenousHash: assistance.Digest("domain-frame/" + string(role)),
				Streams:       []evals.Stream{{Domain: "domain", Seed: assistance.Digest("domain/" + string(role))}},
				PolicyHash:    assistance.Digest(tr.Helper),
				HumanHash:     assistance.Digest(tr.Human),
				Scenario:      c.Scenario}
			if tr.Helper.Delivered {
				r.HelperActs++
			}
			alice, bob := unmeasured("alice"), unmeasured("bob")
			h := tr.Human.Human.Human
			c.Sources = append(c.Sources, choiceRecord(&alice, m.arm, "domain:"+string(role),
				h.At, h)...)
			r.Outcomes = append(r.Outcomes, alice, bob)
			c.Runs = append(c.Runs, r)
		}
		if e := c.Validate(); e != nil {
			return nil, fmt.Errorf("domain/%s: %w", role, e)
		}
		out = append(out, c)
	}
	return out, nil
}

// temporalPolicies maps this consumer's policy strings onto the evaluation
// arms. context_off is a fourth policy this consumer implements and is not one
// of the required arms, so it is not run here rather than relabelled as one.
var temporalPolicies = []struct {
	policy string
	arm    evals.Arm
}{
	{"none", evals.NoAssistant},
	{"simple", evals.SimpleAssistance},
	{"temporal", evals.SinglePerspective},
}

// temporalComparisons executes the life-rhythms consumer. It runs three arms:
// multi_perspective has no distinct policy in this consumer and is reported not
// executed rather than filled with a duplicate of another arm.
func temporalComparisons(ctx context.Context) ([]evals.Comparison, error) {
	out := []evals.Comparison{}
	for _, scene := range temporalexperiment.Scenes() {
		for _, seed := range upliftSeeds {
			c := evals.Comparison{Version: evals.UpliftVersion,
				Scenario: core.ID("scenario:life_changes:" + scene.Name),
				Family:   "life_changes", Seed: seed,
				Affected: []core.ID{"alice", "bob"}}
			for _, m := range temporalPolicies {
				tr, e := temporalexperiment.Run(ctx, scene, m.policy, 30, seed, "", nil)
				if e != nil {
					return nil, fmt.Errorf("temporal/%s/%s/%d: %w", scene.Name, m.policy, seed, e)
				}
				r := evals.ArmRun{Arm: m.arm, Seed: seed,
					WorldHash:     assistance.Digest(fmt.Sprintf("temporal-world/%s/%d", scene.Name, seed)),
					ExogenousHash: assistance.Digest(fmt.Sprintf("temporal-scene/%s/%d", scene.Name, seed)),
					Streams:       []evals.Stream{{Domain: "temporal", Seed: assistance.Digest(fmt.Sprintf("temporal/%s/%d", scene.Name, seed))}},
					PolicyHash:    assistance.Digest(tr.Helper),
					HumanHash:     assistance.Digest(tr.Humans),
					Scenario:      c.Scenario}
				if tr.Helper.Delivered {
					r.HelperActs++
				}
				for _, person := range c.Affected {
					o := unmeasured(person)
					// This consumer counts boundary violations against
					// independently authored boundaries; they are carried
					// through rather than recomputed here.
					for _, h := range tr.Humans {
						if h.Actor != person {
							continue
						}
						o.BoundaryViolations += h.PrivateViolations
						d := h.Decision.Human.Human.Human
						c.Sources = append(c.Sources, choiceRecord(&o, m.arm, "temporal:"+scene.Name,
							d.At, d)...)
					}
					r.Outcomes = append(r.Outcomes, o)
				}
				c.Runs = append(c.Runs, r)
			}
			if e := c.Validate(); e != nil {
				return nil, fmt.Errorf("temporal/%s/%d: %w", scene.Name, seed, e)
			}
			out = append(out, c)
		}
	}
	return out, nil
}

// groupComparisons executes the group-burden consumer across all four arms.
func groupComparisons(ctx context.Context) ([]evals.Comparison, error) {
	out := []evals.Comparison{}
	for _, omitted := range []bool{false, true} {
		name := "reported"
		if omitted {
			name = "omitted_contributions"
		}
		fixture, e := groupexperiment.Fixture(5, 11, omitted, 4)
		if e != nil {
			return nil, fmt.Errorf("group/%s fixture: %w", name, e)
		}
		c := evals.Comparison{Version: evals.UpliftVersion,
			Scenario: core.ID("scenario:group_burden:" + name),
			Family:   "group_burden", Seed: 11,
			Affected: []core.ID{groupexperiment.Person(0), groupexperiment.Person(1)}}
		for _, m := range helperArms {
			rep, e := groupexperiment.RunArm(ctx, fixture, m.consumer)
			if e != nil {
				return nil, fmt.Errorf("group/%s/%s: %w", name, m.consumer, e)
			}
			r := evals.ArmRun{Arm: m.arm, Seed: 11,
				WorldHash:     assistance.Digest(fixture.Native.Scenario),
				ExogenousHash: assistance.Digest(fixture.Native.History),
				Streams:       []evals.Stream{{Domain: "group", Seed: assistance.Digest("group/" + name)}},
				PolicyHash:    assistance.Digest(rep.Helper),
				HumanHash:     assistance.Digest(rep.Native),
				Scenario:      c.Scenario}
			// The group helper cannot execute actions; a reservation is made
			// only by a native human choice, so that is what counts as an act.
			if rep.Execution != "WAIT" {
				r.HelperActs++
			}
			for _, person := range c.Affected {
				o := unmeasured(person)
				for _, tr := range rep.Native.Traces {
					if tr.Actor != person {
						continue
					}
					d := tr.Decision.Human
					c.Sources = append(c.Sources, choiceRecord(&o, m.arm, "group:"+name,
						d.At, d)...)
				}
				r.Outcomes = append(r.Outcomes, o)
			}
			c.Runs = append(c.Runs, r)
		}
		if e := c.Validate(); e != nil {
			return nil, fmt.Errorf("group/%s: %w", name, e)
		}
		out = append(out, c)
	}
	return out, nil
}

// repairComparisons executes the repair follow-through consumer across all four
// arms over both authored schedules.
//
// A trap worth naming, because it is exactly the fabricated uplift #58 asks to
// detect. RepairResponse.Expectation reads "unresolved" in the no-assistant arm
// and "sustained_follow_through_observed" or "repeated_breach_observed" in the
// others. It is tempting to map that onto a delayed outcome — and it would be
// wrong. The relationship history is the SAME authored schedule in every arm;
// the control reads "unresolved" because the helper did not look, not because
// anything went worse for anyone. Mapping it would make the control appear to
// harm people and hand every candidate arm an uplift it did not earn. What the
// helper reported is the policy receipt, not the outcome, and it is recorded as
// exactly that.
func repairComparisons(ctx context.Context) ([]evals.Comparison, error) {
	out := []evals.Comparison{}
	for _, follow := range []bool{false, true} {
		name := "repeated_apology_breach"
		if follow {
			name = "acknowledgement_follow_through"
		}
		c := evals.Comparison{Version: evals.UpliftVersion,
			Scenario: core.ID("scenario:repair:" + name),
			Family:   "repair", Seed: 1,
			Affected: []core.ID{"alice", "bob"}}
		for _, m := range helperArms {
			_, rep, e := repairclient.ScenarioArm(ctx, follow, m.consumer)
			if e != nil {
				return nil, fmt.Errorf("repair/%s/%s: %w", name, m.consumer, e)
			}
			r := evals.ArmRun{Arm: m.arm, Seed: 1,
				WorldHash:     assistance.Digest("repair-fixture/" + name),
				ExogenousHash: assistance.Digest(rep.Actions),
				Streams:       []evals.Stream{{Domain: "repair", Seed: assistance.Digest("repair/" + name)}},
				PolicyHash:    assistance.Digest(rep.Periods),
				// The participants' actions come from an authored command
				// schedule identical in every arm, so this receipt is shared by
				// construction and the comparison will say the assistant did not
				// reach any decision — which is true: no choice is modelled here.
				HumanHash: assistance.Digest(rep.Actions),
				Scenario:  c.Scenario}
			if rep.Periods[len(rep.Periods)-1].Next != "WAIT" {
				r.HelperActs++
			}
			for _, person := range c.Affected {
				// Nobody chooses in this consumer: the schedule is authored, so
				// no one acted and no one had an action available. That holds in
				// every arm alike, which is why it is not a crippled control.
				r.Outcomes = append(r.Outcomes, unmeasured(person))
			}
			c.Runs = append(c.Runs, r)
		}
		if e := c.Validate(); e != nil {
			return nil, fmt.Errorf("repair/%s: %w", name, e)
		}
		out = append(out, c)
	}
	return out, nil
}

// listeningComparisons executes the goal-aware listening consumer. It runs
// three arms: explicit-preference/simple assistance has no faithful realisation
// in this flow, which requires at least one account proposal by contract, so
// the nearest configuration would be identical to single perspective. It is
// reported not executed rather than listed as a second label for one policy.
func listeningComparisons(ctx context.Context) ([]evals.Comparison, error) {
	c := evals.Comparison{Version: evals.UpliftVersion,
		Scenario: core.ID("scenario:conflict_goals:joint_listening"),
		Family:   "conflict_goals", Seed: 4,
		Affected: []core.ID{"alice", "bob"}}
	for _, a := range listeningclient.ListeningArms {
		responses, e := listeningclient.RunArm(ctx, a.Arm)
		if e != nil {
			return nil, fmt.Errorf("listening/%s: %w", a.Arm, e)
		}
		arm := evals.NoAssistant
		switch a.Arm {
		case assistance.Single:
			arm = evals.SinglePerspective
		case assistance.Multi:
			arm = evals.MultiPerspective
		}
		ordered := []assistance.ListeningResponse{responses["alice"], responses["bob"]}
		r := evals.ArmRun{Arm: arm, Seed: 4,
			WorldHash:     assistance.Digest("listening-accounts"),
			ExogenousHash: assistance.Digest("listening-session"),
			Streams:       []evals.Stream{{Domain: "listening", Seed: assistance.Digest("listening/joint")}},
			PolicyHash:    assistance.Digest(ordered),
			// Both speakers author their own accounts before any helper runs,
			// and this flow models no action choice, so the people are identical
			// across arms by construction.
			HumanHash: assistance.Digest("listening-accounts-authored"),
			Scenario:  c.Scenario}
		for _, person := range c.Affected {
			if responses[person].Next != "WAIT" {
				r.HelperActs++
			}
			r.Outcomes = append(r.Outcomes, unmeasured(person))
		}
		c.Runs = append(c.Runs, r)
	}
	if e := c.Validate(); e != nil {
		return nil, fmt.Errorf("listening: %w", e)
	}
	return []evals.Comparison{c}, nil
}
