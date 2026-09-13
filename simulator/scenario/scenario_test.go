package scenario

import (
	"bytes"
	"encoding/json"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator"
	"math"
	"math/rand"
	"strings"
	"testing"
)

func fixture() Scenario {
	f := func(id, owner core.ID) Fact {
		return Fact{ID: id, Observer: owner, Subject: owner, Text: string(id), Confidence: 1, Valid: core.Interval{Start: 0}, Grants: []core.Grant{{Actor: owner, Recipient: owner, Purpose: "simulation", Operation: core.Read}}}
	}
	return Scenario{Version: 1, World: World{ID: "test", Seed: 9007199254740993, Horizon: 100}, Requires: []core.ID{}, Public: Public{Humans: []Human{{"a", "Synthetic A", 30}, {"b", "Synthetic B", 40}}, Resources: []Resource{{"room", 2, 2}}}, Actors: []Actor{{ID: "a", Facts: []Fact{f("a-note", "a")}, Knowledge: []Knowledge{{"a-note", 0}}, Memories: []Memory{{"a-memory", "a-note", "self memory"}}, Relationships: []Relationship{{"a-b", "b", "knows", "a-note"}}}, {ID: "b", Facts: []Fact{f("SECRET_B", "b")}, Knowledge: []Knowledge{{"SECRET_B", 0}}}}, Research: Research{Latent: []Latent{{"b", simulator.Emotion{Valence: 0, Arousal: 0.5}, []simulator.Drive{{Kind: "SECRET_LATENT", Strength: 0.5}}}}, Labels: []Label{{"label", "SECRET_LABEL"}}}, Future: []Scheduled{{ID: "later", At: 50, Kind: "observation", Actor: "a", Text: "SECRET_FUTURE"}}}
}
func engine() Engine {
	return Engine{1, []core.ID{"genesis.v1", "resources.v1", "memory.v1", "relationships.v1", "latent.v1", "schedule.v1"}}
}
func TestCanonicalProperties(t *testing.T) {
	s := fixture()
	want, err := s.Canonical()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(want, []byte("9007199254740993")) {
		t.Fatal("lost seed precision")
	}
	rng := rand.New(rand.NewSource(5))
	for range 100 {
		s := fixture()
		rng.Shuffle(len(s.Public.Humans), func(i, j int) { s.Public.Humans[i], s.Public.Humans[j] = s.Public.Humans[j], s.Public.Humans[i] })
		rng.Shuffle(len(s.Actors), func(i, j int) { s.Actors[i], s.Actors[j] = s.Actors[j], s.Actors[i] })
		s.Public.Groups = []Group{}
		s.Research.Latent[0].Emotion.Valence = math.Copysign(0, -1)
		got, err := s.Canonical()
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("noncanonical permutation: %v", err)
		}
	}
	before, _ := json.Marshal(s)
	_, _ = s.Canonical()
	after, _ := json.Marshal(s)
	if !bytes.Equal(before, after) {
		t.Fatal("canonical mutated source")
	}
	hash, _ := s.Hash()
	s.World.Seed++
	other, _ := s.Hash()
	if hash == other {
		t.Fatal("seed omitted from hash")
	}
	s = fixture()
	s.Future = append(s.Future, Scheduled{ID: "early", At: 1, Kind: "observation", Actor: "b", Text: "first"}, Scheduled{ID: "aaa", At: 50, Kind: "observation", Actor: "b", Text: "tie"})
	b, _ := s.Canonical()
	var c Scenario
	_ = json.Unmarshal(b, &c)
	if c.Future[0].ID != "early" || c.Future[1].ID != "aaa" {
		t.Fatal("schedule ordering")
	}
}
func TestActorViewBoundary(t *testing.T) {
	s := fixture()
	v, err := s.View("a")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(v)
	for _, secret := range []string{"SECRET_", "seed", "horizon", "scenario_hash", "future", "research", "latent"} {
		if bytes.Contains(b, []byte(secret)) {
			t.Fatalf("leaked %s", secret)
		}
	}
	if len(v.Facts) != 1 || len(v.Memories) != 1 || len(v.Relationships) != 1 {
		t.Fatal("lost self state")
	}
	v.Humans[0].Name = "mutated"
	v.Facts[0].Grants[0].Purpose = "mutated"
	if s.Public.Humans[0].Name == "mutated" || s.Actors[0].Facts[0].Grants[0].Purpose == "mutated" {
		t.Fatal("view aliases source")
	}
	if _, err = s.View("unknown"); err == nil {
		t.Fatal("unknown actor")
	}
	s.Actors[0].Knowledge = append(s.Actors[0].Knowledge, Knowledge{"SECRET_B", 0})
	if _, err = s.View("a"); err == nil {
		t.Fatal("foreign knowledge accepted")
	}
}
func TestInvalidScenarios(t *testing.T) {
	cases := map[string]func(*Scenario){
		"version": func(s *Scenario) { s.Version = 2 }, "world": func(s *Scenario) { s.World.ID = "bad id" }, "horizon": func(s *Scenario) { s.World.Horizon = 0 }, "minor": func(s *Scenario) { s.Public.Humans[0].Age = 17 }, "duplicate": func(s *Scenario) { s.Public.Resources[0].ID = "a" }, "group ref": func(s *Scenario) { s.Public.Groups = []Group{{"g", []core.ID{"absent"}}} }, "duplicate member": func(s *Scenario) { s.Public.Groups = []Group{{"g", []core.ID{"a", "a"}}} }, "capacity": func(s *Scenario) { s.Public.Resources[0].Available = 3 }, "owner": func(s *Scenario) { s.Actors[0].Facts[0].Observer = "b" }, "subject": func(s *Scenario) { s.Actors[0].Facts[0].Subject = "missing" }, "interval": func(s *Scenario) { end := core.LogicalTime(101); s.Actors[0].Facts[0].Valid.End = &end }, "future fact": func(s *Scenario) { s.Actors[0].Facts[0].Valid.Start = 1 }, "confidence": func(s *Scenario) { s.Actors[0].Facts[0].Confidence = core.Confidence(math.NaN()) }, "permission": func(s *Scenario) { s.Actors[0].Facts[0].Grants[0].Operation = "wildcard" }, "read required": func(s *Scenario) { s.Actors[0].Facts[0].Grants = nil }, "private sharing": func(s *Scenario) {
			s.Actors[0].Facts[0].Grants = append(s.Actors[0].Facts[0].Grants, core.Grant{Actor: "b", Recipient: "b", Purpose: "simulation", Operation: core.Read})
		}, "actor count": func(s *Scenario) { s.Actors = s.Actors[:1] }, "learned": func(s *Scenario) { s.Actors[0].Knowledge[0].LearnedAt = 1 }, "memory evidence": func(s *Scenario) { s.Actors[0].Memories[0].Evidence = "SECRET_B" }, "relationship evidence": func(s *Scenario) { s.Actors[0].Relationships[0].Evidence = "later" }, "latent": func(s *Scenario) { s.Research.Latent[0].Emotion.Arousal = 2 }, "event at zero": func(s *Scenario) { s.Future[0].At = 0 }, "event horizon": func(s *Scenario) { s.Future[0].At = 100 }, "event kind": func(s *Scenario) { s.Future[0].Kind = "execute-code" }, "observation resource": func(s *Scenario) { s.Future[0].Units = 1 }, "capabilities": func(s *Scenario) { s.Requires = []core.ID{"x", "x"} }, "huge text": func(s *Scenario) { s.Research.Labels[0].Text = strings.Repeat("x", 4097) },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			s := fixture()
			mutate(&s)
			if err := s.Validate(); err == nil {
				t.Fatal("accepted invalid scenario")
			} else if _, ok := err.(*Error); !ok {
				t.Fatal("missing path error")
			}
		})
	}
}
func TestResourceIntervalsAndOverflow(t *testing.T) {
	reservation := func(id core.ID, at, until core.LogicalTime, units int64) Scheduled {
		return Scheduled{ID: id, At: at, Until: until, Units: units, Resource: "room", Actor: "a", Kind: "reservation", Text: "booking"}
	}
	s := fixture()
	s.Future = []Scheduled{reservation("r1", 1, 10, 2), reservation("r2", 10, 20, 2)}
	if err := s.Validate(); err != nil {
		t.Fatal(err)
	}
	s.Future[1].At = 9
	if s.Validate() == nil {
		t.Fatal("overbooking accepted")
	}
	s.Public.Resources[0].Capacity = math.MaxInt64
	s.Public.Resources[0].Available = math.MaxInt64
	s.Future[0].Units = math.MaxInt64
	s.Future[1].Units = math.MaxInt64
	if s.Validate() == nil {
		t.Fatal("overflow accepted")
	}
	s.Future[1].At = 10
	if err := s.Validate(); err != nil {
		t.Fatal("adjacent full capacity", err)
	}
}
func TestGenesisAndCapabilities(t *testing.T) {
	s := fixture()
	g, err := s.Genesis(engine())
	if err != nil {
		t.Fatal(err)
	}
	if err = g.Validate(engine()); err != nil {
		t.Fatal(err)
	}
	for i := range engine().Capabilities {
		e := engine()
		e.Capabilities = append(e.Capabilities[:i], e.Capabilities[i+1:]...)
		if _, err := s.Genesis(e); err == nil {
			t.Fatal("missing inferred capability accepted")
		}
	}
	if s.CheckExecution(Engine{ScenarioVersion: 2}) == nil {
		t.Fatal("wrong engine version")
	}
	s.Requires = []core.ID{"unimplemented.v9"}
	if s.CheckExecution(engine()) == nil {
		t.Fatal("unknown capability accepted")
	}
	for _, mutate := range []func(*Genesis){func(g *Genesis) { g.Version = 2 }, func(g *Genesis) { g.At = 1 }, func(g *Genesis) { g.World = "other" }, func(g *Genesis) { g.ScenarioHash = "bad" }, func(g *Genesis) { g.Payload = append(g.Payload, ' ') }, func(g *Genesis) { g.Payload = []byte(`{}`) }} {
		g, _ := fixture().Genesis(engine())
		mutate(&g)
		if g.Validate(engine()) == nil {
			t.Fatal("tampered genesis accepted")
		}
	}
}
func FuzzCanonical(f *testing.F) {
	f.Add(uint64(0), int64(50))
	f.Add(uint64(math.MaxUint64), int64(math.MaxInt64))
	f.Fuzz(func(t *testing.T, seed uint64, at int64) {
		s := fixture()
		s.World.Seed = seed
		s.Future[0].At = core.LogicalTime(at)
		b, err := s.Canonical()
		if err != nil {
			return
		}
		var decoded Scenario
		if err = json.Unmarshal(b, &decoded); err != nil {
			t.Fatal(err)
		}
		again, err := decoded.Canonical()
		if err != nil || !bytes.Equal(b, again) {
			t.Fatal("unstable canonical")
		}
		if _, err = s.View("a"); err != nil {
			t.Fatal(err)
		}
	})
}

func TestPublicIsNotAutomaticallyKnown(t *testing.T) {
	s := fixture()
	s.Public.Facts = []Fact{{ID: "not-known", Observer: "b", Subject: "b", Text: "PUBLIC_UNLEARNED_CANARY", Confidence: 1, Valid: core.Interval{Start: 0}, Grants: []core.Grant{{Actor: "b", Recipient: "b", Purpose: "simulation", Operation: core.Read}}}}
	v, err := s.View("a")
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(v)
	if bytes.Contains(b, []byte("PUBLIC_UNLEARNED_CANARY")) {
		t.Fatal("public fact bypassed knowledge")
	}
	s.Actors[0].Knowledge = append(s.Actors[0].Knowledge, Knowledge{"not-known", 0})
	if s.Validate() == nil {
		t.Fatal("public fact bypassed read grant")
	}
}
func TestHashCommitsHiddenInputs(t *testing.T) {
	original, _ := fixture().Hash()
	for _, mutate := range []func(*Scenario){func(s *Scenario) { s.Research.Labels[0].Text = "different" }, func(s *Scenario) { s.Future[0].Text = "different" }, func(s *Scenario) { s.Actors[1].Facts[0].Text = "different" }, func(s *Scenario) { s.Research.Latent[0].Emotion.Valence = 0.5 }, func(s *Scenario) { s.Requires = []core.ID{"extra.v1"} }} {
		s := fixture()
		mutate(&s)
		hash, err := s.Hash()
		if err != nil || hash == original {
			t.Fatalf("hidden input not committed: %v", err)
		}
	}
}
func TestProgrammaticBudgets(t *testing.T) {
	s := fixture()
	s.Actors[0].Knowledge = make([]Knowledge, MaxItems+1)
	if s.Validate() == nil {
		t.Fatal("programmatic item budget bypass")
	}
}

func TestForeignKnowledgeOwnershipGuard(t *testing.T) {
	s := fixture()
	s.Actors[0].Knowledge = append(s.Actors[0].Knowledge, Knowledge{"SECRET_B", 0})
	err := s.Validate()
	e, ok := err.(*Error)
	if !ok || e.Code != "invalid" || e.Path != "$.actors[0].knowledge[1].record" || e.Message != "unknown or foreign private record" {
		t.Fatalf("ownership guard: %v", err)
	}
}
