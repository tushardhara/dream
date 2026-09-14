package drives

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"github.com/tushardhara/dream/simulator/dynamics"
	"reflect"
	"testing"
)

// Every drive must respond to a permitted input AND influence a tendency.
// One at a time changes prevent other active drives from masking an inert row.
//
//go:embed testdata/legacy-state.json
var savedLegacy []byte

func TestPerDriveSensitivity(t *testing.T) {
	s := initial(t)
	neutral := observation()
	neutral.Event.Signals = dynamics.Signals{}
	for i := range neutral.Context {
		neutral.Context[i].Value = 0
	}
	baseline, _, _, e := Appraise(s, neutral, 0)
	if e != nil {
		t.Fatal(e)
	}
	signals := []dynamics.Signals{{Effort: 1}, {Rest: 1}, {Scarcity: 1}, {Opportunity: 1}, {OtherNeed: 1}, {Support: 1}, {StatusThreat: 1}, {Exclusion: 1}, {Inclusion: 1}}
	for i, d := range Registry() {
		t.Run(string(d.ID), func(t *testing.T) {
			responsive := false
			for _, x := range signals {
				o := neutral
				o.Event.Signals = x
				n, _, _, e := Appraise(s, o, 0)
				if e != nil {
					t.Fatal(e)
				}
				responsive = responsive || n.Variables[i].Values[0] != baseline.Variables[i].Values[0]
			}
			for j := range neutral.Context {
				o := neutral
				o.Context[j].Value = 1
				n, _, _, e := Appraise(s, o, 0)
				if e != nil {
					t.Fatal(e)
				}
				responsive = responsive || n.Variables[i].Values[0] != baseline.Variables[i].Values[0]
			}
			if !responsive {
				t.Fatal("drive inert under all permitted independent inputs")
			}
			low, e := Intervene(s, i, .05, 0, "low")
			if e != nil {
				t.Fatal(e)
			}
			high, e := Intervene(s, i, .95, 0, "high")
			if e != nil {
				t.Fatal(e)
			}
			a, e := Response(low)
			if e != nil {
				t.Fatal(e)
			}
			b, e := Response(high)
			if e != nil {
				t.Fatal(e)
			}
			if a == b {
				t.Fatal("drive has no behavioral consequence")
			}
		})
	}
}

func TestSavedLegacyCheckpointAndUpgrade(t *testing.T) {
	raw := savedLegacy

	decoded, e := DecodeRecorded(raw)
	if e != nil || decoded.Legacy == nil {
		t.Fatal("saved old dispatch", e)
	}
	old := *decoded.Legacy
	const hash = "b3fe47d86a778eca18f6c22604d3f61c9d1623c318b190cc311e6e088ed83ae8"
	h, e := old.Hash()
	if e != nil || h != hash {
		t.Fatal("saved legacy logical hash changed", e, h)
	}
	b, e := old.Canonical()
	if e != nil || !bytes.Equal(raw, b) {
		t.Fatal("saved legacy bytes changed", e)
	}
	later, e := dynamics.Advance(old, old.At+12*Hour)
	if e != nil {
		t.Fatal(e)
	}
	h, e = later.Hash()
	if e != nil || h != "c3f16c6ede4ac36176ee4b91ef0feda24911985bffe052c30d77aa88dcc7a168" {
		t.Fatal("frozen policy replay changed", e, h)
	}
	upgraded, e := UpgradeLegacy(old, "new:research-branch")
	if e != nil {
		t.Fatal(e)
	}
	if upgraded.Upgrade.SourceHash != hash || upgraded.Upgrade.NewBranch != "new:research-branch" || !reflect.DeepEqual(upgraded.Applied, old.Applied) || !reflect.DeepEqual(upgraded.Causes, old.Causes) {
		t.Fatal("upgrade lost provenance/receipts")
	}
	for _, pair := range [][2]int{{Care, dynamics.Care}, {Status, dynamics.Status}, {Belonging, dynamics.Belonging}} {
		if upgraded.Variables[pair[0]].Values[0] != old.Variables[pair[1]].Level {
			t.Fatal("semantic mapping lost level")
		}
	}
	for i, index := range factorIndices {
		if upgraded.Factors.Variables[i].Values[0] != old.Variables[index].Level {
			t.Fatal("factor lost")
		}
	}
	if upgraded.Variables[Acquisition].Values[2] != 0 || upgraded.Variables[EffortAvoidance].Values[2] != 0 {
		t.Fatal("invented new drive evidence")
	}
	b, e = old.Canonical()
	if e != nil || !bytes.Equal(raw, b) {
		t.Fatal("upgrade rewrote history")
	}
	encoded, e := upgraded.Canonical()
	if e != nil {
		t.Fatal(e)
	}
	if _, e = Decode(encoded); e != nil {
		t.Fatal("upgrade recovery", e)
	}
	if _, e = UpgradeLegacy(old, ""); e == nil {
		t.Fatal("missing new branch accepted")
	}
}

func TestRetainedFactorsRemainDistinct(t *testing.T) {
	old, e := dynamics.New("a", 0, dynamics.DefaultSubstrate())
	if e != nil {
		t.Fatal(e)
	}
	old, e = dynamics.Intervene(old, dynamics.Fatigue, .9, 0, "fatigue-source")
	if e != nil {
		t.Fatal(e)
	}
	s, e := UpgradeLegacy(old, "new:branch")
	if e != nil {
		t.Fatal(e)
	}
	o := observation()
	a, _, _, e := dynamics.Appraise(old, o.Event, Hour)
	if e != nil {
		t.Fatal(e)
	}
	b, _, _, e := Appraise(s, o, Hour)
	if e != nil {
		t.Fatal(e)
	}
	for i, index := range factorIndices {
		v := a.Variables[index]
		want := Variable{[4]float64{v.Level, v.Anchor, float64(v.Confidence), float64(v.AnchorConfidence)}, v.AnchorAt}
		if b.Factors.Variables[i] != want {
			t.Fatal("legacy factor semantics changed", i)
		}
	}
	changed, e := Intervene(s, EffortAvoidance, 1, 0, "avoidance")
	if e != nil {
		t.Fatal(e)
	}
	if changed.Factors != s.Factors {
		t.Fatal("effort avoidance overwrote fatigue")
	}
	lowFatigue := s
	lowFatigue.Factors.Variables[Fatigue] = Variable{[4]float64{.1, .1, 1, 1}, 0}
	low, _, _, e := Appraise(lowFatigue, o, 0)
	if e != nil {
		t.Fatal(e)
	}
	high, _, _, e := Appraise(s, o, 0)
	if e != nil {
		t.Fatal(e)
	}
	if low.Variables[Care] == high.Variables[Care] {
		t.Fatal("distinct fatigue no longer affects appraisal")
	}
}

// Effort/rest have no direct input to approach desire or curiosity. Their current
// fatigue delta must therefore influence the next event, not feed back within
// this event. Moving factor appraisal ahead of effortBurden breaks this test.
func TestRetainedFactorAppraisalOrdering(t *testing.T) {
	s := initial(t)
	effort, rest := observation(), observation()
	effort.Event.Signals = dynamics.Signals{Effort: 1}
	rest.Event.Signals = dynamics.Signals{Rest: 1}
	a, _, _, e := Appraise(s, effort, Hour)
	if e != nil {
		t.Fatal(e)
	}
	b, _, _, e := Appraise(s, rest, Hour)
	if e != nil {
		t.Fatal(e)
	}
	if a.Variables[ApproachDesire].Values[0] != .40264 || a.Variables[Curiosity].Values[0] != .404224 {
		t.Fatalf("pre-event appraisal reference changed: approach=%g curiosity=%g", a.Variables[ApproachDesire].Values[0], a.Variables[Curiosity].Values[0])
	}
	if a.Factors.Variables[Fatigue] == b.Factors.Variables[Fatigue] {
		t.Fatal("control did not change fatigue")
	}
	for _, i := range []int{ApproachDesire, Curiosity} {
		if a.Variables[i] != b.Variables[i] {
			t.Fatal("same-event factor feedback", i)
		}
	}
	// Isolate fatigue from the independently updated effort-avoidance drive.
	b.Variables = a.Variables
	next := observation()
	next.Event.Event = "next"
	next.Event.Rights.Resource = "next"
	next.Event.Signals = dynamics.Signals{}
	aa, _, _, e := Appraise(a, next, Hour)
	if e != nil {
		t.Fatal(e)
	}
	bb, _, _, e := Appraise(b, next, Hour)
	if e != nil {
		t.Fatal(e)
	}
	for _, i := range []int{ApproachDesire, Curiosity} {
		if aa.Variables[i] == bb.Variables[i] {
			t.Fatal("prior fatigue did not affect next appraisal", i)
		}
	}
}

func TestRetainedFactorWireVersion(t *testing.T) {
	raw, e := initial(t).Canonical()
	if e != nil {
		t.Fatal(e)
	}
	if Version != 3 || ModelVersion != "appraisal.drives.v2" {
		t.Fatal("retained-factor format/model version drift")
	}
	for _, change := range []func(map[string]any){
		func(m map[string]any) { m["version"] = 2 },
		func(m map[string]any) { m["model"] = "appraisal.drives.v1" },
		func(m map[string]any) { delete(m, "factors") },
		func(m map[string]any) { m["version"] = 2; m["model"] = "appraisal.drives.v1"; delete(m, "factors") },
	} {
		var m map[string]any
		if e := json.Unmarshal(raw, &m); e != nil {
			t.Fatal(e)
		}
		change(m)
		bad, e := json.Marshal(m)
		if e != nil {
			t.Fatal(e)
		}
		var state State
		if e := json.Unmarshal(bad, &state); e != nil {
			t.Fatal(e)
		}
		// Do not let map key ordering/noncanonical JSON mask a missing version guard.
		if e := state.Validate(); e == nil {
			t.Fatal("draft or mixed state validates")
		}
		if _, e := DecodeRecorded(bad); e == nil {
			t.Fatal("draft or mixed format accepted")
		}
	}
	if r, e := DecodeRecorded(raw); e != nil || r.Current == nil {
		t.Fatal("current format rejected", e)
	}
	if r, e := DecodeRecorded(savedLegacy); e != nil || r.Legacy == nil {
		t.Fatal("integrated legacy rejected", e)
	}
}

func TestRetainedFactorResponseSensitivity(t *testing.T) {
	for i, name := range []string{"fatigue", "scarcity_opportunity", "slow_residue"} {
		t.Run(name, func(t *testing.T) {
			low, high := initial(t), initial(t)
			low.Factors.Variables[i] = Variable{[4]float64{.05, .05, 1, 1}, 0}
			high.Factors.Variables[i] = Variable{[4]float64{.95, .95, 1, 1}, 0}
			a, e := Response(low)
			if e != nil {
				t.Fatal(e)
			}
			b, e := Response(high)
			if e != nil {
				t.Fatal(e)
			}
			if i == ScarcityOpportunity {
				if b.Wait <= a.Wait {
					t.Fatal("scarcity/opportunity lost its wait consequence")
				}
			} else if b.Rest <= a.Rest {
				t.Fatal("retained factor lost its rest consequence")
			}
		})
	}
}

//go:embed testdata/legacy-appraisal-input.json
var savedLegacyAppraisalInput []byte

//go:embed testdata/legacy-appraisal-output.json
var savedLegacyAppraisalOutput []byte

func TestSavedLegacyAppraisalContinuation(t *testing.T) {
	recorded, e := DecodeRecorded(savedLegacy)
	if e != nil || recorded.Legacy == nil {
		t.Fatal(e)
	}
	var input dynamics.Perceived
	if e := json.Unmarshal(savedLegacyAppraisalInput, &input); e != nil {
		t.Fatal(e)
	}
	next, _, duplicate, e := dynamics.Appraise(*recorded.Legacy, input, input.LearnedAt)
	if e != nil || duplicate {
		t.Fatal("legacy appraisal failed", e)
	}
	raw, e := next.Canonical()
	if e != nil || !bytes.Equal(raw, savedLegacyAppraisalOutput) {
		t.Fatal("frozen legacy appraisal bytes/gains changed", e)
	}
	again, _, duplicate, e := dynamics.Appraise(next, input, input.LearnedAt)
	if e != nil || !duplicate {
		t.Fatal("legacy receipt replay failed", e)
	}
	raw, e = again.Canonical()
	if e != nil || !bytes.Equal(raw, savedLegacyAppraisalOutput) {
		t.Fatal("legacy duplicate changed state", e)
	}
}
