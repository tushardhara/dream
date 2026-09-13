package core_test

import (
	"testing"

	"github.com/tushardhara/dream/core"
)

func grant(op core.Operation) core.Grant {
	return core.Grant{Actor: "alice", Recipient: "alice", Purpose: "research", Operation: op}
}
func TestPermissionsDenyUnknownAndSeparateOperations(t *testing.T) {
	operations := []core.Operation{core.Read, core.Derive, core.Disclose, core.Attribute, core.Aggregate, core.Match, core.Retain, core.Export, core.ShareOnRequest}
	for _, op := range operations {
		r := core.Rights{Resource: "source", Grants: []core.Grant{grant(op)}}
		for _, requestOp := range operations {
			if got := r.Allows(core.PermissionRequest{Resource: "source", Context: grant(requestOp)}); got != (op == requestOp) {
				t.Fatalf("%s implicitly granted %s", op, requestOp)
			}
		}
		for _, q := range []core.PermissionRequest{
			{}, {Resource: "other", Context: grant(op)},
			{Resource: "source", Context: core.Grant{Actor: "bob", Recipient: "alice", Purpose: "research", Operation: op}},
			{Resource: "source", Context: core.Grant{Actor: "alice", Recipient: "bob", Purpose: "research", Operation: op}},
			{Resource: "source", Context: core.Grant{Actor: "alice", Recipient: "alice", Purpose: "other", Operation: op}},
			{Resource: "source", Context: grant("future-operation")},
		} {
			if r.Allows(q) {
				t.Fatalf("unknown context allowed: %+v", q)
			}
		}
		r.Revoked = true
		if r.Allows(core.PermissionRequest{Resource: "source", Context: grant(op)}) {
			t.Fatal("revoked rights allowed")
		}
	}
	r := core.Rights{Resource: "source", Grants: []core.Grant{grant(core.Read), grant(core.Read)}}
	if r.Allows(core.PermissionRequest{Resource: "source", Context: grant(core.Read)}) {
		t.Fatal("malformed grants must deny")
	}
}

func TestDerivationIntersection(t *testing.T) {
	a := core.Rights{Resource: "a", Grants: []core.Grant{grant(core.Derive), grant(core.Read), grant(core.Export)}}
	b := core.Rights{Resource: "b", Grants: []core.Grant{grant(core.Read), grant(core.Derive)}}
	out, err := core.DeriveRights("derived", grant(core.Derive), []core.Rights{a, b})
	if err != nil {
		t.Fatal(err)
	}
	if !out.Allows(core.PermissionRequest{Resource: "derived", Context: grant(core.Read)}) {
		t.Fatal("intersection lost read")
	}
	if out.Allows(core.PermissionRequest{Resource: "derived", Context: grant(core.Export)}) {
		t.Fatal("derived rights widened")
	}
	out.Grants[0].Actor = "mutated"
	if a.Grants[0].Actor != "alice" {
		t.Fatal("aliased source grant")
	}
	for name, sources := range map[string][]core.Rights{
		"none": nil, "duplicate": {a, a}, "self": {a, {Resource: "derived", Grants: b.Grants}},
		"revoked":   {a, {Resource: "b", Revoked: true, Grants: b.Grants}},
		"no derive": {a, {Resource: "b", Grants: []core.Grant{grant(core.Read)}}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := core.DeriveRights("derived", grant(core.Derive), sources); err == nil {
				t.Fatal("unauthorized derivation")
			}
		})
	}
	if _, err := core.DeriveRights("derived", grant(core.Read), []core.Rights{a, b}); err == nil {
		t.Fatal("read grants derive")
	}
}

func TestStateCannotBroadenSourceRights(t *testing.T) {
	for _, relation := range []string{"parent", "support", "contradict"} {
		t.Run(relation, func(t *testing.T) {
			s := fixture()
			m := &s.Memories[0].Meta
			switch relation {
			case "parent":
				m.Parents = []core.ID{"evidence"}
			case "support":
				m.Supporting = []core.ID{"evidence"}
			case "contradict":
				m.Contradicting = []core.ID{"evidence"}
			}
			m.Rights.Grants = []core.Grant{grant(core.Export)}
			if _, err := core.Canonical(s); err == nil {
				t.Fatal("source rights broadened")
			}
			m.Rights.Grants = nil
			m.Sensitivity = core.Public
			if _, err := core.Canonical(s); err == nil {
				t.Fatal("restricted source declassified")
			}
			m.Sensitivity = core.Restricted
			s.Evidence[0].Meta.Rights.Revoked = true
			if _, err := core.Canonical(s); err == nil {
				t.Fatal("revoked provenance resurrected")
			}
		})
	}
}

func FuzzRightsNeverWiden(f *testing.F) {
	f.Add(uint16(511), uint16(3))
	f.Add(uint16(1), uint16(1))
	f.Fuzz(func(t *testing.T, a, b uint16) {
		ops := []core.Operation{core.Read, core.Derive, core.Disclose, core.Attribute, core.Aggregate, core.Match, core.Retain, core.Export, core.ShareOnRequest}
		sources := []core.Rights{{Resource: "a"}, {Resource: "b"}}
		for i, mask := range []uint16{a, b} {
			for bit, op := range ops {
				if mask&(1<<bit) != 0 {
					sources[i].Grants = append(sources[i].Grants, grant(op))
				}
			}
		}
		out, err := core.DeriveRights("out", grant(core.Derive), sources)
		if err != nil {
			return
		}
		for _, op := range ops {
			if out.Allows(core.PermissionRequest{Resource: "out", Context: grant(op)}) {
				for _, source := range sources {
					if !source.Allows(core.PermissionRequest{Resource: source.Resource, Context: grant(op)}) {
						t.Fatal("derived right absent from source")
					}
				}
			}
		}
	})
}
