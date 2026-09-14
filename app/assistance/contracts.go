// Package assistance hosts bounded, separately permissioned helper decisions.
// It has no world, simulated private state, evaluator, transport or provider dependency.
package assistance

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"

	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/core"
)

const Version = "assistance.v1"
const MaxHistory = 32

var ErrDenied = errors.New("assistance unavailable or not permitted")
var ErrInvalid = errors.New("invalid assistance contract")

type Arm string

const (
	None   Arm = "no_assistant"
	Simple Arm = "explicit_preference"
	Single Arm = "single_perspective"
	Multi  Arm = "multi_perspective"
)

func (a Arm) Valid() bool { return a == None || a == Simple || a == Single || a == Multi }

type Goal string

const (
	Unknown    Goal = "unknown"
	Listen     Goal = "listen"
	Understand Goal = "understand"
	Coordinate Goal = "coordinate"
	Pause      Goal = "pause"
)

func (g Goal) Valid() bool {
	return g == Unknown || g == Listen || g == Understand || g == Coordinate || g == Pause
}

type Action string

const (
	Wait        Action = "WAIT"
	Clarify     Action = "clarify_goal"
	Acknowledge Action = "acknowledge_request"
	Propose     Action = "propose_coordination"
)

func (a Action) Valid() bool { return a == Wait || a == Clarify || a == Acknowledge || a == Propose }

// Request contains host-selected evidence references, never raw caller text or
// simulated state. Goal is an explicit user choice, not an inferred relationship goal.
type Request struct {
	Version                   string
	ID, Helper, User, Purpose core.ID
	Participants              []core.ID
	Arm                       Arm
	Goal                      Goal
	At                        core.LogicalTime
	Seed                      uint64
	Contexts                  []graph.ContextProposal
}

func (r Request) Validate() error {
	if r.Version != Version || r.ID.Validate() != nil || r.Helper.Validate() != nil || r.User.Validate() != nil || r.Purpose.Validate() != nil || r.At.Validate() != nil || !r.Arm.Valid() || !r.Goal.Valid() || len(r.Participants) < 1 || len(r.Participants) > 8 || len(r.Contexts) > 8 {
		return ErrInvalid
	}
	seen := map[core.ID]bool{}
	for _, p := range r.Participants {
		if p.Validate() != nil || p == r.Helper || seen[p] {
			return ErrInvalid
		}
		seen[p] = true
	}
	if !seen[r.User] {
		return ErrInvalid
	}
	if (r.Arm == None || r.Arm == Simple) && len(r.Contexts) != 0 || r.Arm == Single && len(r.Contexts) > 1 {
		return ErrInvalid
	}
	owners := map[core.ID]bool{}
	for _, p := range r.Contexts {
		if p.Version != 1 || p.Query.Validate() != nil || p.Binding != r.ID || p.Query.Actor != r.Helper || p.Recipient != r.Helper || p.Query.Purpose != r.Purpose || p.Mode != graph.ExternalContext || p.Operation != core.Read || p.Query.KnownAt != r.At || p.Query.ValidAt != r.At || !seen[p.Query.Scope.Owner] || owners[p.Query.Scope.Owner] || len(p.Sources) < 1 || len(p.Sources) > 16 {
			return ErrInvalid
		}
		if r.Arm == Single && p.Query.Scope.Owner != r.User {
			return ErrInvalid
		}
		owners[p.Query.Scope.Owner] = true
	}
	b, e := json.Marshal(r)
	if e != nil || len(b) > 32768 {
		return ErrInvalid
	}
	return nil
}

type EvidenceRef struct{ Owner, Source core.ID }
type Candidate struct {
	Action    Action
	Recipient core.ID
	Evidence  []EvidenceRef
	Reason    string
}

func (c Candidate) validate(r Request, items []graph.SafeContextItem) bool {
	if !c.Action.Valid() || len(c.Evidence) > 16 {
		return false
	}
	if c.Action == Wait {
		return c.Recipient == "" && len(c.Evidence) == 0 && (c.Reason == "restraint" || c.Reason == "disabled" || c.Reason == "unavailable")
	}
	// v1 delivers only fixed, non-identifying templates to the requesting person.
	// No free text, paraphrase, source identity or multi-person disclosure leaves here.
	if c.Recipient != r.User {
		return false
	}
	switch c.Action {
	case Clarify:
		if c.Reason != "goal_unknown" || r.Goal != Unknown {
			return false
		}
	case Acknowledge:
		if c.Reason != "explicit_listening" || r.Goal != Listen {
			return false
		}
	case Propose:
		if c.Reason != "explicit_coordination" || r.Goal != Coordinate {
			return false
		}
	}
	for i, e := range c.Evidence {
		found := false
		for _, item := range items {
			found = found || item.Observer == e.Owner && item.Source == e.Source
		}
		if !found {
			return false
		}
		for _, before := range c.Evidence[:i] {
			if before == e {
				return false
			}
		}
	}
	return true
}

type Input struct {
	Version          string
	ID, Helper, User core.ID
	Arm              Arm
	Goal             Goal
	At               core.LogicalTime
	Seed             uint64
	Context          []graph.SafeContextItem
	History          []Interaction
}
type Result struct {
	Version    string
	Candidates []Candidate
	Selected   int
}

func (o Result) validate(r Request, items []graph.SafeContextItem) bool {
	if o.Version != Version || len(o.Candidates) < 1 || len(o.Candidates) > 4 || o.Selected < 0 || o.Selected >= len(o.Candidates) {
		return false
	}
	wait := false
	for _, c := range o.Candidates {
		if !c.validate(r, items) {
			return false
		}
		wait = wait || c.Action == Wait
	}
	return wait
}
func DecodeResult(b []byte) (Result, error) {
	var out Result
	if len(b) > 8192 {
		return out, ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if d.Decode(&out) != nil || d.Decode(new(any)) != io.EOF {
		return Result{}, ErrInvalid
	}
	return out, nil
}

type Outcome struct {
	Version                                    string
	Interaction, Participant, Observer, Source core.ID
	OccurredAt, LearnedAt                      core.LogicalTime
	Confidence                                 core.Confidence
	Status                                     core.OutcomeStatus
	Kind                                       string // expected or observed; neither is inferred from delivery
	Benefit, Burden                            *float64
}

func (o Outcome) Validate() error {
	if o.Version != Version || o.Interaction.Validate() != nil || o.Participant.Validate() != nil || o.Observer.Validate() != nil || o.Source.Validate() != nil || o.OccurredAt.Validate() != nil || o.LearnedAt < o.OccurredAt || o.Confidence.Validate() != nil || (o.Kind != "expected" && o.Kind != "observed") {
		return ErrInvalid
	}
	if o.Status != core.Unknown && o.Status != core.Censored && o.Status != core.Observed {
		return ErrInvalid
	}
	for _, v := range []*float64{o.Benefit, o.Burden} {
		if v != nil && !(*v >= -1 && *v <= 1) {
			return ErrInvalid
		}
	}
	if o.Status != core.Observed && (o.Benefit != nil || o.Burden != nil) {
		return ErrInvalid
	}
	return nil
}

// Interaction is an event in the helper's independent history. Delivered is
// mechanical receipt status only; outcomes require separately attributed evidence.
type Interaction struct {
	Arm              Arm
	Seed             uint64
	Version          string
	ID, Helper, User core.ID
	At               core.LogicalTime
	RequestHash      string
	Result           Result
	Delivered        bool
}

func Digest(v any) string {
	b, e := json.Marshal(v)
	if e != nil {
		return ""
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func clone[T any](v T) T { b, _ := json.Marshal(v); var out T; _ = json.Unmarshal(b, &out); return out }
