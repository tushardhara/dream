package belief

import (
	"github.com/tushardhara/dream/core"
	"math"
	"testing"
	"time"
)

func TestObserverAttributedComparison(t *testing.T) {
	h := core.Hypothesis{Meta: core.Metadata{ID: "belief", Observer: "alice", Source: "alice", Sensitivity: core.Restricted, Supporting: []core.ID{"perception"}, Confidence: .4, Valid: core.Interval{}, RecordedAt: time.Unix(1, 0), Rights: core.Rights{Resource: "belief"}}, Subject: core.Subject{Principal: "bob"}, Proposition: "synthetic hypothesis"}
	estimate := .8
	actual := Actual{h.Subject, "research", .2}
	out, err := Compare(actual, Believed{h, &estimate})
	if err != nil || out.Observer != "alice" || out.Status != "compared" || math.Abs(*out.Difference-.6) > 1e-12 {
		t.Fatal(out, err)
	}
	h.Meta.Supporting = nil
	out, err = Compare(actual, Believed{h, &estimate})
	if err != nil || out.Status != "unknown" || out.Difference != nil {
		t.Fatal(out, err)
	}
	h.Subject = core.Subject{Reference: &core.NonparticipantReference{Observer: "bob", LocalID: "same-name"}}
	if _, err = Compare(actual, Believed{h, &estimate}); err == nil {
		t.Fatal("foreign observer reference accepted")
	}
	h.Subject = core.Subject{Principal: "alice"}
	if _, err = Compare(actual, Believed{h, &estimate}); err == nil {
		t.Fatal("subject mismatch accepted")
	}
	h.Subject = actual.Subject
	estimate = math.NaN()
	if _, err = Compare(actual, Believed{h, &estimate}); err == nil {
		t.Fatal("nonfinite estimate accepted")
	}
}
