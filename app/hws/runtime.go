package hws

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator"
	rt "github.com/tushardhara/dream/simulator/runtime"
	"github.com/tushardhara/dream/simulator/scenario"
	"time"
)

var ErrDeadline = errors.New("operational deadline crossed during commit")
var ErrLease = errors.New("lease absent, expired or fenced")
var ErrConflict = errors.New("runtime version or operation conflict")
var ErrCommand = errors.New("runtime idempotency conflict")

type Scope struct {
	Actor     core.ID            `json:"actor"`
	Namespace core.ID            `json:"namespace"`
	World     simulator.WorldID  `json:"world"`
	Branch    simulator.BranchID `json:"branch"`
	Run       simulator.RunID    `json:"run"`
}

func (s Scope) Validate() error {
	for _, id := range []core.ID{s.Actor, s.Namespace, core.ID(s.World), core.ID(s.Branch), core.ID(s.Run)} {
		if err := id.Validate(); err != nil {
			return err
		}
	}
	return nil
}

type Manifest struct {
	Version     int              `json:"version"`
	Scope       Scope            `json:"scope"`
	Genesis     scenario.Genesis `json:"genesis"`
	Budget      rt.Budgets       `json:"budget"`
	MaxDuration time.Duration    `json:"max_duration_ns"`
}

func (m Manifest) Validate() error {
	if err := m.Scope.Validate(); err != nil {
		return err
	}
	if m.Version != 1 || m.Genesis.World != m.Scope.World || m.MaxDuration <= 0 || m.MaxDuration > 24*time.Hour {
		return fmt.Errorf("invalid manifest")
	}
	_, err := rt.New(m.Genesis, m.Budget)
	return err
}

type Lease struct {
	Holder  core.ID
	Fence   int64
	Expires time.Time
}
type Snapshot struct {
	Revision int64
	State    rt.State
	Deadline time.Time
}
type Receipt struct {
	Revision int64  `json:"revision"`
	Hash     string `json:"hash"`
	Status   string `json:"status"`
	Done     bool   `json:"done"`
}
type Operation struct {
	Command rt.Command
	Digest  string
	Receipt Receipt
}
type ModelUse struct {
	Key  core.ID `json:"key"`
	Hash string  `json:"hash"`
}

func RuntimeCommandDigest(c rt.Command, model *ModelUse) (string, error) {
	if model == nil {
		return CommandDigest(c)
	}
	if c.Validate() != nil || c.Kind != "step" || model.Key.Validate() != nil || len(model.Hash) != 64 {
		return "", ErrModel
	}
	return ModelDigest(struct {
		Command rt.Command
		Model   *ModelUse
	}{c, model})
}

type Commit struct {
	Model             *ModelUse
	CheckDeadlineOnly bool
	Scope             Scope
	Lease             Lease
	Key               core.ID
	Command           rt.Command
	Expected          int64
	State             rt.State
	Transition        *rt.Transition
	Done              bool
}
type RuntimeStore interface {
	CreateRun(context.Context, Manifest) (Snapshot, error)
	Acquire(context.Context, Scope, core.ID, time.Duration) (Lease, error)
	Renew(context.Context, Scope, Lease, time.Duration) (Lease, error)
	LoadRun(context.Context, Scope) (Snapshot, error)
	Operation(context.Context, Scope, core.ID) (*Operation, error)
	CommitRun(context.Context, Commit) (Receipt, error)
}

// OperationalClock is separate from the simulator's virtual Clock. Database
// lease/deadline checks are authoritative; this clock avoids needless work.
type OperationalClock interface{ Now() time.Time }
type Runtime struct {
	BeforeCommit func(context.Context) error
	Store        RuntimeStore
	Clock        OperationalClock
	Handler      rt.Handler
	Model        *ModelUse
}

func CommandDigest(c rt.Command) (string, error) {
	if err := c.Validate(); err != nil {
		return "", err
	}
	b, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}

// Execute advances at most one commit boundary. Repeat the SAME run-until key
// until Done; retries after an uncertain success continue durable progress, never
// restart the operation. Step/inject/control retries return their first receipt.
func (r Runtime) Execute(ctx context.Context, scope Scope, lease Lease, key core.ID, c rt.Command) (Receipt, error) {
	if err := scope.Validate(); err != nil {
		return Receipt{}, err
	}
	if err := key.Validate(); err != nil {
		return Receipt{}, err
	}
	digest, err := RuntimeCommandDigest(c, r.Model)
	if err != nil {
		return Receipt{}, err
	}
	op, err := r.Store.Operation(ctx, scope, key)
	if err != nil {
		return Receipt{}, err
	}
	if op != nil {
		if op.Digest != digest {
			return Receipt{}, ErrCommand
		}
		if op.Receipt.Done {
			return op.Receipt, nil
		}
	}
	snap, err := r.Store.LoadRun(ctx, scope)
	if err != nil {
		return Receipt{}, err
	}
	var next rt.State
	var tr *rt.Transition
	done := true
	checkDeadlineOnly := false
	if r.Clock != nil && !r.Clock.Now().Before(snap.Deadline) {
		next, err = snap.State.Clone()
		next.Status = "budget"
		checkDeadlineOnly = true
	} else {
		next, tr, done, err = rt.Apply(snap.State, c, r.Handler)
	}
	if err != nil {
		return Receipt{}, err
	}
	if err = ctx.Err(); err != nil {
		return Receipt{}, err
	}
	if r.BeforeCommit != nil {
		if err = r.BeforeCommit(ctx); err != nil {
			return Receipt{}, err
		}
	}
	commit := Commit{Model: r.Model, CheckDeadlineOnly: checkDeadlineOnly, Scope: scope, Lease: lease, Key: key, Command: c, Expected: snap.Revision, State: next, Transition: tr, Done: done}
	receipt, err := r.Store.CommitRun(ctx, commit)
	if errors.Is(err, ErrDeadline) {
		return r.Store.CommitRun(ctx, commit)
	} // one bounded retry; DB writes a budget-only checkpoint
	return receipt, err
}
