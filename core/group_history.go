package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"sort"
)

const GroupHistoryVersion = "group-history.v1"
const MaxGroupDecisions = 24
const MaxGroupAccounts = 128

// GroupQuantity keeps an unobserved quantity distinct from an observed zero.
// These are participant reports, not objective welfare or calibrated estimates.
type GroupQuantity struct {
	Status OutcomeStatus
	Value  *float64
}

func (q GroupQuantity) Validate(min, max float64) error {
	switch q.Status {
	case Observed:
		if q.Value == nil || math.IsNaN(*q.Value) || math.IsInf(*q.Value, 0) || *q.Value < min || *q.Value > max {
			return fmt.Errorf("invalid observed group quantity")
		}
	case Unknown, Censored:
		if q.Value != nil {
			return fmt.Errorf("unobserved group quantity asserts value")
		}
	default:
		return fmt.Errorf("invalid group observation status")
	}
	return nil
}
func UnknownGroupQuantity() GroupQuantity { return GroupQuantity{Status: Unknown} }
func ObservedGroupQuantity(v float64) GroupQuantity {
	return GroupQuantity{Status: Observed, Value: &v}
}

type GroupTask struct {
	Task, Owner ID
	Units       int64
}
type GroupRequirement struct {
	Task  ID
	Units int64
}
type GroupOption struct {
	ID       ID
	Resource ID
	Units    int64
	Tasks    []GroupTask
}

// GroupDecision is an attributed published intention/option record. Invited is
// an invitation receipt, not attendance, consent, benefit or an obligation.
type GroupDecision struct {
	Version                    string
	Meta                       Metadata
	ID, Group, Intention       ID
	Focus                      RelationshipFocus
	At                         LogicalTime
	Window                     Interval
	Members, Affected, Invited []ID
	Required                   []GroupRequirement
	Options                    []GroupOption
}

func containsGroupID(list []ID, id ID) bool {
	for _, v := range list {
		if v == id {
			return true
		}
	}
	return false
}
func groupIDs(list []ID, min, max int) bool {
	return len(list) >= min && len(list) <= max && unique(list) == nil
}
func (d GroupDecision) Validate() error {
	if d.Version != GroupHistoryVersion || d.Meta.Validate() != nil || ids(d.ID, d.Group, d.Intention) != nil || d.ID == d.Meta.ID || d.Meta.Observer != d.Meta.Source || d.Focus.Validate() != nil || d.Focus.RoleContext == "" || d.Focus.Account != "" || d.At < 0 || d.Meta.Valid.Start != d.At || d.Window.Validate() != nil || d.Window.End == nil || d.Window.Start < d.At || !groupIDs(d.Members, 1, 24) || !groupIDs(d.Affected, 2, 24) || !groupIDs(d.Invited, 0, 24) || !containsGroupID(d.Affected, d.Meta.Observer) || len(d.Required) > 8 || len(d.Options) < 1 || len(d.Options) > 3 {
		return fmt.Errorf("invalid group decision")
	}
	for _, id := range d.Members {
		if !containsGroupID(d.Affected, id) {
			return fmt.Errorf("member omitted from affected set")
		}
	}
	for _, id := range d.Invited {
		if !containsGroupID(d.Affected, id) {
			return fmt.Errorf("invited outside affected set")
		}
	}
	required := map[ID]int64{}
	for _, r := range d.Required {
		if r.Task.Validate() != nil || r.Units < 1 || r.Units > 100 || required[r.Task] != 0 {
			return fmt.Errorf("invalid/duplicate required task")
		}
		required[r.Task] = r.Units
	}
	options := map[ID]bool{}
	for _, o := range d.Options {
		if o.ID.Validate() != nil || options[o.ID] || o.Resource.Validate() != nil || o.Units < 0 || o.Units > 100 || len(o.Tasks) > 24 {
			return fmt.Errorf("invalid/duplicate group option")
		}
		options[o.ID] = true
		totals := map[ID]int64{}
		seen := map[[2]ID]bool{}
		for _, task := range o.Tasks {
			key := [2]ID{task.Task, task.Owner}
			if required[task.Task] == 0 || !containsGroupID(d.Affected, task.Owner) || task.Units < 1 || task.Units > 100 || seen[key] {
				return fmt.Errorf("invalid/duplicate task assignment")
			}
			seen[key] = true
			totals[task.Task] += task.Units
		}
		for task, units := range required {
			if totals[task] != units {
				return fmt.Errorf("option silently drops or inflates responsibility")
			}
		}
	}
	raw, e := json.Marshal(d)
	if e != nil || len(raw) > 8192 {
		return fmt.Errorf("group decision bound")
	}
	return nil
}

// GroupAccount is one member's own account. A newcomer, quiet or absent member
// is not inferred to be uncaring, consenting, available or unburdened.
type GroupAccount struct {
	Version                           string
	Meta                              Metadata
	Group, Decision, Member           ID
	Focus                             RelationshipFocus
	OccurredAt, LearnedAt             LogicalTime
	Phase                             string // planning, experienced
	Membership                        string // member, newcomer, affected_nonmember, unknown
	Participation                     string // unknown, attended, not_invited, declined, absent
	CoPresent                         []ID   // this member's observed co-presence, never roster-inferred
	Intention                         ID
	ApprovedOptions                   []ID
	Capacity, Effort, Benefit, Burden GroupQuantity
	NormStance                        string // unspecified, supports, disputes: self-declared only
	Supersedes                        ID     `json:",omitempty"`
}

func (a GroupAccount) Validate() error {
	if a.Version != GroupHistoryVersion || a.Meta.Validate() != nil || ids(a.Group, a.Decision, a.Member, a.Intention) != nil || a.Meta.Observer != a.Member || a.Meta.Source != a.Member || a.Focus.Validate() != nil || a.Focus.RoleContext == "" || a.Focus.Account != "" || a.OccurredAt < 0 || a.LearnedAt < a.OccurredAt || a.Meta.Valid.Start != a.OccurredAt || len(a.Meta.Supporting) < 1 || len(a.Meta.Supporting) > 8 || !groupIDs(a.ApprovedOptions, 0, 3) || !groupIDs(a.CoPresent, 0, 24) || a.Capacity.Validate(0, 100) != nil || a.Effort.Validate(0, 100) != nil || a.Benefit.Validate(-1, 1) != nil || a.Burden.Validate(0, 100) != nil {
		return fmt.Errorf("invalid independent group account")
	}
	if a.Supersedes != "" && (a.Supersedes.Validate() != nil || a.Supersedes == a.Meta.ID) {
		return fmt.Errorf("invalid group correction")
	}
	switch a.Membership {
	case "member", "newcomer", "affected_nonmember", "unknown":
	default:
		return fmt.Errorf("invalid membership observation")
	}
	switch a.Participation {
	case "unknown", "attended", "not_invited", "declined", "absent":
	default:
		return fmt.Errorf("invalid participation observation")
	}
	if len(a.CoPresent) > 0 && (a.Participation != "attended" || !containsGroupID(a.CoPresent, a.Member)) {
		return fmt.Errorf("unobserved co-presence asserted")
	}
	switch a.NormStance {
	case "unspecified", "supports", "disputes":
	default:
		return fmt.Errorf("invalid declared norm stance")
	}
	switch a.Phase {
	case "planning":
		if a.Participation != "unknown" || len(a.CoPresent) > 0 || a.Effort.Status == Observed || a.Benefit.Status == Observed || a.Burden.Status == Observed {
			return fmt.Errorf("planning invents experienced effects")
		}
	case "experienced":
		if len(a.ApprovedOptions) != 0 {
			return fmt.Errorf("later experience invents prior agreement")
		}
	default:
		return fmt.Errorf("invalid group account phase")
	}
	raw, e := json.Marshal(a)
	if e != nil || len(raw) > 4096 {
		return fmt.Errorf("group account bound")
	}
	return nil
}

type GroupHistory struct {
	Version   string
	Decisions []GroupDecision
	Accounts  []GroupAccount
}

func sameGroupAccount(a, b GroupAccount) bool {
	return a.Group == b.Group && a.Decision == b.Decision && a.Member == b.Member && a.Focus == b.Focus && a.Phase == b.Phase
}
func (h GroupHistory) Validate() error {
	if h.Version != GroupHistoryVersion || len(h.Decisions) > MaxGroupDecisions || len(h.Accounts) > MaxGroupAccounts {
		return fmt.Errorf("group history bound/version")
	}
	idsSeen := map[ID]bool{}
	decisions := map[ID]GroupDecision{}
	for _, d := range h.Decisions {
		if d.Validate() != nil || idsSeen[d.Meta.ID] {
			return fmt.Errorf("invalid/duplicate group decision record")
		}
		if _, ok := decisions[d.ID]; ok {
			return fmt.Errorf("duplicate group decision identity")
		}
		// Published decision receipts are roots. A revised option set is a new
		// decision requiring fresh agreements, never an undeclared overwritten key.
		if len(d.Meta.Parents)+len(d.Meta.Supporting)+len(d.Meta.Contradicting) != 0 {
			return fmt.Errorf("decision root has undeclared lineage")
		}
		idsSeen[d.Meta.ID] = true
		decisions[d.ID] = d
	}
	accounts := map[ID]GroupAccount{}
	latest := map[[3]ID]ID{}
	var learned LogicalTime
	for _, a := range h.Accounts {
		if a.Validate() != nil || idsSeen[a.Meta.ID] || a.LearnedAt < learned {
			return fmt.Errorf("invalid/duplicate group account record")
		}
		learned = a.LearnedAt
		d, ok := decisions[a.Decision]
		if !ok || d.Group != a.Group || d.Focus != a.Focus || d.Intention != a.Intention || !containsGroupID(d.Affected, a.Member) || a.OccurredAt < d.At || !containsGroupID(a.Meta.Supporting, d.Meta.ID) {
			return fmt.Errorf("foreign group account/decision")
		}
		for _, person := range a.CoPresent {
			if !containsGroupID(d.Affected, person) {
				return fmt.Errorf("foreign co-presence participant")
			}
		}
		if a.Phase == "experienced" && a.OccurredAt < d.Window.Start {
			return fmt.Errorf("future group experience")
		}
		for _, option := range a.ApprovedOptions {
			found := false
			for _, o := range d.Options {
				found = found || o.ID == option
			}
			if !found {
				return fmt.Errorf("agreement to unknown option")
			}
		}
		key := [3]ID{a.Decision, a.Member, ID(a.Phase)}
		oldID := latest[key]
		if oldID != "" && a.Supersedes != oldID || oldID == "" && a.Supersedes != "" {
			return fmt.Errorf("undeclared duplicate or forked group account")
		}
		if a.Supersedes != "" {
			old := accounts[a.Supersedes]
			if !sameGroupAccount(old, a) || a.Meta.RecordedAt.Before(old.Meta.RecordedAt) {
				return fmt.Errorf("foreign group account correction")
			}
		}
		for _, list := range [][]ID{a.Meta.Parents, a.Meta.Supporting, a.Meta.Contradicting} {
			for _, id := range list {
				if id == d.Meta.ID {
					continue
				}
				old, ok := accounts[id]
				if !ok || old.Group != a.Group || old.Focus != a.Focus || old.Member != a.Member {
					return fmt.Errorf("missing/private foreign group lineage")
				}
			}
		}
		idsSeen[a.Meta.ID] = true
		accounts[a.Meta.ID] = a
		latest[key] = a.Meta.ID
	}
	return nil
}
func EncodeGroupHistory(h GroupHistory) ([]byte, error) {
	if e := h.Validate(); e != nil {
		return nil, e
	}
	return json.Marshal(h)
}
func DecodeGroupHistory(raw []byte) (GroupHistory, error) {
	var h GroupHistory
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if len(raw) > MaxGroupDecisions*8192+MaxGroupAccounts*4096 || d.Decode(&h) != nil || d.Decode(new(any)) != io.EOF || h.Validate() != nil {
		return h, fmt.Errorf("invalid group history encoding")
	}
	return h, nil
}

func groupGrants(grants []Grant) bool {
	if len(grants) < 2 || len(grants) > 8 {
		return false
	}
	seen := map[Grant]bool{}
	reads := 0
	for _, g := range grants {
		if g.Validate() != nil || seen[g] || g.Operation != Read && g.Operation != Derive && g.Operation != ShareOnRequest {
			return false
		}
		seen[g] = true
	}
	for _, g := range grants {
		if g.Operation == Read || g.Operation == Derive {
			if g.Actor != g.Recipient {
				return false
			}
			pair := g
			if g.Operation == Read {
				reads++
				pair.Operation = Derive
			} else {
				pair.Operation = Read
			}
			if !seen[pair] {
				return false
			}
		}
	}
	return reads > 0
}
func groupMetaAllows(m Metadata, at LogicalTime, grants []Grant) bool {
	if m.Valid.Start > at || m.Valid.End != nil && at >= *m.Valid.End {
		return false
	}
	for _, g := range grants {
		if !m.Rights.Allows(PermissionRequest{Resource: m.ID, Context: g}) {
			return false
		}
	}
	return true
}

// CurrentGroupAccounts checks complete current rights and correction ancestry;
// membership itself supplies NO grant. Historical queries keep current revocation.
func CurrentGroupAccounts(h GroupHistory, group ID, focus RelationshipFocus, at LogicalTime, grants []Grant) ([]GroupAccount, error) {
	if h.Validate() != nil || group.Validate() != nil || focus.Validate() != nil || focus.RoleContext == "" || focus.Account != "" || at < 0 || !groupGrants(grants) {
		return nil, fmt.Errorf("invalid group projection")
	}
	ds := map[ID]GroupDecision{}
	for _, d := range h.Decisions {
		ds[d.ID] = d
	}
	as := map[ID]GroupAccount{}
	replaced := map[ID]bool{}
	for _, a := range h.Accounts {
		as[a.Meta.ID] = a
		if a.LearnedAt <= at && a.Supersedes != "" {
			replaced[a.Supersedes] = true
		}
	}
	type key struct {
		id       ID
		ancestor bool
	}
	memo := map[key]bool{}
	visited := map[key]bool{}
	var allowed func(ID, bool) bool
	allowed = func(id ID, ancestor bool) (result bool) {
		k := key{id, ancestor}
		if visited[k] {
			return memo[k]
		}
		visited[k] = true
		defer func() { memo[k] = result }()
		a, ok := as[id]
		if !ok || a.Group != group || a.Focus != focus || a.LearnedAt > at || replaced[id] && !ancestor || !groupMetaAllows(a.Meta, at, grants) {
			return false
		}
		d := ds[a.Decision]
		if d.At > at || !groupMetaAllows(d.Meta, at, grants) {
			return false
		}
		for _, list := range [][]ID{a.Meta.Parents, a.Meta.Supporting, a.Meta.Contradicting} {
			for _, parent := range list {
				if parent != d.Meta.ID && !allowed(parent, false) {
					return false
				}
			}
		}
		if a.Supersedes != "" && !allowed(a.Supersedes, true) {
			return false
		}
		return true
	}
	out := []GroupAccount{}
	for _, a := range h.Accounts {
		if allowed(a.Meta.ID, false) {
			out = append(out, a)
		}
	}
	raw, _ := json.Marshal(out)
	var owned []GroupAccount
	_ = json.Unmarshal(raw, &owned)
	return owned, nil
}

type GroupPattern struct {
	Intention ID
	Kind      string
	Evidence  []ID
	Stance    string
}
type GroupPerspective struct {
	Member                            ID
	Evidence                          []ID
	Patterns                          []GroupPattern
	Participation, Membership         string
	CoPresent                         []ID
	Capacity, Effort, Benefit, Burden GroupQuantity
	Inclusion, Exclusion              GroupQuantity
}

// GroupPerspectiveFor derives repeated practice from this observer's own reports.
// It never ranks people/coalitions or promotes one account to a group-wide norm.
func GroupPerspectiveFor(h GroupHistory, group ID, focus RelationshipFocus, member ID, at LogicalTime, grants []Grant) (GroupPerspective, error) {
	p := GroupPerspective{Member: member, Evidence: []ID{}, Patterns: []GroupPattern{}, Participation: "unknown", Membership: "unknown", Capacity: UnknownGroupQuantity(), Effort: UnknownGroupQuantity(), Benefit: UnknownGroupQuantity(), Burden: UnknownGroupQuantity(), Inclusion: UnknownGroupQuantity(), Exclusion: UnknownGroupQuantity()}
	if member.Validate() != nil {
		return p, fmt.Errorf("invalid group member")
	}
	accounts, e := CurrentGroupAccounts(h, group, focus, at, grants)
	if e != nil {
		return p, e
	}
	// Corrections arrive later but do not make an older occasion current.
	sort.SliceStable(accounts, func(i, j int) bool { return accounts[i].OccurredAt < accounts[j].OccurredAt })
	attended, omitted := map[ID][]ID{}, map[ID][]ID{}
	stances := map[ID]string{}
	order := []ID{}
	windows := map[ID]LogicalTime{}
	for _, d := range h.Decisions {
		windows[d.ID] = d.Window.Start
	}
	seenPractice := map[struct {
		intent ID
		kind   string
		at     LogicalTime
	}]bool{}
	for _, a := range accounts {
		if a.Member != member {
			continue
		}
		p.Evidence = append(p.Evidence, a.Meta.ID)
		p.Capacity = a.Capacity
		if _, ok := stances[a.Intention]; !ok {
			order = append(order, a.Intention)
		}
		if a.NormStance != "unspecified" || stances[a.Intention] == "" {
			stances[a.Intention] = a.NormStance
		}
		if a.Phase != "experienced" {
			continue
		}
		p.Participation = a.Participation
		p.Membership = a.Membership
		p.CoPresent = []ID{}
		for _, d := range h.Decisions {
			if d.ID == a.Decision && at >= d.Window.Start && at < *d.Window.End {
				p.CoPresent = append([]ID{}, a.CoPresent...)
			}
		}
		p.Effort = a.Effort
		p.Benefit = a.Benefit
		p.Burden = a.Burden
		k := struct {
			intent ID
			kind   string
			at     LogicalTime
		}{a.Intention, a.Participation, windows[a.Decision]}
		if seenPractice[k] {
			continue
		}
		seenPractice[k] = true
		if a.Participation == "attended" {
			attended[a.Intention] = append(attended[a.Intention], a.Meta.ID)
		}
		if a.Participation == "not_invited" && (a.Membership == "member" || a.Membership == "newcomer") {
			omitted[a.Intention] = append(omitted[a.Intention], a.Meta.ID)
		}
	}
	for _, intent := range order {
		for _, kind := range []string{"repeated_participation", "repeated_omission"} {
			sources := attended[intent]
			if kind == "repeated_omission" {
				sources = omitted[intent]
			}
			if len(sources) >= 2 {
				p.Patterns = append(p.Patterns, GroupPattern{Intention: intent, Kind: kind, Evidence: append([]ID{}, sources...), Stance: stances[intent]})
			}
		}
	}
	known := len(attended)+len(omitted) > 0
	if known {
		a, o := 0, 0
		for _, ids := range attended {
			a += len(ids)
		}
		for _, ids := range omitted {
			o += len(ids)
		}
		if a+o > 0 {
			p.Inclusion = ObservedGroupQuantity(float64(a) / float64(a+o))
			p.Exclusion = ObservedGroupQuantity(float64(o) / float64(a+o))
		}
	}
	return p, nil
}
