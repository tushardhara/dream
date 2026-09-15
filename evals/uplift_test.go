package evals

import (
	"strings"
	"testing"

	"github.com/tushardhara/dream/core"
)

func person(i int) core.ID { return core.ID([]string{"person:01", "person:02"}[i]) }

func evidence(p core.ID, tier EvidenceTier, v float64) TierEvidence {
	return TierEvidence{Person: p, Tier: tier, Provenance: SyntheticProvenance, Metric: "reported_benefit", Value: core.ObservedGroupQuantity(v)}
}

func outcome(i int, acted bool) PersonOutcome {
	return PersonOutcome{
		Person: person(i), Benefit: core.ObservedGroupQuantity(.2), Burden: core.UnknownGroupQuantity(),
		Appropriateness: core.ObservedGroupQuantity(.5), DelayedOutcome: "unresolved", Acted: acted,
		Observations: []TierEvidence{evidence(person(i), BehaviouralObservation, .1)},
	}
}

func armRun(a Arm, stream string) ArmRun {
	return ArmRun{Arm: a, Seed: 7, WorldHash: "world-1", ExogenousHash: "exo-1", RNGStream: stream,
		Scenario: "scenario:ordinary", Outcomes: []PersonOutcome{outcome(0, true), outcome(1, false)}}
}

func comparison() Comparison {
	c := Comparison{Version: UpliftVersion, Scenario: "scenario:ordinary", Seed: 7}
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
		{"an arm is missing", "comparison must run every arm", func(c *Comparison) {
			c.Runs = c.Runs[:3]
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
