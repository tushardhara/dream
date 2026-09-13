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
	"github.com/tushardhara/dream/simulator/dynamics"
	rt "github.com/tushardhara/dream/simulator/runtime"
	"github.com/tushardhara/dream/simulator/scenario"
)

const cognitivePrefix = "cognitive.v1:"

// New codec, not a larger runtime limit. Both compressed and expanded sizes are
// bounded. Receipts never evict: exhaustion requires an explicit future migration.
type CognitiveCheckpoint struct {
	ModelHash   string                `json:"model_hash,omitempty"`
	Version     int                   `json:"version"`
	Policy      string                `json:"policy"`
	Actors      []behavior.Actor      `json:"actors"`
	Outcomes    []behavior.Outcome    `json:"outcomes"`
	Commitments []behavior.Commitment `json:"commitments"`
	Last        *behavior.Decision    `json:"last,omitempty"`
}

func (c CognitiveCheckpoint) validate() error {
	if c.Version != 1 || c.Policy != behavior.Policy || len(c.Actors) < 2 || len(c.Actors) > 4 || len(c.Outcomes) > behavior.MaxOutcomes || len(c.Commitments) > 16 {
		return fmt.Errorf("cognitive checkpoint envelope/budget")
	}
	actors := map[core.ID]bool{}
	for _, a := range c.Actors {
		if a.Validate() != nil || actors[a.State.Actor] {
			return fmt.Errorf("invalid checkpoint actor")
		}
		actors[a.State.Actor] = true
	}
	ids := map[core.ID]bool{}
	for _, o := range c.Outcomes {
		if o.Validate() != nil || !actors[o.Observer] || ids[o.Decision] || (o.Other != "" && !actors[o.Other]) {
			return fmt.Errorf("invalid checkpoint outcome")
		}
		ids[o.Decision] = true
	}
	ids = map[core.ID]bool{}
	for _, p := range c.Commitments {
		if p.Validate() != nil || ids[p.ID] || !actors[p.Actor] || !actors[p.Recipient] {
			return fmt.Errorf("invalid checkpoint commitment")
		}
		ids[p.ID] = true
	}
	if c.Last != nil {
		decoded, e := hex.DecodeString(c.ModelHash)
		if e != nil || len(decoded) != 32 {
			return fmt.Errorf("checkpoint model binding")
		}
		if c.Last.Validate() != nil || !actors[c.Last.Actor] {
			return fmt.Errorf("invalid checkpoint decision")
		}
		found := false
		for _, a := range c.Actors {
			if a.State.Actor == c.Last.Actor {
				for _, r := range a.State.Applied {
					found = found || r.Event == c.Last.Event
				}
			}
		}
		if !found {
			return fmt.Errorf("decision lacks appraisal receipt")
		}
	}
	return nil
}
func (c CognitiveCheckpoint) Encode() (string, error) {
	if err := c.validate(); err != nil {
		return "", err
	}
	raw, err := json.Marshal(c)
	if err != nil || len(raw) > 65536 {
		return "", fmt.Errorf("expanded cognitive budget")
	}
	var b bytes.Buffer
	z, _ := zlib.NewWriterLevel(&b, zlib.BestCompression)
	_, err = z.Write(raw)
	if err != nil {
		return "", err
	}
	if err = z.Close(); err != nil {
		return "", err
	}
	out := cognitivePrefix + base64.RawStdEncoding.EncodeToString(b.Bytes())
	if len(out) > 4096 {
		return "", fmt.Errorf("cognitive checkpoint exceeds runtime 4096 bytes")
	}
	return out, nil
}
func DecodeCognitiveCheckpoint(s string) (CognitiveCheckpoint, error) {
	var c CognitiveCheckpoint
	if len(s) > 4096 || !strings.HasPrefix(s, cognitivePrefix) {
		return c, fmt.Errorf("unknown cognitive encoding")
	}
	b, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(s, cognitivePrefix))
	if err != nil {
		return c, err
	}
	r, err := zlib.NewReader(bytes.NewReader(b))
	if err != nil {
		return c, err
	}
	defer r.Close()
	raw, err := io.ReadAll(io.LimitReader(r, 65537))
	if err != nil || len(raw) > 65536 {
		return c, fmt.Errorf("expanded cognitive budget")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err = d.Decode(&c); err != nil {
		return c, err
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return c, fmt.Errorf("trailing cognitive input")
	}
	if err = c.validate(); err != nil {
		return c, err
	}
	// Pin canonical wire form and reject compressed trailing bytes/alternate spellings.
	canonical, err := c.Encode()
	if err != nil || canonical != s {
		return c, fmt.Errorf("noncanonical cognitive checkpoint")
	}
	return c, nil
}

type CognitiveFrame struct {
	Situation behavior.Situation
	// The preparation service validates a recorded model artifact. No generation
	// or storage I/O occurs inside Transition. No raw model text becomes an action.
	ModelHash string
	// Fixed delivery text for own-fiction disclosure only, after #12 policy Write.
	Disclosure string
	// Response resolves an earlier decision only when the observer actually learns it.
	Response *CognitiveResponse
}
type CognitiveResponse struct {
	Decision core.ID
	Other    core.ID
	Code     string
}
type CognitiveSource interface {
	Frame(rt.Input) (CognitiveFrame, error)
}
type CognitiveHandler struct{ Source CognitiveSource }
type deliveredBehavior struct {
	Version   int           `json:"version"`
	Decision  core.ID       `json:"decision"`
	Sender    core.ID       `json:"sender"`
	Recipient core.ID       `json:"recipient"`
	Kind      behavior.Kind `json:"kind"`
	Text      string        `json:"text,omitempty"`
}

func (h CognitiveHandler) Transition(current rt.State, input rt.Input, clock rt.Clock, rng *rt.Random) (rt.Output, error) {
	if current.Validate() != nil || h.Source == nil || rng == nil || clock.Now() != input.At || input.Kind != "observation" {
		return rt.Output{}, fmt.Errorf("cognitive transition boundary")
	}
	var sc scenario.Scenario
	if json.Unmarshal(current.Genesis.Payload, &sc) != nil {
		return rt.Output{}, fmt.Errorf("invalid genesis")
	}
	var c CognitiveCheckpoint
	var err error
	if current.Data == "" {
		initial, e := initialAppraisal(sc, current.Genesis.ScenarioHash)
		if e != nil {
			return rt.Output{}, e
		}
		c = CognitiveCheckpoint{Version: 1, Policy: behavior.Policy, Actors: []behavior.Actor{}, Outcomes: []behavior.Outcome{}, Commitments: []behavior.Commitment{}}
		for _, a := range initial.Actors {
			c.Actors = append(c.Actors, behavior.Actor{State: a, Memory: []behavior.Memory{}, Beliefs: []behavior.Belief{}})
		}
	} else {
		c, err = DecodeCognitiveCheckpoint(current.Data)
		if err != nil {
			return rt.Output{}, err
		}
	}
	if c.validate() != nil || len(c.Actors) != len(sc.Public.Humans) {
		return rt.Output{}, fmt.Errorf("cognitive actor set")
	}
	for _, human := range sc.Public.Humans {
		found := false
		for _, a := range c.Actors {
			found = found || a.State.Actor == human.ID
		}
		if !found {
			return rt.Output{}, fmt.Errorf("cognitive actor identity")
		}
	}
	frame, err := h.Source.Frame(input)
	if err != nil {
		return rt.Output{}, err
	}
	s := frame.Situation
	if s.Perceived.Actor != input.Actor || s.Perceived.Event != input.ID || s.Horizon > current.Budget.Horizon || len(frame.Disclosure) > 2048 {
		return rt.Output{}, fmt.Errorf("cognitive input mismatch")
	}
	// Binding is mandatory for model-backed cognition. Operational failure carries
	// a durable failure digest; policy/auth errors must never become outages.
	if len(frame.ModelHash) != 64 {
		return rt.Output{}, fmt.Errorf("recorded cognition required")
	}
	s.Resources, err = rt.SpendableResources(current)
	if err != nil {
		return rt.Output{}, err
	}
	if current.Data == "" {
		c.Commitments = append([]behavior.Commitment{}, s.Commitments...)
		if c.validate() != nil {
			return rt.Output{}, fmt.Errorf("initial commitments")
		}
	} else if len(s.Commitments) != 0 {
		return rt.Output{}, fmt.Errorf("commitments already initialized")
	}
	s.Commitments = c.Commitments
	s.Present = []core.ID{}
	for _, a := range sc.Public.Humans {
		s.Present = append(s.Present, a.ID)
	} // local synthetic venue, never inferred travel
	if s.DisclosureRecipient != "" && frame.Disclosure == "" {
		return rt.Output{}, fmt.Errorf("missing validated disclosure")
	}
	if len(c.Outcomes) >= behavior.MaxOutcomes {
		return rt.Output{}, fmt.Errorf("outcome ledger full; no silent eviction")
	}
	index := -1
	for i, a := range c.Actors {
		if a.State.Actor == input.Actor {
			index = i
		} else {
			c.Actors[i].State, err = dynamics.Advance(a.State, input.At)
			if err != nil {
				return rt.Output{}, err
			}
		}
	}
	if index < 0 {
		return rt.Output{}, fmt.Errorf("unknown cognitive actor")
	}
	if frame.Response != nil {
		found := false
		for i, o := range c.Outcomes {
			if o.Decision == frame.Response.Decision {
				c.Actors[index], c.Outcomes[i], err = behavior.Resolve(c.Actors[index], o, s.Perceived, frame.Response.Other, frame.Response.Code)
				if err != nil {
					return rt.Output{}, err
				}
				found = true
			}
		}
		if !found {
			return rt.Output{}, fmt.Errorf("unknown response decision")
		}
	}
	for i, outcome := range c.Outcomes {
		if outcome.Status == core.Unknown && input.At >= outcome.Horizon {
			c.Outcomes[i], err = behavior.Censor(outcome, input.At)
			if err != nil {
				return rt.Output{}, err
			}
		}
	}
	draw, err := rng.Draw(core.ID("human-choice:" + string(input.Actor)))
	if err != nil {
		return rt.Output{}, err
	}
	next, decision, err := behavior.Choose(c.Actors[index], s, input.At, draw)
	if err != nil {
		return rt.Output{}, err
	}
	c.Actors[index] = next
	c.Last = &decision
	c.ModelHash = frame.ModelHash
	outcome, err := behavior.Track(decision, s.Horizon)
	if err != nil {
		return rt.Output{}, err
	}
	c.Outcomes = append(c.Outcomes, outcome)
	selected := decision.Candidates[decision.Selected].Offer
	out := rt.Output{Events: []rt.Input{}}
	if selected.Kind == behavior.Help {
		out.Consume = []rt.Consumption{{Resource: selected.Resource, Units: selected.Units}}
	}
	if selected.Commitment != "" {
		for i, p := range c.Commitments {
			if p.ID == selected.Commitment {
				if selected.Kind == behavior.Help {
					// Resource use is recorded, but fulfillment needs later observed evidence.
					c.Commitments[i].Status = "pending"
				} else if selected.Kind == behavior.BreakPromise {
					c.Commitments[i].Status = "broken"
				}
			}
		}
	}
	if selected.Recipient != "" {
		delivery := deliveredBehavior{Version: 1, Decision: decision.ID, Sender: input.Actor, Recipient: selected.Recipient, Kind: selected.Kind}
		if selected.Kind == behavior.SelfDisclose {
			delivery.Text = frame.Disclosure
		}
		text, e := json.Marshal(delivery)
		if e != nil {
			return rt.Output{}, e
		}
		out.Events = append(out.Events, rt.Input{ID: core.ID("delivery:" + dynamics.Key(input.Actor, input.ID)), At: input.At + selected.Duration, Kind: "observation", Actor: selected.Recipient, Text: string(text), Priority: 1})
	}
	out.Data, err = c.Encode()
	return out, err
}
