package evals

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/behavior"
	"github.com/tushardhara/dream/simulator/experiment"
	"time"
)

type Split string

const (
	Train       Split = "train"
	Calibration Split = "calibration"
	Holdout     Split = "holdout"
)

type SourceKind string

const (
	Observation     SourceKind = "observation"
	SelfReport      SourceKind = "self_report"
	PrivateSurvey   SourceKind = "private_survey"
	PublicStatement SourceKind = "public_statement"
	LaterAction     SourceKind = "later_action"
)

type Source struct {
	ID         core.ID          `json:"id"`
	Kind       SourceKind       `json:"kind"`
	Observer   core.ID          `json:"observer"`
	Subject    core.ID          `json:"subject"`
	OccurredAt core.LogicalTime `json:"occurred_at"`
	LearnedAt  core.LogicalTime `json:"learned_at"`
	Synthetic  bool             `json:"synthetic"`
	Provenance core.ID          `json:"provenance"`
	Consent    Consent          `json:"consent"`
	Text       string           `json:"text"`
}
type Consent struct {
	Generate bool      `json:"generate"`
	Subject  core.ID   `json:"subject"`
	Purpose  core.ID   `json:"purpose"`
	Evaluate bool      `json:"evaluate"`
	Retain   bool      `json:"retain"`
	Revoked  bool      `json:"revoked"`
	Expires  time.Time `json:"expires"`
}

// ValidateImport describes a consent-aware schema. The supported import path is
// synthetic only; a schema-valid human record still has no ingestion authority.
func ValidateImport(s Source, now time.Time) error {
	validKind := s.Kind == Observation || s.Kind == SelfReport || s.Kind == PrivateSurvey || s.Kind == PublicStatement || s.Kind == LaterAction
	if !validKind || s.ID.Validate() != nil || s.Observer.Validate() != nil || s.Subject.Validate() != nil || s.Provenance.Validate() != nil || s.OccurredAt < 0 || s.LearnedAt < s.OccurredAt || len(s.Text) > 2048 || !s.Synthetic || s.Consent.Subject != s.Subject || s.Consent.Purpose != "evaluation" || !s.Consent.Evaluate || !s.Consent.Retain || s.Consent.Revoked || !now.Before(s.Consent.Expires) {
		return fmt.Errorf("source import not authorized")
	}
	if (s.Kind == SelfReport || s.Kind == PrivateSurvey) && s.Observer != s.Subject {
		return fmt.Errorf("report perspective mismatch")
	}
	return nil
}

type OutcomeStatus string

const (
	Observed   OutcomeStatus = "observed"
	Missing    OutcomeStatus = "missing"
	Censored   OutcomeStatus = "censored"
	NotTaken   OutcomeStatus = "not_taken"
	Unresolved OutcomeStatus = "unresolved"
)

type Label struct {
	Dimension  string           `json:"dimension"`
	Horizon    core.LogicalTime `json:"horizon"`
	Status     OutcomeStatus    `json:"status"`
	Value      *bool            `json:"value,omitempty"`
	Source     core.ID          `json:"source,omitempty"`
	Annotation string           `json:"annotation,omitempty"`
}
type Case struct {
	ID     core.ID          `json:"id"`
	Family core.ID          `json:"family"`
	People []core.ID        `json:"people"`
	Groups []core.ID        `json:"groups"`
	Split  Split            `json:"split"`
	Input  experiment.Input `json:"input"`
	Labels []Label          `json:"labels"`
}
type Dataset struct {
	Version string   `json:"version"`
	ID      core.ID  `json:"id"`
	Sources []Source `json:"sources"`
	Cases   []Case   `json:"cases"`
	Hash    string   `json:"hash"`
}

func Digest(value any) (string, error) {
	raw, e := json.Marshal(value)
	if e != nil {
		return "", e
	}
	h := sha256.Sum256(raw)
	return hex.EncodeToString(h[:]), nil
}
func SealDataset(d Dataset) (Dataset, error) { d.Hash = ""; h, e := Digest(d); d.Hash = h; return d, e }
func actionDimension(s string) bool {
	switch s {
	case "wait", "observe", "ask", "self_disclose", "help", "decline", "invite", "break_promise", "third_party_support":
		return true
	}
	return false
}
func (d Dataset) Validate(now time.Time) error {
	if d.Version != "evaluation-dataset.v1" || d.ID.Validate() != nil || len(d.Cases) < 1 || len(d.Cases) > 64 || len(d.Sources) > 512 {
		return fmt.Errorf("dataset envelope/budget")
	}
	copy := d
	copy.Hash = ""
	hash, e := Digest(copy)
	if e != nil || d.Hash != hash {
		return fmt.Errorf("dataset hash mismatch")
	}
	sources := map[core.ID]Source{}
	for _, s := range d.Sources {
		if _, ok := sources[s.ID]; ok || ValidateImport(s, now) != nil {
			return fmt.Errorf("invalid/unauthorized source")
		}
		sources[s.ID] = s
	}
	ids := map[core.ID]bool{}
	families := map[core.ID]Split{}
	people := map[core.ID]Split{}
	groups := map[core.ID]Split{}
	first := map[Split]core.LogicalTime{}
	last := map[Split]core.LogicalTime{}
	assigned := map[Split]bool{}
	for _, c := range d.Cases {
		if c.ID.Validate() != nil || c.Family.Validate() != nil || ids[c.ID] || c.Input.ID != c.ID || c.Input.Validate() != nil || len(c.People) < 1 || len(c.People) > 24 || len(c.Groups) < 1 || len(c.Groups) > 8 || len(c.Labels) < 1 || len(c.Labels) > 32 || (c.Split != Train && c.Split != Calibration && c.Split != Holdout) {
			return fmt.Errorf("invalid evaluation case")
		}
		ids[c.ID] = true
		for _, axis := range []struct {
			keys     []core.ID
			assigned map[core.ID]Split
		}{{[]core.ID{c.Family}, families}, {c.People, people}, {c.Groups, groups}} {
			local := map[core.ID]bool{}
			for _, id := range axis.keys {
				if id.Validate() != nil || local[id] {
					return fmt.Errorf("invalid split identity")
				}
				local[id] = true
				if previous, ok := axis.assigned[id]; ok && previous != c.Split {
					return fmt.Errorf("family/person/group crosses split")
				}
				axis.assigned[id] = c.Split
			}
		}
		ownerPresent := false
		for _, id := range c.People {
			ownerPresent = ownerPresent || id == c.Input.Initial.State.Actor
		}
		if err := validateInputSources(c, sources); err != nil {
			return err
		}
		if !ownerPresent {
			return fmt.Errorf("missing person split identity")
		}
		if !assigned[c.Split] || c.Input.Initial.State.At < first[c.Split] {
			first[c.Split] = c.Input.Initial.State.At
		}
		assigned[c.Split] = true
		if c.Input.AsOf > last[c.Split] {
			last[c.Split] = c.Input.AsOf
		}
		dimensions := map[string]bool{}
		for _, l := range c.Labels {
			key := fmt.Sprintf("%s/%d", l.Dimension, l.Horizon)
			if dimensions[key] || l.Horizon < 1 || l.Horizon > experiment.MaxHorizon || len(l.Annotation) > 256 {
				return fmt.Errorf("invalid/duplicate label")
			}
			dimensions[key] = true
			if c.Input.AsOf+l.Horizon > last[c.Split] {
				last[c.Split] = c.Input.AsOf + l.Horizon
			}
			if !actionDimension(l.Dimension) && l.Dimension != "latent_trust" && l.Dimension != "latent_motive" {
				return fmt.Errorf("unknown outcome dimension")
			}
			if l.Status == Observed {
				src, ok := sources[l.Source]
				if !ok || l.Value == nil || !actionDimension(l.Dimension) || src.Kind != LaterAction || src.Subject != c.Input.Initial.State.Actor || src.OccurredAt != c.Input.AsOf+l.Horizon {
					return fmt.Errorf("unresolved/latent assertion is not an observed outcome")
				}
				if src.LearnedAt > last[c.Split] {
					last[c.Split] = src.LearnedAt
				}
			} else if l.Status != Missing && l.Status != Censored && l.Status != NotTaken && l.Status != Unresolved {
				return fmt.Errorf("unknown label status")
			} else if l.Value != nil {
				return fmt.Errorf("unresolved outcome has a truth value")
			}
		}
	}
	// An explicit chronological boundary supplements family/person/group isolation.
	if assigned[Train] && assigned[Calibration] && last[Train] >= first[Calibration] || assigned[Calibration] && assigned[Holdout] && last[Calibration] >= first[Holdout] || assigned[Train] && assigned[Holdout] && last[Train] >= first[Holdout] {
		return fmt.Errorf("time leakage across splits")
	}
	raw, e := json.Marshal(d)
	if e != nil || len(raw) > 8<<20 {
		return fmt.Errorf("dataset byte budget")
	}
	return nil
}

// Every input provenance reference must be consented, known at its use time and
// in the declared person split. Labels have a separate later-action path above.
func validateInputSources(c Case, sources map[core.ID]Source) error {
	people := map[core.ID]bool{}
	for _, p := range c.People {
		people[p] = true
	}
	source := func(id core.ID, at core.LogicalTime) error {
		s, ok := sources[id]
		if !ok || !s.Consent.Generate || s.Observer != c.Input.Initial.State.Actor || s.LearnedAt > at || s.OccurredAt > at || !people[s.Subject] || !people[s.Observer] {
			return fmt.Errorf("unavailable input source/perspective")
		}
		return nil
	}
	memory := func(ms []behavior.Memory, at core.LogicalTime) error {
		for _, m := range ms {
			if !people[m.Other] {
				return fmt.Errorf("undeclared memory person")
			}
			for _, id := range m.Evidence {
				if e := source(id, at); e != nil {
					return e
				}
			}
		}
		return nil
	}
	beliefs := func(bs []behavior.Belief, at core.LogicalTime) error {
		for _, b := range bs {
			if !people[b.Observer] {
				return fmt.Errorf("undeclared belief observer")
			}
			if e := source(b.Source, at); e != nil {
				return e
			}
		}
		return nil
	}
	for _, cause := range c.Input.Initial.State.Causes {
		if e := source(cause, c.Input.Initial.State.At); e != nil {
			return e
		}
	}
	for _, receipt := range c.Input.Initial.State.Applied {
		if e := source(receipt.Event, c.Input.Initial.State.At); e != nil {
			return e
		}
	}
	if e := memory(c.Input.Initial.Memory, c.Input.Initial.State.At); e != nil {
		return e
	}
	if e := beliefs(c.Input.Initial.Beliefs, c.Input.Initial.State.At); e != nil {
		return e
	}
	situations := append([]experiment.Observation{}, c.Input.History...)
	situations = append(situations, experiment.Observation{At: c.Input.AsOf, Situation: c.Input.Current})
	for _, o := range situations {
		s := o.Situation
		if e := source(s.Perceived.Event, o.At); e != nil {
			return e
		}
		src := sources[s.Perceived.Event]
		if src.OccurredAt != s.Perceived.OccurredAt || src.LearnedAt != s.Perceived.LearnedAt {
			return fmt.Errorf("perception time mismatch")
		}
		if e := memory(s.Relationships, o.At); e != nil {
			return e
		}
		if e := beliefs(s.Beliefs, o.At); e != nil {
			return e
		}
		for _, id := range s.Present {
			if !people[id] {
				return fmt.Errorf("undeclared present person")
			}
		}
		for _, offer := range s.Offers {
			if offer.Recipient != "" && !people[offer.Recipient] {
				return fmt.Errorf("undeclared offer recipient")
			}
		}
		for _, commitment := range s.Commitments {
			if !people[commitment.Actor] || !people[commitment.Recipient] {
				return fmt.Errorf("undeclared commitment person")
			}
		}
		if s.DisclosureRecipient != "" && !people[s.DisclosureRecipient] {
			return fmt.Errorf("undeclared disclosure recipient")
		}
	}
	return nil
}
