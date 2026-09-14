package hws

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/behavior"
	"github.com/tushardhara/dream/simulator/drives"
	"github.com/tushardhara/dream/simulator/dynamics"
	rt "github.com/tushardhara/dream/simulator/runtime"
	"github.com/tushardhara/dream/simulator/scenario"
)

const actionCognitivePrefix = "cognitive.v2:"

// ActionFrame contains trusted affordances. The service supplies Sources and
// Disclosure from current capabilities; a planner cannot supply either grant.
type ActionFrame struct {
	Relationships []core.RelationshipContext
	Focus         core.ID
	Offers        []behavior.ActionOffer
	Contexts      []behavior.DisclosureContext
	DriveContext  *[drives.ContextCount]drives.Cue
	Sources       []core.ID
	Disclosure    *behavior.DisclosureGrant
}
type ActionCheckpoint struct {
	Version      int                      `json:"version"`
	Policy       string                   `json:"policy"`
	RegistryHash string                   `json:"registry_hash"`
	ModelHash    string                   `json:"model_hash,omitempty"`
	Actors       []behavior.ActionActor   `json:"actors"`
	Outcomes     []behavior.ActionOutcome `json:"outcomes"`
	Commitments  []behavior.Commitment    `json:"commitments"`
	Last         *behavior.ActionDecision `json:"last,omitempty"`
}

func (c ActionCheckpoint) validate() error {
	if c.Version != 2 || c.Policy != behavior.ActionPolicy || c.RegistryHash != behavior.ActionRegistryHash() || len(c.Actors) < 2 || len(c.Actors) > 4 || len(c.Outcomes) > behavior.MaxOutcomes || len(c.Commitments) > 16 {
		return fmt.Errorf("action checkpoint version/size")
	}
	actors := map[core.ID]bool{}
	for _, a := range c.Actors {
		if a.Validate() != nil || actors[a.Drives.Actor] {
			return fmt.Errorf("invalid action checkpoint actor")
		}
		actors[a.Drives.Actor] = true
	}
	ids := map[core.ID]bool{}
	for _, o := range c.Outcomes {
		if o.Validate() != nil || !actors[o.Outcome.Observer] || o.Outcome.Other != "" && !actors[o.Outcome.Other] || ids[o.Outcome.Decision] {
			return fmt.Errorf("invalid checkpoint outcome")
		}
		ids[o.Outcome.Decision] = true
	}
	ids = map[core.ID]bool{}
	for _, p := range c.Commitments {
		if p.Validate() != nil || !actors[p.Actor] || !actors[p.Recipient] || ids[p.ID] {
			return fmt.Errorf("invalid checkpoint commitment")
		}
		ids[p.ID] = true
	}
	if c.Last != nil {
		digest, e := hex.DecodeString(c.ModelHash)
		if e != nil || len(digest) != 32 || c.Last.Validate() != nil || !actors[c.Last.Actor] || c.Last.Stages[11] != "done" {
			return fmt.Errorf("invalid committed action")
		}
		found := false
		for _, a := range c.Actors {
			if a.Drives.Actor == c.Last.Actor {
				for _, r := range a.Drives.Applied {
					found = found || r.Event == c.Last.Event
				}
			}
		}
		if !found {
			return fmt.Errorf("decision has no appraisal receipt")
		}
	}
	return nil
}
func (c ActionCheckpoint) Encode() (string, error) {
	if e := c.validate(); e != nil {
		return "", e
	}
	raw, e := json.Marshal(c)
	if e != nil || len(raw) > 65536 {
		return "", fmt.Errorf("expanded action checkpoint budget")
	}
	var b bytes.Buffer
	z, _ := zlib.NewWriterLevel(&b, zlib.BestCompression)
	if _, e = z.Write(raw); e != nil {
		return "", e
	}
	if e = z.Close(); e != nil {
		return "", e
	}
	out := actionCognitivePrefix + base64.RawStdEncoding.EncodeToString(b.Bytes())
	if len(out) > 4096 {
		return "", fmt.Errorf("action checkpoint exceeds unchanged4096-byte runtime limit")
	}
	return out, nil
}
func DecodeActionCheckpoint(s string) (ActionCheckpoint, error) {
	var c ActionCheckpoint
	if len(s) > 4096 || !strings.HasPrefix(s, actionCognitivePrefix) {
		return c, fmt.Errorf("unknown action checkpoint")
	}
	b, e := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(s, actionCognitivePrefix))
	if e != nil {
		return c, e
	}
	r, e := zlib.NewReader(bytes.NewReader(b))
	if e != nil {
		return c, e
	}
	defer r.Close()
	raw, e := io.ReadAll(io.LimitReader(r, 65537))
	if e != nil || len(raw) > 65536 {
		return c, fmt.Errorf("expanded action checkpoint limit")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if e = d.Decode(&c); e != nil {
		return c, e
	}
	if d.Decode(new(any)) != io.EOF {
		return c, fmt.Errorf("trailing checkpoint")
	}
	canonical, e := c.Encode()
	if e != nil || canonical != s {
		return c, fmt.Errorf("noncanonical action checkpoint")
	}
	return c, nil
}

type deliveredAction struct {
	Version   int           `json:"version"`
	Decision  core.ID       `json:"decision"`
	Sender    core.ID       `json:"sender"`
	Recipient core.ID       `json:"recipient"`
	Kind      behavior.Kind `json:"kind"`
	Text      string        `json:"text,omitempty"`
	Sources   []core.ID     `json:"sources,omitempty"`
}

func (h CognitiveHandler) transitionActions(current rt.State, input rt.Input, clock rt.Clock, rng *rt.Random) (rt.Output, error) {
	if current.Validate() != nil || h.Source == nil || rng == nil || clock.Now() != input.At || input.Kind != "observation" {
		return rt.Output{}, fmt.Errorf("action transition boundary")
	}
	var sc scenario.Scenario
	if json.Unmarshal(current.Genesis.Payload, &sc) != nil {
		return rt.Output{}, fmt.Errorf("invalid action genesis")
	}
	c := ActionCheckpoint{}
	var e error
	if current.Data != "" {
		c, e = DecodeActionCheckpoint(current.Data)
		if e != nil {
			return rt.Output{}, e
		}
	} else {
		c = ActionCheckpoint{Version: 2, Policy: behavior.ActionPolicy, RegistryHash: behavior.ActionRegistryHash(), Actors: []behavior.ActionActor{}, Outcomes: []behavior.ActionOutcome{}, Commitments: []behavior.Commitment{}}
		for _, human := range sc.Public.Humans {
			a, err := behavior.NewActionActor(human.ID, 0)
			if err != nil {
				return rt.Output{}, err
			}
			a.Drives.Causes = []core.ID{core.ID("genesis:" + current.Genesis.ScenarioHash)}
			for _, latent := range sc.Research.Latent {
				if latent.Actor != human.ID {
					continue
				}
				if latent.Emotion.Valence != 0 || latent.Emotion.Arousal != 0 {
					return rt.Output{}, fmt.Errorf("new action run requires explicit new-model emotion semantics")
				}
				for _, v := range latent.Drives {
					found := false
					for i, d := range drives.Registry() {
						if d.ID == v.Kind {
							a.Drives, err = drives.Intervene(a.Drives, i, v.Strength, 0, a.Drives.Causes[0])
							if err != nil {
								return rt.Output{}, err
							}
							found = true
						}
					}
					if !found {
						return rt.Output{}, fmt.Errorf("legacy drive identifier cannot silently initialize new cognition")
					}
				}
			}
			c.Actors = append(c.Actors, a)
		}
	}
	if c.validate() != nil || len(c.Actors) != len(sc.Public.Humans) {
		return rt.Output{}, fmt.Errorf("action actor set")
	}
	for _, human := range sc.Public.Humans {
		found := false
		for _, a := range c.Actors {
			found = found || a.Drives.Actor == human.ID
		}
		if !found {
			return rt.Output{}, fmt.Errorf("action actor identity")
		}
	}
	frame, e := h.Source.Frame(input)
	if e != nil {
		return rt.Output{}, e
	}
	if frame.Actions == nil || len(frame.Situation.Offers) != 0 || frame.Situation.Perceived.Actor != input.Actor || frame.Situation.Perceived.Event != input.ID || frame.Situation.Horizon > current.Budget.Horizon || len(frame.Disclosure) > 2048 {
		return rt.Output{}, fmt.Errorf("action frame mismatch")
	}
	hash, e := hex.DecodeString(frame.ModelHash)
	if e != nil || len(hash) != 32 {
		return rt.Output{}, fmt.Errorf("recorded model binding required")
	}
	f := frame.Actions
	s := behavior.ActionSituation{Observation: drives.Observation{Event: frame.Situation.Perceived}, Beliefs: frame.Situation.Beliefs, Relationships: frame.Situation.Relationships, Offers: f.Offers, Contexts: f.Contexts, Sources: f.Sources, Disclosure: f.Disclosure, Outage: frame.Situation.Outage, Horizon: frame.Situation.Horizon}
	// Missing context stays explicitly zero-confidence. No inferred trust/consent.
	if f.DriveContext != nil {
		s.Observation.Context = *f.DriveContext
	} else {
		for i := range s.Observation.Context {
			p := s.Observation.Event
			p.Confidence = 0
			p.Signals = dynamics.Signals{}
			s.Observation.Context[i] = drives.Cue{Evidence: p}
		}
	}
	focusFound := f.Focus == ""
	for _, relation := range f.Relationships {
		focusFound = focusFound || relation.Other == f.Focus
		s, e = behavior.ApplyRelationship(s, relation, input.Actor, input.At, relation.Other == f.Focus)
		if e != nil {
			return rt.Output{}, e
		}
	}
	if !focusFound {
		return rt.Output{}, fmt.Errorf("unknown relationship focus")
	}
	s.Resources, e = rt.SpendableResources(current)
	if e != nil {
		return rt.Output{}, e
	}
	for _, a := range c.Actors {
		if a.Contact != "left" {
			s.Present = append(s.Present, a.Drives.Actor)
		}
	}
	if current.Data == "" {
		c.Commitments = append([]behavior.Commitment{}, frame.Situation.Commitments...)
	} else if len(frame.Situation.Commitments) != 0 {
		return rt.Output{}, fmt.Errorf("commitments already initialized")
	}
	s.Commitments = c.Commitments
	if c.validate() != nil || len(c.Outcomes) >= behavior.MaxOutcomes {
		return rt.Output{}, fmt.Errorf("invalid action ledger/budget")
	}
	index := -1
	for i, a := range c.Actors {
		if a.Drives.Actor == input.Actor {
			index = i
		} else {
			c.Actors[i].Drives, e = drives.Advance(a.Drives, input.At)
			if e != nil {
				return rt.Output{}, e
			}
		}
	}
	if index < 0 {
		return rt.Output{}, fmt.Errorf("unknown action actor")
	}
	if frame.Response != nil {
		found := false
		for i, o := range c.Outcomes {
			if o.Outcome.Decision != frame.Response.Decision {
				continue
			}
			c.Actors[index], c.Outcomes[i], e = behavior.ResolveAction(c.Actors[index], o, s.Observation.Event, frame.Response.Other, frame.Response.Code)
			if e != nil {
				return rt.Output{}, e
			}
			found = true
			if frame.Response.CommitmentStatus != "" {
				if o.Action != behavior.Help || o.Commitment == "" || frame.Response.CommitmentStatus != "fulfilled" && frame.Response.CommitmentStatus != "broken" {
					return rt.Output{}, fmt.Errorf("unsupported commitment outcome")
				}
				matched := false
				for j, p := range c.Commitments {
					if p.ID == o.Commitment && p.Actor == input.Actor && p.Recipient == frame.Response.Other && p.Status == "pending" {
						if s.Observation.Event.OccurredAt > p.Due && frame.Response.CommitmentStatus == "fulfilled" {
							return rt.Output{}, fmt.Errorf("late fulfillment is not established")
						}
						c.Commitments[j].Status = frame.Response.CommitmentStatus
						matched = true
					}
				}
				if !matched {
					return rt.Output{}, fmt.Errorf("unknown pending commitment")
				}
			}
		}
		if !found {
			return rt.Output{}, fmt.Errorf("unknown action response")
		}
	}
	for i, o := range c.Outcomes {
		if o.Outcome.Status == core.Unknown && input.At >= o.Outcome.Horizon {
			c.Outcomes[i], e = behavior.CensorAction(o, input.At)
			if e != nil {
				return rt.Output{}, e
			}
		}
	}
	s.Commitments = c.Commitments
	draw, e := rng.Draw(core.ID("human-choice:" + string(input.Actor)))
	if e != nil {
		return rt.Output{}, e
	}
	next, decision, e := behavior.ChooseAction(c.Actors[index], s, input.At, draw)
	if e != nil {
		return rt.Output{}, e
	}
	c.Actors[index] = next
	selected := decision.Candidates[decision.Selected].Offer
	out := rt.Output{Events: []rt.Input{}}
	if selected.Kind == behavior.Help {
		out.Consume = []rt.Consumption{{Resource: selected.Resource, Units: selected.Units}}
	}
	if selected.Kind == behavior.Promise {
		c.Commitments = append(c.Commitments, behavior.Commitment{ID: selected.Commitment, Actor: input.Actor, Recipient: selected.Recipient, Resource: selected.Resource, Units: selected.Units, Due: selected.Due, Status: "pending"})
	}
	if selected.Kind == behavior.BreakPromise {
		for i, p := range c.Commitments {
			if p.ID == selected.Commitment {
				c.Commitments[i].Status = "broken"
			}
		}
	}
	if selected.Delivered() {
		kind := selected.Kind
		if kind == behavior.Lie {
			kind = behavior.Say
		}
		delivery := deliveredAction{Version: 2, Decision: decision.ID, Sender: input.Actor, Recipient: selected.Recipient, Kind: kind}
		if selected.Mode != "" {
			if f.Disclosure == nil || f.Disclosure.Mode != selected.Mode || f.Disclosure.Recipient != selected.Recipient || frame.Disclosure == "" {
				return rt.Output{}, fmt.Errorf("missing policy-validated fictional output")
			}
			delivery.Text = frame.Disclosure
			delivery.Sources = append([]core.ID{}, f.Disclosure.Sources...)
		}
		b, err := json.Marshal(delivery)
		if err != nil {
			return rt.Output{}, err
		}
		out.Events = append(out.Events, rt.Input{ID: core.ID("delivery:" + drives.Key(input.Actor, input.ID)), At: input.At + selected.Duration, Kind: "observation", Actor: selected.Recipient, Text: string(b), Priority: 1})
	}
	decision.Stages[11] = "done"
	c.Last = &decision
	c.ModelHash = frame.ModelHash
	o, e := behavior.TrackAction(decision, s.Horizon)
	if e != nil {
		return rt.Output{}, e
	}
	c.Outcomes = append(c.Outcomes, o)
	out.Data, e = c.Encode()
	return out, e
}
