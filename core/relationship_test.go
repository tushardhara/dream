package core

import (
	"math"
	"strings"
	"testing"
)

func TestRelationshipSourceFieldsAndUnknown(t *testing.T) {
	r := RelationshipContext{Version: 1, Observer: "a", Other: "b", Types: []ID{"spouse", "cofounder"}, Valid: Interval{}}
	for _, k := range strings.Fields("origin history view_of_relationship view_of_other communication positive_patterns friction_patterns significant_memories open_loops commitments recent_events current_state trajectory hypotheses") {
		r.Details = append(r.Details, RelationshipDetail{Kind: ID(k), Sources: []ID{ID(k + ":claim")}})
	}
	for _, k := range strings.Fields("trust closeness expectation sensitivity fear pride shame expected_reaction protective_intent social_norm stress prior_outcome friction") {
		r.Measures = append(r.Measures, RelationshipMeasure{Kind: ID(k), Value: .2, Confidence: .4, Source: ID(k + ":source")})
	}
	if e := r.Validate("a", "b"); e != nil {
		t.Fatal(e)
	}
	if len(r.Sources()) != 27 {
		t.Fatal("source fields lost")
	}
	r.Details = nil
	r.Measures = nil
	if e := r.Validate("a", "b"); e != nil {
		t.Fatal("unknown must be representable", e)
	}
	for _, bad := range []float64{math.NaN(), math.Inf(1), -1.1, 1.1} {
		r.Measures = []RelationshipMeasure{{Kind: "trust", Value: bad, Source: "e"}}
		if r.Validate("a", "b") == nil {
			t.Fatal("bad measure")
		}
	}
	r.Measures = nil
	r.Observer = "b"
	if r.Validate("a", "b") == nil {
		t.Fatal("foreign perspective")
	}
}
