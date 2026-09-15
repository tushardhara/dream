package scenario

import (
	"bytes"
	"encoding/json"
	"github.com/tushardhara/dream/core"
	"testing"
)

func TestDomainScenarioMigrationCapabilityAndLegacyBytes(t *testing.T) {
	old := fixture()
	before, e := old.Canonical()
	if e != nil {
		t.Fatal(e)
	}
	profile := core.RelationshipContext{Version: 2, Account: "care-family", Observer: "a", Other: "b", Domain: core.Childcare, RoleContext: "family", ContextSource: "a-note", Types: []core.ID{"knows"}, Valid: core.Interval{}, Measures: []core.RelationshipMeasure{{Kind: "trust", Value: .8, Confidence: .7, Source: "a-note"}}}
	business := profile
	business.Account = "care-business"
	business.RoleContext = "business"
	business.Measures = []core.RelationshipMeasure{{Kind: "trust", Value: -.6, Confidence: .8, Source: "a-note"}}
	conflict := profile
	conflict.Account = "care-conflict"
	conflict.Measures = []core.RelationshipMeasure{{Kind: "trust", Value: -.9, Confidence: .3, Source: "a-note"}}
	next, e := WithDomainContexts(old, map[core.ID][]core.RelationshipContext{"a": {profile, business, conflict}})
	if e != nil {
		t.Fatal(e)
	}
	after, _ := old.Canonical()
	if !bytes.Equal(before, after) {
		t.Fatal("migration changed legacy source")
	}
	eng := engine()
	eng.Capabilities = append(eng.Capabilities, "relationships.v2")
	if next.CheckExecution(eng) == nil {
		t.Fatal("old engine silently ignored domains")
	}
	next.Requires = nil
	if next.CheckExecution(eng) == nil {
		t.Fatal("removing declaration bypassed inferred capability")
	}
	eng.Capabilities = append(eng.Capabilities, "relationships.v3")
	if next.CheckExecution(eng) != nil {
		t.Fatal("domain-capable engine rejected")
	}
	encoded, e := next.Canonical()
	if e != nil {
		t.Fatal(e)
	}
	var decoded Scenario
	e = json.Unmarshal(encoded, &decoded)
	if e != nil {
		t.Fatal(e)
	}
	genesis, e := next.Genesis(eng)
	if e != nil || genesis.Validate(eng) != nil {
		t.Fatal("domain genesis replay", e)
	}
	again, _ := decoded.Canonical()
	if !bytes.Equal(encoded, again) {
		t.Fatal("domain scenario replay changed")
	}
	view, e := next.View("a")
	if e != nil || len(view.Contexts) != 3 {
		t.Fatal("contradictory/overlapping accounts lost", e)
	}
	next.Actors[0].Contexts[0].ContextSource = "SECRET_B"
	if next.Validate() == nil {
		t.Fatal("foreign frame source authorized context")
	}
}
