package hws

import (
	"context"
	"fmt"

	"github.com/tushardhara/dream/core"
)

// ModelCapabilities is provider-neutral descriptive metadata. It is not evidence
// of permission, determinism or successful execution. Gateway execution is #10.
type ModelCapabilities struct {
	Adapter        core.ID
	TypedDecisions bool
	RecordedReplay bool
}

func (c ModelCapabilities) Validate() error {
	if err := c.Adapter.Validate(); err != nil {
		return err
	}
	if !c.TypedDecisions && !c.RecordedReplay {
		return fmt.Errorf("no declared decision capability")
	}
	return nil
}

// CapabilityProvider belongs to the application consumer, never pure entities.
// No provider implementation or model request is introduced by this contract.
type CapabilityProvider interface {
	Capabilities(context.Context) (ModelCapabilities, error)
}
