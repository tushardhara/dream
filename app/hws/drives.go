package hws

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/drives"
	rt "github.com/tushardhara/dream/simulator/runtime"
	"github.com/tushardhara/dream/simulator/scenario"
	"sort"
)

// DrivePerceptionSource is a trusted host port, not authentication. It supplies
// individually permitted observer context; no raw prompt, label or global state.
type DrivePerceptionSource interface {
	PerceiveDrives(rt.Input) (drives.Observation, error)
}

// DriveAppraisalHandler explicitly selects the new registry for new runs. Legacy
// AppraisalHandler remains frozen for existing recorded six-variable checkpoints.
type DriveAppraisalHandler struct{ Source DrivePerceptionSource }
type DriveCheckpoint struct {
	Version int               `json:"version"`
	Model   string            `json:"model"`
	Actors  []drives.State    `json:"actors"`
	Last    *drives.Rationale `json:"last,omitempty"`
}

func (c DriveCheckpoint) Canonical() ([]byte, error) {
	if c.Version != drives.Version || c.Model != drives.ModelVersion || len(c.Actors) < 2 || len(c.Actors) > 24 {
		return nil, fmt.Errorf("unsupported drive checkpoint version or actor count")
	}
	out := c
	out.Actors = append([]drives.State{}, c.Actors...)
	seen := map[core.ID]bool{}
	matched := c.Last == nil
	for i, s := range out.Actors {
		if seen[s.Actor] {
			return nil, fmt.Errorf("duplicate drive actor")
		}
		seen[s.Actor] = true
		raw, e := s.Canonical()
		if e != nil {
			return nil, e
		}
		out.Actors[i], e = drives.Decode(raw)
		if e != nil {
			return nil, e
		}
		if c.Last != nil {
			for _, r := range s.Applied {
				if r.Event == c.Last.Cause && drives.Key(s.Actor, r.Event) == c.Last.Key {
					matched = true
				}
			}
		}
	}
	if !matched || c.Last != nil && c.Last.Validate() != nil {
		return nil, fmt.Errorf("drive rationale not bound to receipt")
	}
	sort.Slice(out.Actors, func(i, j int) bool { return out.Actors[i].Actor < out.Actors[j].Actor })
	raw, e := json.Marshal(out)
	if len(raw) > 4096 {
		return nil, fmt.Errorf("drive checkpoint exceeds unchanged 4096-byte runtime budget")
	}
	return raw, e
}
func DecodeDriveCheckpoint(raw string) (DriveCheckpoint, error) {
	if len(raw) > 4096 {
		return DriveCheckpoint{}, fmt.Errorf("drive checkpoint byte budget")
	}
	var c DriveCheckpoint
	d := json.NewDecoder(bytes.NewBufferString(raw))
	d.DisallowUnknownFields()
	if e := d.Decode(&c); e != nil {
		return c, e
	}
	b, e := c.Canonical()
	if e != nil {
		return c, e
	}
	if string(b) != raw {
		return c, fmt.Errorf("noncanonical drive checkpoint")
	}
	return c, nil
}
func (h DriveAppraisalHandler) Transition(current rt.State, input rt.Input, clock rt.Clock, _ *rt.Random) (rt.Output, error) {
	if e := current.Validate(); e != nil {
		return rt.Output{}, e
	}
	if h.Source == nil || clock.Now() != input.At || input.At < current.At {
		return rt.Output{}, fmt.Errorf("drive source/time required")
	}
	var sc scenario.Scenario
	if e := json.Unmarshal(current.Genesis.Payload, &sc); e != nil {
		return rt.Output{}, e
	}
	var c DriveCheckpoint
	var e error
	if current.Data != "" {
		c, e = DecodeDriveCheckpoint(current.Data)
		if e != nil {
			return rt.Output{}, e
		}
	} else {
		c = DriveCheckpoint{Version: drives.Version, Model: drives.ModelVersion, Actors: []drives.State{}}
		for _, person := range sc.Public.Humans {
			s, e := drives.New(person.ID, 0, drives.DefaultSubstrate())
			if e != nil {
				return rt.Output{}, e
			}
			s.Causes = []core.ID{core.ID("genesis:" + current.Genesis.ScenarioHash)}
			for _, latent := range sc.Research.Latent {
				if latent.Actor != person.ID {
					continue
				}
				if latent.Emotion.Valence != 0 || latent.Emotion.Arousal != 0 {
					return rt.Output{}, fmt.Errorf("new drive model has no emotion parameter mapping")
				}
				for _, v := range latent.Drives {
					index := -1
					for i, d := range drives.Registry() {
						if d.ID == v.Kind {
							index = i
						}
					}
					if index < 0 {
						return rt.Output{}, fmt.Errorf("unsupported initial drive for hws-section6.v1")
					}
					s, e = drives.Intervene(s, index, v.Strength, 0, s.Causes[0])
					if e != nil {
						return rt.Output{}, e
					}
				}
			}
			c.Actors = append(c.Actors, s)
		}
	}
	if len(c.Actors) != len(sc.Public.Humans) {
		return rt.Output{}, fmt.Errorf("drive checkpoint actor set mismatch")
	}
	for _, a := range sc.Public.Humans {
		found := false
		for _, s := range c.Actors {
			found = found || s.Actor == a.ID
		}
		if !found {
			return rt.Output{}, fmt.Errorf("drive checkpoint actor set mismatch")
		}
	}
	o, e := h.Source.PerceiveDrives(input)
	if e != nil {
		return rt.Output{}, e
	}
	if o.Event.Event != input.ID || o.Event.Actor != input.Actor || o.Event.OccurredAt != input.At {
		return rt.Output{}, fmt.Errorf("drive perception differs from runtime input")
	}
	found := false
	for i, s := range c.Actors {
		if s.Actor == input.Actor {
			next, r, _, err := drives.Appraise(s, o, clock.Now())
			if err != nil {
				return rt.Output{}, err
			}
			c.Actors[i] = next
			c.Last = &r
			found = true
		} else {
			c.Actors[i], e = drives.Advance(s, clock.Now())
			if e != nil {
				return rt.Output{}, e
			}
		}
	}
	if !found {
		return rt.Output{}, fmt.Errorf("unknown drive actor")
	}
	raw, e := c.Canonical()
	return rt.Output{Data: string(raw), Events: []rt.Input{}}, e
}
