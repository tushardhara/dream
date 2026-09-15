package assistance

import (
	"context"
	"sort"

	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/core"
)

const ListeningFlowVersion = "listening-flow.v1"

type ListeningRequest struct {
	Version                          string
	ID, Session, Helper, User, Other core.ID
	Purpose                          core.ID
	At                               core.LogicalTime
	Focus                            core.RelationshipFocus
	Mode                             string // private, joint
	Invite                           bool
	Accounts, Summaries              []graph.ContextProposal
}

func (r ListeningRequest) Validate() error {
	if r.Version != ListeningFlowVersion || r.ID.Validate() != nil || r.Session.Validate() != nil || r.Helper.Validate() != nil || r.User.Validate() != nil || r.Other.Validate() != nil || r.Purpose.Validate() != nil || r.User == r.Other || r.Helper == r.User || r.Helper == r.Other || r.At.Validate() != nil || r.Focus.Validate() != nil || r.Focus.RoleContext == "" || r.Mode != "private" && r.Mode != "joint" || len(r.Accounts) < 1 || len(r.Accounts) > 2 || len(r.Summaries) > 2 {
		return ErrInvalid
	}
	for _, group := range []struct {
		proposals []graph.ContextProposal
		sharing   bool
	}{{r.Accounts, false}, {r.Summaries, true}} {
		seen := map[core.ID]bool{}
		for _, p := range group.proposals {
			owner := p.Query.Scope.Owner
			if p.Version != 2 || p.Query.Validate() != nil || p.Query.Scope.Namespace != r.Session || owner != r.User && owner != r.Other || seen[owner] || p.Binding != r.ID || p.Query.Actor != r.Helper || p.Query.Purpose != r.Purpose || p.Query.ValidAt != r.At || p.Query.KnownAt != r.At || len(p.Sources) != 1 {
				return ErrInvalid
			}
			seen[owner] = true
			if group.sharing {
				if p.Mode != graph.AssistantDisclosure || p.Operation != core.ShareOnRequest || p.Recipient != r.User || r.Mode != "joint" {
					return ErrInvalid
				}
			} else if p.Mode != graph.ExternalContext || p.Operation != core.Read || p.Recipient != r.Helper || r.Mode == "private" && owner != r.User {
				return ErrInvalid
			}
		}
		if !group.sharing && !seen[r.User] {
			return ErrInvalid
		}
	}
	return nil
}

type ListeningSnapshot struct {
	Now        core.LogicalTime
	Current    map[core.ID]core.ID
	Boundaries []core.Boundary
}

// ListeningJournal authenticates exact participant requests and supplies current
// account identities. Commit shares a lock/transaction with authorization,
// account correction and revocation. Models run outside that critical section.
// Records are helper-private; only Response may be returned to the requesting user.
type ListeningJournal interface {
	Snapshot(context.Context, ListeningRequest) (ListeningSnapshot, error)
	History(context.Context, ListeningRequest) ([]ListeningRecord, error)
	Commit(context.Context, ListeningRequest, ListeningRecord, func(context.Context) error) error
}

type ListeningInput struct {
	Binding, Helper core.ID
	Account         core.ListeningAccount
	Evidence        graph.SafeContextItem
}
type ListeningInterpretation struct {
	Source, Speaker core.ID
	Code            string // uncertain, supported, contradicted: proposals, not facts
	Confidence      core.Confidence
}
type ListeningInterpreter interface {
	Interpret(context.Context, ListeningInput) (ListeningInterpretation, error)
}
type ListeningDifference struct {
	Key    core.ID
	Kind   string // different_observation_reports, different_values, alternative_hypotheses
	Owners []core.ID
}
type ListeningSummary struct {
	Speaker core.ID
	Words   string
}
type ListeningFactSupport struct {
	Speaker, Key core.ID
	Status       string // participant_report_only, never independent corroboration
}
type ListeningResponse struct {
	Version      string
	Format       string                 // selected text or transcript rendering
	Own          *core.ListeningAccount `json:",omitempty"`
	PartnerState string                 // unknown, independently_joined; never inferred feelings
	Next         string
	Prompt       string
	Options      []string
	Shared       []ListeningSummary
	Invitation   string `json:",omitempty"` // fixed opt-in invitation, delivered only to User
}
type ListeningRecord struct {
	Version                        string
	ID, Session, Helper, User      core.ID
	At                             core.LogicalTime
	Focus                          core.RelationshipFocus
	RequestHash, AuthorizationHash string
	Accounts                       []core.ListeningAccount
	Interpretations                []ListeningInterpretation
	Differences                    []ListeningDifference
	FactualSupport                 []ListeningFactSupport
	Response                       ListeningResponse
}

type ListeningHost struct {
	Policy      *graph.PolicyService
	Journal     ListeningJournal
	Interpreter ListeningInterpreter
}

func listeningScope(r ListeningRequest, class core.InteractionClass) core.InteractionScope {
	return core.InteractionScope{Version: core.InteractionScopeVersion, Initiator: r.User, Target: r.Other, Topic: core.ID(r.Focus.Domain), Class: class}
}

func listeningBoundary(r ListeningRequest, s ListeningSnapshot) bool {
	class := core.PrivatePreparation
	if r.Mode == "joint" {
		class = core.Discussion
	}
	classes := []core.InteractionClass{class}
	if len(r.Summaries) > 0 {
		classes = append(classes, core.SummarySharing)
	}
	if r.Invite {
		classes = append(classes, core.Discussion)
	}
	for _, c := range classes {
		d, e := core.EvaluateBoundaries(s.Boundaries, listeningScope(r, c), s.Now)
		if e != nil || !d.Allowed {
			return false
		}
	}
	return true
}

func listeningWait() ListeningResponse {
	return ListeningResponse{Version: ListeningFlowVersion, PartnerState: "unknown", Next: "WAIT", Shared: []ListeningSummary{}, Options: []string{}}
}

func (h ListeningHost) Execute(ctx context.Context, request ListeningRequest) (ListeningResponse, error) {
	r := clone(request)
	if r.Validate() != nil || h.Policy == nil || h.Journal == nil {
		return ListeningResponse{}, ErrInvalid
	}
	snapshot, err := h.Journal.Snapshot(ctx, r)
	if err != nil || snapshot.Now != r.At || len(snapshot.Current) > 2 || ctx.Err() != nil {
		return ListeningResponse{}, ErrDenied
	}
	if !listeningBoundary(r, snapshot) {
		return listeningWait(), nil
	}
	var caps []graph.ApprovedContext
	var accounts []core.ListeningAccount
	var items []graph.SafeContextItem
	for _, proposal := range r.Accounts {
		owner := proposal.Query.Scope.Owner
		if snapshot.Current[owner] != proposal.Sources[0] {
			return ListeningResponse{}, ErrDenied
		}
		cap, decision, e := h.Policy.Approve(ctx, proposal)
		if e != nil || !decision.Allowed {
			return ListeningResponse{}, ErrDenied
		}
		safe, decision, e := h.Policy.Revalidate(ctx, cap, r.ID)
		if e != nil || !decision.Allowed || len(safe.Items()) != 1 {
			return ListeningResponse{}, ErrDenied
		}
		item := safe.Items()[0]
		a, e := core.DecodeListeningAccount([]byte(item.Text))
		if e != nil || a.Source != item.Source || a.Speaker != owner || item.Observer != owner || item.Reporter != owner || item.Subject.Principal != owner || item.Subject.Reference != nil || a.Other != r.User && a.Other != r.Other || a.Focus != r.Focus {
			return ListeningResponse{}, ErrDenied
		}
		caps = append(caps, cap)
		accounts = append(accounts, a)
		items = append(items, item)
	}
	var own core.ListeningAccount
	for _, a := range accounts {
		if a.Speaker == r.User {
			own = a
		}
	}
	revalidate := func(c context.Context) error {
		current, e := h.Journal.Snapshot(c, r)
		if e != nil || c.Err() != nil || Digest(current) != Digest(snapshot) || !listeningBoundary(r, current) {
			return ErrDenied
		}
		for _, cap := range caps {
			if _, d, e := h.Policy.Revalidate(c, cap, r.ID); e != nil || !d.Allowed {
				return ErrDenied
			}
		}
		return nil
	}
	// Shareable words come from separately authored participant selections. Reading
	// a private account is not authority to disclose it or paraphrase its identity.
	shared := []ListeningSummary{}
	for _, p := range r.Summaries {
		selected := false
		for _, a := range accounts {
			selected = selected || a.Speaker == p.Query.Scope.Owner && a.Summary == p.Sources[0] && a.Confirmation != "unconfirmed"
		}
		if !selected {
			return ListeningResponse{}, ErrDenied
		}
		cap, d, e := h.Policy.Approve(ctx, p)
		if e != nil || !d.Allowed {
			return ListeningResponse{}, ErrDenied
		}
		out, d, e := h.Policy.Write(ctx, cap, r.ID, listeningQuote{owner: p.Query.Scope.Owner})
		if e != nil || !d.Allowed {
			return ListeningResponse{}, ErrDenied
		}
		caps = append(caps, cap)
		shared = append(shared, ListeningSummary{Speaker: p.Query.Scope.Owner, Words: out.Text})
	}
	history, e := h.Journal.History(ctx, r)
	if e != nil || len(history) > MaxHistory {
		return ListeningResponse{}, ErrDenied
	}
	for _, old := range history {
		if old.ID == r.ID {
			if old.RequestHash != Digest(r) || old.AuthorizationHash != Digest(snapshot) || revalidate(ctx) != nil {
				return ListeningResponse{}, ErrDenied
			}
			return clone(old.Response), nil
		}
	}
	if len(history) == MaxHistory {
		return ListeningResponse{}, ErrDenied
	}
	response := listeningWait()
	response.Own = &own
	response.Shared = shared
	if len(accounts) == 2 {
		response.PartnerState = "independently_joined"
	}
	interpretations := []ListeningInterpretation{}
	languageSupported := own.Preferences.Language == "en" || own.Preferences.Language == "es"
	if languageSupported && own.DesiredHelp != "pause" {
		for i, a := range accounts {
			if h.Interpreter == nil {
				break
			}
			interpretation, e := h.Interpreter.Interpret(ctx, ListeningInput{Binding: r.ID, Helper: r.Helper, Account: clone(a), Evidence: clone(items[i])})
			if e != nil {
				break
			}
			if interpretation.Source != a.Source || interpretation.Speaker != a.Speaker || interpretation.Confidence.Validate() != nil || interpretation.Code != "uncertain" && interpretation.Code != "supported" && interpretation.Code != "contradicted" {
				return ListeningResponse{}, ErrInvalid
			}
			// This is proposal confidence, not calibrated confidence in hidden intent.
			if interpretation.Confidence > .5 {
				interpretation.Confidence = .5
			}
			interpretations = append(interpretations, interpretation)
		}
		if len(interpretations) == len(accounts) {
			ownCode := "uncertain"
			for _, p := range interpretations {
				if p.Speaker == r.User {
					ownCode = p.Code
				}
			}
			response.Next = listeningNext(own, ownCode)
		}
	}
	if response.Next == "ask_goal" || response.Next == "ask_meaning" {
		asked := 0
		for _, old := range history {
			if old.User == r.User && old.Session == r.Session && old.Focus == r.Focus && (old.Response.Next == "ask_goal" || old.Response.Next == "ask_meaning") {
				asked++
				if old.At > r.At || r.At-old.At < ClarificationCooldown {
					response.Next = "WAIT"
				}
			}
		}
		if asked >= MaxClarifications {
			response.Next = "WAIT"
		}
	}
	response.Prompt, response.Options = listeningPrompt(response.Next, own.Preferences.Language)
	response.Format = own.Preferences.Channel
	response.Prompt = listeningPreferencePrompt(response.Next, response.Prompt, own.Preferences)
	if r.Invite && response.Next != "WAIT" {
		response.Invitation = "A shared discussion is optional; either person may decline or pause."
		if own.Preferences.Language == "es" {
			response.Invitation = "La conversación conjunta es opcional; cualquiera puede rechazarla o pausarla."
		}
	}
	if revalidate(ctx) != nil {
		return ListeningResponse{}, ErrDenied
	}
	facts := []ListeningFactSupport{}
	for _, a := range accounts {
		for _, clause := range a.Clauses {
			if clause.Kind == "observation" {
				facts = append(facts, ListeningFactSupport{Speaker: a.Speaker, Key: clause.Key, Status: "participant_report_only"})
			}
		}
	}
	record := ListeningRecord{Version: ListeningFlowVersion, ID: r.ID, Session: r.Session, Helper: r.Helper, User: r.User, At: r.At, Focus: r.Focus, RequestHash: Digest(r), AuthorizationHash: Digest(snapshot), Accounts: accounts, Interpretations: interpretations, Differences: listeningDifferences(accounts), FactualSupport: facts, Response: response}
	if h.Journal.Commit(ctx, r, clone(record), revalidate) != nil {
		return ListeningResponse{}, ErrDenied
	}
	return clone(response), nil
}

type listeningQuote struct{ owner core.ID }

func (w listeningQuote) Write(_ context.Context, safe graph.SafeContext) (graph.WriterDraft, error) {
	items := safe.Items()
	if len(items) != 1 || items[0].Observer != w.owner || items[0].Reporter != w.owner || items[0].Subject.Principal != w.owner || items[0].Subject.Reference != nil {
		return graph.WriterDraft{}, ErrDenied
	}
	return graph.WriterDraft{Spans: []graph.WriterSpan{{Source: items[0].Source, Start: 0, End: len(items[0].Text)}}}, nil
}

func listeningNext(a core.ListeningAccount, code string) string {
	if a.DesiredHelp == "unknown" {
		return "ask_goal"
	}
	if a.Confirmation == "unconfirmed" || code != "supported" {
		return "ask_meaning"
	}
	switch a.DesiredHelp {
	case "listen":
		return "acknowledge"
	case "understand":
		return "reflect_accounts"
	case "coordinate":
		return "offer_plan"
	}
	return "WAIT"
}

func listeningPrompt(next string, language core.ID) (string, []string) {
	options := []string{}
	if language == "es" {
		switch next {
		case "ask_goal":
			return "¿Prefieres escucha, comprensión, un plan o una pausa?", []string{"escucha", "comprensión", "plan", "pausa"}
		case "ask_meaning":
			return "No daré por supuesto lo que significa. Puedes aclararlo, corregirlo o pausar.", []string{"aclarar", "corregir", "pausar"}
		case "acknowledge":
			return "Has pedido que te escuche. No hace falta acordar un plan.", options
		case "reflect_accounts":
			return "Podemos separar lo observado, las interpretaciones y lo que importa a cada persona.", options
		case "offer_plan":
			return "Puedes elegir un siguiente paso práctico. No requiere acuerdo sobre los valores.", options
		}
	} else if language == "en" {
		switch next {
		case "ask_goal":
			return "Would you like listening, understanding, a practical plan, or a pause?", []string{"listening", "understanding", "plan", "pause"}
		case "ask_meaning":
			return "I will not assume what that means. You can clarify, correct it, or pause.", []string{"clarify", "correct", "pause"}
		case "acknowledge":
			return "You asked to be heard. You do not need to agree on a plan.", options
		case "reflect_accounts":
			return "We can separate reported observations, interpretations, and what each person values.", options
		case "offer_plan":
			return "You can choose a practical next step. Agreement about values is not required.", options
		}
	}
	return "", options
}

func listeningPreferencePrompt(next, prompt string, p core.CommunicationPreferences) string {
	if prompt == "" {
		return prompt
	}
	if next == "ask_meaning" && p.Style == "indirect" {
		prompt = "Would you like to explain what you mean, or leave it for now?"
		if p.Language == "es" {
			prompt = "¿Quieres explicar lo que quieres decir, o dejarlo por ahora?"
		}
	}
	if p.PlainLanguage && next == "reflect_accounts" {
		prompt = "What did each person notice? What matters to each person? Different answers can stay different."
		if p.Language == "es" {
			prompt = "¿Qué notó cada persona? ¿Qué le importa a cada una? Las respuestas pueden ser distintas."
		}
	}
	if p.ShortTurns {
		short := map[string]string{"ask_goal": "Listen, understand, plan, or pause?", "ask_meaning": "Clarify, correct, or pause?", "acknowledge": "I will listen. No plan is required.", "reflect_accounts": "Different accounts can stay different.", "offer_plan": "Choose a practical next step, or pause."}
		if p.Language == "es" {
			short = map[string]string{"ask_goal": "¿Escucha, comprensión, plan o pausa?", "ask_meaning": "¿Aclarar, corregir o pausar?", "acknowledge": "Te escucho. No hace falta un plan.", "reflect_accounts": "Los relatos pueden seguir siendo distintos.", "offer_plan": "Elige un paso práctico, o una pausa."}
		}
		prompt = short[next]
	}
	if p.Channel == "transcript" {
		label := "Helper: "
		if p.Language == "es" {
			label = "Asistente: "
		}
		prompt = label + prompt
	}
	return prompt
}

// These are exact authored-key/text comparisons, not semantic contradiction
// detection. Independent participation never corroborates another account by
// default; no branch chooses a winner or converts a value into a factual verdict.
func listeningDifferences(accounts []core.ListeningAccount) []ListeningDifference {
	out := []ListeningDifference{}
	if len(accounts) != 2 {
		return out
	}
	for _, a := range accounts[0].Clauses {
		for _, b := range accounts[1].Clauses {
			if a.Key != b.Key || a.Kind != b.Kind || a.Text == b.Text {
				continue
			}
			kind := map[string]string{"observation": "different_observation_reports", "value": "different_values", "hypothesis": "alternative_hypotheses"}[a.Kind]
			out = append(out, ListeningDifference{Key: a.Key, Kind: kind, Owners: []core.ID{accounts[0].Speaker, accounts[1].Speaker}})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}
