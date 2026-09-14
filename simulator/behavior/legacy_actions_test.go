package behavior_test

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/behavior"
	"github.com/tushardhara/dream/simulator/dynamics"
	"math"
	"os"
	"testing"
)

type record struct {
	Kind     behavior.Kind     `json:"kind"`
	Actor    behavior.Actor    `json:"actor"`
	Decision behavior.Decision `json:"decision"`
	Outcome  behavior.Outcome  `json:"outcome"`
}

func TestFrozenLegacyActionsV1(t *testing.T) {
	raw, e := os.ReadFile("testdata/legacy-actions-v1.json")
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "dd5344f2c8df187f610a59ce91e727c822beb2535d1328b1cf5951b7e109ca6a" {
		t.Fatal("legacy fixture modified")
	}
	all := []record{}
	for _, k := range behavior.Registry() {
		state, e := dynamics.New("a", 0, dynamics.DefaultSubstrate())
		if e != nil {
			t.Fatal(e)
		}
		a := behavior.Actor{State: state, Memory: []behavior.Memory{}, Beliefs: []behavior.Belief{}}
		id := core.ID("saved-action:" + string(k))
		p := dynamics.Perceived{Event: id, Actor: "a", OccurredAt: 1, LearnedAt: 1, Confidence: .8, Signals: dynamics.Signals{OtherNeed: .7}, Rights: core.Rights{Resource: id, Grants: []core.Grant{{Actor: "a", Recipient: "a", Purpose: "simulation", Operation: core.Read}, {Actor: "a", Recipient: "a", Purpose: "simulation", Operation: core.Derive}}}}
		o := behavior.Offer{Kind: k, Duration: 1}
		if k != behavior.Wait && k != behavior.Observe {
			o.Recipient = "b"
		}
		s := behavior.Situation{Perceived: p, Horizon: 20, Present: []core.ID{"a", "b"}, Resources: map[core.ID]int64{"time": 2}, DisclosureRecipient: "b"}
		if k == behavior.Help {
			o.Resource = "time"
			o.Units = 1
		}
		if k == behavior.BreakPromise {
			o.Commitment = "promise"
			s.Commitments = []behavior.Commitment{{ID: "promise", Actor: "a", Recipient: "b", Resource: "time", Units: 1, Due: 10, Status: "pending"}}
		}
		s.Offers = []behavior.Offer{o}
		n, d, e := behavior.Choose(a, s, 1, math.MaxUint64)
		if e != nil {
			t.Fatal(e)
		}
		out, e := behavior.Track(d, 20)
		if e != nil {
			t.Fatal(e)
		}
		all = append(all, record{k, n, d, out})
	}
	actual, e := json.Marshal(all)
	if e != nil || !bytes.Equal(actual, raw) {
		t.Fatal("v1 decision/state/outcome semantics changed", e)
	}
	var decoded []record
	if json.Unmarshal(raw, &decoded) != nil {
		t.Fatal("legacy decode")
	}
	for _, r := range decoded {
		if r.Actor.Validate() != nil || r.Decision.Validate() != nil || r.Outcome.Validate() != nil {
			t.Fatal("saved v1 record rejected")
		}
	}
}
