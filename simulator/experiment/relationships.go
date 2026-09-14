package experiment

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"reflect"

	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/behavior"
	"github.com/tushardhara/dream/simulator/internal/reference"
)

const RelationshipExperimentVersion = "relationship-reference-experiment.v1"

// Shared with #42's controlled-comparison tests. This is synthetic input data,
// not evaluator labels, expected results, human observations or role weights.
var relationshipFixture = reference.Relationships()

type NamedRelationship struct {
	Name    string                   `json:"name"`
	Context core.RelationshipContext `json:"context"`
}
type RelationshipFixture struct {
	Version   string                   `json:"version"`
	At        core.LogicalTime         `json:"at"`
	Actor     behavior.ActionActor     `json:"actor"`
	Situation behavior.ActionSituation `json:"situation"`
	Profiles  []NamedRelationship      `json:"profiles"`
}

func digestValue(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func RelationshipFixtureHash() string {
	h := sha256.Sum256(relationshipFixture)
	return hex.EncodeToString(h[:])
}

// Every call detaches all slices/maps before the engine can consume them.
func ReadRelationshipFixture() (RelationshipFixture, error) {
	var f RelationshipFixture
	d := json.NewDecoder(bytes.NewReader(relationshipFixture))
	d.DisallowUnknownFields()
	if e := d.Decode(&f); e != nil {
		return f, e
	}
	var extra any
	if d.Decode(&extra) != io.EOF || f.Version != "relationship-comparisons.v1" || f.At != 5 || f.Actor.Validate() != nil || f.Actor.Drives.Actor != "a" || len(f.Profiles) != 7 {
		return f, fmt.Errorf("invalid reference fixture")
	}
	for _, p := range f.Profiles {
		if p.Context.Validate("a", "b") != nil {
			return f, fmt.Errorf("invalid reference relationship")
		}
	}
	return f, nil
}

// The child receives only a frozen public fixture identity and seeds. No field
// accepts labels, evaluator scores, private files, network addresses or policy.
type RelationshipProbeRequest struct {
	Version     string   `json:"version"`
	FixtureHash string   `json:"fixture_hash"`
	Seeds       []uint64 `json:"seeds"`
}

func (r RelationshipProbeRequest) Validate() error {
	if r.Version != RelationshipExperimentVersion || r.FixtureHash != RelationshipFixtureHash() || len(r.Seeds) < 2 || len(r.Seeds) > 16 {
		return fmt.Errorf("invalid relationship experiment plan")
	}
	seen := map[uint64]bool{}
	for _, s := range r.Seeds {
		if seen[s] {
			return fmt.Errorf("duplicate experiment seed")
		}
		seen[s] = true
	}
	return nil
}
func (r RelationshipProbeRequest) Hash() string { return digestValue(r) }

// Full-width deterministic draws avoid interpreting tiny integer seeds directly
// as near-zero uniform draws. Each controlled case shares its seed's draw.
func RelationshipDraw(seed uint64) uint64 {
	raw, _ := json.Marshal(struct {
		Version string
		Seed    uint64
	}{RelationshipExperimentVersion, seed})
	h := sha256.Sum256(raw)
	return binary.BigEndian.Uint64(h[:8])
}

type RelationshipTrial struct {
	Case             string                  `json:"case"`
	Seed             uint64                  `json:"seed"`
	InputHash        string                  `json:"input_hash"`
	BeforeDrivesHash string                  `json:"before_drives_hash"`
	AfterDrivesHash  string                  `json:"after_drives_hash"`
	AfterStateHash   string                  `json:"after_state_hash"`
	Decision         behavior.ActionDecision `json:"decision"`
}
type RelationshipProbe struct {
	Version     string                   `json:"version"`
	Request     RelationshipProbeRequest `json:"request"`
	RequestHash string                   `json:"request_hash"`
	Trials      []RelationshipTrial      `json:"trials"`
}

type relationalCase struct {
	Name    string
	Context *core.RelationshipContext
	Focus   bool
}

func relationshipCases(f RelationshipFixture) []relationalCase {
	cases := []relationalCase{}
	for _, p := range f.Profiles {
		r := p.Context
		cases = append(cases, relationalCase{p.Name, &r, true})
	}
	for _, role := range []core.ID{"sibling", "friend", "acquaintance", "manager"} {
		r := f.Profiles[0].Context
		r.Types = []core.ID{role}
		cases = append(cases, relationalCase{"label_" + string(role), &r, true})
	}
	for _, kind := range []core.ID{"expectation", "prior_outcome", "stress"} {
		raw, _ := json.Marshal(f.Profiles[4].Context)
		var r core.RelationshipContext
		_ = json.Unmarshal(raw, &r)
		found := false
		for i := range r.Measures {
			if r.Measures[i].Kind == kind {
				r.Measures[i].Value = -.8
				found = true
			}
		}
		if !found {
			r.Measures = append(r.Measures, core.RelationshipMeasure{Kind: kind, Value: .9, Confidence: 1, Source: "e"})
		}
		cases = append(cases, relationalCase{"ablate_" + string(kind), &r, true})
	}
	r := f.Profiles[4].Context
	cases = append(cases, relationalCase{"without_appraisal", &r, false}, relationalCase{"unknown", nil, true})
	return cases
}
func inputDigest(f RelationshipFixture, c relationalCase) string {
	return digestValue(struct {
		Actor     behavior.ActionActor
		Situation behavior.ActionSituation
		At        core.LogicalTime
		Context   *core.RelationshipContext
		Focus     bool
	}{f.Actor, f.Situation, f.At, c.Context, c.Focus})
}
func RelationshipCaseNames() []string {
	f, e := ReadRelationshipFixture()
	if e != nil {
		return nil
	}
	names := []string{}
	for _, c := range relationshipCases(f) {
		names = append(names, c.Name)
	}
	return names
}

// GenerateRelationshipProbe performs one bounded action/state transition per
// case. It makes no claim about long-run worlds, human adequacy or counterfactual
// runtime branching. Appraisal and actions use the same #40–#42 engine as HWS.
func GenerateRelationshipProbe(ctx context.Context, r RelationshipProbeRequest) (RelationshipProbe, error) {
	out := RelationshipProbe{Version: RelationshipExperimentVersion, Request: r, RequestHash: r.Hash()}
	out.Request.Seeds = append([]uint64{}, r.Seeds...)
	if e := r.Validate(); e != nil {
		return out, e
	}
	for _, seed := range r.Seeds {
		for index := range RelationshipCaseNames() {
			if e := ctx.Err(); e != nil {
				return out, e
			}
			f, e := ReadRelationshipFixture()
			if e != nil {
				return out, e
			}
			c := relationshipCases(f)[index]
			input := inputDigest(f, c)
			before := digestValue(f.Actor.Drives.Variables)
			s := f.Situation
			if c.Context != nil {
				s, e = behavior.ApplyRelationship(s, *c.Context, "a", f.At, c.Focus)
				if e != nil {
					return out, e
				}
			}
			a, d, e := behavior.ChooseAction(f.Actor, s, f.At, RelationshipDraw(seed))
			if e != nil {
				return out, e
			}
			out.Trials = append(out.Trials, RelationshipTrial{c.Name, seed, input, before, digestValue(a.Drives.Variables), digestValue(a), d})
		}
	}
	return out, out.Validate(r)
}

func (p RelationshipProbe) Validate(expected RelationshipProbeRequest) error {
	if expected.Validate() != nil || p.Version != RelationshipExperimentVersion || !reflect.DeepEqual(p.Request, expected) || p.RequestHash != expected.Hash() {
		return fmt.Errorf("relationship evidence request binding")
	}
	f, e := ReadRelationshipFixture()
	if e != nil {
		return e
	}
	cases := relationshipCases(f)
	if len(p.Trials) != len(cases)*len(expected.Seeds) {
		return fmt.Errorf("incomplete relationship trials")
	}
	validHash := func(s string) bool {
		b, e := hex.DecodeString(s)
		return e == nil && len(b) == 32 && hex.EncodeToString(b) == s
	}
	for i, t := range p.Trials {
		c := cases[i%len(cases)]
		seed := expected.Seeds[i/len(cases)]
		if t.Case != c.Name || t.Seed != seed || t.InputHash != inputDigest(f, c) || t.BeforeDrivesHash != digestValue(f.Actor.Drives.Variables) || !validHash(t.AfterDrivesHash) || !validHash(t.AfterStateHash) || t.Decision.Validate() != nil || t.Decision.Actor != "a" || t.Decision.Event != "e" || t.Decision.At != f.At || t.Decision.Draw != RelationshipDraw(seed) || t.Decision.Operational {
			return fmt.Errorf("invalid controlled relationship trial")
		}
	}
	return nil
}
