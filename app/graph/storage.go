package graph

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tushardhara/dream/core"
)

type PayloadClass string

const (
	ObservablePayload PayloadClass = "observable"
	PrivatePayload    PayloadClass = "private"
	ResearchPayload   PayloadClass = "research"
)

// Payload v1 is a bounded typed research text value. It is stored separately from
// immutable event metadata and can be purged. No provider transcript is implied.
type Payload struct {
	Version uint32 `json:"version"`
	Text    string `json:"text"`
}

func (p Payload) Validate() error {
	if p.Version != 1 || strings.TrimSpace(p.Text) == "" || len(p.Text) > 4096 || !utf8.ValidString(p.Text) {
		return fmt.Errorf("invalid payload schema/text")
	}
	return nil
}

type AppendCommand struct {
	Actor             core.ID
	Namespace         core.ID
	Operation         core.ID
	Key               core.ID
	ExpectedVersion   int64
	Event             core.Event
	Class             PayloadClass
	Payload           Payload
	Supersedes        core.ID
	DerivationContext *core.Grant
}

func (c AppendCommand) Validate() error {
	for _, id := range []core.ID{c.Actor, c.Namespace, c.Operation, c.Key} {
		if err := id.Validate(); err != nil {
			return err
		}
	}
	if c.ExpectedVersion < 0 {
		return fmt.Errorf("negative expected version")
	}
	if err := c.Event.Validate(); err != nil {
		return err
	}
	if c.Actor != c.Event.Meta.Observer {
		return fmt.Errorf("actor must own event perspective")
	}
	if c.Event.Meta.Rights.Revoked {
		return fmt.Errorf("new event cannot start revoked; use revocation transaction")
	}
	switch c.Class {
	case ObservablePayload:
		if c.Event.Meta.Sensitivity != core.Public {
			return fmt.Errorf("restricted event cannot have observable payload")
		}
	case PrivatePayload, ResearchPayload:
	default:
		return fmt.Errorf("unknown payload class")
	}
	if c.Supersedes != "" {
		if err := c.Supersedes.Validate(); err != nil {
			return err
		}
	}
	return c.Payload.Validate()
}

// Digest identifies command retries while excluding caller-supplied recorded
// wall time, which the database stamps itself. Logical time remains significant.
func (c AppendCommand) Digest() ([32]byte, error) {
	if err := c.Validate(); err != nil {
		return [32]byte{}, err
	}
	c.Event.Meta.RecordedAt = time.Unix(0, 0).UTC()
	canonical, err := core.CanonicalEvent(c.Event)
	if err != nil {
		return [32]byte{}, err
	}
	if err = json.Unmarshal(canonical, &c.Event); err != nil {
		return [32]byte{}, err
	}
	raw, err := json.Marshal(c)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(raw), nil
}

type AppendResult struct {
	EventID       core.ID
	Sequence      int64
	StreamVersion int64
}
type EventWriter interface {
	Append(context.Context, AppendCommand) (AppendResult, error)
}
