package hws

import (
	"context"
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/core"
)

func modelInputFixture() ProviderInput {
	return ProviderInput{Version: 1, Key: "key", Capability: ModelAppraisal, Versions: ModelVersions{Schema: "model.v1", Capability: "cognition.v1", Model: "fake", Prompt: "cognition.v1", Policy: "policy.v1"}, Actor: "alice", MaxOutputTokens: 100, Context: []graph.SafeContextItem{{Source: "own", Observer: "alice", Subject: core.Subject{Principal: "alice"}, Kind: graph.EpisodicMemory, Confidence: .7, Text: "Own fictional memory"}}}
}
func modelOutputFixture() ModelOutput {
	return ModelOutput{Version: 1, Capability: ModelAppraisal, Observer: "alice", Findings: []ModelFinding{{Code: "care", Confidence: .5, Evidence: []core.ID{"own"}}}}
}
func TestModelTypedOutputNegatives(t *testing.T) {
	input := modelInputFixture()
	good := modelOutputFixture()
	raw, _ := json.Marshal(good)
	if _, e := DecodeModelOutput(raw, input); e != nil {
		t.Fatal(e)
	}
	for name, mutate := range map[string]func(*ModelOutput){
		"version": func(o *ModelOutput) { o.Version = 2 }, "capability": func(o *ModelOutput) { o.Capability = ModelCandidates }, "observer": func(o *ModelOutput) { o.Observer = "bob" }, "empty": func(o *ModelOutput) { o.Findings = nil }, "enum": func(o *ModelOutput) { o.Findings[0].Code = "execute" }, "confidence": func(o *ModelOutput) { o.Findings[0].Confidence = 1.1 }, "nan": func(o *ModelOutput) { o.Findings[0].Value = math.NaN() }, "bound": func(o *ModelOutput) { o.Findings[0].Value = 2 }, "source": func(o *ModelOutput) { o.Findings[0].Evidence = []core.ID{"god-state"} }, "duplicate": func(o *ModelOutput) { o.Findings = append(o.Findings, o.Findings[0]) }, "no-provenance": func(o *ModelOutput) { o.Findings[0].Evidence = nil },
	} {
		t.Run(name, func(t *testing.T) {
			o := modelOutputFixture()
			mutate(&o)
			if o.Validate(input) == nil {
				t.Fatal("invalid output accepted")
			}
		})
	}
	for _, bad := range [][]byte{append(append([]byte{}, raw...), raw...), []byte(strings.Replace(string(raw), `"version":1`, `"version":1,"tool":"leak"`, 1)), []byte(strings.Repeat("x", 16385))} {
		if _, e := DecodeModelOutput(bad, input); e == nil {
			t.Fatal("malformed body accepted")
		}
	}
	for _, field := range []string{"schema", "capability", "model", "prompt", "policy"} {
		i := input
		switch field {
		case "schema":
			i.Versions.Schema = "model.v2"
		case "capability":
			i.Versions.Capability = "unknown"
		case "model":
			i.Versions.Model = ""
		case "prompt":
			i.Versions.Prompt = "user-provided-prompt"
		case "policy":
			i.Versions.Policy = "allow-all"
		}
		if i.Validate() == nil {
			t.Fatal("unknown version accepted", field)
		}
	}
}
func TestCognitionCannotReuseDisclosureCapability(t *testing.T) {
	runtime, journal, realm, grants := viewFixture(t)
	grants[0].Operations = append(grants[0].Operations, core.Derive)
	journal.entries[0].Event.Meta.Rights.Grants = append(journal.entries[0].Event.Meta.Rights.Grants, core.Grant{Actor: "alice", Recipient: "alice", Purpose: "simulation", Operation: core.Derive})
	views, e := NewViewService(runtime, journal, viewClock{}, grants, viewAudit{})
	if e != nil {
		t.Fatal(e)
	}
	permit, e := views.Permit("alice", realm, ActorViewKind, "simulation")
	if e != nil {
		t.Fatal(e)
	}
	disclose, d, e := views.Propose(context.Background(), permit, []core.ID{"permitted"}, "bob", core.Disclose, graph.SyntheticSelfDisclosure)
	if e != nil || !d.Allowed {
		t.Fatal(d, e)
	}
	if _, e = views.ModelContext(context.Background(), permit, disclose, realm.Scope, "alice"); e == nil {
		t.Fatal("disclosure repurposed for cognition")
	}
	approved, d, e := views.Propose(context.Background(), permit, []core.ID{"permitted"}, "alice", core.Derive, graph.InternalContext)
	if e != nil || !d.Allowed {
		t.Fatal(d, e)
	}
	safe, e := views.ModelContext(context.Background(), permit, approved, realm.Scope, "alice")
	if e != nil || len(safe.Items()) != 1 || len(safe.Revision()) != 64 || safe.KnownAt() != runtime.snapshot.State.At {
		t.Fatal("valid cognition blocked", e)
	}
	views.Revoke(permit)
	if _, e = views.ModelContext(context.Background(), permit, approved, realm.Scope, "alice"); e == nil {
		t.Fatal("revoked cognition scope usable")
	}
}
func FuzzDecodeModelOutput(f *testing.F) {
	raw, _ := json.Marshal(modelOutputFixture())
	f.Add(raw)
	f.Add([]byte(`{"version":1,"findings":[]}`))
	f.Fuzz(func(t *testing.T, b []byte) {
		o, e := DecodeModelOutput(b, modelInputFixture())
		if e == nil && o.Validate(modelInputFixture()) != nil {
			t.Fatal("decoder admitted invalid result")
		}
	})
}
