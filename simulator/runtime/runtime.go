package runtime

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/scenario"
	"sort"
	"unicode/utf8"
)

const EngineVersion = "runtime.v1"
const BranchEngineVersion = "runtime.branch.v1"

func SupportedEngine(v string) bool { return v == EngineVersion || v == BranchEngineVersion }

const MaxQueue = 4096

// Budgets are hard ceilings per run, not replenished by resume or lease reclaim.
type Budgets struct {
	Steps   uint64           `json:"steps"`
	Events  uint64           `json:"events"`
	Horizon core.LogicalTime `json:"horizon"`
}
type Input struct {
	ID       core.ID          `json:"id"`
	At       core.LogicalTime `json:"at"`
	Kind     core.ID          `json:"kind"`
	Actor    core.ID          `json:"actor"`
	Text     string           `json:"text"`
	Resource core.ID          `json:"resource,omitempty"`
	Units    int64            `json:"units,omitempty"`
	Priority int              `json:"priority"`
}
type State struct {
	RandomDomain    core.ID            `json:"random_domain,omitempty"`
	ExogenousDomain core.ID            `json:"exogenous_domain,omitempty"`
	Coupled         bool               `json:"coupled,omitempty"`
	Version         int                `json:"version"`
	Genesis         scenario.Genesis   `json:"genesis"`
	Engine          string             `json:"engine"`
	RNG             string             `json:"rng"`
	At              core.LogicalTime   `json:"at"`
	Step            uint64             `json:"step"`
	Events          uint64             `json:"events"`
	Budget          Budgets            `json:"budget"`
	Status          string             `json:"status"`
	Queue           []Input            `json:"queue"`
	Seen            map[core.ID]bool   `json:"seen"`
	Positions       map[core.ID]uint64 `json:"positions"`
	Data            string             `json:"data"`
	Available       map[core.ID]int64  `json:"available"`
}
type Consumption struct {
	Resource core.ID `json:"resource"`
	Units    int64   `json:"units"`
}
type Output struct {
	Consume []Consumption
	Data    string
	Events  []Input
}
type Handler interface {
	Transition(State, Input, Clock, *Random) (Output, error)
}
type Transition struct {
	Consumed  []Consumption `json:"consumed,omitempty"`
	Input     Input         `json:"input"`
	Draws     []Draw        `json:"draws"`
	Output    string        `json:"output"`
	Generated []Input       `json:"generated"`
}
type Command struct {
	Kind  string           `json:"kind"`
	Until core.LogicalTime `json:"until,omitempty"`
	Input *Input           `json:"input,omitempty"`
}

func Capabilities() scenario.Engine {
	return scenario.Engine{ScenarioVersion: 1, Capabilities: []core.ID{"genesis.v1", "resources.v1", "memory.v1", "relationships.v1", "relationships.v2", "relationships.v3", "latent.v1", "schedule.v1"}}
}

// New validates the genesis event. The repository must persist it before this
// checkpoint becomes visible. Initial research fields are retained, not evolved.
func New(g scenario.Genesis, b Budgets) (State, error) {
	if err := g.Validate(Capabilities()); err != nil {
		return State{}, err
	}
	var sc scenario.Scenario
	if err := json.Unmarshal(g.Payload, &sc); err != nil {
		return State{}, err
	}
	if b.Steps == 0 || b.Steps > 1000000 || b.Events == 0 || b.Events > 1000000 || b.Horizon <= 0 || b.Horizon > sc.World.Horizon {
		return State{}, fmt.Errorf("invalid run budgets")
	}
	s := State{Version: 1, Genesis: g, Engine: EngineVersion, RNG: RNGVersion, Budget: b, Status: "paused", Queue: []Input{}, Seen: map[core.ID]bool{}, Positions: map[core.ID]uint64{}, Available: map[core.ID]int64{}}
	for _, r := range sc.Public.Resources {
		s.Available[r.ID] = r.Available
	}
	for _, e := range sc.Future {
		i := Input{ID: e.ID, At: e.At, Kind: e.Kind, Actor: e.Actor, Text: e.Text, Resource: e.Resource, Units: e.Units, Priority: 1}
		if e.Kind == "reservation" {
			releaseHash := sha256.Sum256([]byte(e.ID))
			release := i
			release.ID = core.ID("release:" + hex.EncodeToString(releaseHash[:]))
			release.At = e.Until
			release.Kind = "release"
			release.Priority = 0
			if s.Seen[release.ID] {
				return State{}, fmt.Errorf("expanded schedule ID collision")
			}
			s.Queue = append(s.Queue, release)
			s.Seen[release.ID] = true
		}
		if s.Seen[i.ID] {
			return State{}, fmt.Errorf("expanded schedule ID collision")
		}
		s.Queue = append(s.Queue, i)
		s.Seen[i.ID] = true
	}
	if len(s.Queue) > MaxQueue {
		return State{}, fmt.Errorf("initial queue budget")
	}
	order(s.Queue)
	return s, s.Validate()
}
func (s State) Validate() error {
	if s.Version != 1 || !SupportedEngine(s.Engine) || s.RNG != RNGVersion || s.At < 0 || s.At > s.Budget.Horizon || s.Step > s.Budget.Steps || s.Events > s.Budget.Events || s.Budget.Steps == 0 || s.Budget.Events == 0 || s.Budget.Steps > 1000000 || s.Budget.Events > 1000000 || len(s.Queue) > MaxQueue || len(s.Seen) > MaxQueue*4 || len(s.Positions) > 64 || len(s.Data) > 4096 || !utf8.ValidString(s.Data) {
		return fmt.Errorf("invalid runtime state")
	}
	if (s.Engine == EngineVersion && (s.RandomDomain != "" || s.ExogenousDomain != "" || s.Coupled)) || (s.Engine == BranchEngineVersion && (s.RandomDomain.Validate() != nil || (s.ExogenousDomain != "" && s.ExogenousDomain.Validate() != nil) || (!s.Coupled && s.ExogenousDomain != ""))) {
		return fmt.Errorf("invalid branch RNG profile")
	}
	switch s.Status {
	case "paused", "running", "cancelled", "completed", "budget":
	default:
		return fmt.Errorf("invalid status")
	}
	if err := s.Genesis.Validate(Capabilities()); err != nil {
		return err
	}
	var sc scenario.Scenario
	if err := json.Unmarshal(s.Genesis.Payload, &sc); err != nil {
		return err
	}
	if s.Budget.Horizon <= 0 || s.Budget.Horizon > sc.World.Horizon {
		return fmt.Errorf("invalid virtual horizon")
	}
	resources := map[core.ID]int64{}
	actors := map[core.ID]bool{}
	for _, a := range sc.Public.Humans {
		actors[a.ID] = true
	}
	for _, r := range sc.Public.Resources {
		resources[r.ID] = r.Available
	}
	if len(s.Available) != len(resources) {
		return fmt.Errorf("resource state mismatch")
	}
	for id, n := range s.Available {
		max, ok := resources[id]
		if !ok || n < 0 || n > max {
			return fmt.Errorf("invalid available capacity")
		}
	}
	for id := range s.Positions {
		if id.Validate() != nil {
			return fmt.Errorf("invalid RNG stream")
		}
	}
	queued := map[core.ID]bool{}
	for _, i := range s.Queue {
		if i.At < s.At || i.ID.Validate() != nil || !s.Seen[i.ID] || queued[i.ID] || !actors[i.Actor] || len(i.Text) == 0 || len(i.Text) > 4096 || !utf8.ValidString(i.Text) {
			return fmt.Errorf("invalid queued event")
		}
		queued[i.ID] = true
		switch i.Kind {
		case "observation":
			if i.Priority != 1 || i.Resource != "" || i.Units != 0 {
				return fmt.Errorf("invalid observation")
			}
		case "reservation", "release":
			max, ok := resources[i.Resource]
			if !ok || i.Units <= 0 || i.Units > max || (i.Kind == "release" && i.Priority != 0) || (i.Kind == "reservation" && i.Priority != 1) {
				return fmt.Errorf("invalid reservation")
			}
		default:
			return fmt.Errorf("unknown event kind")
		}

	}
	return nil
}
func (s State) Hash() (string, error) {
	b, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}
func (s State) Clone() (State, error) {
	b, err := json.Marshal(s)
	if err != nil {
		return State{}, err
	}
	if len(b) > 4<<20 {
		return State{}, fmt.Errorf("checkpoint size budget")
	}
	var out State
	err = json.Unmarshal(b, &out)
	return out, err
}
func (c Command) Validate() error {
	switch c.Kind {
	case "pause", "resume", "cancel", "step":
		if c.Until != 0 || c.Input != nil {
			return fmt.Errorf("unexpected command fields")
		}
	case "run-until":
		if c.Until <= 0 || c.Input != nil {
			return fmt.Errorf("invalid run-until")
		}
	case "inject":
		if c.Input == nil || c.Until != 0 {
			return fmt.Errorf("invalid injection")
		}
	default:
		return fmt.Errorf("unknown command")
	}
	return nil
}

// Apply commits at most one logical transition. run-until is continued by the
// application with durable progress; no unbounded loop exists in this function.
func Apply(current State, c Command, h Handler) (State, *Transition, bool, error) {
	if err := current.Validate(); err != nil {
		return State{}, nil, false, err
	}
	if err := c.Validate(); err != nil {
		return State{}, nil, false, err
	}
	s, err := current.Clone()
	if err != nil {
		return State{}, nil, false, err
	}
	if s.Status == "cancelled" || s.Status == "completed" || s.Status == "budget" {
		return State{}, nil, false, fmt.Errorf("run is terminal")
	}
	switch c.Kind {
	case "pause":
		s.Status = "paused"
		return s, nil, true, nil
	case "resume":
		s.Status = "running"
		return s, nil, true, nil
	case "cancel":
		s.Status = "cancelled"
		return s, nil, true, nil
	case "inject":
		if err := s.inject(*c.Input); err != nil {
			return State{}, nil, false, err
		}
		return s, nil, true, nil
	}
	if c.Kind == "run-until" && (c.Until < s.At || c.Until > s.Budget.Horizon) {
		return State{}, nil, false, fmt.Errorf("run-until outside monotonic budget")
	}
	if s.Status != "running" {
		return State{}, nil, false, fmt.Errorf("resume before stepping")
	}
	if s.Step >= s.Budget.Steps || s.Events >= s.Budget.Events {
		s.Status = "budget"
		return s, nil, true, nil
	}
	if len(s.Queue) == 0 {
		s.Status = "completed"
		return s, nil, true, nil
	}
	i := s.Queue[0]
	if i.At > s.Budget.Horizon {
		s.Status = "budget"
		return s, nil, true, nil
	}
	if c.Kind == "run-until" && i.At > c.Until {
		s.At = c.Until
		return s, nil, true, nil
	}
	if h == nil {
		return State{}, nil, false, fmt.Errorf("handler required")
	}
	var sc scenario.Scenario
	if err = json.Unmarshal(s.Genesis.Payload, &sc); err != nil {
		return State{}, nil, false, err
	}
	rng := NewScopedRandom(sc.World.Seed, s.Positions, s.RandomDomain, s.ExogenousDomain, s.Coupled)
	handlerState, err := s.Clone()
	if err != nil {
		return State{}, nil, false, err
	}
	out, err := h.Transition(handlerState, i, Clock{i.At}, rng)
	if err != nil {
		return State{}, nil, false, err
	}
	if len(out.Data) > 4096 || !utf8.ValidString(out.Data) || len(out.Events) > 128 {
		return State{}, nil, false, fmt.Errorf("transition output budget")
	}
	if len(out.Consume) > 16 {
		return State{}, nil, false, fmt.Errorf("consumption budget")
	}
	spendable := s.Available
	if len(out.Consume) > 0 {
		spendable, err = SpendableResources(s)
		if err != nil {
			return State{}, nil, false, err
		}
	}
	for _, use := range out.Consume {
		if use.Resource.Validate() != nil || use.Units <= 0 || spendable[use.Resource] < use.Units {
			return State{}, nil, false, fmt.Errorf("invalid action resource consumption")
		}
		s.Available[use.Resource] -= use.Units
		spendable[use.Resource] -= use.Units
	}
	s.Queue = s.Queue[1:]
	s.At = i.At
	s.Step++
	s.Events++
	s.Data = out.Data
	s.Positions = rng.positions
	if i.Kind == "reservation" {
		if s.Available[i.Resource] < i.Units {
			return State{}, nil, false, fmt.Errorf("resource capacity conflict")
		}
		s.Available[i.Resource] -= i.Units
	}
	if i.Kind == "release" {
		s.Available[i.Resource] += i.Units
	}
	for _, e := range out.Events {
		if err = s.inject(e); err != nil {
			return State{}, nil, false, err
		}
	}
	order(s.Queue)
	done := c.Kind == "step" || (c.Kind == "run-until" && s.At >= c.Until && (len(s.Queue) == 0 || s.Queue[0].At > c.Until))
	if s.Step == s.Budget.Steps || s.Events == s.Budget.Events {
		s.Status = "budget"
		done = true
	}
	if len(s.Queue) == 0 {
		s.Status = "completed"
		done = true
	}
	tr := &Transition{Consumed: out.Consume, Input: i, Draws: rng.draws, Output: out.Data, Generated: out.Events}
	return s, tr, done, s.Validate()
}
func (s *State) inject(i Input) error {
	if i.ID.Validate() != nil || i.At <= s.At || i.At > s.Budget.Horizon || i.Kind != "observation" || i.Resource != "" || i.Units != 0 || i.Priority != 1 || len(i.Text) == 0 || len(i.Text) > 4096 || !utf8.ValidString(i.Text) || s.Seen[i.ID] || len(s.Queue) >= MaxQueue || len(s.Seen) >= MaxQueue*4 {
		return fmt.Errorf("invalid, retroactive, duplicate or over-budget injection")
	}
	var sc scenario.Scenario
	if err := json.Unmarshal(s.Genesis.Payload, &sc); err != nil {
		return err
	}
	found := false
	for _, a := range sc.Public.Humans {
		found = found || a.ID == i.Actor
	}
	if !found {
		return fmt.Errorf("unknown injected actor")
	}
	s.Queue = append(s.Queue, i)
	s.Seen[i.ID] = true
	order(s.Queue)
	return nil
}
func order(q []Input) {
	sort.Slice(q, func(i, j int) bool {
		a, b := q[i], q[j]
		if a.At != b.At {
			return a.At < b.At
		}
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
		return a.ID < b.ID
	})
}

// SpendableResources is the resource executor's permanent-consumption ceiling.
// Existing reservation commitments remain reserved, even before their interval
// starts. This is an affordability constraint, not actor access to future text.
func SpendableResources(s State) (map[core.ID]int64, error) {
	if err := s.Validate(); err != nil {
		return nil, err
	}
	var sc scenario.Scenario
	if err := json.Unmarshal(s.Genesis.Payload, &sc); err != nil {
		return nil, err
	}
	maximum := map[core.ID]int64{}
	for _, r := range sc.Public.Resources {
		maximum[r.ID] = r.Available
	}
	available := map[core.ID]int64{}
	ceiling := map[core.ID]int64{}
	for id, n := range s.Available {
		available[id] = n
		ceiling[id] = n
	}
	queue := append([]Input{}, s.Queue...)
	order(queue)
	for _, event := range queue {
		n := available[event.Resource]
		switch event.Kind {
		case "reservation":
			if event.Units > n {
				return nil, fmt.Errorf("committed resource schedule impossible")
			}
			n -= event.Units
		case "release":
			if n > maximum[event.Resource]-event.Units {
				return nil, fmt.Errorf("committed resource release invalid")
			}
			n += event.Units
		default:
			continue
		}
		available[event.Resource] = n
		if n < ceiling[event.Resource] {
			ceiling[event.Resource] = n
		}
	}
	return ceiling, nil
}
