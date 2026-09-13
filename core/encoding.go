package core

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
)

func subjectKey(s Subject) string {
	if s.Reference != nil {
		return "reference/" + string(s.Reference.Observer) + "/" + string(s.Reference.LocalID)
	}
	return "principal/" + string(s.Principal)
}
func sortGrants(g []Grant) {
	slices.SortFunc(g, func(a, b Grant) int {
		return strings.Compare(string(a.Actor)+"/"+string(a.Recipient)+"/"+string(a.Purpose)+"/"+string(a.Operation), string(b.Actor)+"/"+string(b.Recipient)+"/"+string(b.Purpose)+"/"+string(b.Operation))
	})
}
func normalize(m *Metadata) {
	m.RecordedAt = m.RecordedAt.UTC()
	slices.Sort(m.Parents)
	slices.Sort(m.Supporting)
	slices.Sort(m.Contradicting)
	if m.Parents == nil {
		m.Parents = []ID{}
	}
	if m.Supporting == nil {
		m.Supporting = []ID{}
	}
	if m.Contradicting == nil {
		m.Contradicting = []ID{}
	}
	if m.Rights.Grants == nil {
		m.Rights.Grants = []Grant{}
	}
	sortGrants(m.Rights.Grants)
	if m.Confidence == 0 {
		m.Confidence = 0
	}
	if m.Probability != nil && m.Probability.Value == 0 {
		m.Probability.Value = 0
	}
}

// Canonical encodes schema v1 with sorted set collections, normalized UTC system
// timestamps, exact integers and finite floats. Input and nested slices/pointers
// are not mutated. Logical IDs and times are preserved. Strings are exact UTF-8;
// Unicode normalization is intentionally not performed.
func Canonical(s State) ([]byte, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(s)
	if err != nil {
		return nil, err
	}
	var c State
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, err
	}
	if c.Principals == nil {
		c.Principals = []Principal{}
	}
	slices.SortFunc(c.Principals, func(a, b Principal) int { return strings.Compare(string(a.ID), string(b.ID)) })
	if c.References == nil {
		c.References = []NonparticipantReference{}
	}
	slices.SortFunc(c.References, func(a, b NonparticipantReference) int {
		return strings.Compare(string(a.Observer)+"/"+string(a.LocalID), string(b.Observer)+"/"+string(b.LocalID))
	})
	if c.Evidence == nil {
		c.Evidence = []Evidence{}
	}
	for i := range c.Evidence {
		normalize(&c.Evidence[i].Meta)
	}
	slices.SortFunc(c.Evidence, func(a, b Evidence) int { return strings.Compare(string(a.Meta.ID), string(b.Meta.ID)) })
	if c.Events == nil {
		c.Events = []Event{}
	}
	for i := range c.Events {
		normalize(&c.Events[i].Meta)
	}
	slices.SortFunc(c.Events, func(a, b Event) int { return strings.Compare(string(a.Meta.ID), string(b.Meta.ID)) })
	if c.Claims == nil {
		c.Claims = []Claim{}
	}
	for i := range c.Claims {
		normalize(&c.Claims[i].Meta)
	}
	slices.SortFunc(c.Claims, func(a, b Claim) int { return strings.Compare(string(a.Meta.ID), string(b.Meta.ID)) })
	if c.Hypotheses == nil {
		c.Hypotheses = []Hypothesis{}
	}
	for i := range c.Hypotheses {
		normalize(&c.Hypotheses[i].Meta)
	}
	slices.SortFunc(c.Hypotheses, func(a, b Hypothesis) int { return strings.Compare(string(a.Meta.ID), string(b.Meta.ID)) })
	if c.Relationships == nil {
		c.Relationships = []Relationship{}
	}
	for i := range c.Relationships {
		normalize(&c.Relationships[i].Meta)
	}
	slices.SortFunc(c.Relationships, func(a, b Relationship) int { return strings.Compare(string(a.Meta.ID), string(b.Meta.ID)) })
	if c.Groups == nil {
		c.Groups = []Group{}
	}
	for i := range c.Groups {
		normalize(&c.Groups[i].Meta)
	}
	slices.SortFunc(c.Groups, func(a, b Group) int { return strings.Compare(string(a.Meta.ID), string(b.Meta.ID)) })
	if c.Memories == nil {
		c.Memories = []Memory{}
	}
	for i := range c.Memories {
		normalize(&c.Memories[i].Meta)
	}
	slices.SortFunc(c.Memories, func(a, b Memory) int { return strings.Compare(string(a.Meta.ID), string(b.Meta.ID)) })
	if c.OpenLoops == nil {
		c.OpenLoops = []OpenLoop{}
	}
	for i := range c.OpenLoops {
		normalize(&c.OpenLoops[i].Meta)
	}
	slices.SortFunc(c.OpenLoops, func(a, b OpenLoop) int { return strings.Compare(string(a.Meta.ID), string(b.Meta.ID)) })
	if c.Intents == nil {
		c.Intents = []Intent{}
	}
	for i := range c.Intents {
		normalize(&c.Intents[i].Meta)
	}
	slices.SortFunc(c.Intents, func(a, b Intent) int { return strings.Compare(string(a.Meta.ID), string(b.Meta.ID)) })
	if c.Proposals == nil {
		c.Proposals = []Proposal{}
	}
	for i := range c.Proposals {
		normalize(&c.Proposals[i].Meta)
	}
	slices.SortFunc(c.Proposals, func(a, b Proposal) int { return strings.Compare(string(a.Meta.ID), string(b.Meta.ID)) })
	if c.Decisions == nil {
		c.Decisions = []Decision{}
	}
	for i := range c.Decisions {
		normalize(&c.Decisions[i].Meta)
	}
	slices.SortFunc(c.Decisions, func(a, b Decision) int { return strings.Compare(string(a.Meta.ID), string(b.Meta.ID)) })
	if c.Outcomes == nil {
		c.Outcomes = []Outcome{}
	}
	for i := range c.Outcomes {
		normalize(&c.Outcomes[i].Meta)
	}
	slices.SortFunc(c.Outcomes, func(a, b Outcome) int { return strings.Compare(string(a.Meta.ID), string(b.Meta.ID)) })
	for i := range c.Groups {
		slices.SortFunc(c.Groups[i].Members, func(a, b Subject) int { return strings.Compare(subjectKey(a), subjectKey(b)) })
	}
	encoded, err := json.Marshal(c)
	if err != nil {
		return nil, err
	}
	if len(encoded) > 16<<20 {
		return nil, fmt.Errorf("state exceeds 16 MiB encoding cap")
	}
	return encoded, nil
}

// Decode accepts only canonical persisted representations. Rejecting duplicate
// keys, unknown fields, alternate ordering/whitespace and versions avoids multiple
// byte representations of one record. HTTP DTO parsing belongs in transport.
func Decode(raw []byte) (State, error) {
	if len(raw) > 16<<20 {
		return State{}, fmt.Errorf("state exceeds 16 MiB decoding cap")
	}
	var s State
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&s); err != nil {
		return State{}, err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return State{}, fmt.Errorf("trailing JSON")
	}
	canonical, err := Canonical(s)
	if err != nil {
		return State{}, err
	}
	if !bytes.Equal(raw, canonical) {
		return State{}, fmt.Errorf("noncanonical encoding")
	}
	return s, nil
}

// LogicalHash omits recorded_at at every metadata node. This is NOT a persisted
// record checksum and must not be used for authorization or authenticity.
func LogicalHash(s State) ([32]byte, error) {
	raw, err := Canonical(s)
	if err != nil {
		return [32]byte{}, err
	}
	raw, err = withoutRecorded(raw)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(raw), nil
}
func withoutRecorded(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return raw, nil
	}
	switch raw[0] {
	case '{':
		var m map[string]json.RawMessage
		if err := json.Unmarshal(raw, &m); err != nil {
			return nil, err
		}
		delete(m, "recorded_at")
		for k, v := range m {
			n, err := withoutRecorded(v)
			if err != nil {
				return nil, err
			}
			m[k] = n
		}
		return json.Marshal(m)
	case '[':
		var a []json.RawMessage
		if err := json.Unmarshal(raw, &a); err != nil {
			return nil, err
		}
		for i, v := range a {
			n, err := withoutRecorded(v)
			if err != nil {
				return nil, err
			}
			a[i] = n
		}
		return json.Marshal(a)
	}
	return raw, nil
}
