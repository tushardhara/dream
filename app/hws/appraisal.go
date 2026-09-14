package hws

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/dynamics"
	rt "github.com/tushardhara/dream/simulator/runtime"
	"github.com/tushardhara/dream/simulator/scenario"
	"sort"
)

// PerceptionSource supplies current, trusted observer-scoped read/derive rights
// and structured signals, never arbitrary model text. #12 owns the full runtime
// information service; a Perceived value alone is not authenticated evidence.
type PerceptionSource interface {
	Perceive(rt.Input) (dynamics.Perceived, error)
}
type AppraisalHandler struct{ Source PerceptionSource }
type AppraisalCheckpoint struct {
	Version int                 `json:"version"`
	Model   string              `json:"model"`
	Actors  []dynamics.State    `json:"actors"`
	Last    *dynamics.Rationale `json:"last,omitempty"`
}

func (c AppraisalCheckpoint) canonical() ([]byte, error) {
	if c.Version != 1 || c.Model != dynamics.ModelVersion || len(c.Actors) < 2 || len(c.Actors) > 24 {
		return nil, fmt.Errorf("invalid appraisal checkpoint version or actor count")
	}
	out := c
	out.Actors = append([]dynamics.State{}, c.Actors...)
	seen := map[core.ID]bool{}
	for i, s := range out.Actors {
		if seen[s.Actor] {
			return nil, fmt.Errorf("duplicate appraisal actor")
		}
		seen[s.Actor] = true
		b, err := s.Canonical()
		if err != nil {
			return nil, err
		}
		normalized, err := dynamics.Decode(b)
		if err != nil {
			return nil, err
		}
		out.Actors[i] = normalized
	}
	if out.Last != nil {
		if err := out.Last.Validate(); err != nil {
			return nil, err
		}
		matched := false
		for _, s := range out.Actors {
			for _, r := range s.Applied {
				if r.Event == out.Last.Cause && dynamics.Key(s.Actor, r.Event) == out.Last.Key {
					matched = true
				}
			}
		}
		if !matched {
			return nil, fmt.Errorf("rationale is not tied to an applied event")
		}
	}
	sort.Slice(out.Actors, func(i, j int) bool { return out.Actors[i].Actor < out.Actors[j].Actor })
	b, err := json.Marshal(out)
	if len(b) > 4096 {
		return nil, fmt.Errorf("appraisal checkpoint exceeds runtime's 4096-byte state budget")
	}
	return b, err
}
func DecodeAppraisalCheckpoint(raw string) (AppraisalCheckpoint, error) {
	if len(raw) > 4096 {
		return AppraisalCheckpoint{}, fmt.Errorf("appraisal checkpoint byte budget")
	}
	var c AppraisalCheckpoint
	d := json.NewDecoder(bytes.NewBufferString(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&c); err != nil {
		return c, err
	}
	b, err := c.canonical()
	if err != nil {
		return c, err
	}
	if string(b) != raw {
		return c, fmt.Errorf("noncanonical appraisal checkpoint")
	}
	return c, nil
}

// Transition implements #6's pure handler boundary. It seeds state exclusively
// from the pinned genesis and versioned defaults; restarts cannot replace it by
// silently changing host configuration. No live I/O adapter is included here.
func (h AppraisalHandler) Transition(current rt.State, input rt.Input, clock rt.Clock, _ *rt.Random) (rt.Output, error) {
	if err := current.Validate(); err != nil {
		return rt.Output{}, err
	}
	if h.Source == nil || clock.Now() != input.At || input.At < current.At {
		return rt.Output{}, fmt.Errorf("perception source and matching virtual clock required")
	}
	var sc scenario.Scenario
	if err := json.Unmarshal(current.Genesis.Payload, &sc); err != nil {
		return rt.Output{}, err
	}
	var checkpoint AppraisalCheckpoint
	var err error
	if current.Data != "" {
		checkpoint, err = DecodeAppraisalCheckpoint(current.Data)
		if err != nil {
			return rt.Output{}, err
		}
	} else {
		checkpoint, err = initialAppraisal(sc, current.Genesis.ScenarioHash)
		if err != nil {
			return rt.Output{}, err
		}
	}
	if len(checkpoint.Actors) != len(sc.Public.Humans) {
		return rt.Output{}, fmt.Errorf("checkpoint actor set differs from genesis")
	}
	for _, a := range sc.Public.Humans {
		found := false
		for _, s := range checkpoint.Actors {
			found = found || s.Actor == a.ID
		}
		if !found {
			return rt.Output{}, fmt.Errorf("checkpoint actor set differs from genesis")
		}
	}
	p, err := h.Source.Perceive(input)
	if err != nil {
		return rt.Output{}, err
	}
	if p.Event != input.ID || p.Actor != input.Actor {
		return rt.Output{}, fmt.Errorf("perception does not match committed input")
	}
	found := false
	for i, s := range checkpoint.Actors {
		if s.Actor == p.Actor {
			next, rationale, _, e := dynamics.Appraise(s, p, clock.Now())
			if e != nil {
				return rt.Output{}, e
			}
			checkpoint.Actors[i] = next
			checkpoint.Last = &rationale
			found = true
		} else {
			next, e := dynamics.Advance(s, clock.Now())
			if e != nil {
				return rt.Output{}, e
			}
			checkpoint.Actors[i] = next
		}
	}
	if !found {
		return rt.Output{}, fmt.Errorf("unknown perception actor")
	}
	b, err := checkpoint.canonical()
	if err != nil {
		return rt.Output{}, err
	}
	return rt.Output{Data: string(b), Events: []rt.Input{}}, nil
}

func initialAppraisal(sc scenario.Scenario, hash string) (AppraisalCheckpoint, error) {
	var checkpoint AppraisalCheckpoint
	checkpoint = AppraisalCheckpoint{Version: 1, Model: dynamics.ModelVersion, Actors: []dynamics.State{}}
	for _, a := range sc.Public.Humans {
		s, e := dynamics.New(a.ID, 0, dynamics.DefaultSubstrate())
		if e != nil {
			return AppraisalCheckpoint{}, e
		}
		s.Causes = []core.ID{core.ID("genesis:" + hash)}
		checkpoint.Actors = append(checkpoint.Actors, s)
	}
	for _, latent := range sc.Research.Latent {
		if latent.Emotion.Valence != 0 || latent.Emotion.Arousal != 0 {
			return AppraisalCheckpoint{}, fmt.Errorf("appraisal.v1 has no emotion-parameter mapping; refusing to ignore nonneutral initial emotion")
		}
		for _, drive := range latent.Drives {
			index := -1
			for i, d := range dynamics.Registry() {
				if d.ID == drive.Kind {
					index = i
				}
			}
			if index < 0 {
				return AppraisalCheckpoint{}, fmt.Errorf("initial drive is outside ticket7-subset.v1")
			}
			for i, s := range checkpoint.Actors {
				if s.Actor == latent.Actor {
					v := &s.Variables[index]
					v.Level = drive.Strength
					v.Anchor = drive.Strength
					checkpoint.Actors[i] = s
				}
			}
		}
	}
	return checkpoint, nil
}
