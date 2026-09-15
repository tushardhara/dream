package main

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/evals"
)

// A later report that is UNKNOWN must not serialize identically to an observed
// one. Exporting only provenance metadata lost each measurement's value and
// status, so a report consumer could not audit the data a comparison used.
// Reproduces the reviewer's case on the real consumer path.
func TestSerializedReportPreservesEachMeasurement(t *testing.T) {
	cs, e := realComparisons(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	var target *evals.TierEvidence
	var srcID core.ID
	var ci0 int
	for ci := range cs {
		for ri := range cs[ci].Runs {
			for oi := range cs[ci].Runs[ri].Outcomes {
				obs := cs[ci].Runs[ri].Outcomes[oi].Observations
				for i := range obs {
					if obs[i].Tier == evals.AttributedLater {
						target, srcID, ci0 = &obs[i], obs[i].Source, ci
					}
				}
			}
		}
	}
	if target == nil {
		t.Fatal("positive control needs a real later observation")
	}
	a, e := evals.SummariseUplift(cs)
	if e != nil {
		t.Fatal("positive control:", e)
	}
	before, _ := json.Marshal(a.People)
	// A legitimate different input: the report itself is unknown, so the
	// observation AND the record it came from both carry unknown.
	target.Value = core.UnknownGroupQuantity()
	for i := range cs[ci0].Sources {
		if cs[ci0].Sources[i].ID == srcID {
			cs[ci0].Sources[i].Value = core.UnknownGroupQuantity()
		}
	}
	b, e := evals.SummariseUplift(cs)
	if e != nil {
		t.Fatal("unknown report input:", e)
	}
	after, _ := json.Marshal(b.People)
	if bytes.Equal(before, after) {
		t.Fatal("per_person JSON identical when a real later observation becomes UNKNOWN; measured value/status lost")
	}
}
