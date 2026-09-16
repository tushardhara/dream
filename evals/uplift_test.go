package evals

import (
	"fmt"
	"strings"
	"testing"

	"github.com/tushardhara/dream/core"
)

func ptr(c Comparison) *Comparison { return &c }

func person(i int) core.ID { return core.ID([]string{"person:01", "person:02"}[i]) }

// armEvidence namespaces a source id by arm, because a record collected in one
// arm is a different record from the one collected in another.
func armEvidence(a Arm, p core.ID, tier EvidenceTier, v float64) TierEvidence {
	e := evidence(p, tier, v)
	e.Source = core.ID(string(a) + ":" + string(e.Source))
	return e
}

func evidence(p core.ID, tier EvidenceTier, v float64) TierEvidence {
	e := TierEvidence{Person: p, Tier: tier, Provenance: SyntheticProvenance, Metric: "reported_benefit", Value: core.ObservedGroupQuantity(v)}
	e.Observer, e.At = p, 5
	e.Source = core.ID(fmt.Sprintf("record:%s:%s:%v", tier, p, v))
	return e
}

func outcome(i int, acted bool) PersonOutcome { return armOutcome(NoAssistant, i, acted) }

func armOutcome(a Arm, i int, acted bool) PersonOutcome {
	_ = fmt.Sprint
	return PersonOutcome{
		Person: person(i), Benefit: core.ObservedGroupQuantity(.2), Burden: core.UnknownGroupQuantity(),
		BurdenReduction: core.UnknownGroupQuantity(),
		Appropriateness: core.ObservedGroupQuantity(.5), DelayedOutcome: "unresolved", Acted: acted, CouldAct: true,
		Observations: []TierEvidence{armEvidence(a, person(i), BehaviouralObservation, .1)},
	}
}

func armRun(a Arm, policy string) ArmRun {
	return ArmRun{Arm: a, Seed: 7, WorldHash: "world-1", ExogenousHash: "exo-1",
		Streams:    []Stream{{Domain: "human", Seed: "h-1"}, {Domain: "exogenous", Seed: "x-1"}, {Domain: "helper", Seed: "p-1"}},
		PolicyHash: policy, HumanHash: "human-" + policy,
		Scenario: "scenario:ordinary", Outcomes: []PersonOutcome{armOutcome(a, 0, true), armOutcome(a, 1, false)}}
}

func comparison() Comparison {
	c := Comparison{Version: UpliftVersion, Scenario: "scenario:ordinary", Family: "ordinary_joy", Seed: 7, Affected: []core.ID{person(0), person(1)}}
	for i, a := range Arms {
		c.Runs = append(c.Runs, armRun(a, string(rune('a'+i))+"-policy"))
	}
	ledger(&c)
	return c
}

// ledger synthesises the evaluator-owned records that the fixture's evidence
// cites, so the untouched fixture is a valid positive control. Negatives below
// break the binding deliberately.
// sealed rebuilds each comparison's source ledger after a test has added
// evidence, so the citation binding stays a positive control and only the
// property under test varies.
func sealed(cs ...*Comparison) []Comparison {
	out := []Comparison{}
	for _, c := range cs {
		ledger(c)
		out = append(out, *c)
	}
	return out
}

func ledger(c *Comparison) {
	c.Sources = []SourceRecord{}
	seen := map[core.ID]bool{}
	for _, a := range Arms {
		c.Sources = append(c.Sources, SourceRecord{ID: core.ID(string(a) + ":event:opportunity"),
			Kind: "observed_choice", Subject: person(0), Observer: person(0), At: 0, Arm: a,
			Metric: "observed_choice", Value: core.ObservedGroupQuantity(1),
			Content: "the event later reports are about"})
		seen[core.ID(string(a)+":event:opportunity")] = true
	}
	for _, r := range c.Runs {
		for _, o := range r.Outcomes {
			for _, ob := range o.Observations {
				// Records are per-arm: the same evidence id in another arm is a
				// different record, so the key includes the arm.
				if seen[ob.Source] {
					continue
				}
				seen[ob.Source] = true
				rec := SourceRecord{ID: ob.Source, Kind: tierRequires[ob.Tier], Subject: ob.Person,
					Observer: ob.Observer, At: ob.At, Arm: r.Arm, Metric: ob.Metric, Value: ob.Value,
					Content: "synthetic fixture record"}
				if ob.Tier == AttributedLater {
					rec.About = core.ID(string(r.Arm) + ":event:opportunity")
				}
				c.Sources = append(c.Sources, rec)
			}
		}
	}
}

// POSITIVE CONTROL: the untouched fixture must validate, so every negative
// below is attributable to the single field it changes.
func TestUpliftComparisonPositiveControl(t *testing.T) {
	if e := comparison().Validate(); e != nil {
		t.Fatal("positive control rejected:", e)
	}
}

func TestUpliftContractRejectionsAreAttributable(t *testing.T) {
	for _, tc := range []struct {
		name, want string
		mutate     func(*Comparison)
	}{
		{"no-assistant control is crippled relative to the candidates", "control leaves everyone unable to act", func(c *Comparison) {
			for i := range c.Runs[0].Outcomes {
				c.Runs[0].Outcomes[i].Acted, c.Runs[0].Outcomes[i].CouldAct = false, false
			}
		}},
		{"a person acted without any action available", "acted without an available action", func(c *Comparison) {
			c.Runs[0].Outcomes[0].CouldAct = false
		}},
		{"no-assistant arm acts as a helper", "no-assistant arm performed helper actions", func(c *Comparison) {
			c.Runs[0].HelperActs = 1
		}},
		{"arms do not share the matched world", "arms do not share the matched initial world", func(c *Comparison) {
			c.Runs[2].WorldHash = "world-2"
		}},
		{"arms share exogenous events unequally", "arms do not share the matched initial world", func(c *Comparison) {
			c.Runs[1].ExogenousHash = "exo-2"
		}},
		{"an arm draws its own rng stream", "do not share the matched rng streams", func(c *Comparison) {
			c.Runs[3].Streams[0].Seed = "a-private-stream"
		}},
		{"an arm drops one of the matched streams", "do not share the matched rng streams", func(c *Comparison) {
			c.Runs[3].Streams = c.Runs[3].Streams[:2]
		}},
		{"an arm records no rng stream at all", "records no rng stream", func(c *Comparison) {
			c.Runs[3].Streams = nil
		}},
		{"two stream domains are the same stream renamed", "not independent of another", func(c *Comparison) {
			for i := range c.Runs {
				c.Runs[i].Streams[1].Seed = c.Runs[i].Streams[0].Seed
			}
		}},
		{"a stream domain is recorded twice", "duplicate rng stream domain", func(c *Comparison) {
			for i := range c.Runs {
				c.Runs[i].Streams[1].Domain = c.Runs[i].Streams[0].Domain
			}
		}},
		{"a stream receipt is empty", "invalid rng stream receipt", func(c *Comparison) {
			c.Runs[3].Streams[0].Seed = ""
		}},
		{"an arm reports no policy receipt", "invalid arm run identity", func(c *Comparison) {
			c.Runs[3].PolicyHash = ""
		}},
		{"an arm reports no human receipt", "invalid arm run identity", func(c *Comparison) {
			c.Runs[3].HumanHash = ""
		}},
		{"a comparison without the control is not a comparison", "must include the no-assistant control", func(c *Comparison) {
			c.Runs = c.Runs[1:]
		}},
		{"a single arm is not a comparison", "control and at least one candidate", func(c *Comparison) {
			c.Runs = c.Runs[:1]
		}},
		{"engagement cannot qualify as benefit", "metric cannot qualify as benefit", func(c *Comparison) {
			c.Runs[1].Outcomes[0].Observations[0].Metric = "session_time"
		}},
		{"action counts cannot qualify as benefit", "metric cannot qualify as benefit", func(c *Comparison) {
			c.Runs[1].Outcomes[0].Observations[0].Metric = "notification_opens"
		}},
		{"synthetic provenance cannot be dropped", "evidence provenance must be synthetic", func(c *Comparison) {
			c.Runs[1].Outcomes[0].Observations[0].Provenance = "REAL_HUMAN_STUDY"
		}},
		{"a system assertion cannot be promoted to later evidence", "evidence tier promoted above its source", func(c *Comparison) {
			o := &c.Runs[1].Outcomes[0].Observations[0]
			o.Tier, o.DerivedFrom = AttributedLater, SystemAssertion
		}},
		{"evidence attributed to another person", "observation attributed to another person", func(c *Comparison) {
			c.Runs[1].Outcomes[0].Observations[0].Person = person(1)
		}},
		{"delayed outcomes must keep a disposition", "invalid delayed outcome disposition", func(c *Comparison) {
			c.Runs[1].Outcomes[0].DelayedOutcome = ""
		}},
		{"one person cannot appear twice in an arm", "duplicate person in arm", func(c *Comparison) {
			// Move the evidence with the person so the duplicate is the ONLY violation.
			c.Runs[1].Outcomes[1].Person = person(0)
			c.Runs[1].Outcomes[1].Observations[0].Person = person(0)
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := comparison()
			tc.mutate(&c)
			e := c.Validate()
			if e == nil {
				t.Fatalf("accepted invalid comparison, want %q", tc.want)
			}
			if !strings.Contains(e.Error(), tc.want) {
				t.Fatalf("rejected for the wrong reason: want %q, got %q", tc.want, e.Error())
			}
		})
	}
}

// Uplift is never positive by construction.
func TestUpliftIsNotClaimedWithoutAttributedLaterEvidence(t *testing.T) {
	cs := []Comparison{comparison()}
	// The fixture carries only behavioural observations, plus helper activity.
	cs[0].Runs[3].HelperActs = 50
	f, e := CompareArms(cs, NoAssistant, MultiPerspective)
	if e != nil {
		t.Fatal(e)
	}
	if f.Status != NotTested {
		t.Fatalf("helper activity alone produced status %q, want %q", f.Status, NotTested)
	}
	if f.HumanValidity != NotTested || !f.SyntheticOnly {
		t.Fatal("finding must stay synthetic and human-validity NOT_TESTED")
	}
}

// Attributed later evidence that does not favour the candidate must not be
// reported as uplift.
func TestUpliftAllowsNoUplift(t *testing.T) {
	c := comparison()
	c.Runs[0].Outcomes[0].Observations = append(c.Runs[0].Outcomes[0].Observations, evidence(person(0), AttributedLater, .9))
	c.Runs[3].Outcomes[0].Observations = append(c.Runs[3].Outcomes[0].Observations, evidence(person(0), AttributedLater, .1))
	f, e := CompareArms(sealed(&c), NoAssistant, MultiPerspective)
	if e != nil {
		t.Fatal(e)
	}
	if f.Status != Inconclusive {
		t.Fatalf("status %q, want %q when later evidence does not favour the candidate", f.Status, Inconclusive)
	}
}

// An unobserved burden must stay unknown, never read as zero burden.
func TestUnobservedBurdenIsNotZeroBurden(t *testing.T) {
	o := outcome(0, true)
	if o.Burden.Status == core.Observed {
		t.Fatal("fixture burden should be unobserved")
	}
	if o.Burden.Value != nil {
		t.Fatal("an unknown burden must carry no value")
	}
	if e := o.Validate(); e != nil {
		t.Fatal("an unknown burden is a valid, honest outcome:", e)
	}
}

// Discordant per-person results are retained, not collapsed into consensus.
func TestDiscordantParticipantResultsAreRetained(t *testing.T) {
	c := comparison()
	c.Runs[3].Outcomes[0].Benefit = core.ObservedGroupQuantity(.8)
	c.Runs[3].Outcomes[1].Benefit = core.ObservedGroupQuantity(-.6)
	c.Runs[3].Outcomes[1].Unwanted = 3
	c.Runs[3].Outcomes[1].BoundaryViolations = 1
	if e := c.Validate(); e != nil {
		t.Fatal("a comparison recording harm to one participant must remain valid:", e)
	}
	if c.Runs[3].Outcomes[1].Unwanted != 3 || c.Runs[3].Outcomes[1].BoundaryViolations != 1 {
		t.Fatal("adverse per-person counts must be retained verbatim")
	}
}

// Record-level and comparison-level guards that the table above cannot reach.
func TestUpliftRemainingGuardsAreAttributable(t *testing.T) {
	// POSITIVE CONTROLS first, so each negative is attributable.
	if e := outcome(0, true).Validate(); e != nil {
		t.Fatal("positive control: clean outcome rejected:", e)
	}
	if e := armRun(NoAssistant, "a-policy").Validate(); e != nil {
		t.Fatal("positive control: clean arm run rejected:", e)
	}

	t.Run("evidence subject must be identified", func(t *testing.T) {
		o := evidence(person(0), BehaviouralObservation, .1)
		o.Metric = ""
		assertErr(t, o.Validate(), "invalid observation subject")
	})
	t.Run("unknown evidence tier", func(t *testing.T) {
		o := evidence(person(0), EvidenceTier("rumour"), .1)
		assertErr(t, o.Validate(), "invalid evidence tier")
	})
	t.Run("unknown source tier", func(t *testing.T) {
		o := evidence(person(0), BehaviouralObservation, .1)
		o.DerivedFrom = EvidenceTier("rumour")
		assertErr(t, o.Validate(), "invalid evidence tier")
	})
	t.Run("negative unwanted count", func(t *testing.T) {
		o := outcome(0, true)
		o.Unwanted = -1
		assertErr(t, o.Validate(), "invalid person outcome")
	})
	t.Run("out-of-range outcome quantity", func(t *testing.T) {
		o := outcome(0, true)
		o.Benefit = core.ObservedGroupQuantity(9)
		assertErr(t, o.Validate(), "invalid outcome quantity")
	})
	t.Run("unknown arm", func(t *testing.T) {
		a := armRun(NoAssistant, "a-policy")
		a.Arm = Arm("marketing")
		assertErr(t, a.Validate(), "unknown comparison arm")
	})
	t.Run("arm run identity", func(t *testing.T) {
		a := armRun(SimpleAssistance, "a-policy")
		a.WorldHash = ""
		assertErr(t, a.Validate(), "invalid arm run identity")
	})
	t.Run("arm with no affected people", func(t *testing.T) {
		a := armRun(SimpleAssistance, "a-policy")
		a.Outcomes = nil
		assertErr(t, a.Validate(), "arm reports no affected people")
	})
	t.Run("wrong envelope version", func(t *testing.T) {
		c := comparison()
		c.Version = "uplift-evaluation.v2"
		assertErr(t, c.Validate(), "invalid comparison envelope")
	})
	t.Run("duplicate arm", func(t *testing.T) {
		c := comparison()
		c.Runs[3].Arm = c.Runs[0].Arm
		c.Runs[3].HelperActs = 0
		assertErr(t, c.Validate(), "duplicate comparison arm")
	})
	t.Run("arm run from another comparison", func(t *testing.T) {
		c := comparison()
		c.Runs[2].Seed = 99
		assertErr(t, c.Validate(), "arm run does not belong to this comparison")
	})
}

func TestCompareArmsRejectsInvalidRequests(t *testing.T) {
	if _, e := CompareArms(sealed(ptr(comparison())), MultiPerspective, MultiPerspective); e == nil || !strings.Contains(e.Error(), "invalid arm comparison") {
		t.Fatalf("comparing an arm with itself: got %v", e)
	}
	if _, e := CompareArms(nil, NoAssistant, MultiPerspective); e == nil || !strings.Contains(e.Error(), "no comparisons supplied") {
		t.Fatalf("empty comparison set: got %v", e)
	}
}

func assertErr(t *testing.T, e error, want string) {
	t.Helper()
	if e == nil {
		t.Fatalf("accepted invalid record, want %q", want)
	}
	if !strings.Contains(e.Error(), want) {
		t.Fatalf("rejected for the wrong reason: want %q, got %q", want, e.Error())
	}
}

// A single favourable observation must never be able to claim uplift.
func TestSingleFavourableObservationCannotClaimUplift(t *testing.T) {
	c := comparison()
	c.Runs[0].Outcomes[0].Observations = append(c.Runs[0].Outcomes[0].Observations, evidence(person(0), AttributedLater, .1))
	c.Runs[3].Outcomes[0].Observations = append(c.Runs[3].Outcomes[0].Observations, evidence(person(0), AttributedLater, .9))
	f, e := CompareArms(sealed(&c), NoAssistant, MultiPerspective)
	if e != nil {
		t.Fatal(e)
	}
	if f.Status == Pass {
		t.Fatalf("one independent unit claimed uplift: %+v", f)
	}
	if f.Status != Inconclusive || !strings.Contains(f.Evidence, "independent world unit") {
		t.Fatalf("want an independent-unit refusal, got %q / %q", f.Status, f.Evidence)
	}
}

// Overlapping evidence across enough units is still not uplift.
func TestOverlappingEvidenceIsNotUplift(t *testing.T) {
	a, b := comparison(), comparison()
	b.Scenario, b.Seed = "scenario:repair", 9
	for i := range b.Runs {
		b.Runs[i].Scenario, b.Runs[i].Seed = b.Scenario, b.Seed
		b.Runs[i].WorldHash, b.Runs[i].ExogenousHash = "world-2", "exo-2"
	}
	for _, c := range []*Comparison{&a, &b} {
		c.Runs[0].Outcomes[0].Observations = append(c.Runs[0].Outcomes[0].Observations, evidence(person(0), AttributedLater, .5))
		c.Runs[3].Outcomes[0].Observations = append(c.Runs[3].Outcomes[0].Observations, evidence(person(0), AttributedLater, .4))
	}
	f, e := CompareArms(sealed(&a, &b), NoAssistant, MultiPerspective)
	if e != nil {
		t.Fatal(e)
	}
	if f.Status != Inconclusive || !(strings.Contains(f.Evidence, "overlap") || strings.Contains(f.Evidence, "disagree")) {
		t.Fatalf("overlapping evidence must not be uplift: %q / %q", f.Status, f.Evidence)
	}
}

// The strict criterion can still report a separation when one genuinely exists.
func TestNonOverlappingSeparationAcrossUnitsIsReported(t *testing.T) {
	a, b := comparison(), comparison()
	b.Scenario, b.Seed = "scenario:repair", 9
	for i := range b.Runs {
		b.Runs[i].Scenario, b.Runs[i].Seed = b.Scenario, b.Seed
		b.Runs[i].WorldHash, b.Runs[i].ExogenousHash = "world-2", "exo-2"
	}
	for _, c := range []*Comparison{&a, &b} {
		c.Runs[0].Outcomes[0].Observations = append(c.Runs[0].Outcomes[0].Observations, evidence(person(0), AttributedLater, .1))
		c.Runs[3].Outcomes[0].Observations = append(c.Runs[3].Outcomes[0].Observations, evidence(person(0), AttributedLater, .8))
	}
	f, e := CompareArms(sealed(&a, &b), NoAssistant, MultiPerspective)
	if e != nil {
		t.Fatal(e)
	}
	if f.Status != Pass || !strings.Contains(f.Evidence, "without overlap") {
		t.Fatalf("a genuine non-overlapping separation should be reported: %q / %q", f.Status, f.Evidence)
	}
	// Even then it stays synthetic and claims nothing about real people.
	if f.HumanValidity != NotTested || !f.SyntheticOnly {
		t.Fatal("a reported separation must remain synthetic with human validity NOT_TESTED")
	}
}

// --- Regressions reproducing Codex review 5682... on PR #69 ---

// R3: relabelling a system assertion as later evidence must fail even when
// DerivedFrom is left empty. The old guard only caught the honest case.
func TestR3RelabelledAssertionCannotBecomeAttributedLater(t *testing.T) {
	o := TierEvidence{Person: person(0), Tier: SystemAssertion, Provenance: SyntheticProvenance, Metric: "reported_benefit", Value: core.ObservedGroupQuantity(.9)}
	if e := o.Validate(); e != nil {
		t.Fatal("positive control: a plain system assertion is valid:", e)
	}
	o.Tier = AttributedLater // relabel only; DerivedFrom deliberately empty
	assertErr(t, o.Validate(), "attributed later evidence needs a source record")

	o.Source = "experience:1"
	o.Observer = person(1) // not the subject
	assertErr(t, o.Validate(), "attributed later evidence must be authored by its subject")
}

// R2: positive-looking harmful help cannot be reported as uplift.
func TestR2HarmfulHelpCannotBeUplift(t *testing.T) {
	a, b := twoUnits()
	for _, c := range []*Comparison{a, b} {
		c.Runs[0].Outcomes[0].Observations = append(c.Runs[0].Outcomes[0].Observations, evidence(person(0), AttributedLater, .5))
		c.Runs[3].Outcomes[0].Observations = append(c.Runs[3].Outcomes[0].Observations, evidence(person(0), AttributedLater, .9))
		// the other participant is harmed, exactly as the review reproduced
		h := &c.Runs[3].Outcomes[1]
		h.Benefit, h.Burden = core.ObservedGroupQuantity(-1), core.ObservedGroupQuantity(1)
		h.Unwanted, h.BoundaryViolations, h.DelayedOutcome = 20, 10, "censored"
	}
	f, e := CompareArms(sealed(a, b), NoAssistant, MultiPerspective)
	if e != nil {
		t.Fatal(e)
	}
	if f.Status == Pass {
		t.Fatalf("harmful help reported as uplift: %+v", f)
	}
	if len(f.Harms) == 0 || !strings.Contains(f.Evidence, "harm") {
		t.Fatalf("harm must be named in the result, got %+v", f)
	}
}

// R2: duplicating one record must not manufacture independent support.
func TestR2DuplicateRecordsDoNotCreateIndependentSupport(t *testing.T) {
	c := comparison()
	c.Runs[0].Outcomes[0].Observations = append(c.Runs[0].Outcomes[0].Observations, evidence(person(0), AttributedLater, .5))
	dup := evidence(person(0), AttributedLater, 1)
	c.Runs[3].Outcomes[0].Observations = append(c.Runs[3].Outcomes[0].Observations, dup, dup)
	f, e := CompareArms(sealed(&c), NoAssistant, MultiPerspective)
	if e != nil {
		t.Fatal(e)
	}
	if f.Status == Pass {
		t.Fatalf("duplicated records within one world claimed uplift: %+v", f)
	}
}

// R4: an arm may not drop an affected person.
func TestR4ArmCannotOmitAnAffectedPerson(t *testing.T) {
	c := comparison()
	c.Runs[3].Outcomes = c.Runs[3].Outcomes[:1]
	assertErr(t, c.Validate(), "arm omits an affected person")
}

// R4: the emitted summary keeps per-person evidence and compares against simple.
func TestR4SummaryKeepsPerPersonEvidenceAndComparesSimple(t *testing.T) {
	s, e := SummariseUplift(UpliftFixture())
	if e != nil {
		t.Fatal(e)
	}
	if len(s.People) == 0 {
		t.Fatal("per-person evidence was summarised away")
	}
	simple := false
	for _, f := range s.Findings {
		if f.Baseline == SimpleAssistance {
			simple = true
		}
	}
	if !simple {
		t.Fatal("no comparison against simple assistance")
	}
	for _, c := range s.ScenarioCoverage {
		if !c.Covered && !strings.Contains(c.Note, "NOT COVERED") {
			t.Fatalf("an uncovered family must say so: %+v", c)
		}
	}
}

func twoUnits() (*Comparison, *Comparison) {
	a, b := comparison(), comparison()
	b.Scenario, b.Seed = "scenario:repair", 9
	for i := range b.Runs {
		b.Runs[i].Scenario, b.Runs[i].Seed = b.Scenario, b.Seed
		b.Runs[i].WorldHash, b.Runs[i].ExogenousHash = "world-2", "exo-2"
	}
	return &a, &b
}

func TestNewContractGuardsAreAttributable(t *testing.T) {
	t.Run("attributed later evidence needs a time", func(t *testing.T) {
		o := evidence(person(0), AttributedLater, .5)
		if e := o.Validate(); e != nil {
			t.Fatal("positive control:", e)
		}
		o.At = -1
		assertErr(t, o.Validate(), "attributed later evidence needs an observation time")
	})
	t.Run("affected roster rejects duplicates", func(t *testing.T) {
		c := comparison()
		c.Affected = []core.ID{person(0), person(0)}
		assertErr(t, c.Validate(), "invalid affected roster")
	})
	t.Run("affected roster rejects an invalid id", func(t *testing.T) {
		c := comparison()
		c.Affected = []core.ID{person(0), ""}
		assertErr(t, c.Validate(), "invalid affected roster")
	})
	t.Run("an arm cannot report someone off the roster", func(t *testing.T) {
		c := comparison()
		c.Runs[2].Outcomes[1].Person = "person:99"
		c.Runs[2].Outcomes[1].Observations[0].Person = "person:99"
		assertErr(t, c.Validate(), "arm reports a person outside the affected roster")
	})
}

// --- Regressions for the Codex refresh review (probes 1 and 2) ---

// Probe 1: a difference between worlds each observed in only ONE arm is not
// matched-arm uplift. Missingness must be preserved, not absorbed.
func TestUnpairedWorldsAreNotUplift(t *testing.T) {
	a, b := twoUnits()
	// world A: only the baseline arm has later evidence
	a.Runs[0].Outcomes[0].Observations = append(a.Runs[0].Outcomes[0].Observations, evidence(person(0), AttributedLater, .5))
	// world B: only the candidate arm has later evidence
	b.Runs[3].Outcomes[0].Observations = append(b.Runs[3].Outcomes[0].Observations, evidence(person(0), AttributedLater, .9))
	f, e := CompareArms(sealed(a, b), NoAssistant, MultiPerspective)
	if e != nil {
		t.Fatal(e)
	}
	if f.Status == Pass {
		t.Fatalf("unpaired worlds reported as uplift: %+v", f)
	}
	if !strings.Contains(f.Evidence, "both arms") {
		t.Fatalf("the result must say no unit was observed in both arms, got %q", f.Evidence)
	}
}

// Probe 2: relabelling a scenario must not turn one world into two
// independent units.
func TestRenamingAScenarioDoesNotCreateIndependentUnits(t *testing.T) {
	a := comparison()
	a.Runs[0].Outcomes[0].Observations = append(a.Runs[0].Outcomes[0].Observations, evidence(person(0), AttributedLater, .5))
	a.Runs[3].Outcomes[0].Observations = append(a.Runs[3].Outcomes[0].Observations, evidence(person(0), AttributedLater, .9))
	b := a
	b.Runs = append([]ArmRun{}, a.Runs...)
	// same WorldHash, ExogenousHash, seed, people and records — only labels differ
	b.Scenario = "scenario:relabelled"
	for i := range b.Runs {
		b.Runs[i].Scenario = b.Scenario
	}
	f, e := CompareArms(sealed(&a, &b), NoAssistant, MultiPerspective)
	if e != nil {
		t.Fatal(e)
	}
	if f.Status == Pass {
		t.Fatalf("a relabelled copy of one world manufactured independent support: %+v", f)
	}
}

// A genuine paired separation across two distinct worlds is still reportable.
func TestPairedSeparationAcrossDistinctWorldsIsReported(t *testing.T) {
	a, b := twoUnits()
	for _, c := range []*Comparison{a, b} {
		c.Runs[0].Outcomes[0].Observations = append(c.Runs[0].Outcomes[0].Observations, evidence(person(0), AttributedLater, .1))
		c.Runs[3].Outcomes[0].Observations = append(c.Runs[3].Outcomes[0].Observations, evidence(person(0), AttributedLater, .8))
	}
	f, e := CompareArms(sealed(a, b), NoAssistant, MultiPerspective)
	if e != nil {
		t.Fatal(e)
	}
	if f.Status != Pass || !strings.Contains(f.Evidence, "per-person pairs") {
		t.Fatalf("a genuine paired separation should be reported: %q / %q", f.Status, f.Evidence)
	}
	if f.Margin == nil || f.Margin.Units != 2 || !strings.Contains(f.Margin.Method, "not a calibrated") {
		t.Fatalf("a reported separation must carry a clustered, honestly-labelled margin: %+v", f.Margin)
	}
}

// Probe 3 on the paired estimand: recorded harm still blocks any conclusion.
func TestHarmBlocksEvenAGenuinePairedSeparation(t *testing.T) {
	a, b := twoUnits()
	for _, c := range []*Comparison{a, b} {
		c.Runs[0].Outcomes[0].Observations = append(c.Runs[0].Outcomes[0].Observations, evidence(person(0), AttributedLater, .5))
		c.Runs[3].Outcomes[0].Observations = append(c.Runs[3].Outcomes[0].Observations, evidence(person(0), AttributedLater, .9))
		h := &c.Runs[3].Outcomes[1]
		h.Benefit, h.Burden = core.ObservedGroupQuantity(-1), core.ObservedGroupQuantity(1)
		h.Unwanted, h.BoundaryViolations, h.DelayedOutcome = 20, 10, "censored"
	}
	f, e := CompareArms(sealed(a, b), NoAssistant, MultiPerspective)
	if e != nil {
		t.Fatal(e)
	}
	if f.Status == Pass || len(f.Harms) == 0 {
		t.Fatalf("harm must block a conclusion and be named: %+v", f)
	}
}

// Qualifications must survive early returns: a not-tested or refused result
// still carries what was observed and what was not measured.
func TestQualificationsSurviveEarlyReturns(t *testing.T) {
	a, b := twoUnits()
	// no attributed later evidence at all -> not-tested, but harm is present
	h := &a.Runs[3].Outcomes[1]
	h.BoundaryViolations, h.DelayedOutcome = 4, "censored"
	f, e := CompareArms(sealed(a, b), NoAssistant, MultiPerspective)
	if e != nil {
		t.Fatal(e)
	}
	if f.Status != NotTested {
		t.Fatalf("expected not-tested without paired evidence, got %q", f.Status)
	}
	if len(f.Harms) == 0 {
		t.Fatal("a not-tested finding still discarded its harm qualifications")
	}
	if len(f.Uncertainty) == 0 {
		t.Fatal("a not-tested finding still discarded what was not measured")
	}
}

// Burden reduction is never recorded as burden.
func TestBurdenReductionIsNotBurden(t *testing.T) {
	o := outcome(0, true)
	o.BurdenReduction = core.ObservedGroupQuantity(.7)
	if e := o.Validate(); e != nil {
		t.Fatal(e)
	}
	if o.Burden.Status == core.Observed {
		t.Fatal("an observed burden reduction must not make burden observed")
	}
}

// Different people observed in different arms are not paired evidence.
func TestDifferentObservedPeopleAreNotPaired(t *testing.T) {
	a, b := twoUnits()
	for _, c := range []*Comparison{a, b} {
		// only person 0 observed in the baseline arm
		c.Runs[0].Outcomes[0].Observations = append(c.Runs[0].Outcomes[0].Observations, evidence(person(0), AttributedLater, .5))
		// only person 1 observed in the candidate arm
		c.Runs[3].Outcomes[1].Observations = append(c.Runs[3].Outcomes[1].Observations, evidence(person(1), AttributedLater, .9))
	}
	f, e := CompareArms(sealed(a, b), NoAssistant, MultiPerspective)
	if e != nil {
		t.Fatal(e)
	}
	if f.Status == Pass {
		t.Fatalf("different people's levels were treated as a per-person improvement: %+v", f)
	}
}

// Known harm is retained on a not-tested result with no paired observation.
func TestHarmRetainedWhenNothingIsPaired(t *testing.T) {
	a, b := twoUnits()
	for _, c := range []*Comparison{a, b} {
		c.Runs[3].Outcomes[1].BoundaryViolations = 10
	}
	f, e := CompareArms(sealed(a, b), NoAssistant, MultiPerspective)
	if e != nil {
		t.Fatal(e)
	}
	if f.Status != NotTested {
		t.Fatalf("expected not-tested, got %q", f.Status)
	}
	if len(f.Harms) == 0 {
		t.Fatal("known adverse evidence was discarded by the not-tested return")
	}
}

// R3: relabelling a system assertion fails because the RESOLVED record's kind
// does not justify the tier. A populated Source field is not attribution.
func TestR3TierMustBeJustifiedByTheResolvedRecord(t *testing.T) {
	c := comparison()
	c.Sources = append(c.Sources, SourceRecord{ID: "assertion:invented", Kind: "system_assertion",
		Subject: person(0), Observer: person(0), At: 3, Arm: MultiPerspective,
		Metric: "reported_benefit", Value: core.ObservedGroupQuantity(.9), Content: "a system assertion"})
	o := TierEvidence{Person: person(0), Tier: SystemAssertion, Provenance: SyntheticProvenance,
		Source: "assertion:invented", Observer: person(0), At: 3, Metric: "reported_benefit",
		Value: core.ObservedGroupQuantity(.9)}
	c.Runs[3].Outcomes[0].Observations = append(c.Runs[3].Outcomes[0].Observations, o)
	if e := c.Validate(); e != nil {
		t.Fatal("positive control: a properly cited system assertion is valid:", e)
	}
	// relabel only
	c.Runs[3].Outcomes[0].Observations[len(c.Runs[3].Outcomes[0].Observations)-1].Tier = AttributedLater
	assertErr(t, c.Validate(), "does not justify tier")
}

// Evidence that cites nothing real is not evidence.
func TestEvidenceMustCiteAResolvedRecord(t *testing.T) {
	c := comparison()
	c.Runs[1].Outcomes[0].Observations[0].Source = "record:does-not-exist"
	assertErr(t, c.Validate(), "unresolved source record")
}

// A later self-report must actually be later than the event it reports.
func TestLaterSelfReportMustBeLaterThanItsEvent(t *testing.T) {
	c := comparison()
	e := evidence(person(0), AttributedLater, .4)
	c.Runs[3].Outcomes[0].Observations = append(c.Runs[3].Outcomes[0].Observations, e)
	ledger(&c)
	if err := c.Validate(); err != nil {
		t.Fatal("positive control:", err)
	}
	for i := range c.Sources {
		if c.Sources[i].ID == e.Source {
			c.Sources[i].At = 0 // same time as the event it is about
		}
	}
	c.Runs[3].Outcomes[0].Observations[len(c.Runs[3].Outcomes[0].Observations)-1].At = 0
	assertErr(t, c.Validate(), "not later than the event")
}

// Every source-ledger guard, each asserting its exact root cause.
func TestSourceLedgerGuardsAreAttributable(t *testing.T) {
	good := SourceRecord{ID: "rec:1", Kind: "later_self_report", Subject: person(0), Observer: person(0),
		At: 5, About: "event:opportunity", Arm: NoAssistant, Metric: "reported_benefit",
		Value: core.ObservedGroupQuantity(.5), Content: "a later self-report"}
	if e := good.Validate(); e != nil {
		t.Fatal("positive control: clean source record rejected:", e)
	}
	t.Run("empty content", func(t *testing.T) {
		r := good
		r.Content = ""
		assertErr(t, r.Validate(), "invalid source record")
	})
	t.Run("negative time", func(t *testing.T) {
		r := good
		r.At = -1
		assertErr(t, r.Validate(), "invalid source record")
	})
	t.Run("unknown kind", func(t *testing.T) {
		r := good
		r.Kind = "hearsay"
		assertErr(t, r.Validate(), "unknown source record kind")
	})
	t.Run("self-report by someone else", func(t *testing.T) {
		r := good
		r.Observer = person(1)
		assertErr(t, r.Validate(), "must be authored by its subject")
	})
	t.Run("self-report about nothing", func(t *testing.T) {
		r := good
		r.About = ""
		assertErr(t, r.Validate(), "must name the event it is about")
	})
	t.Run("ledger wraps a bad record", func(t *testing.T) {
		c := comparison()
		c.Sources = append(c.Sources, SourceRecord{ID: "rec:bad", Kind: "system_assertion",
			Subject: person(0), Observer: person(0), At: 1, Arm: NoAssistant,
			Metric: "reported_benefit", Value: core.ObservedGroupQuantity(.1), Content: ""})
		assertErr(t, c.Validate(), "source ledger")
	})
	t.Run("duplicate record", func(t *testing.T) {
		c := comparison()
		c.Sources = append(c.Sources, c.Sources[0])
		assertErr(t, c.Validate(), "duplicate source record")
	})
	t.Run("record attributes someone else", func(t *testing.T) {
		c := comparison()
		for i := range c.Sources {
			if c.Sources[i].ID == c.Runs[1].Outcomes[0].Observations[0].Source {
				c.Sources[i].Subject = person(1)
				c.Sources[i].Observer = person(1)
			}
		}
		assertErr(t, c.Validate(), "does not attribute this evidence")
	})
	t.Run("evidence time disagrees with its record", func(t *testing.T) {
		c := comparison()
		c.Runs[1].Outcomes[0].Observations[0].At = 99
		assertErr(t, c.Validate(), "time disagrees with its source record")
	})
	t.Run("self-report about an unresolved event", func(t *testing.T) {
		c := comparison()
		e := evidence(person(0), AttributedLater, .33)
		c.Runs[3].Outcomes[0].Observations = append(c.Runs[3].Outcomes[0].Observations, e)
		ledger(&c)
		for i := range c.Sources {
			if c.Sources[i].ID == e.Source {
				c.Sources[i].About = "event:vanished"
			}
		}
		assertErr(t, c.Validate(), "about an unresolved event")
	})
}

// R4(b): a burden reduction valid on its source scale must not be rejected by
// the outcome validator. core.OrdinaryExperience allows -100..100.
func TestR4BurdenReductionKeepsItsSourceScale(t *testing.T) {
	o := outcome(0, true)
	o.BurdenReduction = core.ObservedGroupQuantity(5)
	if e := o.Validate(); e != nil {
		t.Fatal("a source-valid burden reduction of 5 was rejected:", e)
	}
	o.BurdenReduction = core.ObservedGroupQuantity(500)
	assertErr(t, o.Validate(), "invalid burden reduction quantity")
}

// Uncertainty is computed at the independent world unit, and one unit pointing
// the other way is disagreement between worlds rather than uplift.
func TestUnitsMustAgreeAndMarginIsClusteredAtUnits(t *testing.T) {
	a, b := twoUnits()
	// world A favours the candidate, world B does not
	a.Runs[0].Outcomes[0].Observations = append(a.Runs[0].Outcomes[0].Observations, evidence(person(0), AttributedLater, .1))
	a.Runs[3].Outcomes[0].Observations = append(a.Runs[3].Outcomes[0].Observations, evidence(person(0), AttributedLater, .8))
	b.Runs[0].Outcomes[0].Observations = append(b.Runs[0].Outcomes[0].Observations, evidence(person(0), AttributedLater, .8))
	b.Runs[3].Outcomes[0].Observations = append(b.Runs[3].Outcomes[0].Observations, evidence(person(0), AttributedLater, .1))
	f, e := CompareArms(sealed(a, b), NoAssistant, MultiPerspective)
	if e != nil {
		t.Fatal(e)
	}
	if f.Status == Pass {
		t.Fatalf("worlds disagreeing were reported as uplift: %+v", f)
	}
	if !strings.Contains(f.Evidence, "disagree") {
		t.Fatalf("disagreement between units must be named: %q", f.Evidence)
	}
	if f.Margin == nil || f.Margin.Units != 2 || !strings.Contains(f.Margin.Method, "per-unit summaries") {
		t.Fatalf("margin must be clustered at independent units: %+v", f.Margin)
	}
}

// R3(i): a claimed value cannot change while its source record stays the same.
func TestR3EvidenceValueMustMatchItsSourceRecord(t *testing.T) {
	c := comparison()
	if e := c.Validate(); e != nil {
		t.Fatal("positive control:", e)
	}
	// tamper with ONLY the reported quantity; leave the ledger untouched
	c.Runs[1].Outcomes[0].Observations[0].Value = core.ObservedGroupQuantity(.99)
	assertErr(t, c.Validate(), "measurement disagrees with its source record")
}

// R3(ii): one arm may not borrow another arm's collected record.
func TestR3EvidenceCannotBorrowAnotherArmsRecord(t *testing.T) {
	c := comparison()
	if e := c.Validate(); e != nil {
		t.Fatal("positive control:", e)
	}
	// hand the candidate arm the baseline arm's report verbatim
	c.Runs[3].Outcomes[0].Observations[0] = c.Runs[0].Outcomes[0].Observations[0]
	assertErr(t, c.Validate(), "borrows a source record collected in arm")
}

// A source record must name the arm it was collected in and what it measured.
func TestSourceRecordNeedsArmAndMetric(t *testing.T) {
	good := SourceRecord{ID: "rec:9", Kind: "system_assertion", Subject: person(0), Observer: person(0),
		At: 1, Arm: NoAssistant, Metric: "reported_benefit", Value: core.ObservedGroupQuantity(.2),
		Content: "an assertion"}
	if e := good.Validate(); e != nil {
		t.Fatal("positive control:", e)
	}
	noArm := good
	noArm.Arm = ""
	assertErr(t, noArm.Validate(), "invalid source record")
	noMetric := good
	noMetric.Metric = ""
	assertErr(t, noMetric.Validate(), "invalid source record")
}

// R3: an event link may not cross arms either. Checking only the citing
// record's arm left borrowing possible through About.
func TestR3LaterReportCannotBeAboutAnotherArmsEvent(t *testing.T) {
	c := UpliftFixture()[1]
	if e := c.Validate(); e != nil {
		t.Fatal("positive control:", e)
	}
	target := c.Runs[3].Outcomes[0].Observations[1].Source
	other := c.Runs[0].Outcomes[0].Observations[0].Source
	for i := range c.Sources {
		if c.Sources[i].ID == target {
			c.Sources[i].About = other
		}
	}
	assertErr(t, c.Validate(), "is about an event from arm")
}

// R1: coverage must be a receipt of execution, not a name match. These probes
// establish each half separately: that a family cannot be credited by naming,
// and that a genuinely executed family still is.
func TestR1FamilyMustBeDeclaredAndRequired(t *testing.T) {
	c := comparison()
	if e := c.Validate(); e != nil {
		t.Fatal("positive control:", e)
	}
	undeclared := c
	undeclared.Family = ""
	assertErr(t, undeclared.Validate(), "required scenario family")
	// A family name that merely contains a required one is not that family.
	nearMiss := c
	nearMiss.Family = "ordinary_joyful"
	assertErr(t, nearMiss.Validate(), "required scenario family")
}

func TestR1CoverageIgnoresScenarioNamesAndReadsTheExecutedManifest(t *testing.T) {
	covered := func(s UpliftSummary, family string) bool {
		for _, sc := range s.ScenarioCoverage {
			if sc.Family == family {
				return sc.Covered
			}
		}
		t.Fatalf("family %q absent from coverage", family)
		return false
	}
	// A scenario whose name contains "repair" but which executes the
	// ordinary_joy family must not credit repair. The old substring rule did.
	c := comparison()
	c.Scenario, c.Family = "scenario:repair_of_ordinary_joy", "ordinary_joy"
	for i := range c.Runs {
		c.Runs[i].Scenario = c.Scenario
	}
	ledger(&c)
	if e := c.Validate(); e != nil {
		t.Fatal("positive control:", e)
	}
	s, e := SummariseUplift([]Comparison{c})
	if e != nil {
		t.Fatal(e)
	}
	if covered(s, "repair") {
		t.Fatal("a family was credited by scenario name alone")
	}
	if !covered(s, "ordinary_joy") {
		t.Fatal("the executed family was not credited")
	}
	// The manifest backing that claim must name the arms that actually ran.
	if len(s.Executed) != len(c.Runs) {
		t.Fatalf("manifest records %d executed arms, comparison ran %d", len(s.Executed), len(c.Runs))
	}
	policy := map[Arm]string{}
	streams := map[Arm][]Stream{}
	for _, r := range c.Runs {
		policy[r.Arm], streams[r.Arm] = r.PolicyHash, r.Streams
	}
	for _, u := range s.Executed {
		if u.Family != "ordinary_joy" || u.People != len(c.Affected) {
			t.Fatalf("manifest unit misreports its execution: %+v", u)
		}
		// The manifest must carry the arm's own policy receipt: it is what lets
		// a reader recompute which arms actually did the same thing.
		if u.PolicyHash != policy[u.Arm] {
			t.Fatalf("manifest reports policy %q for arm %s, which ran %q", u.PolicyHash, u.Arm, policy[u.Arm])
		}
		// and its streams, so matching and domain independence are recheckable.
		if streamKey(u.Streams) != streamKey(streams[u.Arm]) {
			t.Fatalf("manifest reports streams %v for arm %s, which drew %v", u.Streams, u.Arm, streams[u.Arm])
		}
	}
}

func TestR1CandidateArmsWithoutAMatchedControlAreNotCoverage(t *testing.T) {
	c := comparison()
	// Keep one candidate arm and drop the control. Validation would reject this
	// comparison, but SummariseUplift does not re-validate its input, so this
	// is the reachable path on which coverage must not credit the family: no
	// baseline/candidate pair is both-executed, so CompareArms never runs and
	// coverage is what decides.
	c.Runs = c.Runs[1:2]
	s, e := SummariseUplift([]Comparison{c})
	if e != nil {
		t.Fatal(e)
	}
	for _, sc := range s.ScenarioCoverage {
		if sc.Family != "ordinary_joy" {
			continue
		}
		if sc.Covered {
			t.Fatal("candidate arms alone were counted as coverage")
		}
		if !strings.Contains(sc.Note, "no matched no-assistant control") {
			t.Fatalf("note does not say why it is uncovered: %q", sc.Note)
		}
	}
}

// An arm that produced no outcomes ran nobody. Counting it as execution would
// let an empty arm supply the control half of a "covered" comparison.
func TestR1AnArmWithNoOutcomesIsNotExecution(t *testing.T) {
	// Positive control: the same single arm, with its outcomes intact, is
	// recorded as one executed unit. Only the emptying below may change that.
	c := comparison()
	c.Runs = c.Runs[:1]
	s, e := SummariseUplift([]Comparison{c})
	if e != nil {
		t.Fatal("positive control:", e)
	}
	if len(s.Executed) != 1 || s.Executed[0].People != len(c.Affected) {
		t.Fatalf("positive control manifest is wrong: %+v", s.Executed)
	}
	// A single unpaired arm keeps CompareArms out of the path, so the manifest
	// is what reports execution here.
	empty := c
	empty.Runs = []ArmRun{c.Runs[0]}
	empty.Runs[0].Outcomes = nil // the no-assistant control ran nobody
	s, e = SummariseUplift([]Comparison{empty})
	if e != nil {
		t.Fatal(e)
	}
	if len(s.Executed) != 0 {
		t.Fatalf("an arm that produced no outcomes was recorded as executed: %+v", s.Executed)
	}
}

// Two arms that produced the same policy output are the same intervention
// under two labels. Whatever the levels say, no uplift is attributable.
func TestIdenticalPolicyOutputCannotBeUplift(t *testing.T) {
	// Positive control: with distinct policy receipts the pair is evaluated on
	// its evidence, and the identical-arm note is absent.
	c := UpliftFixture()[1]
	f, e := CompareArms([]Comparison{c}, NoAssistant, MultiPerspective)
	if e != nil {
		t.Fatal("positive control:", e)
	}
	for _, u := range f.Uncertainty {
		if strings.Contains(u, "identical policy output") {
			t.Fatalf("positive control already reports identical arms: %q", u)
		}
	}
	// Now make the two arms report the same policy receipt, changing nothing
	// else but the observed burden that would otherwise stop the comparison
	// earlier on harm. Harm is reported before identical arms by design, so the
	// burden is cleared here to put the identical-arm rule on the decisive path.
	same := c
	same.Runs = append([]ArmRun{}, c.Runs...)
	for i := range same.Runs {
		same.Runs[i].PolicyHash = "one-policy"
		same.Runs[i].Outcomes = append([]PersonOutcome{}, c.Runs[i].Outcomes...)
		for j := range same.Runs[i].Outcomes {
			same.Runs[i].Outcomes[j].Burden = core.UnknownGroupQuantity()
		}
	}
	f, e = CompareArms([]Comparison{same}, NoAssistant, MultiPerspective)
	if e != nil {
		t.Fatal(e)
	}
	if f.Status == Pass {
		t.Fatal("uplift was credited between two arms that ran identically")
	}
	if !strings.Contains(f.Evidence, "identical policy output") {
		t.Fatalf("the identical arms are not named in the evidence: %q", f.Evidence)
	}
	found := false
	for _, u := range f.Uncertainty {
		found = found || strings.Contains(u, "identical policy output")
	}
	if !found {
		t.Fatalf("identical arms absent from uncertainty: %+v", f.Uncertainty)
	}
}

// A consumer that never measured a delayed outcome is ignorant, not harmful.
// Collapsing not_instrumented into "missing" would manufacture harm findings
// out of a consumer that simply carries no instrument.
func TestNotInstrumentedIsUncertaintyNotHarm(t *testing.T) {
	c := UpliftFixture()[1]
	set := func(d string) UpliftFinding {
		v := c
		v.Runs = append([]ArmRun{}, c.Runs...)
		for i := range v.Runs {
			v.Runs[i].Outcomes = append([]PersonOutcome{}, c.Runs[i].Outcomes...)
			for j := range v.Runs[i].Outcomes {
				v.Runs[i].Outcomes[j].DelayedOutcome = d
				v.Runs[i].Outcomes[j].Burden = core.UnknownGroupQuantity()
			}
		}
		f, e := CompareArms([]Comparison{v}, NoAssistant, MultiPerspective)
		if e != nil {
			t.Fatal(d, e)
		}
		return f
	}
	// Positive control: "missing" is adverse and must be named as harm.
	if h := set("missing").Harms; len(h) == 0 {
		t.Fatal("a missing outcome was not reported as harm")
	}
	f := set("not_instrumented")
	for _, h := range f.Harms {
		if strings.Contains(h, "not_instrumented") || strings.Contains(h, "instrument") {
			t.Fatalf("an unmeasured outcome was reported as harm: %q", h)
		}
	}
	found := false
	for _, u := range f.Uncertainty {
		found = found || strings.Contains(u, "no delayed-outcome instrument")
	}
	if !found {
		t.Fatalf("an unmeasured outcome was not reported as uncertainty: %+v", f.Uncertainty)
	}
}

// Arms that drew the same streams in a different order are still matched: the
// order a run happens to record its receipts in is not a property of the design.
func TestMatchedStreamsAreOrderIndependent(t *testing.T) {
	c := comparison()
	if e := c.Validate(); e != nil {
		t.Fatal("positive control:", e)
	}
	st := c.Runs[3].Streams
	c.Runs[3].Streams = []Stream{st[2], st[0], st[1]}
	if e := c.Validate(); e != nil {
		t.Fatalf("reordering one arm's stream receipts broke the match: %v", e)
	}
	// Reordering must not make a genuinely different stream set look matched.
	c.Runs[3].Streams = []Stream{st[2], st[0], {Domain: "exogenous", Seed: "different"}}
	assertErr(t, c.Validate(), "do not share the matched rng streams")
}

// An uncovered family must say why. A note for a family that IS covered is
// ignored, so a stale blocker cannot mask real coverage.
func TestUncoveredFamiliesCarryTheirReason(t *testing.T) {
	c := comparison()
	s, e := SummariseUplift([]Comparison{c},
		FamilyNote{Family: "repair", Reason: "no consumer executes this across arms"},
		FamilyNote{Family: "ordinary_joy", Reason: "STALE: this family is actually executed"})
	if e != nil {
		t.Fatal(e)
	}
	for _, sc := range s.ScenarioCoverage {
		switch sc.Family {
		case "repair":
			if !strings.Contains(sc.Note, "no consumer executes this across arms") {
				t.Fatalf("uncovered family lost its reason: %q", sc.Note)
			}
		case "ordinary_joy":
			if !sc.Covered {
				t.Fatal("positive control: the executed family is not covered")
			}
			if strings.Contains(sc.Note, "STALE") {
				t.Fatalf("a stale blocker was applied to a covered family: %q", sc.Note)
			}
		case "group_burden":
			// No note supplied: the generic note must still stand alone.
			if !strings.Contains(sc.Note, "NOT COVERED") {
				t.Fatalf("a family with no reason lost its note: %q", sc.Note)
			}
		}
	}
}

// A policy that correctly stays silent produces nothing to measure. Scoring
// that as a missing outcome would make restraint look like harm.
func TestNoInterventionIsUncertaintyNotHarm(t *testing.T) {
	c := UpliftFixture()[1]
	set := func(d string) UpliftFinding {
		v := c
		v.Runs = append([]ArmRun{}, c.Runs...)
		for i := range v.Runs {
			v.Runs[i].Outcomes = append([]PersonOutcome{}, c.Runs[i].Outcomes...)
			for j := range v.Runs[i].Outcomes {
				v.Runs[i].Outcomes[j].DelayedOutcome = d
				v.Runs[i].Outcomes[j].Burden = core.UnknownGroupQuantity()
			}
		}
		f, e := CompareArms([]Comparison{v}, NoAssistant, MultiPerspective)
		if e != nil {
			t.Fatal(d, e)
		}
		return f
	}
	if h := set("missing").Harms; len(h) == 0 {
		t.Fatal("positive control: a missing outcome was not reported as harm")
	}
	f := set("no_intervention")
	if len(f.Harms) != 0 {
		t.Fatalf("correct restraint was reported as harm: %+v", f.Harms)
	}
	found := false
	for _, u := range f.Uncertainty {
		found = found || strings.Contains(u, "no intervention occurred")
	}
	if !found {
		t.Fatalf("restraint was not reported as uncertainty: %+v", f.Uncertainty)
	}
}

// #58 names appropriateness alongside benefit and burden. An unknown quantity
// describes itself, but a reader of a finding would never learn the measure
// was absent everywhere unless the finding says so.
func TestUnmeasuredAppropriatenessAndBurdenReductionAreReported(t *testing.T) {
	c := UpliftFixture()[1]
	report := func(v Comparison) []string {
		f, e := CompareArms([]Comparison{v}, NoAssistant, MultiPerspective)
		if e != nil {
			t.Fatal(e)
		}
		return f.Uncertainty
	}
	has := func(in []string, want string) bool {
		for _, u := range in {
			if strings.Contains(u, want) {
				return true
			}
		}
		return false
	}
	// Positive control: the fixture observes appropriateness, so it must NOT be
	// reported absent. Burden reduction it does not observe, so it must be.
	base := report(c)
	if has(base, "appropriateness not observed") {
		t.Fatalf("an observed appropriateness was reported as absent: %+v", base)
	}
	if !has(base, "burden reduction not observed") {
		t.Fatalf("an unobserved burden reduction was not reported: %+v", base)
	}
	// Now withhold appropriateness, changing nothing else.
	unknown := c
	unknown.Runs = append([]ArmRun{}, c.Runs...)
	for i := range unknown.Runs {
		unknown.Runs[i].Outcomes = append([]PersonOutcome{}, c.Runs[i].Outcomes...)
		for j := range unknown.Runs[i].Outcomes {
			unknown.Runs[i].Outcomes[j].Appropriateness = core.UnknownGroupQuantity()
		}
	}
	if u := report(unknown); !has(u, "appropriateness not observed") {
		t.Fatalf("an unmeasured appropriateness was never reported: %+v", u)
	}
}

// pairedUnit builds one harm-free comparison in its own independent world unit,
// carrying controlled independently-attributed later evidence: later[arm][i] is
// person i's reported benefit in that arm. Everything else is held constant so
// only the property under test varies.
func pairedUnit(unit int, later map[Arm][]float64) *Comparison {
	id := fmt.Sprintf("unit-%d", unit)
	c := Comparison{Version: UpliftVersion, Scenario: core.ID("scenario:" + id), Family: "ordinary_joy",
		Seed: uint64(unit), Affected: []core.ID{person(0), person(1)}}
	for _, a := range Arms {
		r := ArmRun{Arm: a, Seed: uint64(unit), WorldHash: "world-" + id, ExogenousHash: "exo-" + id,
			Streams: []Stream{{Domain: "human", Seed: "h-" + id}, {Domain: "exogenous", Seed: "x-" + id},
				{Domain: "helper", Seed: "p-" + id}},
			PolicyHash: "policy-" + string(a), HumanHash: "human-" + string(a), Scenario: c.Scenario}
		for i := 0; i < 2; i++ {
			o := armOutcome(a, i, true)
			// Harm-free: the outcome resolves and nothing adverse is observed.
			o.DelayedOutcome = "resolved"
			o.Benefit = core.ObservedGroupQuantity(.2)
			o.Burden = core.ObservedGroupQuantity(0)
			o.Appropriateness = core.ObservedGroupQuantity(.5)
			o.BurdenReduction = core.ObservedGroupQuantity(0)
			if v, ok := later[a]; ok && i < len(v) {
				o.Observations = append(o.Observations, TierEvidence{Person: person(i), Tier: AttributedLater,
					Provenance: SyntheticProvenance,
					Source:     core.ID(fmt.Sprintf("%s:%s:later:%s", id, a, person(i))),
					Observer:   person(i), At: 5, Metric: "reported_benefit", Value: core.ObservedGroupQuantity(v[i])})
			}
			r.Outcomes = append(r.Outcomes, o)
		}
		c.Runs = append(c.Runs, r)
	}
	return &c
}

// A candidate favoured in one independent world and not in another has not
// shown uplift: that is disagreement between worlds. Without this rule a single
// favourable world could carry the result.
func TestIndependentWorldsMustAgree(t *testing.T) {
	// Positive control: both units favour the candidate, so the comparison is
	// allowed to proceed past this rule.
	agree := sealed(
		pairedUnit(1, map[Arm][]float64{NoAssistant: {.1, .1}, MultiPerspective: {.5, .5}}),
		pairedUnit(2, map[Arm][]float64{NoAssistant: {.1, .1}, MultiPerspective: {.5, .5}}))
	f, e := CompareArms(agree, NoAssistant, MultiPerspective)
	if e != nil {
		t.Fatal("positive control:", e)
	}
	if strings.Contains(f.Evidence, "independent world units disagree") {
		t.Fatalf("units that agree were reported as disagreeing: %q", f.Evidence)
	}
	// Now flip the second world only. Nothing else changes.
	disagree := sealed(
		pairedUnit(1, map[Arm][]float64{NoAssistant: {.1, .1}, MultiPerspective: {.5, .5}}),
		pairedUnit(2, map[Arm][]float64{NoAssistant: {.5, .5}, MultiPerspective: {.1, .1}}))
	f, e = CompareArms(disagree, NoAssistant, MultiPerspective)
	if e != nil {
		t.Fatal(e)
	}
	if f.Status == Pass {
		t.Fatal("uplift was credited while one independent world pointed the other way")
	}
	// The wording matters: the per-person overlap message also contains the
	// word "disagree", so matching on that alone passes whether or not the
	// world-level rule ran. The failure must be attributed to the WORLDS
	// disagreeing, with their counts, not to margins overlapping.
	if !strings.Contains(f.Evidence, "independent world units disagree") {
		t.Fatalf("world disagreement is not named in the evidence: %q", f.Evidence)
	}
	if !strings.Contains(f.Evidence, "1 favour") {
		t.Fatalf("the evidence does not report how the worlds split: %q", f.Evidence)
	}
}

// A unit is summarised by its WEAKEST paired person. Taking its strongest would
// let one person carry a world in which someone else did worse.
func TestAUnitIsCarriedByItsWeakestPersonNotItsStrongest(t *testing.T) {
	// Person 0 gains, person 1 loses, in both worlds. The unit summary must be
	// negative, so no uplift may be reported.
	mixed := sealed(
		pairedUnit(1, map[Arm][]float64{NoAssistant: {.1, .5}, MultiPerspective: {.9, .1}}),
		pairedUnit(2, map[Arm][]float64{NoAssistant: {.1, .5}, MultiPerspective: {.9, .1}}))
	f, e := CompareArms(mixed, NoAssistant, MultiPerspective)
	if e != nil {
		t.Fatal(e)
	}
	if f.Status == Pass {
		t.Fatal("a unit was carried by its most favourable person while another person did worse")
	}
	if f.Margin == nil {
		t.Fatal("no margin was reported for a paired comparison")
	}
	if f.Margin.High > 0 {
		t.Fatalf("the unit summary took a favourable person rather than the weakest: %+v", f.Margin)
	}
}

// A person observed in only one arm is missing from the other. Missingness is
// not evidence of a difference, and must not be read as one.
func TestAPersonObservedInOnlyOneArmIsNotEvidence(t *testing.T) {
	both := sealed(
		pairedUnit(1, map[Arm][]float64{NoAssistant: {.1, .1}, MultiPerspective: {.5, .5}}),
		pairedUnit(2, map[Arm][]float64{NoAssistant: {.1, .1}, MultiPerspective: {.5, .5}}))
	if _, e := CompareArms(both, NoAssistant, MultiPerspective); e != nil {
		t.Fatal("positive control:", e)
	}
	// Observe person 1 in the candidate arm only, in one world.
	oneArmed := sealed(
		pairedUnit(1, map[Arm][]float64{NoAssistant: {.1}, MultiPerspective: {.5, .5}}),
		pairedUnit(2, map[Arm][]float64{NoAssistant: {.1, .1}, MultiPerspective: {.5, .5}}))
	f, e := CompareArms(oneArmed, NoAssistant, MultiPerspective)
	if e != nil {
		t.Fatal(e)
	}
	if f.Status == Pass {
		t.Fatal("a person present in only one arm was counted as evidence of a difference")
	}
	if !strings.Contains(f.Evidence, "only one arm") {
		t.Fatalf("one-armed observation is not named in the evidence: %q", f.Evidence)
	}
}

// The most important property in this file. "No uplift was found" only means
// something if this contract is CAPABLE of finding uplift. A comparison that
// can never return pass is not a cautious evaluation, it is a constant, and
// every honesty assertion built on it — including the compiled gate's — would
// be vacuous. Given evidence that genuinely separates the arms across
// independent worlds, with no harm and nothing unobserved, it must say so.
func TestUpliftIsReachableSoRefusingItMeansSomething(t *testing.T) {
	clear := sealed(
		pairedUnit(1, map[Arm][]float64{NoAssistant: {.1, .1}, MultiPerspective: {.5, .5}}),
		pairedUnit(2, map[Arm][]float64{NoAssistant: {.1, .1}, MultiPerspective: {.5, .5}}))
	f, e := CompareArms(clear, NoAssistant, MultiPerspective)
	if e != nil {
		t.Fatal(e)
	}
	if f.Status != Pass {
		t.Fatalf("evidence that separates every pair across independent worlds was refused: %q (%s)", f.Status, f.Evidence)
	}
	if len(f.Harms) != 0 {
		t.Fatalf("a harm-free fixture reported harm: %+v", f.Harms)
	}
	if f.Margin == nil || f.Margin.Units < MinIndependentUnits {
		t.Fatalf("a passing finding reported no clustered margin: %+v", f.Margin)
	}
	// Even here the synthetic ceiling holds: this is not human validity.
	if !f.SyntheticOnly || f.HumanValidity != NotTested {
		t.Fatalf("a passing finding dropped its synthetic ceiling: synthetic=%v validity=%q", f.SyntheticOnly, f.HumanValidity)
	}
	// And narrowing the separation until the arms overlap must withdraw it.
	overlap := sealed(
		pairedUnit(1, map[Arm][]float64{NoAssistant: {.1, .1}, MultiPerspective: {.5, .05}}),
		pairedUnit(2, map[Arm][]float64{NoAssistant: {.1, .1}, MultiPerspective: {.5, .5}}))
	f, e = CompareArms(overlap, NoAssistant, MultiPerspective)
	if e != nil {
		t.Fatal(e)
	}
	if f.Status == Pass {
		t.Fatal("uplift survived a pair that does not separate")
	}
}

// In several consumers the assistant's output is never an input to the human's
// choice, so the people decide identically whatever the arm does. That does not
// invalidate a comparison — an intervention can change what someone experiences
// without changing what they do — but it must be reported, or a null result
// reads as evidence about a policy that could not have reached the decision.
func TestArmsThatNeverReachedThePeopleAreReported(t *testing.T) {
	reached := func(c Comparison) []string {
		f, e := CompareArms([]Comparison{c}, NoAssistant, MultiPerspective)
		if e != nil {
			t.Fatal(e)
		}
		out := []string{}
		for _, u := range f.Uncertainty {
			if strings.Contains(u, "did not reach their decision") {
				out = append(out, u)
			}
		}
		return out
	}
	// Positive control: distinct human receipts, so nothing is reported.
	c := UpliftFixture()[1]
	if got := reached(c); len(got) != 0 {
		t.Fatalf("arms with different human decisions were reported as unreached: %v", got)
	}
	same := c
	same.Runs = append([]ArmRun{}, c.Runs...)
	for i := range same.Runs {
		same.Runs[i].HumanHash = "everyone-decided-the-same"
	}
	got := reached(same)
	if len(got) == 0 {
		t.Fatal("the people decided identically in both arms and the finding does not say so")
	}
	// It qualifies the result; it does not by itself refuse one. A policy can
	// change what someone experiences without changing what they do.
	f, e := CompareArms([]Comparison{same}, NoAssistant, MultiPerspective)
	if e != nil {
		t.Fatal(e)
	}
	if f.Status == NotTested && strings.Contains(f.Evidence, "did not reach") {
		t.Fatal("an unreached decision was treated as a reason to refuse rather than to qualify")
	}
}

// Everyone correctly declining to act is a RESULT, not a broken control, and
// this evaluation has to be able to look at exactly those scenarios. What #58
// asks to detect is a control crippled relative to the candidates, which is an
// asymmetry between arms — so symmetric inability must pass and asymmetric
// inability must not.
func TestUniversalRestraintIsAResultNotABrokenControl(t *testing.T) {
	// Nobody acts anywhere, but everyone had the option: correct restraint.
	restraint := comparison()
	for i := range restraint.Runs {
		for j := range restraint.Runs[i].Outcomes {
			restraint.Runs[i].Outcomes[j].Acted = false
			restraint.Runs[i].Outcomes[j].CouldAct = true
		}
	}
	ledger(&restraint)
	if e := restraint.Validate(); e != nil {
		t.Fatalf("a scenario of universal correct restraint was rejected: %v", e)
	}
	// Nobody can act anywhere: the world offered no choice, in every arm alike.
	// That is a property of the world, not a rigged control.
	noChoice := comparison()
	for i := range noChoice.Runs {
		for j := range noChoice.Runs[i].Outcomes {
			noChoice.Runs[i].Outcomes[j].Acted = false
			noChoice.Runs[i].Outcomes[j].CouldAct = false
		}
	}
	ledger(&noChoice)
	if e := noChoice.Validate(); e != nil {
		t.Fatalf("a world that offered nobody a choice was rejected as a broken control: %v", e)
	}
	// But cripple ONLY the control and it must be caught.
	rigged := comparison()
	for j := range rigged.Runs[0].Outcomes {
		rigged.Runs[0].Outcomes[j].Acted = false
		rigged.Runs[0].Outcomes[j].CouldAct = false
	}
	ledger(&rigged)
	assertErr(t, rigged.Validate(), "control leaves everyone unable to act")
}
