package scenario

import (
	"encoding/json"
	"github.com/tushardhara/dream/core"
)

// WithDomainContexts is an explicit scenario migration. Callers supply authored
// replacement accounts and already-known frame/evidence records. It never guesses
// domains from role labels or copies an old unscoped numerical trust vector.
func WithDomainContexts(s Scenario, replacements map[core.ID][]core.RelationshipContext) (Scenario, error) {
	if s.Validate() != nil || len(replacements) == 0 || len(replacements) > 24 {
		return Scenario{}, fail("$.actors", "invalid domain migration")
	}
	raw, _ := json.Marshal(s)
	var out Scenario
	_ = json.Unmarshal(raw, &out)
	found := map[core.ID]bool{}
	for i, a := range out.Actors {
		profiles, ok := replacements[a.ID]
		if !ok {
			continue
		}
		found[a.ID] = true
		for _, p := range profiles {
			if p.Version != 2 || p.Validate(a.ID, p.Other) != nil {
				return Scenario{}, fail("$.actors", "explicit v2 domain accounts required")
			}
		}
		raw, _ := json.Marshal(profiles)
		_ = json.Unmarshal(raw, &out.Actors[i].Contexts)
	}
	if len(found) != len(replacements) {
		return Scenario{}, fail("$.actors", "unknown migration observer")
	}
	has := false
	for _, c := range out.Requires {
		has = has || c == "relationships.v3"
	}
	if !has {
		out.Requires = append(out.Requires, "relationships.v3")
	}
	if e := out.Validate(); e != nil {
		return Scenario{}, e
	}
	return out, nil
}
