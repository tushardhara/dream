package assistanceclient

import (
	"context"
	"testing"

	"github.com/tushardhara/dream/app/assistance"
	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/core"
)

func domainProfiles() []core.RelationshipContext {
	out := []core.RelationshipContext{}
	for _, owner := range []core.ID{"alice", "bob"} {
		other := core.ID("alice")
		if owner == "alice" {
			other = "bob"
		}
		out = append(out, DomainProfile(owner, other, core.ID(string(owner)+"-care"), core.Childcare, "family", .8, .6), DomainProfile(owner, other, core.ID(string(owner)+"-business"), core.Childcare, "business", -.7, -.6))
	}
	return out
}
func domainFixture(t *testing.T, arm assistance.Arm, focus core.RelationshipFocus, profiles []core.RelationshipContext) (*Local, assistance.Request) {
	t.Helper()
	l, r, e := DomainFixture(arm, focus, profiles)
	if e != nil {
		t.Fatal(e)
	}
	consent(t, l, r)
	return l, r
}
func TestDomainHelperActuallyUsesFrameDomainAndSeparateAccounts(t *testing.T) {
	for _, name := range []string{"family", "business", "ambiguous", "finance", "private", "labels", "opposed", "uncertain"} {
		t.Run(name, func(t *testing.T) {
			profiles := domainProfiles()
			focus := core.RelationshipFocus{Version: core.RelationshipFocusVersion, Domain: core.Childcare, RoleContext: "family"}
			switch name {
			case "business":
				focus.RoleContext = "business"
			case "ambiguous":
				focus.RoleContext = ""
			case "finance":
				focus.Domain = core.Finances
			case "private":
				focus.Domain = core.Confidentiality
			case "labels":
				for i := range profiles {
					profiles[i].Types = []core.ID{"stranger"}
				}
			case "opposed":
				profiles[2].Measures[0].Value = -.9
			case "uncertain":
				profiles[2].Measures[0].Confidence = 0
			}
			l, r := domainFixture(t, assistance.Multi, focus, profiles)
			calls := 0
			out, e := l.Host(planFunc(func(ctx context.Context, in assistance.Input) (assistance.Result, error) {
				calls++
				if len(in.Relationships) != 2 || in.Relationships[0].Account.Observer == in.Relationships[1].Account.Observer {
					t.Fatal("perspectives collapsed")
				}
				return (assistance.DomainPlanner{}).Plan(ctx, in)
			})).Execute(context.Background(), r, nil)
			want := name == "family" || name == "labels"
			if e != nil || out.Delivered != want {
				t.Fatal(name, out, e)
			}
			unknown := name == "ambiguous" || name == "finance" || name == "private"
			if unknown && (calls != 0 || out.Result.Candidates[out.Result.Selected].Reason != "relationship_context") {
				t.Fatal("unknown context reached planner", out, calls)
			}
			if !unknown && calls != 1 {
				t.Fatal("actual consumer not exercised")
			}
		})
	}
}

func TestDomainHelperConflictsRequireOwnExplicitChoice(t *testing.T) {
	for _, arm := range []assistance.Arm{assistance.Single, assistance.Multi} {
		for _, chosen := range []bool{false, true} {
			profiles := domainProfiles()
			for i := range profiles {
				profiles[i].RoleContext = "family"
			}
			focus := core.RelationshipFocus{Version: core.RelationshipFocusVersion, Domain: core.Childcare, RoleContext: "family"}
			if chosen {
				focus.Account = "alice-care"
			}
			l, r := domainFixture(t, arm, focus, profiles)
			out, e := l.Host(assistance.FakePlanner{}).Execute(context.Background(), r, nil)
			if e != nil || out.Delivered != (chosen && arm == assistance.Single) {
				t.Fatal("other observer conflict overwritten", arm, chosen, out, e)
			}
		}
	}
}
func TestDomainHelperCurrentEvidenceAndAtomicRevocation(t *testing.T) {
	for _, stage := range []string{"before", "planning", "commit", "external_replay", "stored_replay"} {
		for _, part := range []string{"alice-care", "alice-care-frame", "alice-care-outcome"} {
			t.Run(stage+"/"+part, func(t *testing.T) {
				focus := core.RelationshipFocus{Version: core.RelationshipFocusVersion, Domain: core.Childcare, RoleContext: "family"}
				l, r := domainFixture(t, assistance.Multi, focus, domainProfiles())
				h := l.Host(assistance.FakePlanner{})
				var record *assistance.Interaction
				if stage == "external_replay" || stage == "stored_replay" {
					out, e := h.Execute(context.Background(), r, nil)
					if e != nil || !out.Delivered {
						t.Fatal(e)
					}
					if stage == "external_replay" {
						record = &out
						l, r = domainFixture(t, assistance.Multi, focus, domainProfiles())
						h = l.Host(nil)
					}
				}
				revoke := func() {
					for scope, entries := range l.entries {
						for i := range entries {
							if entries[i].Event.Meta.ID == core.ID(part) {
								entries[i].Event.Meta.Rights.Revoked = true
							}
						}
						l.SetMemory(scope, entries)
					}
				}
				switch stage {
				case "planning":
					h.Planner = planFunc(func(ctx context.Context, in assistance.Input) (assistance.Result, error) {
						revoke()
						return (assistance.FakePlanner{}).Plan(ctx, in)
					})
				case "commit":
					h.Journal = beforeCommit{Local: l, before: revoke}
				default:
					revoke()
				}
				before := l.Deliveries()
				out, e := h.Execute(context.Background(), r, record)
				if e == nil || out.Version != "" || l.Deliveries() != before {
					t.Fatal("revoked domain evidence consumed", out, e)
				}
			})
		}
	}
}
func TestDomainHelperReplayPinsFocusAndStillPermittedEvidence(t *testing.T) {
	focus := core.RelationshipFocus{Version: core.RelationshipFocusVersion, Domain: core.Childcare, RoleContext: "family"}
	for _, change := range []string{"frame", "account", "source", "confidence", "measure"} {
		for _, stored := range []bool{false, true} {
			if stored && (change == "frame" || change == "account") {
				continue
			} // registered request is immutable
			t.Run(change+map[bool]string{true: "/stored", false: "/external"}[stored], func(t *testing.T) {
				l, r := domainFixture(t, assistance.Multi, focus, domainProfiles())
				out, e := l.Host(assistance.FakePlanner{}).Execute(context.Background(), r, nil)
				if e != nil || !out.Delivered {
					t.Fatal(e)
				}
				if !stored {
					l, r = domainFixture(t, assistance.Multi, focus, domainProfiles())
				}
				switch change {
				case "frame":
					r.Focus.RoleContext = "business"
					l.requests[r.ID] = copyValue(r)
				case "account":
					r.Focus.Account = "alice-care"
					l.requests[r.ID] = copyValue(r)
				default:
					for scope, entries := range l.entries {
						for i := range entries {
							if entries[i].Event.Meta.ID == "alice-care-outcome" {
								if change == "source" {
									entries[i].Content.Text = "Still permitted, changed synthetic observation"
								} else if change == "confidence" {
									entries[i].Event.Meta.Confidence = .9
								}
							}
							if change == "measure" && entries[i].Event.Meta.ID == "alice-care" {
								p := DomainProfile("alice", "bob", "alice-care", core.Childcare, "family", .7, .6)
								from, to := core.Subject{Principal: "alice"}, core.Subject{Principal: "bob"}
								entries[i].Content.Text, e = graph.EncodeRelation(graph.RelationState{Version: 3, ID: "alice-relationship", Kind: "edge", From: &from, To: &to, Types: p.Types, Context: &p}, "alice")
								if e != nil {
									t.Fatal(e)
								}
							}
						}
						l.SetMemory(scope, entries)
					}
				}
				before := l.Deliveries()
				var record *assistance.Interaction
				if !stored {
					record = &out
				}
				if _, e = l.Host(nil).Execute(context.Background(), r, record); e == nil || l.Deliveries() != before {
					t.Fatal("replay ignored pinned domain authority", change, e)
				}
			})
		}
	}
}
func TestDomainHelperConsentAndUnknownContextCannotBeForged(t *testing.T) {
	for _, noConsent := range []bool{false, true} {
		focus := core.RelationshipFocus{Version: core.RelationshipFocusVersion, Domain: core.Childcare, RoleContext: "family"}
		profiles := domainProfiles()
		if !noConsent {
			focus.Domain = core.Finances
		}
		l, r, e := DomainFixture(assistance.Multi, focus, profiles)
		if e != nil {
			t.Fatal(e)
		}
		if !noConsent {
			consent(t, l, r)
		}
		out, e := l.Host(nil).Execute(context.Background(), r, nil)
		if e != nil || out.Delivered {
			t.Fatal(out, e)
		}
		if noConsent && len(l.audits) != 0 {
			t.Fatal("numeric trust bypassed pre-read consent")
		}
		out.Result = assistance.ExplicitPreference(r.Goal, r.User)
		out.Result.Version = r.Version
		out.Delivered = true
		if _, e = l.Host(nil).Execute(context.Background(), r, &out); e == nil {
			t.Fatal("forged proposal bypassed WAIT")
		}
	}
}
func TestDomainHelperHistoryAndBaselineVersions(t *testing.T) {
	focus := core.RelationshipFocus{Version: core.RelationshipFocusVersion, Domain: core.Childcare, RoleContext: "family"}
	l, r := domainFixture(t, assistance.Multi, focus, domainProfiles())
	if _, e := l.Host(assistance.FakePlanner{}).Execute(context.Background(), r, nil); e != nil {
		t.Fatal(e)
	}
	for _, frame := range []core.ID{"family", "business"} {
		r.Focus.RoleContext = frame
		r = nextRequest(t, l, r, core.ID("history-"+string(frame)), 3)
		_, e := l.Host(planFunc(func(ctx context.Context, in assistance.Input) (assistance.Result, error) {
			want := 0
			if frame == "family" {
				want = 1
			}
			if len(in.History) != want {
				t.Fatal("cross-frame history leaked", len(in.History), want)
			}
			for _, old := range in.History {
				if old.Focus != nil || old.Scope != nil || old.EvidenceHash != "" {
					t.Fatal("history retained private context bindings")
				}
			}
			return (assistance.FakePlanner{}).Plan(ctx, in)
		})).Execute(context.Background(), r, nil)
		if e != nil {
			t.Fatal(e)
		}
	}
	for _, arm := range []assistance.Arm{assistance.None, assistance.Simple} {
		for _, explicit := range []bool{false, true} {
			f := focus
			if !explicit {
				f.RoleContext = ""
			}
			l, r := domainFixture(t, arm, f, domainProfiles())
			out, e := l.Host(nil).Execute(context.Background(), r, nil)
			if e != nil || out.Delivered != (arm == assistance.Simple && explicit) {
				t.Fatal("baseline context rule", out, e)
			}
		}
	}
	for _, version := range []string{assistance.Version, assistance.ScopedVersion, "future"} {
		bad := copyValue(r)
		bad.Version = version
		if bad.Validate() == nil {
			t.Fatal("domain request silently downgraded")
		}
	}
}

func TestDomainHelperUnknownFrameConfidenceAndMissingMetadata(t *testing.T) {
	for _, change := range []string{"frame_confidence", "future_frame", "missing_frame", "foreign_frame"} {
		t.Run(change, func(t *testing.T) {
			focus := core.RelationshipFocus{Version: core.RelationshipFocusVersion, Domain: core.Childcare, RoleContext: "family"}
			l, r := domainFixture(t, assistance.Multi, focus, domainProfiles())
			if change == "missing_frame" {
				r.Contexts[0].Sources = r.Contexts[0].Sources[1:]
				l.requests[r.ID] = copyValue(r)
			} else {
				for scope, entries := range l.entries {
					for i := range entries {
						if entries[i].Event.Meta.ID == "alice-care-frame" {
							switch change {
							case "frame_confidence":
								entries[i].Event.Meta.Confidence = 0
							case "future_frame":
								entries[i].Content.Learned[1].At = 3
							case "foreign_frame":
								entries[i].Event.Meta.Observer = "bob"
							}
						}
					}
					l.SetMemory(scope, entries)
				}
			}
			out, e := l.Host(assistance.FakePlanner{}).Execute(context.Background(), r, nil)
			if change == "frame_confidence" {
				if e != nil || out.Delivered {
					t.Fatal("unknown confidence inferred positive context", out, e)
				}
			} else if e == nil {
				t.Fatal("unavailable frame consumed", change, out)
			}
		})
	}
}

func TestDomainHelperExplicitOwnChoicePreservesOtherUniquePerspective(t *testing.T) {
	profiles := domainProfiles()
	// Alice explicitly selects her family account. Bob has a unique family account;
	// Alice's private record ID must not be used as a selector for Bob's account.
	focus := core.RelationshipFocus{Version: core.RelationshipFocusVersion, Domain: core.Childcare, RoleContext: "family", Account: "alice-care"}
	l, r := domainFixture(t, assistance.Multi, focus, profiles)
	out, e := l.Host(assistance.FakePlanner{}).Execute(context.Background(), r, nil)
	if e != nil || !out.Delivered {
		t.Fatal("user's private account ID erased another observer's unique context", out, e)
	}
	// The immutable request hash is unchanged: the receipt's focus itself is bound.
	out.Focus.Account = "alice-business"
	if _, e = l.Host(nil).Execute(context.Background(), r, &out); e == nil {
		t.Fatal("forged receipt focus accepted")
	}
}
