package behavior_test

import (
	"bytes"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/behavior"
	"github.com/tushardhara/dream/simulator/dynamics"
	"math"
	"testing"
)

type record struct {
	Kind     behavior.Kind     `json:"kind"`
	Actor    behavior.Actor    `json:"actor"`
	Decision behavior.Decision `json:"decision"`
	Outcome  behavior.Outcome  `json:"outcome"`
}

//go:embed testdata/legacy-actions-v1.json
var legacyActionBytes []byte

func TestFrozenLegacyActionsV1(t *testing.T) {
	verifyLegacyChoices(t, legacyActionBytes, "dd5344f2c8df187f610a59ce91e727c822beb2535d1328b1cf5951b7e109ca6a", false, false)
}
func verifyLegacyChoices(t *testing.T, raw []byte, expectedHash string, contextual, competing bool) {
	t.Helper()
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != expectedHash {
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
		if contextual {
			a.Memory = []behavior.Memory{{Other: "b", Trust: .2, Disclosure: .15, Evidence: []core.ID{"saved-history"}}}
			s.Relationships = []behavior.Memory{{Other: "b", Trust: -.1, Disclosure: .05, Evidence: []core.ID{"saved-relation"}}}
			s.Beliefs = []behavior.Belief{{Source: id, Observer: "a", Code: "uncertain", Value: .4, Confidence: .8}}
		}
		s.Offers = []behavior.Offer{o}
		draw := uint64(math.MaxUint64)
		if competing {
			alternative := behavior.Offer{Kind: behavior.Ask, Recipient: "b", Duration: 1}
			if k == behavior.Ask {
				alternative = behavior.Offer{Kind: behavior.Observe, Duration: 1}
			}
			s.Offers = append(s.Offers, alternative)
			draw /= 2
		}
		n, d, e := behavior.Choose(a, s, 1, draw)
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

//go:embed testdata/legacy-context-actions-v1.json
var legacyContextActionBytes []byte

func TestFrozenLegacyContextActionsV1(t *testing.T) {
	verifyLegacyChoices(t, legacyContextActionBytes, "2ecd36eff74bb09b0fb7b8e2cd25f4c70e5a5a6d37a5847bbfec3a9972deccf9", true, false)
}

//go:embed testdata/legacy-competing-actions-v1.json
var legacyCompetingActionBytes []byte

func TestFrozenLegacyCompetingActionsV1(t *testing.T) {
	verifyLegacyChoices(t, legacyCompetingActionBytes, "4573c0a70c00d327873b0e73a2b7d6f3697ff0a0d333fe5c1986ea26c5439ff6", true, true)
}
