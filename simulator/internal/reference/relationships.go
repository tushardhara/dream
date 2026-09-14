// Package reference holds public synthetic inputs shared by engine tests and probes.
// It contains no evaluator labels, expected scores or runtime filesystem access.
package reference

import _ "embed"

//go:embed relationship-comparisons.v1.json
var relationships []byte

// Relationships returns an independent copy of the frozen reference inputs.
func Relationships() []byte { return append([]byte(nil), relationships...) }
