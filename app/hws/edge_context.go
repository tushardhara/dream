package hws

import (
	"context"
	"fmt"
	"sort"

	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator"
)

type EdgeContextService struct{ Relations graph.RelationService }

// Query supplies an actor's own edge-specific perception to the simulator. It
// does not average multiple observations or infer latent state from membership.
func (s EdgeContextService) Query(ctx context.Context, q graph.MemoryQuery, id core.ID) ([]simulator.EdgeContext, error) {
	if q.Actor != q.Scope.Owner {
		return nil, fmt.Errorf("edge context requires the actor's own perspective")
	}
	projections, err := s.Relations.Query(ctx, q, id)
	if err != nil {
		return nil, err
	}
	out := []simulator.EdgeContext{}
	for _, p := range projections {
		if p.State.Kind != "edge" {
			continue
		}
		c := simulator.EdgeContext{Observer: p.Observer, Relation: p.State.ID, Source: p.Record.Event.Meta.ID, Types: append([]core.ID{}, p.State.Types...), Dimensions: []simulator.EdgeDimension{}, Roles: []core.ID{}, Patterns: []simulator.EdgePattern{}, OpenLoops: append([]core.ID{}, p.OpenLoopSignals...)}
		for _, d := range p.State.Dimensions {
			c.Dimensions = append(c.Dimensions, simulator.EdgeDimension{Name: d.Name, Value: d.Value, Confidence: d.Confidence})
		}
		for _, m := range p.ActiveMembers {
			if m.Member.Principal == q.Actor {
				c.Roles = append(c.Roles, m.Roles...)
			}
		}
		sort.Slice(c.Roles, func(i, j int) bool { return c.Roles[i] < c.Roles[j] })
		for _, pattern := range p.State.Patterns {
			c.Patterns = append(c.Patterns, simulator.EdgePattern{Name: pattern.Name, Confidence: pattern.Confidence, Evidence: append([]core.ID{}, pattern.Evidence...)})
		}
		out = append(out, c)
	}
	return out, nil
}
