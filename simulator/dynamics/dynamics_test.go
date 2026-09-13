package dynamics

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/tushardhara/dream/core"
	"math"
	"testing"
)

func state(t testing.TB) State {
	t.Helper()
	s, err := New("a", 0, DefaultSubstrate())
	if err != nil {
		t.Fatal(err)
	}
	return s
}
func perceived(id core.ID) Perceived {
	return Perceived{Event: id, Actor: "a", Confidence: .8, Signals: Signals{Effort: .4, OtherNeed: .8, StatusThreat: .5, Exclusion: .4}, Rights: core.Rights{Resource: id, Grants: []core.Grant{{Actor: "a", Recipient: "a", Purpose: "simulation", Operation: core.Read}, {Actor: "a", Recipient: "a", Purpose: "simulation", Operation: core.Derive}}}}
}
func TestDecaySubdivisionAndResidue(t *testing.T) {
	s := state(t)
	var err error
	s, err = Intervene(s, Fatigue, 1, 0, "fatigue-intervention")
	if err != nil {
		t.Fatal(err)
	}
	s, err = Intervene(s, SlowResidue, 1, 0, "residue-intervention")
	if err != nil {
		t.Fatal(err)
	}
	end := 48 * Hour
	direct, err := Advance(s, end)
	if err != nil {
		t.Fatal(err)
	}
	split := s
	for i := 1; i <= 997; i++ {
		split, err = Advance(split, core.LogicalTime(int64(end)*int64(i)/997))
		if err != nil {
			t.Fatal(err)
		}
	}
	a, _ := direct.Canonical()
	b, _ := split.Canonical()
	if !bytes.Equal(a, b) {
		t.Fatal("decay depends on subdivision")
	}
	expected := quant(.2 + .8*math.Exp2(-6))
	if math.Abs(direct.Variables[Fatigue].Level-expected) > 1e-9 {
		t.Fatal("analytic decay mismatch")
	}
	if direct.Variables[SlowResidue].Level <= direct.Variables[Fatigue].Level {
		t.Fatal("slow residue did not persist")
	}
	if direct.Substrate != s.Substrate {
		t.Fatal("stable substrate mutated")
	}
	if direct.Variables[Fatigue].Confidence >= s.Variables[Fatigue].Confidence {
		t.Fatal("stale confidence failed to decay")
	}
}
func TestPlasticityBoundsAndCompoundInteraction(t *testing.T) {
	s := state(t)
	p := perceived("e")
	low := s
	low.Substrate.Plasticity = 0
	unchanged, _, _, err := Appraise(low, p, 0)
	if err != nil {
		t.Fatal(err)
	}
	if unchanged.Variables != low.Variables {
		t.Fatal("zero plasticity changed variables")
	}
	high := s
	high.Substrate.Plasticity = 1
	next, r, _, err := Appraise(high, p, 0)
	if err != nil {
		t.Fatal(err)
	}
	if next.Variables == high.Variables || next.Substrate != high.Substrate {
		t.Fatal("plasticity or stable substrate")
	}
	if len(r.Codes) > 7 || r.Stage != ModelVersion || r.Cause != "e" {
		t.Fatal("rationale ownership")
	}
	threat := p
	threat.Signals = Signals{StatusThreat: .5}
	exclusion := p
	exclusion.Signals = Signals{Exclusion: .5}
	both := p
	both.Signals = Signals{StatusThreat: .5, Exclusion: .5}
	a, _, _, _ := Appraise(s, threat, 0)
	b, _, _, _ := Appraise(s, exclusion, 0)
	c, _, _, _ := Appraise(s, both, 0)
	linear := a.Variables[SlowResidue].Level + b.Variables[SlowResidue].Level - 2*s.Variables[SlowResidue].Level
	compound := c.Variables[SlowResidue].Level - s.Variables[SlowResidue].Level
	if compound <= linear {
		t.Fatal("compound interaction missing")
	}
	saturated, err := Intervene(high, Fatigue, .999, 0, "saturate")
	if err != nil {
		t.Fatal(err)
	}
	p.Signals = Signals{Effort: 1, Scarcity: 1}
	saturated, _, _, err = Appraise(saturated, p, 0)
	if err != nil || saturated.Variables[Fatigue].Level != 1 {
		t.Fatal("upper saturation", err)
	}
	p = perceived("recovery")
	p.Signals = Signals{Rest: 1}
	saturated, err = Intervene(high, Fatigue, .001, 0, "low")
	if err != nil {
		t.Fatal(err)
	}
	saturated, _, _, err = Appraise(saturated, p, 0)
	if err != nil || saturated.Variables[Fatigue].Level != 0 {
		t.Fatal("lower saturation", err)
	}
}
func TestPermissionAndStageIdempotency(t *testing.T) {
	s := state(t)
	p := perceived("e")
	next, r, dup, err := Appraise(s, p, 0)
	if err != nil || dup {
		t.Fatal(err)
	}
	again, _, dup, err := Appraise(next, p, Hour)
	if err != nil || !dup {
		t.Fatal("duplicate stage", err)
	}
	a, _ := next.Hash()
	b, _ := again.Hash()
	if a != b || r.Key != Key("a", "e") {
		t.Fatal("double appraisal")
	}
	p.Signals.Effort = .5
	if _, _, _, err = Appraise(next, p, Hour); err == nil {
		t.Fatal("changed duplicate accepted")
	}
	for name, mutate := range map[string]func(*Perceived){"foreign": func(p *Perceived) { p.Actor = "b" }, "no read": func(p *Perceived) { p.Rights.Grants = p.Rights.Grants[1:] }, "no derive": func(p *Perceived) { p.Rights.Grants = p.Rights.Grants[:1] }, "revoked": func(p *Perceived) { p.Rights.Revoked = true }, "future learned": func(p *Perceived) { p.LearnedAt = 1 }, "future occurred": func(p *Perceived) { p.OccurredAt = 1 }, "resource": func(p *Perceived) { p.Rights.Resource = "foreign" }, "nonfinite": func(p *Perceived) { p.Signals.Effort = math.NaN() }, "infinite": func(p *Perceived) { p.Confidence = core.Confidence(math.Inf(1)) }} {
		t.Run(name, func(t *testing.T) {
			p := perceived("e")
			mutate(&p)
			if _, _, _, err := Appraise(s, p, 0); err == nil {
				t.Fatal("invalid perception accepted")
			}
		})
	}
	p = perceived("e")
	p.Rights.Revoked = true
	if _, _, _, err = Appraise(next, p, Hour); err == nil {
		t.Fatal("duplicate bypassed current revocation")
	}
}
func TestInterventionChangesCompetingTendency(t *testing.T) {
	s := state(t)
	before, err := Response(s)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := Intervene(s, Fatigue, 1, 0, "research-intervention")
	if err != nil {
		t.Fatal(err)
	}
	after, err := Response(changed)
	if err != nil {
		t.Fatal(err)
	}
	if after.Rest <= before.Rest || after.Support >= before.Support || after.Wait <= before.Wait {
		t.Fatal("intervention did not change reference response")
	}
	if changed.Substrate != s.Substrate {
		t.Fatal("intervention changed stable identity/substrate")
	}
}
func TestCodecAndCausalLedger(t *testing.T) {
	s := state(t)
	for i := 0; i < MaxReceipts; i++ {
		var err error
		s, _, _, err = Appraise(s, perceived(core.ID(fmt.Sprintf("e-%02d", i))), 0)
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(s.Causes) != MaxReceipts {
		t.Fatal("causal history dropped")
	}
	if _, _, _, err := Appraise(s, perceived("overflow"), 0); err == nil {
		t.Fatal("ledger evicted to allow replay")
	}
	b, err := s.Canonical()
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := Decode(b)
	if err != nil {
		t.Fatal(err)
	}
	h, _ := s.Hash()
	h2, _ := decoded.Hash()
	if h != h2 {
		t.Fatal("hash roundtrip")
	}
	for _, bad := range [][]byte{append(append([]byte{}, b...), ' '), []byte(`{"version":2}`), bytes.Replace(b, []byte(`"version":1`), []byte(`"version":1,"version":1`), 1), bytes.Repeat([]byte("x"), MaxBytes+1)} {
		if _, err := Decode(bad); err == nil {
			t.Fatal("noncanonical state accepted")
		}
	}
	original, _ := json.Marshal(s)
	_, _ = s.Canonical()
	afterCanonical, _ := json.Marshal(s)
	if !bytes.Equal(original, afterCanonical) {
		t.Fatal("canonical mutated source")
	}
	s.Causes[0], s.Causes[1] = s.Causes[1], s.Causes[0]
	permuted, _ := s.Canonical()
	if !bytes.Equal(b, permuted) {
		t.Fatal("causal order affects hash")
	}
}
func TestNonfiniteStatesAndRegistry(t *testing.T) {
	r := Registry()
	r[0].ID = "changed"
	if Registry()[0].ID != "fatigue" || RegistryCompleteness == "" {
		t.Fatal("registry mutation or false completeness")
	}
	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1), -1, 2} {
		s := state(t)
		s.Substrate.Plasticity = bad
		if s.Validate() == nil {
			t.Fatal("nonfinite/out-of-range state")
		}
		if _, err := Intervene(state(t), Fatigue, bad, 0, "x"); err == nil {
			t.Fatal("invalid intervention")
		}
	}
	s := state(t)
	s.Model = "future"
	if s.Validate() == nil {
		t.Fatal("unpinned model")
	}
}
func FuzzDecayAndAppraisal(f *testing.F) {
	f.Add(uint32(25), uint32(50), int64(Hour))
	f.Fuzz(func(t *testing.T, a, b uint32, elapsed int64) {
		if elapsed < 0 {
			return
		}
		s := state(t)
		p := perceived("e")
		p.Signals.Effort = float64(a%101) / 100
		p.Signals.Exclusion = float64(b%101) / 100
		s, _, _, err := Appraise(s, p, 0)
		if err != nil {
			t.Fatal(err)
		}
		direct, err := Advance(s, core.LogicalTime(elapsed))
		if err != nil {
			t.Fatal(err)
		}
		half, err := Advance(s, core.LogicalTime(elapsed/2))
		if err != nil {
			t.Fatal(err)
		}
		split, err := Advance(half, core.LogicalTime(elapsed))
		if err != nil {
			t.Fatal(err)
		}
		x, _ := direct.Hash()
		y, _ := split.Hash()
		if x != y {
			t.Fatal("subdivision drift")
		}
		encoded, err := direct.Canonical()
		if err != nil {
			t.Fatal(err)
		}
		if _, err = Decode(encoded); err != nil {
			t.Fatal(err)
		}
	})
}

func TestPerceptionDigestNormalizesSetsAndZero(t *testing.T) {
	s := state(t)
	p := perceived("e")
	next, _, _, err := Appraise(s, p, 0)
	if err != nil {
		t.Fatal(err)
	}
	p.Signals.Rest = math.Copysign(0, -1)
	p.Rights.Grants[0], p.Rights.Grants[1] = p.Rights.Grants[1], p.Rights.Grants[0]
	_, _, dup, err := Appraise(next, p, 0)
	if err != nil || !dup {
		t.Fatal("semantic permission ordering/zero changed digest", err)
	}
}

func TestReferenceTransitionGolden(t *testing.T) {
	s, r, _, err := Appraise(state(t), perceived("e"), 0)
	if err != nil {
		t.Fatal(err)
	}
	// Independently calculated from the ADR's equations and gain=.55*.5*.8=.22.
	want := [Count]float64{.222, .30132, .53828, .3275, .528875, .107392}
	for i, v := range s.Variables {
		if v.Level != want[i] || v.Confidence != core.Confidence(.566) {
			t.Fatalf("v1 reference vector changed for %s: %+v", Registry()[i].ID, v)
		}
	}
	if err = r.Validate(); err != nil {
		t.Fatal(err)
	}
}
