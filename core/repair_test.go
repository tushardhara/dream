package core

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"
)

func repairFixture() []RepairRecord {
	focus := RelationshipFocus{Version: RelationshipFocusVersion, Domain: Finances, RoleContext: "household"}
	makeRecord := func(id, owner ID, at LogicalTime) RepairRecord {
		grants := []Grant{{Actor: "helper", Recipient: "helper", Purpose: "help", Operation: Read}, {Actor: "helper", Recipient: "helper", Purpose: "help", Operation: Derive}}
		return RepairRecord{Version: RepairVersion, Actor: "alice", Recipient: "bob", Focus: focus, Episode: "expenses", OccurredAt: at, EffectAt: at, LearnedAt: at, Meta: Metadata{ID: id, Observer: owner, Source: owner, Sensitivity: Restricted, Confidence: .8, Valid: Interval{Start: at}, RecordedAt: time.Unix(int64(at), 0).UTC(), Rights: Rights{Resource: id, Grants: grants}}}
	}
	due := LogicalTime(10)
	p := makeRecord("promise", "alice", 1)
	p.Kind = "action"
	p.Action = "promise"
	p.EffectAt = 2
	p.LearnedAt = 2
	p.Commitment = "c"
	p.Due = &due
	p.Resource = "hours"
	p.Units = 1
	h := makeRecord("help", "alice", 3)
	h.Kind = "action"
	h.Action = "help"
	h.EffectAt = 4
	h.LearnedAt = 4
	h.Commitment = "c"
	h.Resource = "hours"
	h.Units = 1
	o := makeRecord("observation", "bob", 5)
	o.Kind = "observation"
	o.Reference = "help"
	o.Commitment = "c"
	o.Finding = "fulfilled"
	o.Meta.Supporting = []ID{"help"}
	i := makeRecord("interpretation", "bob", 6)
	i.Kind = "interpretation"
	i.Reference = "observation"
	i.Phase = "later"
	i.Assessment = "mixed"
	i.Meta.Supporting = []ID{"observation"}
	return []RepairRecord{p, h, o, i}
}
func repairGrant() []Grant {
	return []Grant{{Actor: "helper", Recipient: "helper", Purpose: "help", Operation: Read}, {Actor: "helper", Recipient: "helper", Purpose: "help", Operation: Derive}}
}
func projectRepair(t *testing.T, log []RepairRecord, at LogicalTime) []RepairRecord {
	t.Helper()
	out, e := CurrentRepair(log, "alice", "bob", log[0].Focus, at, repairGrant())
	if e != nil {
		t.Fatal(e)
	}
	return out
}
func hasRepair(records []RepairRecord, id ID) bool {
	for _, r := range records {
		if r.Meta.ID == id {
			return true
		}
	}
	return false
}
func TestRepairContractDirectPositiveAndNegativeControls(t *testing.T) {
	for _, r := range repairFixture() {
		if e := r.Validate(); e != nil {
			t.Fatal(e)
		}
	}
	cases := map[string]func(*RepairRecord){
		"sender cannot observe own fulfilment": func(r *RepairRecord) { r.Meta.Observer = "alice"; r.Meta.Source = "alice" },
		"foreign reporter":                     func(r *RepairRecord) { r.Meta.Source = "alice" },
		"no evidence":                          func(r *RepairRecord) { r.Meta.Supporting = nil },
		"no reference":                         func(r *RepairRecord) { r.Reference = "" },
		"self reference":                       func(r *RepairRecord) { r.Reference = r.Meta.ID },
		"self correction":                      func(r *RepairRecord) { r.Supersedes = r.Meta.ID },
		"no commitment":                        func(r *RepairRecord) { r.Commitment = "" },
		"no invented forgiveness":              func(r *RepairRecord) { r.Finding = "forgiven" },
		"no repaired assertion":                func(r *RepairRecord) { r.Finding = "repaired" },
		"no certainty":                         func(r *RepairRecord) { r.Meta.Confidence = 1 },
		"future learning":                      func(r *RepairRecord) { r.LearnedAt = 4 },
		"observation is not duration":          func(r *RepairRecord) { r.EffectAt = 6; r.LearnedAt = 6 },
		"wrong valid time":                     func(r *RepairRecord) { r.Meta.Valid.Start = 0 },
		"unknown schema":                       func(r *RepairRecord) { r.Version = "repair-evidence.v2" },
		"no global domain":                     func(r *RepairRecord) { r.Focus.Domain = "global" },
		"exact frame required":                 func(r *RepairRecord) { r.Focus.RoleContext = "" },
		"observation has no action":            func(r *RepairRecord) { r.Action = "apologize" },
		"observation has no resource":          func(r *RepairRecord) { r.Units = 1 },
		"observation has no interpretation":    func(r *RepairRecord) { r.Assessment = "eased" },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			r := repairFixture()[2]
			change(&r)
			if r.Validate() == nil {
				t.Fatal("invalid direct contract accepted")
			}
		})
	}
	for _, finding := range []string{"fulfilled", "breached", "unresolved"} {
		r := repairFixture()[2]
		r.Finding = finding
		if e := r.Validate(); e != nil {
			t.Fatal("valid observation shape rejected", e)
		}
	}
	for _, assessment := range []string{"eased", "unchanged", "worse", "mixed", "unresolved", "paused", "ended"} {
		r := repairFixture()[3]
		r.Assessment = assessment
		if e := r.Validate(); e != nil {
			t.Fatal(e)
		}
	}
	r := repairFixture()[3]
	r.Assessment = "repaired"
	if r.Validate() == nil {
		t.Fatal("interpretation manufactures repair")
	}
}
func TestRepairActionContractIsIndependentOfProducer(t *testing.T) {
	cases := map[string]func(*RepairRecord){
		"missing due":                      func(r *RepairRecord) { r.Due = nil },
		"early due":                        func(r *RepairRecord) { v := LogicalTime(1); r.Due = &v },
		"no commitment":                    func(r *RepairRecord) { r.Commitment = "" },
		"zero units":                       func(r *RepairRecord) { r.Units = 0 },
		"negative units":                   func(r *RepairRecord) { r.Units = -1 },
		"resource absent":                  func(r *RepairRecord) { r.Resource = "" },
		"no duration":                      func(r *RepairRecord) { r.EffectAt = r.OccurredAt },
		"foreign promiser":                 func(r *RepairRecord) { r.Meta.Observer = "bob"; r.Meta.Source = "bob" },
		"promise cannot assert fulfilment": func(r *RepairRecord) { r.Finding = "fulfilled" },
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			r := repairFixture()[0]
			change(&r)
			if r.Validate() == nil {
				t.Fatal("invalid promise accepted")
			}
		})
	}
	for _, kind := range []string{"apologize", "acknowledge", "decline", "withdraw", "leave", "wait"} {
		r := repairFixture()[0]
		r.Action = kind
		r.Commitment = ""
		r.Due = nil
		r.Resource = ""
		r.Units = 0
		if kind == "wait" {
			r.EffectAt = r.OccurredAt
		}
		if e := r.Validate(); e != nil {
			t.Fatal(kind, e)
		}
		r.Finding = "fulfilled"
		if r.Validate() == nil {
			t.Fatal("speech/pause asserted completion")
		}
	}
}
func TestRepairLogRequiresLaterMatchingPracticalEvidence(t *testing.T) {
	if e := ValidateRepairLog(repairFixture()); e != nil {
		t.Fatal(e)
	}
	cases := map[string]func([]RepairRecord){
		"promise is not fulfilment": func(l []RepairRecord) { l[2].Reference = "promise"; l[2].Meta.Supporting = []ID{"promise"} },
		"apology is not fulfilment": func(l []RepairRecord) {
			l[1].Action = "apologize"
			l[1].Resource = ""
			l[1].Units = 0
			l[1].Commitment = ""
		},
		"effect is not later observation": func(l []RepairRecord) {
			l[2].OccurredAt = 4
			l[2].EffectAt = 4
			l[2].LearnedAt = 4
			l[2].Meta.Valid.Start = 4
		},
		"past deadline is not fulfilled": func(l []RepairRecord) { due := LogicalTime(4); l[0].Due = &due },
		"wrong resources":                func(l []RepairRecord) { l[1].Resource = "money" },
		"partial help":                   func(l []RepairRecord) { l[1].Units = 2 },
		"wrong commitment":               func(l []RepairRecord) { l[2].Commitment = "different" },
		"foreign topic":                  func(l []RepairRecord) { l[2].Focus.Domain = EmotionalSupport },
		"unrelated support":              func(l []RepairRecord) { l[2].Meta.Supporting = []ID{"promise"} },
		"missing provenance":             func(l []RepairRecord) { l[2].Meta.Parents = []ID{"missing"} },
		"duplicate receipt":              func(l []RepairRecord) { l[3] = l[2] },
		"clock cannot prove breach": func(l []RepairRecord) {
			l[2].Finding = "breached"
			l[2].Reference = "promise"
			l[2].Meta.Supporting = []ID{"promise"}
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			l := repairFixture()
			change(l)
			if ValidateRepairLog(l) == nil {
				t.Fatal("invalid linked evidence accepted")
			}
		})
	}
	l := repairFixture()[:3]
	l[1].Action = "break_promise"
	l[1].Resource = ""
	l[1].Units = 0
	l[2].Finding = "breached"
	if e := ValidateRepairLog(l); e != nil {
		t.Fatal("independent breach rejected", e)
	}
	l = repairFixture()[:1]
	if len(projectRepair(t, l, 100)) != 1 {
		t.Fatal("promise missing")
	}
	if l[0].Finding != "" {
		t.Fatal("clock invented proof")
	}
}
func TestRepairCurrentProvenanceAndCorrectionDoNotResurrect(t *testing.T) {
	original := repairFixture()
	if len(projectRepair(t, original, 9)) != 4 {
		t.Fatal("positive projection")
	}
	for _, source := range []ID{"promise", "help", "observation"} {
		for _, op := range []Operation{Read, Derive} {
			t.Run(string(source)+string(op), func(t *testing.T) {
				l := repairFixture()
				for i := range l {
					if l[i].Meta.ID == source {
						kept := []Grant{}
						for _, g := range l[i].Meta.Rights.Grants {
							if g.Operation != op {
								kept = append(kept, g)
							}
						}
						l[i].Meta.Rights.Grants = kept
					}
				}
				out := projectRepair(t, l, 9)
				if hasRepair(out, "observation") || hasRepair(out, "interpretation") {
					t.Fatal("missing lineage permission still derived")
				}
			})
		}
	}
	l := repairFixture()
	c := l[2]
	c.Meta.ID = "corrected"
	c.Meta.Rights.Resource = c.Meta.ID
	c.Supersedes = "observation"
	c.Finding = "unresolved"
	c.LearnedAt = 8
	c.Meta.RecordedAt = time.Unix(8, 0).UTC()
	l = append(l, c)
	out := projectRepair(t, l, 9)
	if hasRepair(out, "observation") || hasRepair(out, "interpretation") || !hasRepair(out, "corrected") {
		t.Fatal("correction failed", out)
	}
	ancestorRevoked := append([]RepairRecord{}, l...)
	ancestorRevoked[2].Meta.Rights.Revoked = true
	if hasRepair(projectRepair(t, ancestorRevoked, 9), "corrected") {
		t.Fatal("correction ignored revoked original provenance")
	}
	l[4].Meta.Rights.Revoked = true
	out = projectRepair(t, l, 9)
	if hasRepair(out, "observation") || hasRepair(out, "corrected") || hasRepair(out, "interpretation") {
		t.Fatal("revocation resurrected corrected data")
	}
	// Historical time does not undo today's revocation of the original source.
	l[2].Meta.Rights.Revoked = true
	if hasRepair(projectRepair(t, l, 6), "observation") {
		t.Fatal("historical replay bypassed current revocation")
	}
	if !reflect.DeepEqual(original, repairFixture()) {
		t.Fatal("projection mutated history")
	}
}
func TestRepairCorrectionIdentityAndStrictCodec(t *testing.T) {
	l := repairFixture()
	c := l[2]
	c.Meta.ID = "correction"
	c.Meta.Rights.Resource = c.Meta.ID
	c.Supersedes = "observation"
	c.LearnedAt = 8
	c.Meta.RecordedAt = time.Unix(8, 0).UTC()
	c.Finding = "unresolved"
	if e := ValidateRepairLog(append(l, c)); e != nil {
		t.Fatal(e)
	}
	for name, change := range map[string]func(*RepairRecord){"foreign owner": func(r *RepairRecord) { r.Meta.Observer = "alice"; r.Meta.Source = "alice" }, "different ref": func(r *RepairRecord) { r.Reference = "promise"; r.Meta.Supporting = []ID{"promise"} }, "different episode": func(r *RepairRecord) { r.Episode = "new" }, "different occurrence": func(r *RepairRecord) { r.OccurredAt++; r.EffectAt++; r.Meta.Valid.Start++ }} {
		t.Run(name, func(t *testing.T) {
			x := c
			change(&x)
			if ValidateRepairLog(append(l, x)) == nil {
				t.Fatal("invalid correction")
			}
		})
	}
	fork := c
	fork.Meta.ID = "fork"
	fork.Meta.Rights.Resource = "fork"
	if ValidateRepairLog(append(append(l, c), fork)) == nil {
		t.Fatal("correction fork accepted")
	}
	raw, e := EncodeRepairLog(l)
	if e != nil {
		t.Fatal(e)
	}
	decoded, e := DecodeRepairLog(raw)
	if e != nil || !reflect.DeepEqual(l, decoded) {
		t.Fatal("codec", e)
	}
	bad := strings.Replace(string(raw), "\"Version\":", "\"Forgiven\":true,\"Version\":", 1)
	if _, e := DecodeRepairLog([]byte(bad)); e == nil {
		t.Fatal("unknown field accepted")
	}
	if _, e := DecodeRepairLog(append(raw, []byte("{}")...)); e == nil {
		t.Fatal("trailing value accepted")
	}
	raw, _ = json.Marshal(make([]RepairRecord, MaxRepairRecords+1))
	if _, e := DecodeRepairLog(raw); e == nil {
		t.Fatal("unbounded log accepted")
	}
}

func TestRepairExecutedActionReceiptsCannotBeCorrectedByAnotherProducer(t *testing.T) {
	l := repairFixture()
	r := l[0]
	r.Meta.ID = "rewritten-promise"
	r.Meta.Rights.Resource = r.Meta.ID
	r.Supersedes = "promise"
	r.LearnedAt = 8
	r.Meta.RecordedAt = time.Unix(8, 0).UTC()
	if r.Validate() == nil {
		t.Fatal("executed action correction accepted by direct contract")
	}
	if ValidateRepairLog(append(l, r)) == nil {
		t.Fatal("executed action correction accepted by ledger")
	}
}
func TestRepairProjectionNeedsReadAndDeriveNotArbitrarySinglePermission(t *testing.T) {
	l := repairFixture()
	for _, op := range []Operation{Read, Derive, Export} {
		g := []Grant{{Actor: "helper", Recipient: "helper", Purpose: "help", Operation: op}}
		if _, e := CurrentRepair(l, "alice", "bob", l[0].Focus, 9, g); e == nil {
			t.Fatal("projection accepted incomplete permission context", op)
		}
	}
	if _, e := CurrentRepair(l, "alice", "bob", l[0].Focus, 9, repairGrant()); e != nil {
		t.Fatal("positive read+derive rejected", e)
	}
}

func TestRepairDuplicatePromiseCannotMoveTheFulfilmentDeadline(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		tight, duplicate, distinct bool
		want                       string
	}{
		{name: "original deadline permits help"},
		{name: "tight original deadline rejects late help", tight: true, want: "help does not match commitment"},
		{name: "different commitment may have its own deadline", duplicate: true, distinct: true},
		{name: "different deadline cannot extend original commitment", tight: true, duplicate: true, distinct: true, want: "help does not match commitment"},
		{name: "undeclared duplicate cannot rescue late help", tight: true, duplicate: true, want: "duplicate promise"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			log := repairFixture()
			if tc.tight {
				due := LogicalTime(3)
				log[0].Due = &due
			}
			if tc.duplicate {
				second := log[0]
				second.Meta.ID = "second-promise"
				second.Meta.Rights.Resource = second.Meta.ID
				second.OccurredAt = 2
				second.EffectAt = 3
				second.LearnedAt = 3
				second.Meta.Valid.Start = 2
				second.Meta.RecordedAt = time.Unix(2, 0).UTC()
				due := LogicalTime(50)
				second.Due = &due
				if tc.distinct {
					second.Commitment = "different-commitment"
				}
				if e := second.Validate(); e != nil {
					t.Fatal("otherwise valid second promise", e)
				}
				log = append([]RepairRecord{log[0], second}, log[1:]...)
			}
			err := ValidateRepairLog(log)
			if tc.want == "" {
				if err != nil {
					t.Fatal("positive control rejected", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("ValidateRepairLog()=%v, want %q", err, tc.want)
			}
		})
	}
}

// #81: 153 of the 176 guard messages across the epic #49 core files are never
// quoted by any test. The repair guards are the load-bearing ones — they are
// what stops an apology being read as forgiveness — and
// TestRepairLogRequiresLaterMatchingPracticalEvidence above reaches several of
// them while asserting only that *some* error occurred. A mutation that trips a
// neighbouring guard, or a guard replaced by its neighbour, passes that test.
//
// This pins the exact message for each reachable ledger guard. The positive
// control runs first, so the table cannot pass by the fixture being invalid for
// an unrelated reason.
//
// Recorded from probing the existing cases: "foreign topic" and "missing
// provenance" both trip `missing or foreign repair provenance`, and "promise is
// not fulfilment" and "past deadline is not fulfilled" both trip `fulfilment
// needs timely practical help`. The twelve mutations there reach eight distinct
// guards, not twelve.
func TestRepairLedgerGuardReachability(t *testing.T) {
	if e := ValidateRepairLog(repairFixture()); e != nil {
		t.Fatal("positive control rejected; every case below would be unattributable", e)
	}
	for _, c := range []struct {
		name, want string
		change     func([]RepairRecord)
	}{
		{"observation_without_matching_action", "observation/action mismatch", func(l []RepairRecord) {
			l[1].Action = "apologize"
			l[1].Resource = ""
			l[1].Units = 0
			l[1].Commitment = ""
		}},
		{"effect_not_later_than_reference", "completion needs later evidence", func(l []RepairRecord) {
			l[2].OccurredAt = 4
			l[2].EffectAt = 4
			l[2].LearnedAt = 4
			l[2].Meta.Valid.Start = 4
		}},
		{"fulfilled_after_the_deadline", "fulfilment needs timely practical help", func(l []RepairRecord) {
			due := LogicalTime(4)
			l[0].Due = &due
		}},
		{"fulfilled_by_a_promise_not_help", "fulfilment needs timely practical help", func(l []RepairRecord) {
			l[2].Reference = "promise"
			l[2].Meta.Supporting = []ID{"promise"}
		}},
		{"breach_asserted_from_the_clock_alone", "breach needs independent later observation", func(l []RepairRecord) {
			l[2].Finding = "breached"
			l[2].Reference = "promise"
			l[2].Meta.Supporting = []ID{"promise"}
		}},
		{"reference_not_listed_as_supporting", "reference is not supporting evidence", func(l []RepairRecord) {
			l[2].Meta.Supporting = []ID{"promise"}
		}},
		{"help_resource_does_not_match", "help does not match commitment", func(l []RepairRecord) { l[1].Resource = "money" }},
		{"help_units_do_not_match", "help does not match commitment", func(l []RepairRecord) { l[1].Units = 2 }},
		{"observation_names_another_commitment", "foreign or missing commitment", func(l []RepairRecord) { l[2].Commitment = "different" }},
		{"provenance_names_an_absent_parent", "missing or foreign repair provenance", func(l []RepairRecord) { l[2].Meta.Parents = []ID{"missing"} }},
		{"provenance_crosses_domains", "missing or foreign repair provenance", func(l []RepairRecord) { l[2].Focus.Domain = EmotionalSupport }},
		{"same_record_twice", "duplicate repair record", func(l []RepairRecord) { l[3] = l[2] }},
	} {
		t.Run(c.name, func(t *testing.T) {
			l := repairFixture()
			c.change(l)
			e := ValidateRepairLog(l)
			if e == nil {
				t.Fatal("mutated ledger accepted")
			}
			if e.Error() != c.want {
				t.Fatal("wrong guard reached: want "+c.want+", got", e)
			}
		})
	}
}
