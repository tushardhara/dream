// Package scenario defines the versioned, synthetic research input contract.
// It has no I/O and exposes only explicit actor projections to downstream actors.
package scenario

import (
	"fmt"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator"
)

const Version = 1
const MaxItems = 1024

type Error struct {
	Path    string `json:"path"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string        { return e.Path + ": " + e.Message }
func fail(path, message string) error { return &Error{path, "invalid", message} }

type Scenario struct {
	Version  int         `json:"version"`
	World    World       `json:"world"`
	Requires []core.ID   `json:"requires"`
	Public   Public      `json:"public"`
	Actors   []Actor     `json:"actors"`
	Research Research    `json:"research"`
	Future   []Scheduled `json:"future"`
}
type World struct {
	ID      simulator.WorldID `json:"id"`
	Seed    uint64            `json:"seed"`
	Horizon core.LogicalTime  `json:"horizon"`
}
type Human struct {
	ID   core.ID `json:"id"`
	Name string  `json:"name"`
	Age  int     `json:"age"`
}
type Public struct {
	Humans    []Human    `json:"humans"`
	Groups    []Group    `json:"groups"`
	Resources []Resource `json:"resources"`
	Facts     []Fact     `json:"facts"`
}
type Group struct {
	ID      core.ID   `json:"id"`
	Members []core.ID `json:"members"`
}
type Resource struct {
	ID        core.ID `json:"id"`
	Capacity  int64   `json:"capacity"`
	Available int64   `json:"available"`
}

// Facts are fictional observer-owned statements, not global truth. Permissions
// remain exact-context core grants; knowledge does not confer disclosure rights.
type Fact struct {
	ID         core.ID         `json:"id"`
	Observer   core.ID         `json:"observer"`
	Subject    core.ID         `json:"subject"`
	Text       string          `json:"text"`
	Confidence core.Confidence `json:"confidence"`
	Valid      core.Interval   `json:"valid"`
	Grants     []core.Grant    `json:"grants"`
}
type Knowledge struct {
	Record    core.ID          `json:"record"`
	LearnedAt core.LogicalTime `json:"learned_at"`
}
type Relationship struct {
	ID       core.ID `json:"id"`
	Other    core.ID `json:"other"`
	Kind     core.ID `json:"kind"`
	Evidence core.ID `json:"evidence"`
}
type Memory struct {
	ID       core.ID `json:"id"`
	Evidence core.ID `json:"evidence"`
	Text     string  `json:"text"`
}
type Actor struct {
	ID            core.ID        `json:"id"`
	Facts         []Fact         `json:"facts"`
	Knowledge     []Knowledge    `json:"knowledge"`
	Memories      []Memory       `json:"memories"`
	Relationships []Relationship `json:"relationships"`
}
type Latent struct {
	Actor   core.ID           `json:"actor"`
	Emotion simulator.Emotion `json:"emotion"`
	Drives  []simulator.Drive `json:"drives"`
}
type Label struct {
	ID   core.ID `json:"id"`
	Text string  `json:"text"`
}
type Research struct {
	Latent []Latent `json:"latent"`
	Labels []Label  `json:"labels"`
}

// V1 schedules introduce an observation or reserve a public resource for a
// half-open interval. They are engine inputs, never initial actor knowledge.
type Scheduled struct {
	ID       core.ID          `json:"id"`
	At       core.LogicalTime `json:"at"`
	Kind     core.ID          `json:"kind"`
	Actor    core.ID          `json:"actor"`
	Text     string           `json:"text"`
	Resource core.ID          `json:"resource,omitempty"`
	Units    int64            `json:"units,omitempty"`
	Until    core.LogicalTime `json:"until,omitempty"`
}

type Engine struct {
	ScenarioVersion int
	Capabilities    []core.ID
}

// CheckExecution must precede genesis application. Declared and inferred
// capabilities are both required, so removing a declaration cannot bypass it.
func (s Scenario) CheckExecution(e Engine) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if e.ScenarioVersion != Version {
		return fail("$.version", "engine does not support scenario version")
	}
	have := map[core.ID]bool{}
	for _, c := range e.Capabilities {
		have[c] = true
	}
	for _, c := range s.capabilities() {
		if !have[c] {
			return fail("$.requires", fmt.Sprintf("engine lacks capability %s", c))
		}
	}
	return nil
}
func (s Scenario) capabilities() []core.ID {
	c := append([]core.ID{"genesis.v1"}, s.Requires...)
	if len(s.Public.Resources) > 0 {
		c = append(c, "resources.v1")
	}
	for _, a := range s.Actors {
		if len(a.Memories) > 0 {
			c = append(c, "memory.v1")
		}
		if len(a.Relationships) > 0 {
			c = append(c, "relationships.v1")
		}
	}
	if len(s.Research.Latent) > 0 {
		c = append(c, "latent.v1")
	}
	if len(s.Future) > 0 {
		c = append(c, "schedule.v1")
	}
	return c
}
