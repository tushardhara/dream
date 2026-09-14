package core_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/tushardhara/dream/core"
)

func meta(id core.ID) core.Metadata {
	return core.Metadata{ID: id, Observer: "alice", Source: "alice", Sensitivity: core.Restricted, Confidence: 0.5, Valid: core.Interval{Start: 10}, RecordedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Rights: core.Rights{Resource: id}}
}
func fixture() core.State {
	evidence := core.Evidence{Meta: meta("evidence"), Description: "Fictional observation"}
	claimMeta := meta("claim")
	claimMeta.Supporting = []core.ID{"evidence"}
	return core.State{
		Version: 1, Principals: []core.Principal{{ID: "alice"}, {ID: "bob"}},
		References:    []core.NonparticipantReference{{Observer: "alice", LocalID: "neighbor"}, {Observer: "bob", LocalID: "neighbor"}},
		Evidence:      []core.Evidence{evidence},
		Events:        []core.Event{{Meta: meta("event"), Version: 1, Stream: "host-journal", Type: "observation", Subject: core.Subject{Principal: "alice"}, OccurredAt: 5}},
		Claims:        []core.Claim{{Meta: claimMeta, Subject: core.Subject{Reference: &core.NonparticipantReference{Observer: "alice", LocalID: "neighbor"}}, Proposition: "appeared relaxed"}},
		Hypotheses:    []core.Hypothesis{{Meta: meta("hypothesis"), Subject: core.Subject{Principal: "bob"}, Proposition: "may need time"}},
		Relationships: []core.Relationship{{Meta: meta("relationship"), From: core.Subject{Principal: "alice"}, To: core.Subject{Principal: "bob"}, Kind: "colleague"}},
		Groups:        []core.Group{{Meta: meta("group"), Members: []core.Subject{{Principal: "bob"}, {Principal: "alice"}}}},
		Memories:      []core.Memory{{Meta: meta("memory"), Content: "Remember fictional conversation"}},
		OpenLoops:     []core.OpenLoop{{Meta: meta("loop"), Description: "Check later"}},
		Intents:       []core.Intent{{Meta: meta("intent"), Actor: "alice", Description: "Offer support"}},
		Proposals:     []core.Proposal{{Meta: meta("proposal"), Intent: "intent", Action: "ask"}},
		Decisions:     []core.Decision{{Meta: meta("decision"), Actor: "alice", Kind: core.Act, Proposal: "proposal", Reason: "Explicit option"}, {Meta: meta("wait"), Actor: "bob", Kind: core.Wait, Reason: "Insufficient evidence"}},
		Outcomes:      []core.Outcome{{Meta: meta("outcome"), Decision: "wait", AffectedObserver: "bob", Horizon: 100, Status: core.Unknown}},
	}
}

func TestRecordsRoundTrip(t *testing.T) {
	s := fixture()
	raw, err := core.Canonical(s)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := core.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	again, err := core.Canonical(restored)
	if err != nil || !bytes.Equal(raw, again) {
		t.Fatalf("unstable roundtrip: %v", err)
	}
	if s.Groups[0].Members[0].Principal != "bob" {
		t.Fatal("canonicalization mutated input")
	}
	if bytes.Contains(raw, []byte("world")) || bytes.Contains(raw, []byte("learned_at")) {
		t.Fatal("simulation state leaked into core")
	}
}

func TestInvalidRecords(t *testing.T) {
	cases := map[string]func(*core.State){
		"version":             func(s *core.State) { s.Version = 2 },
		"ID":                  func(s *core.State) { s.Principals[0].ID = "alice/../../bob" },
		"duplicate principal": func(s *core.State) { s.Principals = append(s.Principals, s.Principals[0]) },
		"foreign reference":   func(s *core.State) { s.Claims[0].Subject.Reference.Observer = "bob" },
		"unknown reference":   func(s *core.State) { s.Claims[0].Subject.Reference.LocalID = "missing" },
		"ambiguous subject":   func(s *core.State) { s.Claims[0].Subject.Principal = "alice" },
		"unknown subject":     func(s *core.State) { s.Hypotheses[0].Subject.Principal = "missing" },
		"missing observer":    func(s *core.State) { s.Evidence[0].Meta.Observer = "missing" },
		"missing source":      func(s *core.State) { s.Evidence[0].Meta.Source = "missing" },
		"NaN":                 func(s *core.State) { s.Evidence[0].Meta.Confidence = core.Confidence(math.NaN()) },
		"infinity": func(s *core.State) {
			s.Evidence[0].Meta.Probability = &core.CalibratedProbability{Value: math.Inf(1), Calibration: "c"}
		},
		"uncalibrated":          func(s *core.State) { s.Evidence[0].Meta.Probability = &core.CalibratedProbability{Value: 0.5} },
		"invalid sensitivity":   func(s *core.State) { s.Evidence[0].Meta.Sensitivity = "maybe" },
		"negative logical time": func(s *core.State) { s.Evidence[0].Meta.Valid.Start = -1 },
		"empty interval":        func(s *core.State) { v := core.LogicalTime(10); s.Evidence[0].Meta.Valid.End = &v },
		"missing recorded time": func(s *core.State) { s.Evidence[0].Meta.RecordedAt = time.Time{} },
		"invalid recorded year": func(s *core.State) { s.Evidence[0].Meta.RecordedAt = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC) },
		"duplicate record":      func(s *core.State) { s.Evidence = append(s.Evidence, s.Evidence[0]) },
		"missing parent":        func(s *core.State) { s.Evidence[0].Meta.Parents = []core.ID{"missing"} },
		"self lineage":          func(s *core.State) { s.Evidence[0].Meta.Parents = []core.ID{"evidence"} },
		"cyclic lineage": func(s *core.State) {
			s.Evidence[0].Meta.Parents = []core.ID{"memory"}
			s.Memories[0].Meta.Parents = []core.ID{"evidence"}
		},
		"cyclic evidence":          func(s *core.State) { s.Evidence[0].Meta.Parents = []core.ID{"claim"} },
		"duplicate lineage":        func(s *core.State) { s.Memories[0].Meta.Parents = []core.ID{"evidence", "evidence"} },
		"missing evidence":         func(s *core.State) { s.Claims[0].Meta.Supporting = []core.ID{"missing"} },
		"claim without evidence":   func(s *core.State) { s.Claims[0].Meta.Supporting = nil },
		"overlapping evidence":     func(s *core.State) { s.Claims[0].Meta.Contradicting = []core.ID{"evidence"} },
		"rights mismatch":          func(s *core.State) { s.Evidence[0].Meta.Rights.Resource = "other" },
		"wrong event version":      func(s *core.State) { s.Events[0].Version = 0 },
		"negative occurrence":      func(s *core.State) { s.Events[0].OccurredAt = -1 },
		"empty text":               func(s *core.State) { s.Memories[0].Content = " " },
		"invalid UTF8":             func(s *core.State) { s.Memories[0].Content = string([]byte{255}) },
		"group duplicates":         func(s *core.State) { s.Groups[0].Members = append(s.Groups[0].Members, s.Groups[0].Members[0]) },
		"empty group":              func(s *core.State) { s.Groups[0].Members = nil },
		"loop due":                 func(s *core.State) { v := core.LogicalTime(9); s.OpenLoops[0].Due = &v },
		"intent actor":             func(s *core.State) { s.Intents[0].Actor = "missing" },
		"proposal intent":          func(s *core.State) { s.Proposals[0].Intent = "missing" },
		"decision actor":           func(s *core.State) { s.Decisions[0].Actor = "bob" },
		"decision proposal":        func(s *core.State) { s.Decisions[0].Proposal = "missing" },
		"WAIT action":              func(s *core.State) { s.Decisions[1].Proposal = "proposal" },
		"decision kind":            func(s *core.State) { s.Decisions[0].Kind = "maybe" },
		"outcome decision":         func(s *core.State) { s.Outcomes[0].Decision = "missing" },
		"outcome observer":         func(s *core.State) { s.Outcomes[0].AffectedObserver = "missing" },
		"outcome horizon":          func(s *core.State) { s.Outcomes[0].Horizon = 9 },
		"outcome status":           func(s *core.State) { s.Outcomes[0].Status = "maybe" },
		"unknown asserts response": func(s *core.State) { s.Outcomes[0].ImmediateResponse = "success" },
		"observed without evidence": func(s *core.State) {
			s.Outcomes[0].Status = core.Observed
			s.Outcomes[0].ImmediateResponse = "declined"
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			s := fixture()
			mutate(&s)
			if _, err := core.Canonical(s); err == nil {
				t.Fatal("accepted invalid state")
			}
		})
	}
}

func TestContradictoryPerspectivesAndIdentityIsolation(t *testing.T) {
	s := fixture()
	m := meta("other-claim")
	m.Observer = "bob"
	m.Contradicting = []core.ID{"evidence"}
	s.Claims = append(s.Claims, core.Claim{Meta: m, Subject: core.Subject{Reference: &core.NonparticipantReference{Observer: "bob", LocalID: "neighbor"}}, Proposition: "appeared tense"})
	raw, err := core.Canonical(s)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := core.Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(restored.Claims) != 2 || restored.Claims[0].Proposition == restored.Claims[1].Proposition {
		t.Fatal("perspectives collapsed")
	}
	a, _ := core.NewReference("alice", "neighbor")
	b, _ := core.NewReference("bob", "neighbor")
	if a == b {
		t.Fatal("observer-local identities collapsed")
	}
	if err := (core.Subject{Reference: &a}).ValidateFor("bob"); err == nil {
		t.Fatal("foreign reference accepted")
	}
}

func TestOutcomeStatuses(t *testing.T) {
	for _, status := range []core.OutcomeStatus{core.Observed, core.Unknown, core.Censored} {
		s := fixture()
		s.Outcomes[0].Status = status
		if status == core.Observed {
			s.Outcomes[0].Meta.Supporting = []core.ID{"evidence"}
			s.Outcomes[0].ImmediateResponse = "declined"
		}
		if _, err := core.Canonical(s); err != nil {
			t.Fatalf("%s: %v", status, err)
		}
	}
}

func TestLogicalHashAndOrdering(t *testing.T) {
	s := fixture()
	before, err := core.LogicalHash(s)
	if err != nil {
		t.Fatal(err)
	}
	s.Principals[0], s.Principals[1] = s.Principals[1], s.Principals[0]
	s.Groups[0].Members[0], s.Groups[0].Members[1] = s.Groups[0].Members[1], s.Groups[0].Members[0]
	s.Evidence[0].Meta.RecordedAt = s.Evidence[0].Meta.RecordedAt.Add(24 * time.Hour)
	after, err := core.LogicalHash(s)
	if err != nil || before != after {
		t.Fatal("order/wall clock changed logical hash", err)
	}
	s.Events[0].OccurredAt++
	changed, err := core.LogicalHash(s)
	if err != nil || changed == before {
		t.Fatal("logical time missing from hash", err)
	}
	s = fixture()
	s.Evidence[0].Meta.RecordedAt = s.Evidence[0].Meta.RecordedAt.In(time.FixedZone("offset", 3600))
	raw, _ := core.Canonical(s)
	utc, _ := core.Canonical(fixture())
	if !bytes.Equal(raw, utc) {
		t.Fatal("timezone not normalized")
	}
	s.Evidence[0].Meta.Confidence = core.Confidence(math.Copysign(0, -1))
	minus, _ := core.Canonical(s)
	s.Evidence[0].Meta.Confidence = 0
	plus, _ := core.Canonical(s)
	if !bytes.Equal(minus, plus) {
		t.Fatal("signed zero not normalized")
	}
}

func TestCanonicalGolden(t *testing.T) {
	s := core.State{Version: 1, Principals: []core.Principal{{ID: "alice"}}}
	const expected = `{"version":1,"principals":[{"id":"alice"}],"references":[],"evidence":[],"events":[],"claims":[],"hypotheses":[],"relationships":[],"groups":[],"memories":[],"openloops":[],"intents":[],"proposals":[],"decisions":[],"outcomes":[]}`
	raw, err := core.Canonical(s)
	if err != nil || string(raw) != expected {
		t.Fatalf("golden mismatch: %s %v", raw, err)
	}
	for _, bad := range []string{expected + " ", strings.Replace(expected, `"version":1`, `"version":1,"version":1`, 1), strings.Replace(expected, `"version":1`, `"version":1,"unknown":0`, 1), expected + expected} {
		if _, err := core.Decode([]byte(bad)); err == nil {
			t.Fatal("accepted noncanonical input")
		}
	}
}

func FuzzConstructors(f *testing.F) {
	f.Add("alice", 0.5, int64(10))
	f.Add("", math.NaN(), int64(-1))
	f.Add("bad/id", math.Inf(1), int64(0))
	f.Fuzz(func(t *testing.T, id string, confidence float64, at int64) {
		parsed, err := core.NewID(id)
		if err != nil {
			return
		}
		if parsed.Validate() != nil {
			t.Fatal("invalid constructor result")
		}
		if _, err := core.NewPrincipal(parsed); err != nil {
			t.Fatal(err)
		}
		if _, err := core.NewReference(parsed, parsed); err != nil {
			t.Fatal(err)
		}
		m := meta(parsed)
		m.Confidence = core.Confidence(confidence)
		m.Valid.Start = core.LogicalTime(at)
		s := core.State{Version: 1, Principals: []core.Principal{{ID: "alice"}}, Evidence: []core.Evidence{{Meta: m, Description: "fictional"}}}
		raw, err := core.Canonical(s)
		if err != nil {
			return
		}
		if _, err := core.Decode(raw); err != nil {
			t.Fatal(err)
		}
	})
}
func FuzzDecode(f *testing.F) {
	raw, _ := core.Canonical(fixture())
	f.Add(raw)
	f.Add([]byte(`{"version":1}`))
	f.Add([]byte("null"))
	f.Fuzz(func(t *testing.T, raw []byte) {
		s, err := core.Decode(raw)
		if err != nil {
			return
		}
		again, err := core.Canonical(s)
		if err != nil || !bytes.Equal(raw, again) {
			t.Fatalf("unstable decoded state: %v", err)
		}
	})
}
func ExampleNewReference() {
	r, err := core.NewReference("observer", "neighbor")
	fmt.Println(r.Observer, r.LocalID, err) // Output: observer neighbor <nil>
}

// Ensure the plain DTO encoding cannot launder invalid numbers into the approved codec.
func TestNaNEncoding(t *testing.T) {
	s := fixture()
	s.Evidence[0].Meta.Confidence = core.Confidence(math.NaN())
	if _, err := json.Marshal(s); err == nil {
		t.Fatal("NaN serialized")
	}
	if _, err := core.LogicalHash(s); err == nil {
		t.Fatal("NaN hashed")
	}
}

func TestProvenanceGolden(t *testing.T) {
	s := core.State{Version: 1, Principals: []core.Principal{{ID: "alice"}}, Evidence: []core.Evidence{{Meta: meta("evidence"), Description: "Fictional observation"}}}
	const expected = `{"version":1,"principals":[{"id":"alice"}],"references":[],"evidence":[{"meta":{"id":"evidence","observer":"alice","source":"alice","sensitivity":"restricted","parents":[],"supporting":[],"contradicting":[],"confidence":0.5,"valid":{"start":10},"recorded_at":"2026-01-01T00:00:00Z","rights":{"resource":"evidence","revoked":false,"grants":[]}},"description":"Fictional observation"}],"events":[],"claims":[],"hypotheses":[],"relationships":[],"groups":[],"memories":[],"openloops":[],"intents":[],"proposals":[],"decisions":[],"outcomes":[]}`
	raw, err := core.Canonical(s)
	if err != nil || string(raw) != expected {
		t.Fatalf("provenance golden mismatch: %s %v", raw, err)
	}
}

func TestIDConstructorsRejectInvalid(t *testing.T) {
	for _, id := range []string{"", "a/b", "a b", "a\n", string([]byte{255}), strings.Repeat("a", 129)} {
		if _, err := core.NewID(id); err == nil {
			t.Fatalf("accepted %q", id)
		}
		if _, err := core.NewPrincipal(core.ID(id)); err == nil {
			t.Fatal("principal bypassed ID validation")
		}
		if _, err := core.NewReference("alice", core.ID(id)); err == nil {
			t.Fatal("reference bypassed ID validation")
		}
	}
}

func TestLogicalHashGolden(t *testing.T) {
	s := core.State{Version: 1, Principals: []core.Principal{{ID: "alice"}}, Evidence: []core.Evidence{{Meta: meta("evidence"), Description: "Fictional observation"}}}
	hash, err := core.LogicalHash(s)
	if err != nil || fmt.Sprintf("%x", hash) != "adf87239a65b2a9d7d19e0beebd9e6088a70a9f4efa320fea12fc5c6532fb1f3" {
		t.Fatalf("logical hash golden mismatch: %x %v", hash, err)
	}
}
