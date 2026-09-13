package scenario

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator"
	"reflect"
	"sort"
)

const MaxBytes = 1 << 20

// Canonical sorts all v1 collections (sets), normalizes empty sets and signed
// zero, then orders schedules by (at, id). It never mutates the source.
func (s Scenario) Canonical() ([]byte, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	b, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	if len(b) > MaxBytes {
		return nil, fail("$", "scenario exceeds canonical size limit")
	}
	var clone Scenario
	if err = json.Unmarshal(b, &clone); err != nil {
		return nil, err
	}
	normalize(reflect.ValueOf(&clone).Elem())
	sort.Slice(clone.Future, func(i, j int) bool {
		a, b := clone.Future[i], clone.Future[j]
		if a.At != b.At {
			return a.At < b.At
		}
		return a.ID < b.ID
	})
	return json.Marshal(clone)
}
func normalize(v reflect.Value) {
	switch v.Kind() {
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			normalize(v.Field(i))
		}
	case reflect.Pointer:
		if !v.IsNil() {
			normalize(v.Elem())
		}
	case reflect.Float64:
		if v.Float() == 0 {
			v.SetFloat(0)
		}
	case reflect.Slice:
		if v.IsNil() {
			v.Set(reflect.MakeSlice(v.Type(), 0, 0))
		}
		for i := 0; i < v.Len(); i++ {
			normalize(v.Index(i))
		}
		sort.Slice(v.Interface(), func(i, j int) bool {
			a, _ := json.Marshal(v.Index(i).Interface())
			b, _ := json.Marshal(v.Index(j).Interface())
			return bytes.Compare(a, b) < 0
		})
	}
}
func (s Scenario) Hash() (string, error) {
	b, err := s.Canonical()
	if err != nil {
		return "", err
	}
	return digest(b), nil
}
func digest(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }

// Genesis is a versioned simulator event, not an already-applied mutable world.
// The runtime (#6) must persist it before materializing state. Its payload is
// restricted research input; never hand it or its digest to an actor/model.
type Genesis struct {
	Version      int               `json:"version"`
	Kind         string            `json:"kind"`
	World        simulator.WorldID `json:"world"`
	At           core.LogicalTime  `json:"at"`
	ScenarioHash string            `json:"scenario_hash"`
	Payload      json.RawMessage   `json:"payload"`
}

func (s Scenario) Genesis(e Engine) (Genesis, error) {
	if err := s.CheckExecution(e); err != nil {
		return Genesis{}, err
	}
	b, err := s.Canonical()
	if err != nil {
		return Genesis{}, err
	}
	return Genesis{Version: 1, Kind: "scenario.genesis", World: s.World.ID, At: 0, ScenarioHash: digest(b), Payload: b}, nil
}
func (g Genesis) Validate(e Engine) error {
	if g.Version != 1 || g.Kind != "scenario.genesis" || g.At != 0 || len(g.Payload) > MaxBytes {
		return fail("$", "invalid genesis envelope")
	}
	var s Scenario
	d := json.NewDecoder(bytes.NewReader(g.Payload))
	d.DisallowUnknownFields()
	if err := d.Decode(&s); err != nil {
		return fail("$.payload", "invalid scenario payload")
	}
	if err := s.CheckExecution(e); err != nil {
		return err
	}
	b, err := s.Canonical()
	if err != nil {
		return err
	}
	if s.World.ID != g.World || !bytes.Equal(b, g.Payload) || digest(b) != g.ScenarioHash {
		return fail("$.payload", "genesis world, canonical payload or hash mismatch")
	}
	return nil
}

// ActorView is an allowlist projection at genesis. The caller must authenticate
// actor selection. It deliberately carries no scenario reference/hash, seed,
// horizon, future schedule, labels, latent state or other actor's private state.
type ActorView struct {
	Actor         core.ID        `json:"actor"`
	Humans        []Human        `json:"humans"`
	Groups        []Group        `json:"groups"`
	Resources     []Resource     `json:"resources"`
	Facts         []Fact         `json:"facts"`
	Memories      []Memory       `json:"memories"`
	Relationships []Relationship `json:"relationships"`
}

func (s Scenario) View(actor core.ID) (ActorView, error) {
	b, err := s.Canonical()
	if err != nil {
		return ActorView{}, err
	}
	var c Scenario
	if err = json.Unmarshal(b, &c); err != nil {
		return ActorView{}, err
	}
	for _, a := range c.Actors {
		if a.ID != actor {
			continue
		}
		v := ActorView{Actor: actor, Humans: c.Public.Humans, Groups: c.Public.Groups, Resources: c.Public.Resources, Facts: []Fact{}, Memories: a.Memories, Relationships: a.Relationships}
		known := map[core.ID]bool{}
		for _, k := range a.Knowledge {
			known[k.Record] = true
		}
		for _, f := range append(c.Public.Facts, a.Facts...) {
			if known[f.ID] {
				v.Facts = append(v.Facts, f)
			}
		}
		return v, nil
	}
	return ActorView{}, fail("$.actor", "unknown actor")
}
