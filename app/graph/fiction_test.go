package graph

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

func TestTypedFictionModesAndAssistantBoundary(t *testing.T) {
	// Distinct semantic contracts: substituting one mode's content for another
	// must fail even when both are nonempty and carry valid provenance.
	expected := map[string]string{
		"full":         "I feel very worried.",
		"partial":      "I've been having a difficult time.",
		"softened":     "I feel a little worried, though I'm not ready to say more.",
		"joke":         "My feeling-worried meter is working overtime.",
		"deflection":   "I'd rather talk about something else.",
		"lie":          "I do not feel worried.",
		"omission":     "There is more to this, but I am keeping part private.",
		"topic_change": "Let's change the topic.",
		"silence":      "",
	}
	for _, mode := range []string{"full", "partial", "softened", "joke", "deflection", "lie", "omission", "topic_change", "silence"} {
		t.Run(mode, func(t *testing.T) {
			f, p := policyFixture()
			b, _ := json.Marshal(FictionExperience{Version: 1, Actor: "alice", Feeling: "worried", Intensity: .9})
			f.entries[0].Content.Text = string(b)
			policy := newTestPolicy(f, testAuthority{fictional: true})
			cap, d, e := policy.Approve(context.Background(), p)
			if e != nil || !d.Allowed {
				t.Fatal(d, e)
			}
			writer := writerFunc(func(context.Context, SafeContext) (WriterDraft, error) {
				return WriterDraft{Fiction: &FictionDraft{Version: 1, Source: "own", Mode: mode}}, nil
			})
			out, d, e := policy.Write(context.Background(), cap, p.Binding, writer)
			if e != nil || !d.Allowed || out.FictionMode != mode || len(out.Sources) != 1 || out.Sources[0] != "own" || len(out.Evidence) != 1 {
				t.Fatal(out, d, e)
			}
			if out.Text != expected[mode] {
				t.Fatalf("%s semantics: got %q, want %q", mode, out.Text, expected[mode])
			}
			if mode == "silence" {
				if out.Text != "" {
					t.Fatal("silence delivered content")
				}
			} else if out.Text == "" || strings.Contains(string(b), out.Text) {
				t.Fatal("mode not a typed transform", out)
			}
			if mode == "full" && out.Text != "I feel very worried." {
				t.Fatal("full semantics")
			}
			if mode == "partial" && strings.Contains(out.Text, "worried") {
				t.Fatal("partial revealed omitted detail")
			}
			if mode == "lie" && out.Text != "I do not feel worried." {
				t.Fatal("fictional lie semantics")
			}
			p.Mode = AssistantDisclosure
			cap, d, e = policy.Approve(context.Background(), p)
			if e != nil || !d.Allowed {
				t.Fatal(d, e)
			}
			if _, d, e = policy.Write(context.Background(), cap, p.Binding, writer); e != nil || d.Allowed {
				t.Fatal("fictional transform granted assistant deception", d, e)
			}
		})
	}
}
func TestFictionSourceValidationAndRevocation(t *testing.T) {
	for _, name := range []string{"unstructured", "foreign_actor", "unknown_feeling", "unknown_mode", "foreign_source", "mixed", "revoked_during_writer"} {
		t.Run(name, func(t *testing.T) {
			f, p := policyFixture()
			experience := FictionExperience{1, "alice", "tired", .8}
			if name == "foreign_actor" {
				experience.Actor = "bob"
			}
			if name == "unknown_feeling" {
				experience.Feeling = "unrestricted private prose"
			}
			b, _ := json.Marshal(experience)
			f.entries[0].Content.Text = string(b)
			if name == "unstructured" {
				f.entries[0].Content.Text = "unstructured raw provider prose"
			}
			policy := newTestPolicy(f, testAuthority{fictional: true})
			cap, d, e := policy.Approve(context.Background(), p)
			if e != nil || !d.Allowed {
				t.Fatal(d, e)
			}
			writer := writerFunc(func(context.Context, SafeContext) (WriterDraft, error) {
				draft := WriterDraft{Fiction: &FictionDraft{1, "own", "full"}}
				switch name {
				case "unknown_mode":
					draft.Fiction.Mode = "execute_shell"
				case "foreign_source":
					draft.Fiction.Source = "foreign"
				case "mixed":
					draft.Spans = []WriterSpan{{Source: "own", End: 1}}
				case "revoked_during_writer":
					f.entries[0].Revoked = true
				}
				return draft, nil
			})
			if out, d, e := policy.Write(context.Background(), cap, p.Binding, writer); d.Allowed || out.Text != "" {
				t.Fatal("unsafe fictional output", out, d, e)
			}
		})
	}
}
