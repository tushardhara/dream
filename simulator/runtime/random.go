// Package runtime owns deterministic virtual-time transitions, not leases or I/O.
package runtime

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"github.com/tushardhara/dream/core"
	"math"
	"strings"
)

const RNGVersion = "sha256-counter.v1"

type Draw struct {
	Stream   core.ID `json:"stream"`
	Position uint64  `json:"position"`
	Value    uint64  `json:"value"`
}
type Random struct {
	domain    core.ID
	common    core.ID
	coupled   bool
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
	if stream.Validate() != nil {
		return 0, fmt.Errorf("invalid RNG stream")
	}
	domain := r.domain
	if r.coupled && strings.HasPrefix(string(stream), "exogenous:") {
		domain = r.common
	}
	if domain != "" {
		sum := sha256.Sum256([]byte(string(domain) + "\x00" + string(stream)))
		stream = core.ID("branch:" + hex.EncodeToString(sum[:]))
	}
	return r.drawRaw(stream)
}
func NewScopedRandom(seed uint64, positions map[core.ID]uint64, domain, common core.ID, coupled bool) *Random {
	r := NewRandom(seed, positions)
	r.domain = domain
	r.common = common
	r.coupled = coupled
	return r
}
func (r *Random) drawRaw(stream core.ID) (uint64, error) {
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

// ReplayDraw verifies an exact persisted draw, including its stream position.
// It cannot mint a value supplied by a recorder with a different seed/position.
func (r *Random) ReplayDraw(expected Draw) error {
	if r.positions[expected.Stream] != expected.Position {
		return fmt.Errorf("recorded RNG position mismatch")
	}
	value, err := r.drawRaw(expected.Stream)
	if err != nil {
		return err
	}
	if value != expected.Value {
		return fmt.Errorf("recorded RNG value mismatch")
	}
	return nil
}
