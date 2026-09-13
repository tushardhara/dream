package dynamics

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
)

const MaxBytes = 64 << 10

// Canonical validates and normalizes set order/signed zero without mutating the
// source. The pinned struct encoding and registry order are part of version 1.
func (s State) Canonical() ([]byte, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	out := clone(s)
	sort.Slice(out.Causes, func(i, j int) bool { return out.Causes[i] < out.Causes[j] })
	sort.Slice(out.Applied, func(i, j int) bool { return out.Applied[i].Event < out.Applied[j].Event })
	for i := range out.Variables {
		v := &out.Variables[i]
		if v.Level == 0 {
			v.Level = 0
		}
		if v.Anchor == 0 {
			v.Anchor = 0
		}
		if v.Confidence == 0 {
			v.Confidence = 0
		}
		if v.AnchorConfidence == 0 {
			v.AnchorConfidence = 0
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
	b, err := json.Marshal(out)
	if len(b) > MaxBytes {
		return nil, fmt.Errorf("synthetic state byte budget")
	}
	return b, err
}
func Decode(b []byte) (State, error) {
	if len(b) > MaxBytes {
		return State{}, fmt.Errorf("synthetic state byte budget")
	}
	var s State
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(&s); err != nil {
		return State{}, err
	}
	canonical, err := s.Canonical()
	if err != nil {
		return State{}, err
	}
	if !bytes.Equal(b, canonical) {
		return State{}, fmt.Errorf("noncanonical or duplicated state encoding")
	}
	return s, nil
}
func (s State) Hash() (string, error) {
	b, err := s.Canonical()
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}
