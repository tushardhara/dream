package assistance

import (
	"context"
	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/core"
)

const OrdinaryCooldown core.LogicalTime = 10
const OrdinaryBudgetWindow core.LogicalTime = 40
const OrdinaryMaxInterruptions = 2

type OrdinaryRequest struct {
	Version                             string
	ID, Session, Helper, User, Activity core.ID
	Participants                        []core.ID
	Kind                                core.OrdinaryKind
	Arm                                 string // generic, permitted_context; both retain identical safety gates
	At                                  core.LogicalTime
	Window                              core.Interval
	Effort                              map[core.ID]int64
	Requested                           bool
	PlanDecision, PlanOption            core.ID
	Preferences                         []graph.ContextProposal
	Story                               *graph.ContextProposal
}

func (r OrdinaryRequest) Validate() error {
	if r.Version != core.OrdinaryVersion || r.ID.Validate() != nil || r.Session.Validate() != nil || r.Helper.Validate() != nil || r.User.Validate() != nil || r.Activity.Validate() != nil || !r.Kind.Valid() || r.At < 0 || r.Window.Validate() != nil || r.Window.End == nil || r.Window.Start < r.At || len(r.Participants) < 2 || len(r.Participants) > 8 || r.Arm != "generic" && r.Arm != "permitted_context" && r.Arm != "none" {
		return ErrInvalid
	}
	seen := map[core.ID]bool{}
	for _, p := range r.Participants {
		n, ok := r.Effort[p]
		if p.Validate() != nil || p == r.Helper || seen[p] || !ok || n < 0 || n > 100 {
			return ErrInvalid
		}
		seen[p] = true
	}
	if !seen[r.User] || len(r.Effort) != len(seen) || len(r.Preferences) > len(seen) {
		return ErrInvalid
	}
	if r.Kind == core.OrdinaryCoordination {
		if r.PlanDecision.Validate() != nil || r.PlanOption.Validate() != nil {
			return ErrInvalid
		}
	} else if r.PlanDecision != "" || r.PlanOption != "" {
		return ErrInvalid
	}
	sources := map[core.ID]bool{}
	owners := map[core.ID]bool{}
	proposals := append([]graph.ContextProposal{}, r.Preferences...)
	if r.Story != nil {
		if r.Kind != core.OrdinaryAppreciation && r.Kind != core.OrdinaryMemory {
			return ErrInvalid
		}
		proposals = append(proposals, *r.Story)
	}
	for i, p := range proposals {
		owner := p.Query.Scope.Owner
		if p.Version != 2 || p.Query.Validate() != nil || p.Query.Scope.Namespace != r.Session || !seen[owner] || p.Binding != r.ID || p.Query.Actor != r.Helper || p.Query.Purpose != "help" || p.Query.ValidAt != r.At || p.Query.KnownAt != r.At || p.Mode != graph.ExternalContext || p.Operation != core.Read || p.Recipient != r.Helper || len(p.Sources) != 1 || sources[p.Sources[0]] {
			return ErrInvalid
		}
		sources[p.Sources[0]] = true
		if i < len(r.Preferences) {
			if owners[owner] {
				return ErrInvalid
			}
			owners[owner] = true
		}
	}
	return nil
}

type OrdinaryParticipation struct {
	Opportunity, Participant core.ID
	At                       core.LogicalTime
	Choice                   string
}

type OrdinarySnapshot struct {
	Participation      []OrdinaryParticipation
	Now                core.LogicalTime
	CurrentPreferences map[core.ID]core.ID
	CurrentStory       core.ID
	Boundaries         []core.Boundary
	History            []OrdinaryResponse // only this user's mechanical helper deliveries
	Groups             core.GroupHistory
	Reservations       []core.GroupReservation
	Budget             core.GroupBudget
}
type OrdinaryJournal interface {
	// Visit authenticates, projects, and delivers under the same transaction as
	// preference/story changes, revocation, boundaries and resource reservations.
	// Its callback is fixed bounded policy/rendering; no model/provider is invoked.
	VisitOrdinary(context.Context, OrdinaryRequest, func(context.Context, OrdinarySnapshot) (OrdinaryResponse, error)) (OrdinaryResponse, error)
}
type OrdinaryQuote struct {
	Author, Source core.ID
	Words          string
}
type OrdinaryPerson struct {
	Person          core.ID
	Preference      string
	ExpectedBenefit core.GroupQuantity
}
type OrdinaryResponse struct {
	Version                  string
	ID, User, Activity       core.ID
	At                       core.LogicalTime
	Action, Reason, Text     string
	Quote                    *OrdinaryQuote `json:",omitempty"`
	People                   []OrdinaryPerson
	PlanDecision, PlanOption core.ID
	GlobalWelfare            string
}
type OrdinaryHost struct {
	Policy  *graph.PolicyService
	Journal OrdinaryJournal
}

func ordinaryVariants(p graph.ContextProposal, user core.ID) []graph.ContextProposal {
	derive := p
	derive.Mode, derive.Operation = graph.InternalContext, core.Derive
	share := p
	share.Mode, share.Operation, share.Recipient = graph.AssistantDisclosure, core.ShareOnRequest, user
	return []graph.ContextProposal{p, derive, share}
}
func (h OrdinaryHost) ordinaryItem(ctx context.Context, p graph.ContextProposal, user core.ID) (graph.SafeContextItem, bool, error) {
	var item graph.SafeContextItem
	for _, q := range ordinaryVariants(p, user) {
		cap, d, e := h.Policy.Approve(ctx, q)
		if e != nil {
			return item, false, e
		}
		if !d.Allowed {
			return item, false, nil
		}
		safe, d, e := h.Policy.Revalidate(ctx, cap, p.Binding)
		if e != nil {
			return item, false, e
		}
		if !d.Allowed || len(safe.Items()) != 1 {
			return item, false, nil
		}
		next := safe.Items()[0]
		if item.Source != "" && Digest(item) != Digest(next) {
			return item, false, ErrDenied
		}
		item = next
	}
	if item.Source != p.Sources[0] || item.Observer != p.Query.Scope.Owner || item.Reporter != item.Observer || item.Subject.Principal != item.Observer || item.Subject.Reference != nil {
		return item, false, ErrDenied
	}
	return item, true, nil
}
func ordinaryBoundary(r OrdinaryRequest, s OrdinarySnapshot) bool {
	if core.ValidateBoundaryLog(s.Boundaries) != nil {
		return false
	}
	for _, other := range r.Participants {
		if other == r.User {
			continue
		}
		scope := core.InteractionScope{Version: core.InteractionScopeVersion, Initiator: r.User, Target: other, Topic: r.Activity, Class: core.Coordination}
		d, e := core.EvaluateBoundaries(s.Boundaries, scope, s.Now)
		if e != nil || !d.Allowed {
			return false
		}
	}
	return true
}
func ordinaryEffort(r OrdinaryRequest, s OrdinarySnapshot, person core.ID) bool {
	capacity, known := s.Budget.Personal[person]
	if !known {
		return false
	}
	used := int64(0)
	for _, res := range s.Reservations {
		if r.Window.Start < *res.Window.End && res.Window.Start < *r.Window.End {
			for _, task := range res.Tasks {
				if task.Owner == person {
					used += task.Units
				}
			}
		}
	}
	return used+r.Effort[person] <= capacity
}
func (h OrdinaryHost) Execute(ctx context.Context, r OrdinaryRequest, recorded *OrdinaryResponse) (OrdinaryResponse, error) {
	if r.Validate() != nil || h.Policy == nil || h.Journal == nil {
		return OrdinaryResponse{}, ErrInvalid
	}
	return h.Journal.VisitOrdinary(ctx, r, func(tx context.Context, s OrdinarySnapshot) (OrdinaryResponse, error) {
		out := OrdinaryResponse{Version: core.OrdinaryVersion, ID: r.ID, User: r.User, Activity: r.Activity, At: s.Now, Action: "WAIT", Reason: "insufficient_permitted_benefit", People: []OrdinaryPerson{}, GlobalWelfare: "NOT_AGGREGATED"}
		for _, p := range r.Participants {
			out.People = append(out.People, OrdinaryPerson{Person: p, Preference: "unknown", ExpectedBenefit: core.UnknownGroupQuantity()})
		}
		finish := func() (OrdinaryResponse, error) {
			if tx.Err() != nil {
				return OrdinaryResponse{}, ErrDenied
			}
			if recorded != nil && Digest(*recorded) != Digest(out) {
				return OrdinaryResponse{}, ErrDenied
			}
			return clone(out), nil
		}
		if tx.Err() != nil || s.Now != r.At || len(s.History) > MaxHistory || core.ValidateGroupPortfolio(s.Groups, s.Reservations, s.Budget) != nil {
			return OrdinaryResponse{}, ErrDenied
		}
		if r.Arm == "none" {
			out.Reason = "disabled"
			return finish()
		}
		if r.Kind == core.OrdinaryQuiet {
			return finish()
		}
		if s.Now > r.Window.Start || s.Now >= *r.Window.End {
			out.Reason = "timing"
			return finish()
		}
		if !ordinaryBoundary(r, s) {
			out.Reason = "boundary"
			return finish()
		}
		recent := 0
		for _, old := range s.History {
			if old.User != r.User || old.ID == r.ID || old.Action == "WAIT" {
				continue
			}
			if old.At > s.Now {
				return OrdinaryResponse{}, ErrDenied
			}
			if s.Now-old.At < OrdinaryCooldown {
				out.Reason = "interruption_cooldown"
				return finish()
			}
			if s.Now-old.At < OrdinaryBudgetWindow {
				recent++
			}
		}
		if recent >= OrdinaryMaxInterruptions {
			out.Reason = "interruption_budget"
			return finish()
		}
		if len(r.Preferences) != len(r.Participants) {
			return finish()
		}
		for _, p := range r.Preferences {
			if s.CurrentPreferences[p.Query.Scope.Owner] != p.Sources[0] {
				return finish()
			}
			item, allowed, e := h.ordinaryItem(tx, p, r.User)
			if e != nil {
				return OrdinaryResponse{}, ErrDenied
			}
			if !allowed {
				return finish()
			}
			pref, e := core.DecodeOrdinaryPreference([]byte(item.Text))
			if e != nil || pref.Person != item.Observer || pref.Source != item.Source || pref.Activity != r.Activity || pref.Kind != r.Kind {
				return OrdinaryResponse{}, ErrDenied
			}

			// Silence does not imply refusal or uncaring. After two unanswered
			// opportunities, pause until a participant supplies fresh willingness
			// or independently reports welcomed participation in this activity.
			cutoff := item.OccurredAt
			negative := core.LogicalTime(-1)
			for _, participation := range s.Participation {
				if participation.Participant != pref.Person {
					continue
				}
				if participation.At > s.Now {
					return OrdinaryResponse{}, ErrDenied
				}
				if participation.Choice == "welcomed" && participation.At > cutoff {
					cutoff = participation.At
				}
				if (participation.Choice == "declined" || participation.Choice == "unwelcome") && participation.At > negative {
					negative = participation.At
				}
			}
			if negative >= cutoff {
				out.Reason = "non_participation_pause"
				return finish()
			}
			missed := 0
			for _, old := range s.History {
				if old.ID != r.ID && old.User == r.User && old.Activity == r.Activity && old.Action != "WAIT" && old.At >= cutoff {
					missed++
				}
			}
			if missed >= 2 {
				out.Reason = "non_participation_pause"
				return finish()
			}
			for i := range out.People {
				if out.People[i].Person == pref.Person {
					out.People[i].Preference = pref.Choice
					out.People[i].ExpectedBenefit = pref.ExpectedBenefit
				}
			}
			if pref.Choice != "wanted" || pref.Availability != "available" || pref.Window.Start > r.Window.Start || *pref.Window.End < *r.Window.End || pref.ExpectedBenefit.Status != core.Observed || *pref.ExpectedBenefit.Value <= 0 || pref.MaxEffort.Status != core.Observed || *pref.MaxEffort.Value < float64(r.Effort[pref.Person]) || !ordinaryEffort(r, s, pref.Person) {
				return finish()
			}
		}
		if r.Kind == core.OrdinaryCoordination {
			if !r.Requested {
				out.Reason = "explicit_request_required"
				return finish()
			}
			var d core.GroupDecision
			for _, candidate := range s.Groups.Decisions {
				if candidate.ID == r.PlanDecision {
					d = candidate
				}
			}
			if d.Meta.Observer != r.User || Digest(d.Window) != Digest(r.Window) || len(d.Affected) != len(r.Participants) {
				return finish()
			}
			for _, p := range d.Affected {
				if _, ok := r.Effort[p]; !ok {
					return finish()
				}
			}
			gr := GroupRequest{Version: GroupFlowVersion, ID: r.ID, Session: r.Session, User: r.User, Helper: r.Helper, Decision: r.PlanDecision, Purpose: "help", At: r.At}
			gs := GroupSnapshot{Now: s.Now, History: s.Groups, Reservations: s.Reservations, Budget: s.Budget, Boundaries: s.Boundaries}
			if !GroupBoundariesAllow(gr, gs, d) || core.GroupOptionFeasible(s.Groups, s.Reservations, s.Budget, r.PlanDecision, r.PlanOption, s.Now, groupSharingGrants(gr)) != nil {
				return finish()
			}
			// Declared effort must cover the selected plan's actual task load;
			// otherwise a caller could understate somebody else's practical cost.
			for _, option := range d.Options {
				if option.ID != r.PlanOption {
					continue
				}
				assigned := map[core.ID]int64{}
				for _, task := range option.Tasks {
					assigned[task.Owner] += task.Units
				}
				for person, units := range assigned {
					if units > r.Effort[person] {
						return finish()
					}
				}
			}
			out.PlanDecision, out.PlanOption = r.PlanDecision, r.PlanOption
			out.Text = "An agreed plan is available; choose it only if you still want it."
		} else if r.Kind == core.OrdinaryAppreciation || r.Kind == core.OrdinaryMemory {
			if r.Story == nil || r.Story.Sources[0] != s.CurrentStory {
				return finish()
			}
			item, allowed, e := h.ordinaryItem(tx, *r.Story, r.User)
			if e != nil {
				return OrdinaryResponse{}, ErrDenied
			}
			if !allowed {
				return finish()
			}
			story, e := core.DecodeOrdinaryStory([]byte(item.Text))
			if e != nil || story.Source != item.Source || story.Author != item.Observer || story.Kind != r.Kind || story.Activity != r.Activity {
				return OrdinaryResponse{}, ErrDenied
			}
			if r.Arm == "permitted_context" {
				share := ordinaryVariants(*r.Story, r.User)[2]
				cap, d, e := h.Policy.Approve(tx, share)
				if e != nil || !d.Allowed {
					return OrdinaryResponse{}, ErrDenied
				}
				// Reuse the existing exact-span writer; decode only the policy-validated
				// participant record. No generated phrase can be attributed to its author.
				written, d, e := h.Policy.Write(tx, cap, r.ID, listeningQuote{owner: story.Author})
				if e != nil || !d.Allowed {
					return OrdinaryResponse{}, ErrDenied
				}
				sourced, e := core.DecodeOrdinaryStory([]byte(written.Text))
				if e != nil || Digest(sourced) != Digest(story) {
					return OrdinaryResponse{}, ErrDenied
				}
				story = sourced
				out.Quote = &OrdinaryQuote{Author: story.Author, Source: story.Source, Words: story.Words}
				out.Text = "Optional participant-authored words; no reply is required."
			} else {
				out.Text = "If you want, you can share your own words; no reply is required."
			}
		} else {
			out.Text = "An activity you both selected is available; declining is fine."
		}
		out.Action = string(r.Kind)
		out.Reason = "explicit_permitted_preferences"
		return finish()
	})
}
