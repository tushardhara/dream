package core

import (
	"encoding/json"
	"fmt"
	"reflect"
)

type GroupBudget struct {
	Shared   map[ID]int64
	Personal map[ID]int64
}

func (b GroupBudget) Validate() error {
	if len(b.Shared) < 1 || len(b.Shared) > 8 || len(b.Personal) < 2 || len(b.Personal) > 24 {
		return fmt.Errorf("group budget bound")
	}
	for _, m := range []map[ID]int64{b.Shared, b.Personal} {
		for id, n := range m {
			if id.Validate() != nil || n < 0 || n > 1000 {
				return fmt.Errorf("invalid group budget")
			}
		}
	}
	return nil
}

// Reservations are immutable resource receipts, not observations of benefit.
// Revoking a report does not refund already reserved capacity. Expired windows
// cease overlapping; no background scheduler or automatic reassignment exists.
type GroupReservation struct {
	Version                                string
	ID, Decision, Option, Group, Initiator ID
	Focus                                  RelationshipFocus
	At                                     LogicalTime
	Window                                 Interval
	Resource                               ID
	Units                                  int64
	Tasks                                  []GroupTask
}

func (r GroupReservation) Validate() error {
	if r.Version != GroupHistoryVersion || ids(r.ID, r.Decision, r.Option, r.Group, r.Initiator, r.Resource) != nil || r.At < 0 || r.Window.Validate() != nil || r.Window.End == nil || r.At >= *r.Window.End || r.Focus.Validate() != nil || r.Focus.RoleContext == "" || r.Focus.Account != "" || r.Units < 0 || r.Units > 100 || len(r.Tasks) > 24 {
		return fmt.Errorf("invalid group reservation")
	}
	seen := map[[2]ID]bool{}
	for _, t := range r.Tasks {
		key := [2]ID{t.Task, t.Owner}
		if ids(t.Task, t.Owner) != nil || t.Units < 1 || t.Units > 100 || seen[key] {
			return fmt.Errorf("invalid reservation tasks")
		}
		seen[key] = true
	}
	return nil
}
func groupDecisionOption(h GroupHistory, decision, option ID) (GroupDecision, GroupOption, bool) {
	for _, d := range h.Decisions {
		if d.ID == decision {
			for _, o := range d.Options {
				if o.ID == option {
					return d, o, true
				}
			}
		}
	}
	return GroupDecision{}, GroupOption{}, false
}
func ValidateGroupReservations(h GroupHistory, rs []GroupReservation) error {
	if h.Validate() != nil || len(rs) > MaxGroupDecisions {
		return fmt.Errorf("invalid reservation ledger")
	}
	idsSeen := map[ID]bool{}
	decisions := map[ID]bool{}
	for _, r := range rs {
		if r.Validate() != nil || idsSeen[r.ID] {
			return fmt.Errorf("duplicate/invalid reservation receipt")
		}
		if decisions[r.Decision] {
			return fmt.Errorf("group decision already reserved")
		}
		d, o, ok := groupDecisionOption(h, r.Decision, r.Option)
		if !ok || r.Group != d.Group || r.Focus != d.Focus || r.Initiator != d.Meta.Observer || r.At < d.At || !reflect.DeepEqual(r.Window, d.Window) || r.Resource != o.Resource || r.Units != o.Units || !reflect.DeepEqual(r.Tasks, o.Tasks) {
			return fmt.Errorf("reservation changed agreed decision")
		}
		idsSeen[r.ID] = true
		decisions[r.Decision] = true
	}
	return nil
}
func groupWindowsOverlap(a, b Interval) bool {
	return a.End != nil && b.End != nil && a.Start < *b.End && b.Start < *a.End
}

// GroupOptionFeasible requires all affected people's CURRENT explicit agreement,
// including non-attending caregivers. Membership, silence, a leader's approval or
// a benefit to one recipient does not transfer another person's responsibility.
// Caller supplies the complete trusted reservation portfolio across contexts.
func GroupOptionFeasible(h GroupHistory, rs []GroupReservation, b GroupBudget, decision, option ID, at LogicalTime, grants []Grant) error {
	if h.Validate() != nil || ValidateGroupPortfolio(h, rs, b) != nil || !groupGrants(grants) || at < 0 {
		return fmt.Errorf("invalid group planning input")
	}
	d, o, ok := groupDecisionOption(h, decision, option)
	if !ok || d.At > at || at >= *d.Window.End || !groupMetaAllows(d.Meta, at, grants) {
		return fmt.Errorf("unavailable group option")
	}
	for _, r := range rs {
		if r.Decision == decision {
			return fmt.Errorf("group decision already reserved")
		}
	}
	accounts, e := CurrentGroupAccounts(h, d.Group, d.Focus, at, grants)
	if e != nil {
		return e
	}
	byMember := map[ID]GroupAccount{}
	for _, a := range accounts {
		if a.Decision == decision && a.Phase == "planning" {
			byMember[a.Member] = a
		}
	}
	assigned := map[ID]int64{}
	for _, t := range o.Tasks {
		assigned[t.Owner] += t.Units
	}
	for _, person := range d.Affected {
		a, ok := byMember[person]
		if !ok || !containsGroupID(a.ApprovedOptions, option) || a.Capacity.Status != Observed || a.Capacity.Value == nil {
			return fmt.Errorf("missing explicit agreement/capacity")
		}
		capacity, known := b.Personal[person]
		if !known {
			return fmt.Errorf("unknown total personal capacity")
		}
		used := int64(0)
		for _, r := range rs {
			if groupWindowsOverlap(d.Window, r.Window) {
				for _, t := range r.Tasks {
					if t.Owner == person {
						used += t.Units
					}
				}
			}
		}
		if float64(assigned[person]) > *a.Capacity.Value || used+assigned[person] > capacity {
			return fmt.Errorf("overlapping role/capacity conflict")
		}
	}
	capacity, known := b.Shared[o.Resource]
	if !known {
		return fmt.Errorf("unknown shared resource")
	}
	used := int64(0)
	for _, r := range rs {
		if r.Resource == o.Resource && groupWindowsOverlap(d.Window, r.Window) {
			used += r.Units
		}
	}
	if used+o.Units > capacity {
		return fmt.Errorf("shared resource over-reservation")
	}
	return nil
}
func ReserveGroup(h GroupHistory, rs []GroupReservation, b GroupBudget, decision, option, id, actor ID, at LogicalTime, grants []Grant) ([]GroupReservation, error) {
	if GroupOptionFeasible(h, rs, b, decision, option, at, grants) != nil {
		return nil, fmt.Errorf("group reservation denied")
	}
	d, o, _ := groupDecisionOption(h, decision, option)
	if actor != d.Meta.Observer {
		return nil, fmt.Errorf("foreign group initiator")
	}
	r := GroupReservation{Version: GroupHistoryVersion, ID: id, Decision: decision, Option: option, Group: d.Group, Initiator: actor, Focus: d.Focus, At: at, Window: d.Window, Resource: o.Resource, Units: o.Units, Tasks: o.Tasks}
	next := append(append([]GroupReservation{}, rs...), r)
	if e := ValidateGroupReservations(h, next); e != nil {
		return nil, e
	}
	raw, _ := json.Marshal(next)
	var owned []GroupReservation
	_ = json.Unmarshal(raw, &owned)
	return owned, nil
}

// ValidateGroupPortfolio checks a complete trusted imported reservation ledger
// against physical capacities at every interval boundary, across all roles.
func ValidateGroupPortfolio(h GroupHistory, rs []GroupReservation, b GroupBudget) error {
	if ValidateGroupReservations(h, rs) != nil || b.Validate() != nil {
		return fmt.Errorf("invalid group portfolio")
	}
	for _, point := range rs {
		shared, personal := map[ID]int64{}, map[ID]int64{}
		for _, r := range rs {
			if point.Window.Start < r.Window.Start || point.Window.Start >= *r.Window.End {
				continue
			}
			shared[r.Resource] += r.Units
			for _, task := range r.Tasks {
				personal[task.Owner] += task.Units
			}
		}
		for id, n := range shared {
			capacity, ok := b.Shared[id]
			if !ok || n > capacity {
				return fmt.Errorf("imported shared capacity conflict")
			}
		}
		for id, n := range personal {
			capacity, ok := b.Personal[id]
			if !ok || n > capacity {
				return fmt.Errorf("imported personal capacity conflict")
			}
		}
	}
	return nil
}
