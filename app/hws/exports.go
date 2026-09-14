package hws

import (
	"context"
	"github.com/tushardhara/dream/core"
)

// ExportSubmission stores a bounded request and manifest, never exported bytes
// or bearer credentials. Download reconstructs under current authorization.
type ExportSubmission struct {
	Version  int     `json:"version"`
	Scope    Scope   `json:"scope"`
	Caller   core.ID `json:"caller"`
	ID       core.ID `json:"id"`
	Request  []byte  `json:"request"`
	Manifest []byte  `json:"manifest"`
}
type ExportStore interface {
	SaveExport(context.Context, ExportSubmission) error
	LoadExport(context.Context, Scope, core.ID, core.ID) (ExportSubmission, error)
}
