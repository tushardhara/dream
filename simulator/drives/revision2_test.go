package drives

import (
	"bytes"
	_ "embed"
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
