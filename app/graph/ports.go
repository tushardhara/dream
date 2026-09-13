package graph

import (
	"context"

	"github.com/tushardhara/dream/core"
)

// RightsReader resolves current rights from trusted state. Missing resources,
// resolution errors and unknown request contexts must deny at the use-case
// boundary. Implementations must not serve stale grants after revocation.
// Storage and the full authorization service arrive with their owning tickets.
type RightsReader interface {
	Rights(context.Context, core.ID) (core.Rights, error)
}
