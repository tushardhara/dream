package scenario

import (
	"encoding/json"
	"github.com/tushardhara/dream/core"
)

// WithTemporalContexts is an explicit, detached genesis migration. The source
// scenario remains replayable. Age zero is allowed only for explicitly declared
// unknown numerical ages in this synthetic adult cohort; it means no age evidence.
func WithTemporalContexts(s Scenario, contexts map[core.ID][]core.TemporalFact, unknownAges []core.ID) (Scenario, error) {
	if s.Validate() != nil || len(contexts) > 24 || len(unknownAges) > 24 {
		return Scenario{}, fail("$", "invalid temporal migration")
	}
	raw, _ := json.Marshal(s)
	var out Scenario
	_ = json.Unmarshal(raw, &out)
	found := map[core.ID]bool{}
	for i, a := range out.Actors {
		if values, ok := contexts[a.ID]; ok {
			raw, _ := json.Marshal(values)
			_ = json.Unmarshal(raw, &out.Actors[i].Temporal)
			found[a.ID] = true
		}
	}
	if len(found) != len(contexts) {
		return Scenario{}, fail("$.actors", "unknown temporal observer")
	}
	out.Public.UnknownAges = append([]core.ID{}, unknownAges...)
	for i, h := range out.Public.Humans {
		for _, id := range unknownAges {
			if h.ID == id {
				out.Public.Humans[i].Age = 0
			}
		}
	}
	has := false
	for _, c := range out.Requires {
		has = has || c == "temporal-context.v1"
	}
	if !has {
		out.Requires = append(out.Requires, "temporal-context.v1")
	}
	if out.Validate() != nil {
		return Scenario{}, fail("$", "invalid explicit temporal contexts")
	}
	return out, nil
}
