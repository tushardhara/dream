package drives

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/dynamics"
	"math"
	"strings"
	"testing"
)

func initial(t testing.TB) State {
	t.Helper()
	s, e := New("a", 0, DefaultSubstrate())
	if e != nil {
		t.Fatal(e)
	}
	return s
}
func observation() Observation {
	p := func(id core.ID) dynamics.Perceived {
		return dynamics.Perceived{Event: id, Actor: "a", Confidence: .8, Rights: core.Rights{Resource: id, Grants: []core.Grant{{Actor: "a", Recipient: "a", Purpose: "simulation", Operation: core.Read}, {Actor: "a", Recipient: "a", Purpose: "simulation", Operation: core.Derive}}}}
	}
	o := Observation{Event: p("event")}
	o.Event.Signals = dynamics.Signals{Effort: .5, OtherNeed: .8, Scarcity: .4, StatusThreat: .7, Opportunity: .6, Exclusion: .3}
	for i := range o.Context {
		o.Context[i] = Cue{p(core.ID(fmt.Sprintf("context:%d", i))), .5}
	}
	return o
}
func TestExactRegistryWireContract(t *testing.T) {
	want := strings.Split("acquisition comparison status_protection threat_response approach_desire effort_avoidance reward_seeking safety belonging autonomy competence care fairness reciprocity status identity_protection curiosity meaning certainty novelty attachment loss_avoidance", " ")
	r := Registry()
	if len(r) != 22 || RegistryVersion != "hws-section6.v1" || Version != 3 || ValidateRegistry() != nil {
		t.Fatal("registry/version drift")
	}
	for i, d := range r {
		if string(d.ID) != want[i] || !unit(d.Baseline) || d.Gain <= 0 || d.Gain > 1 || d.HalfLife < Hour || d.HalfLife > 168*Hour {
			t.Fatal("registry ordinal/parameters", i, d)
		}
	}
	r[0].ID = "changed"
	if Registry()[0].ID != "acquisition" {
		t.Fatal("registry mutable by caller")
	}
}
func TestNewCodecAndLegacyReplay(t *testing.T) {
	s := initial(t)
	raw, e := s.Canonical()
	if e != nil {
		t.Fatal(e)
	}
	decoded, e := DecodeRecorded(raw)
	if e != nil || decoded.Current == nil || decoded.Legacy != nil {
		t.Fatal("new dispatch", e)
	}
	old, e := dynamics.New("a", 0, dynamics.DefaultSubstrate())
	if e != nil {
		t.Fatal(e)
	}
	old, e = dynamics.Intervene(old, dynamics.Fatigue, 1, 0, "legacy-fatigue")
	if e != nil {
		t.Fatal(e)
	}
	legacy, e := old.Canonical()
	if e != nil {
		t.Fatal(e)
	}
	recorded, e := DecodeRecorded(legacy)
	if e != nil || recorded.Legacy == nil || recorded.Current != nil {
		t.Fatal("legacy dispatch", e)
	}
	again, _ := recorded.Legacy.Canonical()
	if !bytes.Equal(legacy, again) || recorded.Legacy.Variables[dynamics.Fatigue].Level != 1 {
		t.Fatal("legacy ordinal or bytes changed")
	}
	a, e := dynamics.Advance(old, 12*Hour)
	if e != nil {
		t.Fatal(e)
	}
	b, e := dynamics.Advance(*recorded.Legacy, 12*Hour)
	if e != nil {
		t.Fatal(e)
	}
	ha, _ := a.Hash()
	hb, _ := b.Hash()
	if ha != hb {
		t.Fatal("legacy resumed replay changed")
	}
	for _, bad := range [][]byte{legacy, bytes.Replace(raw, []byte(RegistryVersion), []byte(dynamics.RegistryVersion), 1), bytes.Replace(raw, []byte(RegistryHash), []byte(strings.Repeat("0", 64)), 1), append(append([]byte{}, raw...), ' '), bytes.Replace(raw, []byte(`"version":3`), []byte(`"version":3,"version":3`), 1)} {
		if _, e := Decode(bad); e == nil {
			t.Fatal("wrong registry/duplicate/noncanonical accepted")
		}
	}
	if _, e := DecodeRecorded([]byte(`{"version":99,"registry":"future"}`)); e == nil || !strings.Contains(e.Error(), "supported state versions") {
		t.Fatal("supported-version error missing")
	}
	// JSON arrays must not silently pad/drop ordinals, even with zero tail values.
	var obj map[string]json.RawMessage
	_ = json.Unmarshal(raw, &obj)
	var values []json.RawMessage
	_ = json.Unmarshal(obj["variables"], &values)
	obj["variables"], _ = json.Marshal(values[:21])
	bad, _ := json.Marshal(obj)
	if _, e := Decode(bad); e == nil {
		t.Fatal("short ordinal vector accepted")
	}
}
func TestAllContextDimensionsAndCompetingTendencies(t *testing.T) {
	s := initial(t)
	o := observation()
	baseline, r, _, e := Appraise(s, o, 0)
	if e != nil || r.Validate() != nil {
		t.Fatal(e)
	}
	active := 0
	for _, d := range r.Deltas {
		if d != 0 {
			active++
		}
	}
	if active < 15 {
		t.Fatal("drive responses missing", active)
	}
	for i := range o.Context {
		changed := o
		changed.Context[i].Value = 1
		next, _, _, e := Appraise(s, changed, 0)
		if e != nil || next.Variables == baseline.Variables {
			t.Fatal("context dimension ignored", i, e)
		}
	}
	low := s
	low.Substrate.Plasticity = 0
	n, _, _, e := Appraise(low, o, 0)
	if e != nil || n.Variables != low.Variables {
		t.Fatal("zero plasticity", e)
	}
	a, e := Intervene(s, Care, 1, 0, "care")
	if e != nil {
		t.Fatal(e)
	}
	a, e = Intervene(a, ThreatResponse, 1, 0, "threat")
	if e != nil {
		t.Fatal(e)
	}
	before, _ := Response(s)
	after, _ := Response(a)
	if after.Support <= before.Support || after.Defend <= before.Defend || after.Approach >= before.Approach {
		t.Fatal("competing tendencies collapsed")
	}
	tired, _ := Intervene(s, EffortAvoidance, 1, 0, "tired")
	next, _, _, e := Appraise(tired, o, 0)
	if e != nil || next.Variables[Care] == baseline.Variables[Care] {
		t.Fatal("state not used by appraisal")
	}
}
func TestDriveDecayPartitionAndConfidence(t *testing.T) {
	s := initial(t)
	var e error
	for i := range Count {
		s, e = Intervene(s, i, 1, 0, core.ID(fmt.Sprintf("cause:%d", i)))
		if e != nil {
			t.Fatal(e)
		}
	}
	end := 48 * Hour
	direct, e := Advance(s, end)
	if e != nil {
		t.Fatal(e)
	}
	split := s
	for i := 1; i <= 997; i++ {
		split, e = Advance(split, core.LogicalTime(int64(end)*int64(i)/997))
		if e != nil {
			t.Fatal(e)
		}
	}
	a, _ := direct.Canonical()
	b, _ := split.Canonical()
	if !bytes.Equal(a, b) {
		t.Fatal("partitioned decay changed bytes")
	}
	for i, d := range Registry() {
		want := quant(d.Baseline + (1-d.Baseline)*math.Exp2(-float64(end)/float64(d.HalfLife)))
		if direct.Variables[i].Values[0] != want || direct.Variables[i].Values[2] >= 1 {
			t.Fatal("analytic decay/confidence", i)
		}
	}
}
func TestDrivePermissionReceiptAndBounds(t *testing.T) {
	s := initial(t)
	o := observation()
	next, _, _, e := Appraise(s, o, 0)
	if e != nil {
		t.Fatal(e)
	}
	again, r, dup, e := Appraise(next, o, Hour)
	a, _ := next.Hash()
	b, _ := again.Hash()
	if e != nil || !dup || a != b || r.Validate() != nil {
		t.Fatal("duplicate not preserved", e)
	}
	changed := o
	changed.Context[History].Value = .9
	if _, _, _, e := Appraise(next, changed, Hour); e == nil {
		t.Fatal("changed context receipt accepted")
	}
	for i := range o.Context {
		for _, op := range []int{0, 1} {
			bad := o
			bad.Context[i].Evidence.Rights.Grants = append([]core.Grant{}, o.Context[i].Evidence.Rights.Grants...)
			bad.Context[i].Evidence.Rights.Grants = bad.Context[i].Evidence.Rights.Grants[op : op+1]
			if _, _, _, e := Appraise(s, bad, 0); e == nil {
				t.Fatal("missing context permission on fresh event", i)
			}
			if _, _, _, e := Appraise(next, bad, Hour); e == nil {
				t.Fatal("missing context permission on duplicate", i)
			}
		}
	}
	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), -.1, 1.1} {
		badO := o
		badO.Context[Beliefs].Value = bad
		if _, _, _, e := Appraise(s, badO, 0); e == nil {
			t.Fatal("invalid context")
		}
		state := s
		state.Variables[Care].Values[2] = bad
		if state.Validate() == nil {
			t.Fatal("invalid confidence")
		}
		if _, e := Intervene(s, Care, bad, 0, "bad"); e == nil {
			t.Fatal("invalid intervention")
		}
	}
	for i := range 26 {
		event := o
		event.Event.Event = core.ID(fmt.Sprintf("event:%d", i))
		event.Event.Rights.Resource = event.Event.Event
		s, _, _, e = Appraise(s, event, 0)
		if e != nil {
			t.Fatal(e)
		}
	}
	before, _ := s.Hash()
	event := o
	event.Event.Event = "overflow"
	event.Event.Rights.Resource = "overflow"
	if _, _, _, e = Appraise(s, event, 0); e == nil {
		t.Fatal("causal ledger evicted")
	}
	after, _ := s.Hash()
	if before != after {
		t.Fatal("failure mutated input")
	}
}
