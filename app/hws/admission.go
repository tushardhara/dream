package hws

import (
	"errors"
	"github.com/tushardhara/dream/core"
)

var ErrAdmission = errors.New("durable request budget exhausted")

// RequestBudget is supplied by host credential configuration, never wire data.
// Credential IDs are stable across restart; rotating a secret must retain its ID
// to retain its ledger. Allocating a new ID is a new explicit host budget.
type RequestBudget struct {
	Credential core.ID
	Scope      Scope
	PerMinute  uint32
	Total      uint64
}

func (b RequestBudget) Validate() error {
	if b.Credential.Validate() != nil || b.Scope.Validate() != nil || b.PerMinute == 0 || b.PerMinute > 10000 || b.Total == 0 || b.Total > 100000 {
		return ErrAdmission
	}
	return nil
}
