package hws

import (
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/behavior"
	rt "github.com/tushardhara/dream/simulator/runtime"
	"reflect"
	"testing"
)

func TestRelationalContextReachesCognitiveHandler(t *testing.T) {
	s := cognitiveWorld(t)
	i := s.Queue[0]
	run := func(trust float64, foreign bool) (ActionCheckpoint, error) {
		f := actionFrame(i)
		f.Actions.Focus = "b"
		observer := core.ID("a")
		if foreign {
			observer = "b"
		}
		f.Actions.Relationships = []core.RelationshipContext{{Version: 1, Observer: observer, Other: "b", Types: []core.ID{"spouse"}, Valid: core.Interval{}, Measures: []core.RelationshipMeasure{{Kind: "trust", Value: trust, Confidence: .8, Source: i.ID}, {Kind: "prior_outcome", Value: trust, Confidence: .6, Source: i.ID}}}}
		f.Actions.Offers = []behavior.ActionOffer{{Kind: behavior.Ask, Recipient: "b", Evidence: []core.ID{i.ID}, Duration: 1}, {Kind: behavior.Decline, Recipient: "b", Evidence: []core.ID{i.ID}, Duration: 1}}
		h := CognitiveHandler{Policy: behavior.ActionPolicy, Source: cognitiveFunc(func(rt.Input) (CognitiveFrame, error) { return f, nil })}
		out, e := h.Transition(s, i, rt.Clock{At: i.At}, rt.NewRandom(11, nil))
		if e != nil {
			return ActionCheckpoint{}, e
		}
		return DecodeActionCheckpoint(out.Data)
	}
	a, e := run(.7, false)
	if e != nil {
		t.Fatal(e)
	}
	b, e := run(-.7, false)
	if e != nil {
		t.Fatal(e)
	}
	if reflect.DeepEqual(a.Last.Candidates, b.Last.Candidates) || reflect.DeepEqual(a.Actors[0].Drives.Variables, b.Actors[0].Drives.Variables) {
		t.Fatal("relationship bypassed common cognition/appraisal")
	}
	if _, e = run(.7, true); e == nil {
		t.Fatal("foreign perspective reached cognition")
	}
}
