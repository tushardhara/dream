package scenario

import (
	"fmt"
	"github.com/tushardhara/dream/core"
	"reflect"
	"sort"
	"strings"
	"unicode/utf8"
)

func (s Scenario) Validate() error {
	budget := 0
	if err := bounded(reflect.ValueOf(s), "$", &budget); err != nil {
		return err
	}
	if s.Version != Version {
		return fail("$.version", "unsupported schema version")
	}
	if s.World.ID.Validate() != nil {
		return fail("$.world.id", "invalid world ID")
	}
	if s.World.Horizon <= 0 {
		return fail("$.world.horizon", "must be positive nanoseconds")
	}
	ids := map[core.ID]bool{}
	add := func(id core.ID, p string) error {
		if id.Validate() != nil {
			return fail(p, "invalid ID")
		}
		if ids[id] {
			return fail(p, "duplicate ID")
		}
		ids[id] = true
		if len(ids) > MaxItems {
			return fail(p, "too many entities")
		}
		return nil
	}
	humans := map[core.ID]bool{}
	if len(s.Public.Humans) < 2 || len(s.Public.Humans) > 24 {
		return fail("$.public.humans", "requires 2..24 synthetic adults")
	}
	for i, h := range s.Public.Humans {
		p := fmt.Sprintf("$.public.humans[%d]", i)
		if err := add(h.ID, p+".id"); err != nil {
			return err
		}
		if h.Age < 18 || h.Age > 120 || !text(h.Name) {
			return fail(p, "requires an adult age 18..120 and bounded name")
		}
		humans[h.ID] = true
	}
	ref := func(id core.ID, p string) error {
		if !humans[id] {
			return fail(p, "unknown human")
		}
		return nil
	}
	for i, g := range s.Public.Groups {
		p := fmt.Sprintf("$.public.groups[%d]", i)
		if err := add(g.ID, p+".id"); err != nil {
			return err
		}
		if len(g.Members) == 0 {
			return fail(p+".members", "empty group")
		}
		seen := map[core.ID]bool{}
		for j, m := range g.Members {
			q := fmt.Sprintf("%s.members[%d]", p, j)
			if err := ref(m, q); err != nil {
				return err
			}
			if seen[m] {
				return fail(q, "duplicate member")
			}
			seen[m] = true
		}
	}
	resources := map[core.ID]Resource{}
	for i, r := range s.Public.Resources {
		p := fmt.Sprintf("$.public.resources[%d]", i)
		if err := add(r.ID, p+".id"); err != nil {
			return err
		}
		if r.Capacity <= 0 || r.Available < 0 || r.Available > r.Capacity {
			return fail(p, "capacity must be positive; available must be in [0,capacity]")
		}
		resources[r.ID] = r
	}
	facts := map[core.ID]Fact{}
	owners := map[core.ID]core.ID{}
	fact := func(f Fact, owner core.ID, p string) error {
		if err := add(f.ID, p+".id"); err != nil {
			return err
		}
		if err := ref(f.Observer, p+".observer"); err != nil {
			return err
		}
		if err := ref(f.Subject, p+".subject"); err != nil {
			return err
		}
		if owner != "" && f.Observer != owner {
			return fail(p+".observer", "actor fact must belong to enclosing observer")
		}
		if !text(f.Text) || f.Confidence.Validate() != nil {
			return fail(p, "invalid bounded text or confidence")
		}
		if f.Valid.Validate() != nil || f.Valid.Start != 0 || (f.Valid.End != nil && *f.Valid.End > s.World.Horizon) {
			return fail(p+".valid", "genesis facts start at zero and end within horizon")
		}
		if (core.Rights{Resource: f.ID, Grants: f.Grants}).Validate() != nil {
			return fail(p+".grants", "invalid or duplicate permission")
		}
		for j, g := range f.Grants {
			q := fmt.Sprintf("%s.grants[%d]", p, j)
			if err := ref(g.Actor, q+".actor"); err != nil {
				return err
			}
			if err := ref(g.Recipient, q+".recipient"); err != nil {
				return err
			}
			if owner != "" && (g.Actor != owner || g.Recipient != owner) {
				return fail(q, "actor-owned bootstrap facts grant only self context; later sharing requires events")
			}
		}
		facts[f.ID] = f
		owners[f.ID] = owner
		return nil
	}
	for i, f := range s.Public.Facts {
		if err := fact(f, "", fmt.Sprintf("$.public.facts[%d]", i)); err != nil {
			return err
		}
	}
	actors := map[core.ID]bool{}
	for i, a := range s.Actors {
		p := fmt.Sprintf("$.actors[%d]", i)
		if err := ref(a.ID, p+".id"); err != nil {
			return err
		}
		if actors[a.ID] {
			return fail(p+".id", "duplicate actor section")
		}
		actors[a.ID] = true
		for j, f := range a.Facts {
			if err := fact(f, a.ID, fmt.Sprintf("%s.facts[%d]", p, j)); err != nil {
				return err
			}
		}
	}
	if len(actors) != len(humans) {
		return fail("$.actors", "requires exactly one section per human")
	}
	for i, a := range s.Actors {
		p := fmt.Sprintf("$.actors[%d]", i)
		known := map[core.ID]bool{}
		for j, k := range a.Knowledge {
			q := fmt.Sprintf("%s.knowledge[%d]", p, j)
			f, ok := facts[k.Record]
			if !ok || owners[k.Record] != "" && owners[k.Record] != a.ID {
				return fail(q+".record", "unknown or foreign private record")
			}
			if known[k.Record] || k.LearnedAt != 0 {
				return fail(q, "duplicate knowledge or nonzero genesis learned_at")
			}
			if !(core.Rights{Resource: f.ID, Grants: f.Grants}).Allows(core.PermissionRequest{Resource: f.ID, Context: core.Grant{Actor: a.ID, Recipient: a.ID, Purpose: "simulation", Operation: core.Read}}) {
				return fail(q, "missing self read permission for simulation purpose")
			}
			known[k.Record] = true
		}
		for j, m := range a.Memories {
			q := fmt.Sprintf("%s.memories[%d]", p, j)
			if err := add(m.ID, q+".id"); err != nil {
				return err
			}
			if !known[m.Evidence] || !text(m.Text) {
				return fail(q, "memory requires known evidence and bounded text")
			}
		}
		if len(a.Contexts) > 8 {
			return fail(p, "relationship context bound")
		}
		seenContexts := map[core.ID]bool{}
		for _, c := range a.Contexts {
			if c.Validate(a.ID, c.Other) != nil || c.Valid.Start != 0 || seenContexts[c.Other] {
				return fail(p, "invalid observer relationship context")
			}
			seenContexts[c.Other] = true
			found := false
			for _, r := range a.Relationships {
				if r.Other == c.Other {
					for _, kind := range c.Types {
						found = found || kind == r.Kind
					}
				}
			}
			if !found {
				return fail(p, "context has no matching known relationship")
			}
			for _, id := range c.Sources() {
				if !known[id] {
					return fail(p, "unknown relationship context source")
				}
			}
		}
		for j, r := range a.Relationships {
			q := fmt.Sprintf("%s.relationships[%d]", p, j)
			if err := add(r.ID, q+".id"); err != nil {
				return err
			}
			if err := ref(r.Other, q+".other"); err != nil {
				return err
			}
			if r.Other == a.ID || r.Kind.Validate() != nil || !known[r.Evidence] {
				return fail(q, "relationship requires another human, kind and known evidence")
			}
		}
	}
	seen := map[core.ID]bool{}
	for i, l := range s.Research.Latent {
		p := fmt.Sprintf("$.research.latent[%d]", i)
		if err := ref(l.Actor, p+".actor"); err != nil {
			return err
		}
		if seen[l.Actor] || l.Emotion.Validate() != nil {
			return fail(p, "duplicate actor or invalid emotion")
		}
		seen[l.Actor] = true
		drives := map[core.ID]bool{}
		if len(l.Drives) > MaxItems {
			return fail(p+".drives", "too many drives")
		}
		for j, d := range l.Drives {
			if d.Validate() != nil || drives[d.Kind] {
				return fail(fmt.Sprintf("%s.drives[%d]", p, j), "invalid or duplicate drive")
			}
			drives[d.Kind] = true
		}
	}
	for i, l := range s.Research.Labels {
		p := fmt.Sprintf("$.research.labels[%d]", i)
		if err := add(l.ID, p+".id"); err != nil {
			return err
		}
		if !text(l.Text) {
			return fail(p+".text", "invalid bounded text")
		}
	}
	type change struct {
		at    core.LogicalTime
		units int64
		end   bool
	}
	changes := map[core.ID][]change{}
	for i, e := range s.Future {
		p := fmt.Sprintf("$.future[%d]", i)
		if err := add(e.ID, p+".id"); err != nil {
			return err
		}
		if err := ref(e.Actor, p+".actor"); err != nil {
			return err
		}
		if e.At <= 0 || e.At >= s.World.Horizon || !text(e.Text) {
			return fail(p, "event needs bounded text and 0 < at < horizon")
		}
		switch e.Kind {
		case "observation":
			if e.Resource != "" || e.Units != 0 || e.Until != 0 {
				return fail(p, "observation cannot reserve resources")
			}
		case "reservation":
			r, ok := resources[e.Resource]
			if !ok || e.Units <= 0 || e.Units > r.Available || e.Until <= e.At || e.Until > s.World.Horizon {
				return fail(p, "invalid resource, units or half-open reservation interval")
			}
			changes[e.Resource] = append(changes[e.Resource], change{e.At, e.Units, false}, change{e.Until, e.Units, true})
		default:
			return fail(p+".kind", "unsupported scheduled event kind")
		}
	}
	for _, r := range s.Public.Resources {
		c := changes[r.ID]
		sort.Slice(c, func(i, j int) bool {
			if c[i].at != c[j].at {
				return c[i].at < c[j].at
			}
			return c[i].end && !c[j].end
		})
		used := int64(0)
		for _, x := range c {
			if x.end {
				used -= x.units
			} else {
				if x.units > r.Available-used {
					return fail("$.future", "overlapping reservations exceed available capacity")
				}
				used += x.units
			}
		}
	}
	seen = map[core.ID]bool{}
	if len(s.Requires) > MaxItems {
		return fail("$.requires", "too many capabilities")
	}
	for i, c := range s.Requires {
		if c.Validate() != nil || seen[c] {
			return fail(fmt.Sprintf("$.requires[%d]", i), "invalid or duplicate capability")
		}
		seen[c] = true
	}
	return nil
}
func text(s string) bool { return utf8.ValidString(s) && strings.TrimSpace(s) != "" && len(s) <= 4096 }

// Apply the same allocation budget to programmatically constructed scenarios.
func bounded(v reflect.Value, p string, budget *int) error {
	*budget++
	if *budget > 20000 {
		return fail(p, "scenario node budget exceeded")
	}
	switch v.Kind() {
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			name := strings.Split(v.Type().Field(i).Tag.Get("json"), ",")[0]
			if err := bounded(v.Field(i), p+"."+name, budget); err != nil {
				return err
			}
		}
	case reflect.Slice:
		if v.Len() > MaxItems {
			return fail(p, "too many items")
		}
		for i := 0; i < v.Len(); i++ {
			if err := bounded(v.Index(i), fmt.Sprintf("%s[%d]", p, i), budget); err != nil {
				return err
			}
		}
	case reflect.Pointer:
		if !v.IsNil() {
			return bounded(v.Elem(), p, budget)
		}
	case reflect.String:
		if len(v.String()) > 4096 {
			return fail(p, "string exceeds 4096 bytes")
		}
	}
	return nil
}
