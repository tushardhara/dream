// Package runtime owns deterministic virtual-time transitions, not leases or I/O.
package runtime

import (
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"github.com/tushardhara/dream/core"
	"math"
)

const RNGVersion = "sha256-counter.v1"

type Draw struct {
	Stream   core.ID `json:"stream"`
	Position uint64  `json:"position"`
	Value    uint64  `json:"value"`
}
type Random struct {
	seed      uint64
	positions map[core.ID]uint64
	draws     []Draw
}

func NewRandom(seed uint64, positions map[core.ID]uint64) *Random {
	p := map[core.ID]uint64{}
	for k, v := range positions {
		p[k] = v
	}
	return &Random{seed: seed, positions: p, draws: []Draw{}}
}

// Draw is domain-separated and reproducible across Go versions. This is a
// simulator PRNG, not a cryptographic token generator or provider seed guarantee.
func (r *Random) Draw(stream core.ID) (uint64, error) {
	if stream.Validate() != nil || len(r.draws) >= 1024 || r.positions[stream] == math.MaxUint64 {
		return 0, fmt.Errorf("invalid stream or RNG budget exhausted")
	}
	if _, ok := r.positions[stream]; !ok && len(r.positions) >= 64 {
		return 0, fmt.Errorf("RNG stream budget exhausted")
	}
	pos := r.positions[stream]
	b := append([]byte(RNGVersion+"\x00"+string(stream)+"\x00"), make([]byte, 16)...)
	binary.BigEndian.PutUint64(b[len(b)-16:], r.seed)
	binary.BigEndian.PutUint64(b[len(b)-8:], pos)
	hash := sha256.Sum256(b)
	value := binary.BigEndian.Uint64(hash[:8])
	r.positions[stream] = pos + 1
	r.draws = append(r.draws, Draw{stream, pos, value})
	return value, nil
}

type Clock struct{ At core.LogicalTime }

func (c Clock) Now() core.LogicalTime { return c.At }
