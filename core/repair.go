package core

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

const RepairVersion = "repair-evidence.v1"
const MaxRepairRecords = 96

// RepairRecord distinguishes executed action receipts, later recipient observation,
// and each person's interpretation. It never asserts forgiveness or relationship
// repair. Authentication and actual resource execution belong to the trusted host.
type RepairRecord struct {
	Version                         string
	Meta                            Metadata
	Actor, Recipient                ID
	Focus                           RelationshipFocus
	Episode                         ID
	Kind                            string // action, observation, interpretation
	Action                          string // action receipts only
	Commitment                      ID     `json:",omitempty"`
	Reference                       ID     `json:",omitempty"`
	Supersedes                      ID     `json:",omitempty"`
	OccurredAt, EffectAt, LearnedAt LogicalTime
	Due                             *LogicalTime `json:",omitempty"`
	Resource                        ID           `json:",omitempty"`
	Units                           int64        `json:",omitempty"`
	Finding                         string       // fulfilled, breached, unresolved; observations only
	Phase                           string       // immediate, later; interpretations only
	Assessment                      string       // eased, unchanged, worse, mixed, unresolved, paused, ended
}

func (r RepairRecord) Validate() error {
	if r.Version != RepairVersion || r.Meta.Validate() != nil || ids(r.Actor, r.Recipient, r.Episode) != nil || r.Actor == r.Recipient || r.Meta.Observer != r.Actor && r.Meta.Observer != r.Recipient || r.Meta.Source != r.Meta.Observer || r.Focus.Validate() != nil || r.Focus.RoleContext == "" || r.Focus.Account != "" || r.OccurredAt < 0 || r.EffectAt < r.OccurredAt || r.LearnedAt < r.EffectAt || r.Meta.Valid.Start != r.OccurredAt || r.Meta.Confidence >= 1 {
		return fmt.Errorf("invalid repair evidence")
	}
	for _, id := range []ID{r.Reference, r.Supersedes, r.Commitment} {
		if id != "" && (id.Validate() != nil || id == r.Meta.ID) {
			return fmt.Errorf("invalid repair link")
		}
	}
	switch r.Kind {
	case "action":
		if r.Supersedes != "" {
			return fmt.Errorf("executed action receipts are immutable")
		}
		if r.Meta.Observer != r.Actor && r.Action != "decline" && r.Action != "wait" && r.Action != "withdraw" && r.Action != "leave" || r.Reference != "" || r.Finding != "" || r.Phase != "" || r.Assessment != "" {
			return fmt.Errorf("invalid action receipt")
		}
		switch r.Action {
		case "apologize", "acknowledge", "promise", "help", "break_promise", "decline", "wait", "withdraw", "leave":
		default:
			return fmt.Errorf("unknown repair action")
		}
		if r.Action == "wait" {
			if r.EffectAt != r.OccurredAt {
				return fmt.Errorf("WAIT effect")
			}
		} else if r.EffectAt <= r.OccurredAt {
			return fmt.Errorf("action has no duration")
		}
		if r.Action == "promise" || r.Action == "help" {
			if r.Resource.Validate() != nil || r.Units < 1 || r.Units > 100 {
				return fmt.Errorf("invalid practical resources")
			}
		} else if r.Resource != "" || r.Units != 0 {
			return fmt.Errorf("unexpected resources")
		}
		if r.Action == "promise" {
			if r.Commitment.Validate() != nil || r.Due == nil || *r.Due < r.EffectAt {
				return fmt.Errorf("invalid commitment due")
			}
		} else if r.Due != nil {
			return fmt.Errorf("unexpected due")
		}
		if r.Action == "break_promise" && r.Commitment == "" || r.Commitment != "" && r.Action != "promise" && r.Action != "help" && r.Action != "break_promise" {
			return fmt.Errorf("invalid action commitment")
		}
	case "observation", "interpretation":
		if r.Action != "" || r.Resource != "" || r.Units != 0 || r.Due != nil || r.Reference.Validate() != nil || r.EffectAt != r.OccurredAt || len(r.Meta.Supporting) == 0 {
			return fmt.Errorf("unsupported repair account")
		}
		if r.Kind == "observation" {
			if r.Meta.Observer != r.Recipient || r.Commitment.Validate() != nil || r.Phase != "" || r.Assessment != "" || r.Finding != "fulfilled" && r.Finding != "breached" && r.Finding != "unresolved" {
				return fmt.Errorf("invalid independent observation")
			}
		} else if r.Finding != "" || r.Commitment != "" || r.Phase != "immediate" && r.Phase != "later" {
			return fmt.Errorf("invalid interpretation")
		} else {
			switch r.Assessment {
			case "eased", "unchanged", "worse", "mixed", "unresolved", "paused", "ended":
			default:
				return fmt.Errorf("invalid personal assessment")
			}
		}
	default:
		return fmt.Errorf("unknown repair record")
	}
	raw, e := json.Marshal(r)
	if e != nil || len(raw) > 4096 {
		return fmt.Errorf("repair record bound")
	}
	return nil
}

func sameRepairScope(a, b RepairRecord) bool {
	return a.Actor == b.Actor && a.Recipient == b.Recipient && a.Episode == b.Episode && a.Focus == b.Focus
}

// ValidateRepairLog checks links even for callers other than the demo producer.
// No observer can correct another's account, fork corrections, or manufacture
// fulfilment from a promise, apology, reply, clock expiry or sender interpretation.
func ValidateRepairLog(log []RepairRecord) error {
	if len(log) > MaxRepairRecords {
		return fmt.Errorf("repair ledger bound")
	}
	seen := map[ID]RepairRecord{}
	replaced := map[ID]bool{}
	promises := map[ID]RepairRecord{}
	var last LogicalTime
	for _, r := range log {
		if r.Validate() != nil || r.LearnedAt < last {
			return fmt.Errorf("invalid repair log record/time")
		}
		last = r.LearnedAt
		if _, ok := seen[r.Meta.ID]; ok {
			return fmt.Errorf("duplicate repair record")
		}
		for _, list := range [][]ID{r.Meta.Parents, r.Meta.Supporting, r.Meta.Contradicting} {
			for _, id := range list {
				p, ok := seen[id]
				if !ok || !sameRepairScope(p, r) || p.LearnedAt > r.LearnedAt {
					return fmt.Errorf("missing or foreign repair provenance")
				}
			}
		}
		if r.Supersedes != "" {
			old, ok := seen[r.Supersedes]
			if !ok || replaced[r.Supersedes] || !sameRepairScope(old, r) || old.Meta.Observer != r.Meta.Observer || old.Kind != r.Kind || old.Action != r.Action || old.Commitment != r.Commitment || old.Reference != r.Reference || old.Phase != r.Phase || old.OccurredAt != r.OccurredAt || r.Meta.RecordedAt.Before(old.Meta.RecordedAt) {
				return fmt.Errorf("invalid repair correction")
			}
			replaced[r.Supersedes] = true
		}
		if r.Kind == "action" && r.Action == "promise" {
			old, ok := promises[r.Commitment]
			if ok && r.Supersedes != old.Meta.ID {
				return fmt.Errorf("duplicate promise")
			}
			promises[r.Commitment] = r
		} else if r.Commitment != "" {
			p, ok := promises[r.Commitment]
			if !ok || !sameRepairScope(p, r) || r.OccurredAt < p.EffectAt {
				return fmt.Errorf("foreign or missing commitment")
			}
			if r.Kind == "action" && r.Action == "help" && (r.Resource != p.Resource || r.Units != p.Units || r.EffectAt > *p.Due) {
				return fmt.Errorf("help does not match commitment")
			}
		}
		if r.Reference != "" {
			ref, ok := seen[r.Reference]
			if !ok || replaced[r.Reference] || !sameRepairScope(ref, r) || r.OccurredAt <= ref.EffectAt {
				return fmt.Errorf("completion needs later evidence")
			}
			supported := false
			for _, id := range r.Meta.Supporting {
				supported = supported || id == r.Reference
			}
			if !supported {
				return fmt.Errorf("reference is not supporting evidence")
			}
			if r.Kind == "observation" {
				p := promises[r.Commitment]
				if ref.Kind != "action" || ref.Commitment != r.Commitment {
					return fmt.Errorf("observation/action mismatch")
				}
				if r.Finding == "fulfilled" && (ref.Action != "help" || r.OccurredAt > *p.Due) {
					return fmt.Errorf("fulfilment needs timely practical help")
				}
				if r.Finding == "breached" && ref.Action != "break_promise" && !(ref.Action == "promise" && r.OccurredAt > *p.Due) {
					return fmt.Errorf("breach needs independent later observation")
				}
			}
		}
		seen[r.Meta.ID] = r
	}
	return nil
}

func EncodeRepairLog(log []RepairRecord) ([]byte, error) {
	if e := ValidateRepairLog(log); e != nil {
		return nil, e
	}
	return json.Marshal(log)
}
func DecodeRepairLog(raw []byte) ([]RepairRecord, error) {
	var log []RepairRecord
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if len(raw) > MaxRepairRecords*4096 || d.Decode(&log) != nil || d.Decode(new(any)) != io.EOF || ValidateRepairLog(log) != nil {
		return nil, fmt.Errorf("invalid repair encoding")
	}
	return log, nil
}

// CurrentRepair checks every required exact grant on the complete current
// provenance closure. Corrections invalidate dependents; revoking a correction
// never resurrects old evidence. Historical replay still uses CURRENT rights.
func CurrentRepair(log []RepairRecord, actor, recipient ID, focus RelationshipFocus, at LogicalTime, grants []Grant) ([]RepairRecord, error) {
	if ValidateRepairLog(log) != nil || ids(actor, recipient) != nil || actor == recipient || focus.Validate() != nil || focus.RoleContext == "" || focus.Account != "" || at < 0 || len(grants) == 0 || len(grants) > 8 {
		return nil, fmt.Errorf("invalid repair projection")
	}
	seenGrants := map[Grant]bool{}
	pairs := 0
	for _, g := range grants {
		if g.Validate() != nil || seenGrants[g] || g.Operation != Read && g.Operation != Derive && g.Operation != ShareOnRequest {
			return nil, fmt.Errorf("invalid repair grant")
		}
		seenGrants[g] = true
	}
	for _, g := range grants {
		if g.Operation == Read || g.Operation == Derive {
			if g.Actor != g.Recipient {
				return nil, fmt.Errorf("repair read/derive must be self-use")
			}
			pair := g
			if g.Operation == Read {
				pair.Operation = Derive
				pairs++
			} else {
				pair.Operation = Read
			}
			if !seenGrants[pair] {
				return nil, fmt.Errorf("repair projection needs read AND derive")
			}
		}
	}
	if pairs == 0 {
		return nil, fmt.Errorf("repair projection lacks read/derive context")
	}
	seen := map[ID]RepairRecord{}
	replaced := map[ID]bool{}
	for _, r := range log {
		seen[r.Meta.ID] = r
		if r.LearnedAt <= at && r.Supersedes != "" {
			replaced[r.Supersedes] = true
		}
	}
	type visitKey struct {
		id       ID
		ancestor bool
	}
	memo := map[visitKey]bool{}
	visited := map[visitKey]bool{}
	var allowed func(ID, bool) bool
	allowed = func(id ID, correctionAncestor bool) (result bool) {
		key := visitKey{id, correctionAncestor}
		if visited[key] {
			return memo[key]
		}
		visited[key] = true
		defer func() { memo[key] = result }()
		r, ok := seen[id]
		if !ok || r.Actor != actor || r.Recipient != recipient || r.Focus != focus || r.LearnedAt > at || r.OccurredAt > at || replaced[id] && !correctionAncestor || r.Meta.Valid.Start > at || r.Meta.Valid.End != nil && at >= *r.Meta.Valid.End {
			return false
		}
		for _, g := range grants {
			if !r.Meta.Rights.Allows(PermissionRequest{Resource: id, Context: g}) {
				return false
			}
		}
		for _, list := range [][]ID{r.Meta.Parents, r.Meta.Supporting, r.Meta.Contradicting} {
			for _, parent := range list {
				if !allowed(parent, false) {
					return false
				}
			}
		}
		if r.Supersedes != "" && !allowed(r.Supersedes, true) {
			return false
		}
		if r.Reference != "" && !allowed(r.Reference, false) {
			return false
		}
		if r.Commitment != "" && r.Action != "promise" {
			found := false
			for _, p := range log {
				if p.Kind == "action" && p.Action == "promise" && p.Commitment == r.Commitment && !replaced[p.Meta.ID] {
					found = allowed(p.Meta.ID, false)
				}
			}
			if !found {
				return false
			}
		}
		return true
	}
	out := []RepairRecord{}
	for _, r := range log {
		if allowed(r.Meta.ID, false) {
			out = append(out, r)
		}
	}
	raw, _ := json.Marshal(out)
	var owned []RepairRecord
	_ = json.Unmarshal(raw, &owned)
	return owned, nil
}
