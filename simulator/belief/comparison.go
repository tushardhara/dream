// Package belief contains synthetic research comparisons, never actor retrieval.
package belief

import (
	"fmt"
	"github.com/tushardhara/dream/core"
	"math"
)

// Actual is an explicitly supplied synthetic research measurement, not an
// assertion about a real person. The host retains its restricted provenance.
type Actual struct {
	Subject  core.Subject
	Evidence core.ID
	Value    float64
}
type Believed struct {
	Hypothesis core.Hypothesis
	Estimate   *float64
}
type Comparison struct {
	Confidence     core.Confidence
	Observer       core.ID
	Hypothesis     core.ID
	ActualEvidence core.ID
	Status         string
	Difference     *float64
}

// Compare returns belief-minus-synthetic-actual. Missing belief evidence/estimate
// remains unknown; it cannot turn another observer's hypothesis into truth. This
// research-only output must never be included in an actor/model view (#12).
func Compare(actual Actual, believed Believed) (Comparison, error) {
	h := believed.Hypothesis
	if err := h.Validate(); err != nil {
		return Comparison{}, err
	}
	if actual.Evidence.Validate() != nil || actual.Subject.ValidateFor(h.Meta.Observer) != nil || !unit(actual.Value) {
		return Comparison{}, fmt.Errorf("invalid synthetic actual")
	}
	same := actual.Subject.Principal == h.Subject.Principal
	if actual.Subject.Reference != nil || h.Subject.Reference != nil {
		same = actual.Subject.Reference != nil && h.Subject.Reference != nil && *actual.Subject.Reference == *h.Subject.Reference
	}
	if !same {
		return Comparison{}, fmt.Errorf("comparison subject mismatch")
	}
	out := Comparison{Confidence: h.Meta.Confidence, Observer: h.Meta.Observer, Hypothesis: h.Meta.ID, ActualEvidence: actual.Evidence, Status: "unknown"}
	if believed.Estimate != nil && !unit(*believed.Estimate) {
		return Comparison{}, fmt.Errorf("invalid belief estimate")
	}
	if believed.Estimate == nil || h.Meta.Rights.Revoked || len(h.Meta.Supporting) == 0 {
		return out, nil
	}
	delta := *believed.Estimate - actual.Value
	out.Status = "compared"
	out.Difference = &delta
	return out, nil
}
func unit(v float64) bool { return !math.IsNaN(v) && !math.IsInf(v, 0) && v >= 0 && v <= 1 }
