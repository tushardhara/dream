package drives

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/tushardhara/dream/simulator/dynamics"
	"sort"
)

func validHash(s string) bool {
	b, e := hex.DecodeString(s)
	return e == nil && len(b) == 32 && hex.EncodeToString(b) == s
}
func (s State) Canonical() ([]byte, error) {
	if e := s.Validate(); e != nil {
		return nil, e
	}
	out := clone(s)
	sort.Slice(out.Causes, func(i, j int) bool { return out.Causes[i] < out.Causes[j] })
	sort.Slice(out.Applied, func(i, j int) bool { return out.Applied[i].Event < out.Applied[j].Event })
	for i := range out.Variables {
		for j, v := range out.Variables[i].Values {
			if v == 0 {
				out.Variables[i].Values[j] = 0
			}
		}
		if out.Substrate.Baseline[i] == 0 {
			out.Substrate.Baseline[i] = 0
		}
	}
	if out.Substrate.Reactivity == 0 {
		out.Substrate.Reactivity = 0
	}
	if out.Substrate.Plasticity == 0 {
		out.Substrate.Plasticity = 0
	}
	raw, e := json.Marshal(out)
	if len(raw) > MaxBytes {
		return nil, fmt.Errorf("drive state byte budget")
	}
	return raw, e
}
func Decode(raw []byte) (State, error) {
	if len(raw) > MaxBytes {
		return State{}, fmt.Errorf("drive state byte budget")
	}
	var s State
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if e := d.Decode(&s); e != nil {
		return s, e
	}
	canonical, e := s.Canonical()
	if e != nil {
		return State{}, e
	}
	if !bytes.Equal(raw, canonical) {
		return State{}, fmt.Errorf("noncanonical drive encoding, duplicate key or incorrect ordinal count")
	}
	return s, nil
}
func (s State) Hash() (string, error) {
	raw, e := s.Canonical()
	if e != nil {
		return "", e
	}
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:]), nil
}

// RecordedState never maps old ordinals onto new drives. Old fatigue/residue are
// not aliases for acquisition/loss avoidance. Continue old replay with the frozen
// dynamics implementation; starting the new model requires an explicit new run.
type RecordedState struct {
	Legacy  *dynamics.State
	Current *State
}

func DecodeRecorded(raw []byte) (RecordedState, error) {
	if len(raw) > MaxBytes {
		return RecordedState{}, fmt.Errorf("recorded state byte budget")
	}
	var header struct {
		Version  int    `json:"version"`
		Registry string `json:"registry"`
	}
	if json.Unmarshal(raw, &header) != nil {
		return RecordedState{}, fmt.Errorf("invalid recorded state")
	}
	if header.Version == dynamics.Version && header.Registry == dynamics.RegistryVersion {
		old, e := dynamics.Decode(raw)
		if e != nil {
			return RecordedState{}, e
		}
		return RecordedState{Legacy: &old}, nil
	}
	if header.Version == Version && header.Registry == RegistryVersion {
		s, e := Decode(raw)
		if e != nil {
			return RecordedState{}, e
		}
		return RecordedState{Current: &s}, nil
	}
	return RecordedState{}, fmt.Errorf("supported state versions: 1/ticket7-subset.v1 or 2/hws-section6.v1; no implicit ordinal migration")
}
