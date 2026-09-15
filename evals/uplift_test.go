package evals

import (
	"strings"
	"testing"

	"github.com/tushardhara/dream/core"
)

func person(i int) core.ID { return core.ID([]string{"person:01", "person:02"}[i]) }

func evidence(p core.ID, tier EvidenceTier, v float64) TierEvidence {
	e := TierEvidence{Person: p, Tier: tier, Provenance: SyntheticProvenance, Metric: "reported_benefit", Value: core.ObservedGroupQuantity(v)}
	if tier == AttributedLater {
		e.Source, e.Observer, e.At = "experience:fixture", p, 5
	}
	return e
}

func outcome(i int, acted bool) PersonOutcome {
	return PersonOutcome{
		Person: person(i), Benefit: core.ObservedGroupQuantity(.2), Burden: core.UnknownGroupQuantity(),
		BurdenReduction: core.UnknownGroupQuantity(),
		Appropriateness: core.ObservedGroupQuantity(.5), DelayedOutcome: "unresolved", Acted: acted,
		Observations: []TierEvidence{evidence(person(i), BehaviouralObservation, .1)},
	}
}

func armRun(a Arm, stream string) ArmRun {
	return ArmRun{Arm: a, Seed: 7, WorldHash: "world-1", ExogenousHash: "exo-1", RNGStream: stream,
		Scenario: "scenario:ordinary", Outcomes: []PersonOutcome{outcome(0, true), outcome(1, false)}}
}

func comparison() Comparison {
	c := Comparison{Version: UpliftVersion, Scenario: "scenario:ordinary", Seed: 7, Affected: []core.ID{person(0), person(1)}}
	for i, a := range Arms {
		c.Runs = append(c.Runs, armRun(a, string(rune('a'+i))+"-stream"))
	}
	return c
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
		{"broken no-assistant control makes everyone wait", "arm makes every human wait", func(c *Comparison) {
			c.Runs[0].Outcomes[0].Acted = false
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
		{"arms share an rng stream", "share an rng stream", func(c *Comparison) {
			c.Runs[3].RNGStream = c.Runs[0].RNGStream
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
	f, e := CompareArms([]Comparison{c}, NoAssistant, MultiPerspective)
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
	if e := armRun(NoAssistant, "a-stream").Validate(); e != nil {
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
		a := armRun(NoAssistant, "a-stream")
		a.Arm = Arm("marketing")
		assertErr(t, a.Validate(), "unknown comparison arm")
	})
	t.Run("arm run identity", func(t *testing.T) {
		a := armRun(SimpleAssistance, "a-stream")
		a.WorldHash = ""
		assertErr(t, a.Validate(), "invalid arm run identity")
	})
	t.Run("arm with no affected people", func(t *testing.T) {
		a := armRun(SimpleAssistance, "a-stream")
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
	if _, e := CompareArms([]Comparison{comparison()}, MultiPerspective, MultiPerspective); e == nil || !strings.Contains(e.Error(), "invalid arm comparison") {
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
	f, e := CompareArms([]Comparison{c}, NoAssistant, MultiPerspective)
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
	f, e := CompareArms([]Comparison{a, b}, NoAssistant, MultiPerspective)
	if e != nil {
		t.Fatal(e)
	}
	if f.Status != Inconclusive || !strings.Contains(f.Evidence, "overlap") {
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
	f, e := CompareArms([]Comparison{a, b}, NoAssistant, MultiPerspective)
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
	f, e := CompareArms([]Comparison{*a, *b}, NoAssistant, MultiPerspective)
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
	f, e := CompareArms([]Comparison{c}, NoAssistant, MultiPerspective)
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
	f, e := CompareArms([]Comparison{*a, *b}, NoAssistant, MultiPerspective)
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
	f, e := CompareArms([]Comparison{a, b}, NoAssistant, MultiPerspective)
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
	f, e := CompareArms([]Comparison{*a, *b}, NoAssistant, MultiPerspective)
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
	f, e := CompareArms([]Comparison{*a, *b}, NoAssistant, MultiPerspective)
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
	f, e := CompareArms([]Comparison{*a, *b}, NoAssistant, MultiPerspective)
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
	f, e := CompareArms([]Comparison{*a, *b}, NoAssistant, MultiPerspective)
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
	f, e := CompareArms([]Comparison{*a, *b}, NoAssistant, MultiPerspective)
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
