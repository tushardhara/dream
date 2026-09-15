package main

import (
	"context"
	"testing"

	"github.com/tushardhara/dream/app/assistance"
	"github.com/tushardhara/dream/app/hws"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/evals"
	"github.com/tushardhara/dream/examples/responseexperiment"
)

// A sender's account of how their help landed is an expectation about someone
// else's experience. The source contract is explicit that a sender feeling
// useful is not recipient benefit, so no sender-position observation may
// become this person's evidence, and none may be the event a later report is
// bound to.
func TestSenderAccountIsNeverReadAsRecipientEvidence(t *testing.T) {
	scene := responseexperiment.Scene{Expectation: -1, Observation: "observed", ShareReport: true, Followup: true}
	run, e := responseexperiment.Run(context.Background(), scene, assistance.Multi, 11, nil)
	if e != nil {
		t.Fatal(e)
	}
	// Positive control: unmodified, this run yields recipient evidence.
	before := 0
	for _, person := range []core.ID{"alice", "bob"} {
		o, _ := responseOutcomeFor(person, run, evals.MultiPerspective, "unwanted")
		before += len(o.Observations)
	}
	if before == 0 {
		t.Fatal("positive control: no recipient evidence was produced at all")
	}
	// Remove only the recipient's own immediate accounts, leaving the sender's
	// immediate accounts of the same interactions in place. If the sender's
	// account were an acceptable substitute, evidence would survive this. It
	// must not: the sender's expectation is not the recipient's experience.
	stripped := run
	stripped.Outcomes = nil
	kept := 0
	for _, ob := range run.Outcomes {
		if ob.Phase == "immediate" && ob.Position == "recipient" {
			continue
		}
		if ob.Phase == "immediate" && ob.Position == "sender" {
			kept++
		}
		stripped.Outcomes = append(stripped.Outcomes, ob)
	}
	if kept == 0 {
		t.Fatal("positive control: the run carries no sender accounts to fall back to")
	}
	for _, person := range []core.ID{"alice", "bob"} {
		o, recs := responseOutcomeFor(person, stripped, evals.MultiPerspective, "unwanted")
		if len(o.Observations) != 0 {
			t.Fatalf("%s kept attributed evidence with only sender accounts available", person)
		}
		for _, r := range recs {
			if r.Kind == "later_self_report" {
				t.Fatalf("a later report was bound to a sender account: %s", r.ID)
			}
		}
	}
}

// The chain a later report hangs on must be a real earlier account by the same
// person, not a surrogate time. Every emitted later record must name an event
// record that exists, belongs to the same person and arm, and is earlier.
func TestLaterReportsHangOnARealEarlierAccount(t *testing.T) {
	scene := responseexperiment.Scene{Expectation: -1, Observation: "observed", ShareReport: true, Followup: true}
	run, e := responseexperiment.Run(context.Background(), scene, assistance.Multi, 11, nil)
	if e != nil {
		t.Fatal(e)
	}
	found := 0
	for _, person := range []core.ID{"alice", "bob"} {
		_, recs := responseOutcomeFor(person, run, evals.MultiPerspective, "unwanted")
		byID := map[core.ID]evals.SourceRecord{}
		for _, r := range recs {
			byID[r.ID] = r
		}
		for _, r := range recs {
			if r.Kind != "later_self_report" {
				continue
			}
			found++
			about, ok := byID[r.About]
			if !ok {
				t.Fatalf("later report %s names an event that is not in the ledger", r.ID)
			}
			if about.At >= r.At {
				t.Fatalf("later report %s at %d is not later than the event it is about at %d", r.ID, r.At, about.At)
			}
			if about.Subject != r.Subject || about.Arm != r.Arm {
				t.Fatalf("later report %s is about another person's or arm's event", r.ID)
			}
		}
	}
	if found == 0 {
		t.Fatal("positive control: no later self-reports were produced")
	}
}

// mutateOutcomes returns a copy of run with f applied to every observation.
func mutateOutcomes(run hws.ResponsiveAssistanceRun, f func(*core.OutcomeObservation)) hws.ResponsiveAssistanceRun {
	out := run
	out.Outcomes = append([]core.OutcomeObservation{}, run.Outcomes...)
	for i := range out.Outcomes {
		f(&out.Outcomes[i])
	}
	return out
}

func responseEvidence(t *testing.T, run hws.ResponsiveAssistanceRun) int {
	t.Helper()
	n := 0
	for _, person := range []core.ID{"alice", "bob"} {
		o, _ := responseOutcomeFor(person, run, evals.MultiPerspective, "unwanted")
		n += len(o.Observations)
	}
	return n
}

// This consumer emits 237 later recipient observations that its own subject did
// not author across the executed scenes. Reading those as independently
// attributed later evidence would be the exact tier promotion the contract
// exists to prevent, so the basis is checked, not assumed.
func TestLaterEvidenceRequiresTheSubjectsOwnReport(t *testing.T) {
	scene := responseexperiment.Scene{Expectation: -1, Observation: "observed", ShareReport: true, Followup: true}
	run, e := responseexperiment.Run(context.Background(), scene, assistance.Multi, 11, nil)
	if e != nil {
		t.Fatal(e)
	}
	if responseEvidence(t, run) == 0 {
		t.Fatal("positive control: the unmutated run produced no evidence")
	}
	notSelf := mutateOutcomes(run, func(ob *core.OutcomeObservation) {
		if ob.Phase == "later" {
			ob.Basis = "reported"
		}
	})
	if n := responseEvidence(t, notSelf); n != 0 {
		t.Fatalf("%d later reports the subject did not author became attributed evidence", n)
	}
}

// The event a later report hangs on must be the recipient's own earlier
// account. A sender's account of the same interaction is an expectation about
// someone else and may not stand in for it.
func TestTheEventMustBeTheRecipientsOwnEarlierAccount(t *testing.T) {
	scene := responseexperiment.Scene{Expectation: -1, Observation: "observed", ShareReport: true, Followup: true}
	run, e := responseexperiment.Run(context.Background(), scene, assistance.Multi, 11, nil)
	if e != nil {
		t.Fatal(e)
	}
	if responseEvidence(t, run) == 0 {
		t.Fatal("positive control: the unmutated run produced no evidence")
	}
	relabelled := mutateOutcomes(run, func(ob *core.OutcomeObservation) {
		if ob.Phase == "immediate" && ob.Position == "recipient" {
			ob.Position = "sender"
		}
	})
	if n := responseEvidence(t, relabelled); n != 0 {
		t.Fatalf("%d reports were bound to an account that is not the recipient's own", n)
	}
}

// "Later" is a temporal claim about real recorded times, not a label. An event
// recorded at or after the report it is cited by cannot be what the report is
// about.
func TestAReportIsNotLaterThanAnEventAtTheSameTime(t *testing.T) {
	scene := responseexperiment.Scene{Expectation: -1, Observation: "observed", ShareReport: true, Followup: true}
	run, e := responseexperiment.Run(context.Background(), scene, assistance.Multi, 11, nil)
	if e != nil {
		t.Fatal(e)
	}
	if responseEvidence(t, run) == 0 {
		t.Fatal("positive control: the unmutated run produced no evidence")
	}
	// Push every immediate account past every later report, changing nothing
	// else. The reports are then not later than anything.
	latest := core.LogicalTime(0)
	for _, ob := range run.Outcomes {
		if ob.LearnedAt > latest {
			latest = ob.LearnedAt
		}
	}
	shifted := mutateOutcomes(run, func(ob *core.OutcomeObservation) {
		if ob.Phase == "immediate" {
			ob.OccurredAt = latest + 1
		}
	})
	if n := responseEvidence(t, shifted); n != 0 {
		t.Fatalf("%d reports stayed attributed to an event that is not earlier than them", n)
	}
}

// The dispositions and the unwanted count must come from the recipient's own
// account, not from the scene's name. These are the mappings the report's
// harm and uncertainty qualifications are built on, so each is exercised
// against the real consumer rather than asserted.
func TestDispositionsAndUnwantedComeFromTheRecipientsAccount(t *testing.T) {
	seen := map[string]int{}
	unwanted, dismissive, censoredAvailable := 0, 0, false
	for _, sc := range responseScenes {
		for _, m := range helperArms {
			run, e := responseexperiment.Run(context.Background(), sc.scene, m.consumer, 11, nil)
			if e != nil {
				t.Fatal(e)
			}
			for _, ob := range run.Outcomes {
				if ob.Phase != "later" || ob.Position != "recipient" {
					continue
				}
				// Dismissive appraisals are counted on the same basis the
				// adapter counts them on, so the two numbers are comparable.
				if ob.Appraisal == "dismissive" {
					dismissive++
				}
				// Censored observations are NOT self-reports — the recipient is
				// precisely who could not report — so this is not gated on it.
				if ob.Status == core.Censored {
					censoredAvailable = true
				}
			}
			for _, person := range []core.ID{"alice", "bob"} {
				o, _ := responseOutcomeFor(person, run, m.arm, sc.name)
				seen[o.DelayedOutcome]++
				unwanted += o.Unwanted
			}
		}
	}
	// censored is included: a declined participation or absent follow-up is a
	// censored outcome, and it also carries a Missing reason, so a mapping that
	// checked Missing first would silently downgrade every censored result to
	// missing. Both are harms, which is exactly why the distinction has to be
	// asserted rather than inferred from the finding.
	for _, want := range []string{"missing", "censored", "resolved", "unresolved"} {
		if seen[want] == 0 {
			t.Fatalf("no person reached the %q disposition; the mapping is not exercised: %v", want, seen)
		}
	}
	if dismissive == 0 {
		t.Fatal("positive control: no dismissive appraisal exists to count as unwanted")
	}
	if unwanted != dismissive {
		t.Fatalf("counted %d unwanted interventions from %d dismissive appraisals", unwanted, dismissive)
	}
	// Censored is mapped but this consumer does not produce it. Recorded here
	// so the gap is visible rather than mistaken for coverage.
	if !censoredAvailable {
		t.Fatal("positive control: no censored later observation exists to map")
	}
}

// The privacy check must be able to fail. A detector that cannot detect is
// worse than none: it turns an unmeasured zero into a measured-looking one.
func TestTheHelperLeakDetectorActuallyDetects(t *testing.T) {
	scene := responseexperiment.Scene{Expectation: -1, Observation: "observed", ShareReport: true, Followup: true}
	a, e := responseexperiment.Run(context.Background(), scene, assistance.Simple, 11, nil)
	if e != nil {
		t.Fatal(e)
	}
	// Two arms genuinely differ in helper output, so this is a real positive.
	b, e := responseexperiment.Run(context.Background(), scene, assistance.Multi, 11, nil)
	if e != nil {
		t.Fatal(e)
	}
	if !helperOutputDiffers(a, b) {
		t.Fatal("the detector cannot tell two genuinely different helper outputs apart")
	}
	if helperOutputDiffers(a, a) {
		t.Fatal("the detector reports a difference between a run and itself")
	}
}

// And the property it checks must actually hold on the real consumer: changing
// only the recipient's private conditions must not move the helper's output.
func TestPrivateRecipientStateDoesNotReachTheHelper(t *testing.T) {
	for _, sc := range responseScenes {
		for _, m := range helperArms {
			for _, seed := range upliftSeeds {
				base, e := responseexperiment.Run(context.Background(), sc.scene, m.consumer, seed, nil)
				if e != nil {
					t.Fatal(e)
				}
				leak, e := privateStateLeaks(context.Background(), sc.scene, m.consumer, seed, base)
				if e != nil {
					t.Fatalf("%s/%s/%d: %v", sc.name, m.consumer, seed, e)
				}
				if leak {
					t.Fatalf("%s/%s/%d: private recipient state reached the helper's output", sc.name, m.consumer, seed)
				}
			}
		}
	}
}

// The privacy probe is only as good as its variant. A variant that changes
// nothing, or changes only one of the private conditions, would pass while the
// helper leaked the other.
func TestThePrivateVariantChangesEveryPrivateCondition(t *testing.T) {
	for _, sc := range responseScenes {
		v, e := privateVariant(sc.scene)
		if e != nil {
			t.Fatalf("%s: %v", sc.name, e)
		}
		if v.Expectation == sc.scene.Expectation {
			t.Fatalf("%s: the variant keeps the same private expectation", sc.name)
		}
		if v.PrivateFear == sc.scene.PrivateFear {
			t.Fatalf("%s: the variant keeps the same private fear", sc.name)
		}
		// Nothing public may change, or a difference in helper output would no
		// longer be evidence of a leak.
		v.Expectation, v.PrivateFear = sc.scene.Expectation, sc.scene.PrivateFear
		if v != sc.scene {
			t.Fatalf("%s: the variant changed something the helper is allowed to see", sc.name)
		}
	}
	// A scene whose private conditions are at their own midpoint cannot be
	// varied this way, and is refused rather than silently run as a no-op.
	if _, e := privateVariant(responseexperiment.Scene{Expectation: 0, PrivateFear: .5}); e == nil {
		t.Fatal("a vacuous private variant was accepted")
	}
	// Changing only one condition is refused too.
	if _, e := privateVariant(responseexperiment.Scene{Expectation: 1, PrivateFear: .5}); e == nil {
		t.Fatal("a variant that changes only the expectation was accepted")
	}
}
