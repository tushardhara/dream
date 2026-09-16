package evals

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/dynamics"
	"github.com/tushardhara/dream/simulator/experiment"
)

var fixtureNow = time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)

func fixture(t *testing.T) (Dataset, Config) {
	t.Helper()
	d, c, e := SyntheticFixture()
	if e != nil {
		t.Fatal(e)
	}
	if e = d.Validate(fixtureNow); e != nil {
		t.Fatal(e)
	}
	return d, c
}

type batchFunc func(context.Context, []experiment.Request) ([]experiment.Projection, error)

func (f batchFunc) Generate(c context.Context, r []experiment.Request) ([]experiment.Projection, error) {
	return f(c, r)
}
func TestFrozenOfflineReportAndCalibration(t *testing.T) {
	d, c := fixture(t)
	r, e := Run(context.Background(), d, c, experiment.Generator{}, fixtureNow)
	if e != nil {
		t.Fatal(e)
	}
	again, e := Run(context.Background(), d, c, experiment.Generator{}, fixtureNow)
	if e != nil || r.Hash != again.Hash || r.Verify() != nil {
		t.Fatal("report not reproducible", e)
	}
	if r.HumanValidity != NotTested || !r.SyntheticOnly {
		t.Fatal("fabricated human validity")
	}
	for _, m := range r.Metrics {
		if m.Dimension == "latent_trust" {
			if m.Brier != nil || m.Resolved != 0 || m.Unresolved != 4 || m.Status != Inconclusive {
				t.Fatal("latent truth invented", m)
			}
		}
		if m.Dimension == "ask" {
			if m.Missing != 2 || m.Censored != 1 || m.NotTaken != 1 || m.Brier != nil || m.BaseRate != nil {
				t.Fatal("missingness hidden", m)
			}
		}
		if m.Variant == experiment.Baseline && m.Dimension == "wait" && m.Horizon == dynamics.Hour {
			if m.Brier == nil || *m.Brier != .5 || m.BaseRateBrier == nil || *m.BaseRateBrier != .25 || m.BaseRate == nil || *m.BaseRate != .5 || m.BrierCI == nil || m.BrierCI.Units != 4 {
				t.Fatal("Brier/baseline/independence arithmetic", m)
			}
		}
		if m.Horizon == 2*dynamics.Hour && m.Abstentions != len(c.Seeds) {
			t.Fatal("horizon abstention hidden", m)
		}
	}
}
func TestLabelsNeverReachGeneration(t *testing.T) {
	d, c := fixture(t)
	calls := 0
	spy := batchFunc(func(ctx context.Context, requests []experiment.Request) ([]experiment.Projection, error) {
		calls++
		raw, e := json.Marshal(requests)
		if e != nil {
			t.Fatal(e)
		}
		for _, canary := range []string{"EVALUATOR_LABEL_CANARY", "annotation", "labels", "later_action", ":outcome:", "judge_output"} {
			if strings.Contains(string(raw), canary) {
				t.Fatalf("generator received %s", canary)
			}
		}
		return (experiment.Generator{}).Generate(ctx, requests)
	})
	first, e := Run(context.Background(), d, c, spy, fixtureNow)
	if e != nil {
		t.Fatal(e)
	}
	for i := range d.Cases {
		for j := range d.Cases[i].Labels {
			l := &d.Cases[i].Labels[j]
			l.Annotation = "judge_output_DIFFERENT_CANARY"
			if l.Value != nil {
				*l.Value = !*l.Value
			}
		}
	}
	d, _ = SealDataset(d)
	c.DatasetHash = d.Hash
	second, e := Run(context.Background(), d, c, spy, fixtureNow)
	if e != nil || calls != 2 || first.PredictionsHash != second.PredictionsHash {
		t.Fatal("labels changed generation", e)
	}
}
func TestDatasetLeakageAndConsentNegatives(t *testing.T) {
	tests := map[string]func(*Dataset){
		"family":            func(d *Dataset) { d.Cases[4].Family = d.Cases[0].Family },
		"person":            func(d *Dataset) { d.Cases[4].People = append(d.Cases[4].People, d.Cases[0].People[0]) },
		"group":             func(d *Dataset) { d.Cases[4].Groups = d.Cases[0].Groups },
		"undeclared_person": func(d *Dataset) { d.Cases[0].People = d.Cases[0].People[:1] },
		"future_input":      func(d *Dataset) { d.Cases[0].Input.Current.Perceived.LearnedAt = 10000 * 24 * dynamics.Hour },
		"late_train_label": func(d *Dataset) {
			for i := range d.Sources {
				if d.Sources[i].Kind == LaterAction {
					d.Sources[i].LearnedAt = 10000 * 24 * dynamics.Hour
					break
				}
			}
		},
		"no_generation_consent":    func(d *Dataset) { d.Sources[0].Consent.Generate = false },
		"revoked":                  func(d *Dataset) { d.Sources[0].Consent.Revoked = true },
		"expired":                  func(d *Dataset) { d.Sources[0].Consent.Expires = fixtureNow },
		"no_retention":             func(d *Dataset) { d.Sources[0].Consent.Retain = false },
		"real_data":                func(d *Dataset) { d.Sources[0].Synthetic = false },
		"wrong_purpose":            func(d *Dataset) { d.Sources[0].Consent.Purpose = "marketing" },
		"wrong_subject":            func(d *Dataset) { d.Sources[0].Consent.Subject = "outsider" },
		"missing_provenance":       func(d *Dataset) { d.Sources[0].Provenance = "" },
		"missing_source":           func(d *Dataset) { d.Cases[0].Input.Initial.Memory[0].Evidence = []core.ID{"absent"} },
		"latent_truth":             func(d *Dataset) { d.Cases[0].Labels[0].Dimension = "latent_motive" },
		"missing_with_truth":       func(d *Dataset) { d.Cases[0].Labels[0].Status = Missing },
		"wrong_observation_window": func(d *Dataset) { d.Cases[0].Labels[0].Horizon = 3 },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			d, _ := fixture(t)
			mutate(&d)
			d, _ = SealDataset(d)
			if d.Validate(fixtureNow) == nil {
				t.Fatal("invalid dataset accepted")
			}
		})
	}
	d, _ := fixture(t)
	d.Cases[0].Labels[0].Annotation = "changed"
	if d.Validate(fixtureNow) == nil {
		t.Fatal("frozen hash ignored")
	}
}
func TestInvalidGeneratorAndNoData(t *testing.T) {
	for _, kind := range []string{"missing", "nan", "horizon", "abstain", "reorder"} {
		t.Run(kind, func(t *testing.T) {
			d, c := fixture(t)
			g := batchFunc(func(ctx context.Context, r []experiment.Request) ([]experiment.Projection, error) {
				p, e := (experiment.Generator{}).Generate(ctx, r)
				if e != nil {
					return nil, e
				}
				switch kind {
				case "reorder":
					p[0], p[1] = p[1], p[0]
				case "missing":
					p = p[:len(p)-1]
				case "nan":
					p[0].Candidates[0].Probability = math.NaN()
				case "horizon":
					p[0].Horizon = 8
				case "abstain":
					p[0].Abstained = true
				}
				return p, nil
			})
			if _, e := Run(context.Background(), d, c, g, fixtureNow); e == nil {
				t.Fatal("broken predictions accepted")
			}
		})
	}
	d, c := fixture(t)
	d.Cases = d.Cases[:1]
	d.Cases[0].Split = Holdout
	for j := range d.Cases[0].Labels {
		d.Cases[0].Labels[j].Status = Missing
		d.Cases[0].Labels[j].Value = nil
	}
	d, _ = SealDataset(d)
	c.DatasetHash = d.Hash
	r, e := Run(context.Background(), d, c, experiment.Generator{}, fixtureNow)
	if e != nil {
		t.Fatal(e)
	}
	for _, m := range r.Metrics {
		if m.Brier != nil || m.BaseRateBrier != nil || m.Status != Inconclusive || m.SelectionCI != nil {
			t.Fatal("no data/one cluster became PASS", m)
		}
	}
}
func TestConnectedComponentsNotSeeds(t *testing.T) {
	d, c := fixture(t)
	for i := 1; i < 4; i++ {
		d.Cases[i].Groups = d.Cases[0].Groups
	}
	d, _ = SealDataset(d)
	c.DatasetHash = d.Hash
	r, e := Run(context.Background(), d, c, experiment.Generator{}, fixtureNow)
	if e != nil {
		t.Fatal(e)
	}
	for _, m := range r.Metrics {
		if m.Split == Train && m.BrierCI != nil {
			t.Fatal("related cases counted as independent")
		}
	}
}

func TestActualVariantsAndConditionalRollForward(t *testing.T) {
	d, _ := fixture(t)
	input := d.Cases[0].Input
	seen := map[string]experiment.Variant{}
	before, _ := Digest(input)
	for _, v := range experiment.Variants() {
		p, e := (experiment.Generator{}).Predict(context.Background(), input, v, 11)
		if e != nil {
			t.Fatal(v, e)
		}
		signature, _ := Digest(p.Candidates)
		if other, ok := seen[signature]; ok {
			t.Fatalf("%s and %s are identical ablations", v, other)
		}
		seen[signature] = v
	}
	after, _ := Digest(input)
	if before != after {
		t.Fatal("generator mutated historical input")
	}
	p, e := (experiment.Generator{}).Predict(context.Background(), input, experiment.Stateful, 11)
	if e != nil {
		t.Fatal(e)
	}
	input.Horizon = 2 * dynamics.Hour
	q, e := (experiment.Generator{}).Predict(context.Background(), input, experiment.Stateful, 11)
	if e != nil {
		t.Fatal(e)
	}
	ph, _ := Digest(p.Candidates)
	qh, _ := Digest(q.Candidates)
	if ph == qh {
		t.Fatal("roll-forward did not advance dynamics")
	}
	input.Current.Outage = true
	for _, v := range experiment.Variants() {
		p, e := (experiment.Generator{}).Predict(context.Background(), input, v, 11)
		if e != nil || !p.Abstained {
			t.Fatal("provider outage treated as human WAIT", v, e)
		}
	}
	input.Current.Outage = false
	input.Current.Offers[0].Kind = "invented"
	for _, v := range experiment.Variants() {
		if _, e := (experiment.Generator{}).Predict(context.Background(), input, v, 11); e == nil {
			t.Fatal("baseline bypassed malformed input validation")
		}
	}
}

func TestReportRejectsInventedEvidence(t *testing.T) {
	d, c := fixture(t)
	r, e := Run(context.Background(), d, c, experiment.Generator{}, fixtureNow)
	if e != nil {
		t.Fatal(e)
	}
	r.Metrics[0].Resolved = 99
	r, _ = SealReport(r)
	if r.Verify() == nil {
		t.Fatal("impossible counts accepted with recomputed digest")
	}
}

func TestFixtureFamiliesHaveDifferentTemplates(t *testing.T) {
	d, _ := fixture(t)
	seen := map[string]bool{}
	for _, c := range d.Cases {
		h, _ := Digest(c.Input.Current.Perceived.Signals)
		if seen[h] {
			t.Fatal("same scenario template renamed across families")
		}
		seen[h] = true
	}
}

// Before #84 the frozen_split_integrity row was a constant inside the Falsifiers
// literal. It read "pass" whatever the run computed, so a reader of
// bin/evaluation-report.json saw a measured-looking verdict that was a string,
// and #16's "deliberately broken implementations fail corresponding engineering
// checks" did not hold for this row.
//
// The row is now recomputed from the supplied dataset. This test drives that
// computation directly, because the surrounding behaviour hides it: Run refuses
// a leaking dataset before generation, and cmd/hws-eval prints nothing when Run
// returns an error, so no emitted report can ever carry a failing row. That is
// correct fail-closed behaviour and it is also why the row could stay a literal
// unnoticed.
func TestFrozenSplitIntegrityIsComputedNotAsserted(t *testing.T) {
	d, c := fixture(t)
	if f := splitIntegrity(d, c); f.Status != Pass {
		t.Fatal("clean fixture reported", f.Status, f.Evidence)
	}

	for name, mutate := range map[string]func(*Dataset){
		"family": func(d *Dataset) { d.Cases[4].Family = d.Cases[0].Family },
		"person": func(d *Dataset) { d.Cases[4].People = append(d.Cases[4].People, d.Cases[0].People[0]) },
		"group":  func(d *Dataset) { d.Cases[4].Groups = d.Cases[0].Groups },
	} {
		t.Run(name+"_crosses_split", func(t *testing.T) {
			leaky, cfg := fixture(t)
			mutate(&leaky)
			leaky, _ = SealDataset(leaky)
			cfg.DatasetHash = leaky.Hash
			f := splitIntegrity(leaky, cfg)
			if f.Status != Fail {
				t.Fatal("a dataset whose", name, "crosses splits reported", f.Status)
			}
			if !strings.Contains(f.Evidence, "splits") {
				t.Fatal("failure does not name the crossing:", f.Evidence)
			}
			// Run must also refuse it, so the command still fails.
			if _, e := Run(context.Background(), leaky, cfg, experiment.Generator{}, fixtureNow); e == nil {
				t.Fatal("Run accepted a dataset whose splits leak")
			}
		})
	}

	t.Run("time_crosses_split", func(t *testing.T) {
		// The chronological axis, which the identity mutations above cannot
		// reach: every family, person and group stays in exactly one split, and
		// only the calibration split's clock is moved back behind the end of
		// train. Before this case splitIntegrity measured no time at all while
		// its Pass evidence claimed it did, so a regression in Dataset.Validate's
		// separate time-leakage check would have left the row reading "pass".
		leaky, cfg := fixture(t)
		for i := range leaky.Cases {
			if leaky.Cases[i].Split == Calibration {
				leaky.Cases[i].Input.Initial.State.At = leaky.Cases[0].Input.Initial.State.At
				break
			}
		}
		leaky, _ = SealDataset(leaky)
		cfg.DatasetHash = leaky.Hash

		f := splitIntegrity(leaky, cfg)
		if f.Status != Fail {
			t.Fatal("a dataset whose calibration split starts before train ends reported", f.Status, f.Evidence)
		}
		if !strings.Contains(f.Evidence, "at or past the start of") {
			t.Fatal("failure does not name the chronological boundary:", f.Evidence)
		}
		if _, e := Run(context.Background(), leaky, cfg, experiment.Generator{}, fixtureNow); e == nil {
			t.Fatal("Run accepted a dataset whose splits overlap in time")
		}

		// The identity axes are untouched, so this case would pass every check
		// splitIntegrity made before it gained the chronological boundary.
		clean, _ := fixture(t)
		for i := range clean.Cases {
			if clean.Cases[i].Split == Calibration {
				clean.Cases[i].Input.Initial.State.At = clean.Cases[0].Input.Initial.State.At
				break
			}
		}
		identities := map[string]Split{}
		for _, row := range clean.Cases {
			for axis, keys := range [][]core.ID{{row.Family}, row.People, row.Groups} {
				for _, id := range keys {
					key := fmt.Sprintf("%d/%s", axis, id)
					if previous, ok := identities[key]; ok && previous != row.Split {
						t.Fatal("the time mutation also crossed an identity split at", key, "; this case no longer isolates the chronological axis")
					}
					identities[key] = row.Split
				}
			}
		}
	})

	t.Run("configured hash must match the supplied dataset", func(t *testing.T) {
		other, cfg := fixture(t)
		cfg.DatasetHash = "0000000000000000000000000000000000000000000000000000000000000000"
		if f := splitIntegrity(other, cfg); f.Status != Fail {
			t.Fatal("a mismatched dataset hash reported", f.Status)
		}
	})
}
