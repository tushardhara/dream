package scenario

import (
	"bytes"
	"encoding/json"
	"github.com/tushardhara/dream/core"
	"testing"
)

func TestTemporalMigrationUnknownAgePrivateViewsAndLegacy(t *testing.T) {
	legacy := fixture()
	before, e := legacy.Canonical()
	if e != nil {
		t.Fatal(e)
	}
	fact := core.TemporalFact{Version: core.TemporalFactVersion, Account: "life", Observer: "a", Person: "a", With: "b", Channel: "chat", Source: "a-note", Kind: "circumstance", Basis: "self_report", Category: "responsibility", Signal: "busy", FreshFor: 30}
	next, e := WithTemporalContexts(legacy, map[core.ID][]core.TemporalFact{"a": {fact}}, []core.ID{"a"})
	if e != nil {
		t.Fatal(e)
	}
	after, _ := legacy.Canonical()
	if !bytes.Equal(before, after) {
		t.Fatal("migration changed legacy input")
	}
	eng := engine()
	if next.CheckExecution(eng) == nil {
		t.Fatal("legacy engine ignored temporal context")
	}
	next.Requires = nil
	if next.CheckExecution(eng) == nil {
		t.Fatal("missing capability declaration bypassed content inference")
	}
	eng.Capabilities = append(eng.Capabilities, "temporal-context.v1")
	g, e := next.Genesis(eng)
	if e != nil || g.Validate(eng) != nil {
		t.Fatal("temporal genesis", e)
	}
	a, e := next.View("a")
	if e != nil || len(a.Temporal) != 1 || len(a.UnknownAges) != 1 {
		t.Fatal("own context/unknown age lost", e)
	}
	b, e := next.View("b")
	if e != nil || len(b.Temporal) != 0 {
		t.Fatal("private life update leaked", e)
	}
	raw, e := next.Canonical()
	if e != nil {
		t.Fatal(e)
	}
	var restored Scenario
	if json.Unmarshal(raw, &restored) != nil || restored.Validate() != nil {
		t.Fatal("temporal scenario replay")
	}
	again, _ := restored.Canonical()
	if !bytes.Equal(raw, again) {
		t.Fatal("temporal replay bytes changed")
	}
	next.Actors[0].Temporal[0].Source = "SECRET_B"
	if next.Validate() == nil {
		t.Fatal("foreign private circumstance source")
	}
	bad := fixture()
	bad.Public.Humans[0].Age = 0
	if bad.Validate() == nil {
		t.Fatal("legacy numeric age silently reinterpreted")
	}
	for _, age := range []int{18, 35, 80, 120} {
		copy := fixture()
		copy.Public.Humans[0].Age = age
		if copy.Validate() != nil {
			t.Fatal("adult metadata rejected")
		}
	}
}
