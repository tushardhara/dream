package evals

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/simulator/experiment"
)

const RealDay = 24 * time.Hour

// StudyPlan is an executable recorded/fake protocol. Live study activation and
// real-data ingestion are unavailable; provider/budget/infrastructure approval
// are separate owner gates, not boolean claims accepted from a candidate.
type StudyScope struct {
	Owner     core.ID `json:"owner"`
	Namespace core.ID `json:"namespace"`
	ID        core.ID `json:"id"`
}

func (s StudyScope) Validate() error {
	if s.Owner.Validate() != nil || s.Namespace.Validate() != nil || s.ID.Validate() != nil {
		return fmt.Errorf("invalid study scope")
	}
	return nil
}

type StudyProviderRef struct {
	Artifact string  `json:"artifact_sha256"`
	ID       core.ID `json:"id"`
	Kind     string  `json:"kind"`
	Version  string  `json:"version"`
}
type StudyPlan struct {
	Seed               uint64              `json:"seed"`
	Variant            experiment.Variant  `json:"candidate_variant"`
	Version            string              `json:"version"`
	Scope              StudyScope          `json:"scope"`
	StartUTC           time.Time           `json:"start_utc"`
	Days               int                 `json:"real_days"`
	FrozenBaselineHash string              `json:"frozen_baseline_hash"`
	CandidateVersion   string              `json:"candidate_version"`
	Providers          [2]StudyProviderRef `json:"providers"`
	DailyPredictions   int                 `json:"daily_prediction_quota"`
	TotalPredictions   int                 `json:"total_prediction_quota"`
}

func (p StudyPlan) Validate() error {
	hash := regexp.MustCompile(`^[0-9a-f]{64}$`)
	_, offset := p.StartUTC.Zone()
	if !p.Variant.Valid() || p.Version != "study-protocol.v1" || p.Scope.Validate() != nil || p.StartUTC.IsZero() || offset != 0 || p.Days != 30 || !hash.MatchString(p.FrozenBaselineHash) || p.CandidateVersion != experiment.Version || p.DailyPredictions < 2 || p.DailyPredictions > 64 || p.TotalPredictions < p.DailyPredictions || p.TotalPredictions > 30*p.DailyPredictions {
		return fmt.Errorf("invalid frozen 30-real-day fake protocol")
	}
	for _, provider := range p.Providers {
		if !regexp.MustCompile(`^sha256:[0-9a-f]{64}$`).MatchString(provider.Artifact) || provider.ID.Validate() != nil || (provider.Kind != "fake" && provider.Kind != "recorded") || provider.Version != experiment.Version {
			return fmt.Errorf("live/provider configuration requires separate owner authorization and host")
		}
	}
	if p.Providers[0].ID == p.Providers[1].ID {
		return fmt.Errorf("second provider port must have a separate binding")
	}
	return nil
}

type DailyStudyReport struct {
	Day           int       `json:"day"`
	At            time.Time `json:"recorded_at"`
	PlanHash      string    `json:"plan_hash"`
	BaselineHash  string    `json:"frozen_baseline_hash"`
	InputHash     string    `json:"approved_input_hash"`
	OutputHashes  [2]string `json:"output_hashes"`
	Reserved      int       `json:"reserved_predictions"`
	Finished      int       `json:"finished_predictions"`
	Engineering   Status    `json:"engineering"`
	Behavioral    Status    `json:"behavioral_evidence"`
	HumanValidity Status    `json:"real_human_validity"`
	CrossModel    Status    `json:"cross_model_validity"`
	LiveCalls     int       `json:"live_api_calls"`
	APICostMicros int64     `json:"incurred_api_cost_micros"`
}
type StudyEvent struct {
	Version   string            `json:"version"`
	Sequence  int               `json:"sequence"`
	Kind      string            `json:"kind"`
	Previous  string            `json:"previous_hash"`
	At        time.Time         `json:"at"`
	Plan      *StudyPlan        `json:"plan,omitempty"`
	PlanHash  string            `json:"plan_hash"`
	Day       int               `json:"day,omitempty"`
	Claim     core.ID           `json:"claim,omitempty"`
	Reserved  int               `json:"reserved_predictions,omitempty"`
	InputHash string            `json:"input_hash,omitempty"`
	Report    *DailyStudyReport `json:"report,omitempty"`
	Hash      string            `json:"hash"`
}

func sealStudyEvent(e StudyEvent) (StudyEvent, error) {
	e.Hash = ""
	h, err := Digest(e)
	e.Hash = h
	return e, err
}

type StudyCheckpoint struct {
	Plan          StudyPlan          `json:"plan"`
	PlanHash      string             `json:"plan_hash"`
	Deadline      time.Time          `json:"fixed_deadline"`
	NextDue       time.Time          `json:"next_due"`
	Sequence      int                `json:"sequence"`
	Hash          string             `json:"last_event_hash"`
	DaysRecorded  int                `json:"daily_records"`
	Reserved      int                `json:"reserved_predictions"`
	FailureStreak int                `json:"consecutive_failures"`
	Pending       *StudyEvent        `json:"pending,omitempty"`
	Reports       []DailyStudyReport `json:"reports"`
}

// RebuildStudy makes explicit, content-addressed checkpoints from the append-only
// journal. It never selects a "last session", refreshes the start or refunds an
// uncertain reservation. A missing/revoked event invalidates the whole chain.
func RebuildStudy(events []StudyEvent) (StudyCheckpoint, error) {
	s := StudyCheckpoint{Reports: []DailyStudyReport{}}
	if len(events) < 1 || len(events) > 61 {
		return s, fmt.Errorf("incomplete/oversized study journal")
	}
	for index, event := range events {
		sealed, e := sealStudyEvent(event)
		if e != nil || event.Hash != sealed.Hash || event.Version != "study-event.v1" || event.Sequence != index+1 || event.Previous != s.Hash || event.At.IsZero() {
			return s, fmt.Errorf("study event chain mismatch")
		}
		if index == 0 {
			if event.Kind != "registered" || event.Plan == nil || event.Plan.Validate() != nil {
				return s, fmt.Errorf("invalid study registration")
			}
			s.Plan = *event.Plan
			s.PlanHash, _ = Digest(s.Plan)
			if event.PlanHash != s.PlanHash {
				return s, fmt.Errorf("changed study plan")
			}
			s.Deadline = s.Plan.StartUTC.Add(30 * RealDay)
			s.NextDue = s.Plan.StartUTC
		} else {
			if event.Plan != nil || event.PlanHash != s.PlanHash || event.Day != s.DaysRecorded+1 || event.Day > 30 || event.At.Before(events[index-1].At) {
				return s, fmt.Errorf("study sequence/clock regression")
			}
			due := s.Plan.StartUTC.Add(time.Duration(event.Day-1) * RealDay)
			switch event.Kind {
			case "reserved":
				if s.Pending != nil || event.Claim.Validate() != nil || event.Reserved < 2 || event.Reserved%2 != 0 || event.Reserved > s.Plan.DailyPredictions || s.Reserved+event.Reserved > s.Plan.TotalPredictions || event.At.Before(due) || !event.At.Before(due.Add(RealDay)) || s.FailureStreak >= 3 || len(event.InputHash) != 64 || event.Report != nil {
					return s, fmt.Errorf("day not due, already reserved, quota or repeated-failure stop")
				}
				owned := event
				s.Pending = &owned
				s.Reserved += event.Reserved
			case "reported":
				p := s.Pending
				r := event.Report
				if p == nil || r == nil || event.Claim != p.Claim || r.Day != event.Day || r.At != event.At || r.PlanHash != s.PlanHash || r.BaselineHash != s.Plan.FrozenBaselineHash || r.InputHash != p.InputHash || r.Reserved != p.Reserved || r.Finished < 0 || r.Finished > r.Reserved || r.HumanValidity != NotTested || r.CrossModel != NotTested || r.Behavioral != Inconclusive || r.LiveCalls != 0 || r.APICostMicros != 0 || (r.Engineering != Pass && r.Engineering != Inconclusive) {
					return s, fmt.Errorf("invalid daily evidence or scientific claim")
				}
				if r.Engineering == Pass && (r.Finished != r.Reserved || len(r.OutputHashes[0]) != 64 || len(r.OutputHashes[1]) != 64) {
					return s, fmt.Errorf("false completed prediction batch")
				}
				if r.Engineering == Pass {
					s.FailureStreak = 0
				} else {
					s.FailureStreak++
				}
				s.Reports = append(s.Reports, *r)
				s.DaysRecorded++
				s.NextDue = s.Plan.StartUTC.Add(time.Duration(s.DaysRecorded) * RealDay)
				s.Pending = nil
			default:
				return s, fmt.Errorf("unsupported study event")
			}
		}
		s.Sequence = event.Sequence
		s.Hash = event.Hash
	}
	return s, nil
}

// Append must compare-and-swap the exact journal length and MUST reject even an
// identical repeated reservation at an old length. This prevents two workers
// from treating an idempotent append receipt as permission to generate twice.
type StudyJournal interface {
	Load(context.Context, StudyScope) ([]StudyEvent, error)
	Append(context.Context, StudyScope, int, StudyEvent) error
}
type StudyClock interface{ Now() time.Time }
type StudyProvider struct {
	Ref       StudyProviderRef
	Generator BatchGenerator
}
type StudyController struct {
	Journal   StudyJournal
	Clock     StudyClock
	Providers [2]StudyProvider
}

func (c StudyController) Register(ctx context.Context, p StudyPlan) (StudyCheckpoint, error) {
	if c.Journal == nil || c.Clock == nil || p.Validate() != nil {
		return StudyCheckpoint{}, fmt.Errorf("invalid study host/plan")
	}
	events, e := c.Journal.Load(ctx, p.Scope)
	if e != nil {
		return StudyCheckpoint{}, e
	}
	if len(events) > 0 {
		s, e := RebuildStudy(events)
		h, _ := Digest(p)
		if e != nil || s.PlanHash != h {
			return s, fmt.Errorf("existing study has different frozen plan")
		}
		return s, nil
	}
	h, _ := Digest(p)
	event, e := sealStudyEvent(StudyEvent{Version: "study-event.v1", Sequence: 1, Kind: "registered", At: c.Clock.Now().UTC(), Plan: &p, PlanHash: h})
	if e != nil {
		return StudyCheckpoint{}, e
	}
	if e = c.Journal.Append(ctx, p.Scope, 0, event); e != nil {
		return StudyCheckpoint{}, e
	}
	return RebuildStudy([]StudyEvent{event})
}
func (c StudyController) RunDay(ctx context.Context, scope StudyScope, claim core.ID, requests []experiment.Request) (StudyCheckpoint, error) {
	if c.Journal == nil || c.Clock == nil || len(requests) < 1 || len(requests) > 32 || claim.Validate() != nil {
		return StudyCheckpoint{}, fmt.Errorf("invalid daily host/input")
	}
	events, e := c.Journal.Load(ctx, scope)
	if e != nil {
		return StudyCheckpoint{}, e
	}
	s, e := RebuildStudy(events)
	if e != nil {
		return s, e
	}
	if s.Plan.Scope != scope {
		return s, fmt.Errorf("study scope mismatch")
	}
	if s.Pending != nil {
		return s, fmt.Errorf("uncertain reserved day: reconstruct from checkpoint; do not repeat generation")
	}
	for i, p := range c.Providers {
		if p.Generator == nil || p.Ref != s.Plan.Providers[i] {
			return s, fmt.Errorf("provider binding differs from frozen plan")
		}
	}
	for _, r := range requests {
		if r.Input.Validate() != nil || r.Variant != s.Plan.Variant || r.Seed != s.Plan.Seed {
			return s, fmt.Errorf("unapproved daily generation input")
		}
	}
	inputHash, e := Digest(requests)
	if e != nil {
		return s, e
	}
	reserved, e := sealStudyEvent(StudyEvent{Version: "study-event.v1", Sequence: s.Sequence + 1, Kind: "reserved", Previous: s.Hash, At: c.Clock.Now().UTC(), PlanHash: s.PlanHash, Day: s.DaysRecorded + 1, Claim: claim, Reserved: 2 * len(requests), InputHash: inputHash})
	if e != nil {
		return s, e
	}
	proposed := append(append([]StudyEvent{}, events...), reserved)
	if _, e = RebuildStudy(proposed); e != nil {
		return s, e
	}
	if e = c.Journal.Append(ctx, scope, s.Sequence, reserved); e != nil {
		return s, e
	} // no provider under a journal transaction
	report := DailyStudyReport{Day: reserved.Day, PlanHash: s.PlanHash, BaselineHash: s.Plan.FrozenBaselineHash, InputHash: inputHash, Reserved: reserved.Reserved, Engineering: Pass, Behavioral: Inconclusive, HumanValidity: NotTested, CrossModel: NotTested}
	for index, provider := range c.Providers {
		if ctx.Err() != nil || !c.Clock.Now().Before(s.Deadline) || !c.Clock.Now().Before(reserved.At.Add(5*time.Minute)) {
			report.Engineering = Inconclusive
			break
		}
		// Clone every batch: a fake or candidate cannot mutate the next provider's
		// approved inputs or pass its own predictions as context to the other port.
		raw, _ := json.Marshal(requests)
		var detached []experiment.Request
		_ = json.Unmarshal(raw, &detached)
		results, err := provider.Generator.Generate(ctx, detached)
		if err != nil || len(results) != len(requests) {
			report.Engineering = Inconclusive
			break
		}
		valid := true
		for i, p := range results {
			h, _ := experiment.RequestDigest(requests[i])
			valid = valid && p.Validate() == nil && p.RequestHash == h
		}
		if !valid {
			report.Engineering = Inconclusive
			break
		}
		report.OutputHashes[index], _ = Digest(results)
		report.Finished += len(results)
	}
	report.At = c.Clock.Now().UTC()
	finished, e := sealStudyEvent(StudyEvent{Version: "study-event.v1", Sequence: reserved.Sequence + 1, Kind: "reported", Previous: reserved.Hash, At: report.At, PlanHash: s.PlanHash, Day: reserved.Day, Claim: claim, Report: &report})
	if e != nil {
		return s, e
	}
	proposed = append(proposed, finished)
	next, e := RebuildStudy(proposed)
	if e != nil {
		return s, e
	}
	if e = c.Journal.Append(ctx, scope, reserved.Sequence, finished); e != nil {
		return s, e
	}
	return next, nil
}

// AbandonPending records uncertainty only after the bounded worker window. It
// never retries generation or refunds the reservation. A late completion loses
// the same compare-and-swap, so it cannot overwrite this durable uncertainty.
func (c StudyController) AbandonPending(ctx context.Context, scope StudyScope) (StudyCheckpoint, error) {
	if c.Journal == nil || c.Clock == nil {
		return StudyCheckpoint{}, fmt.Errorf("missing trusted study host")
	}
	events, e := c.Journal.Load(ctx, scope)
	if e != nil {
		return StudyCheckpoint{}, e
	}
	s, e := RebuildStudy(events)
	if e != nil {
		return s, e
	}
	if s.Plan.Scope != scope || s.Pending == nil || c.Clock.Now().Before(s.Pending.At.Add(5*time.Minute)) {
		return s, fmt.Errorf("no expired pending worker window")
	}
	p := s.Pending
	report := DailyStudyReport{Day: p.Day, At: c.Clock.Now().UTC(), PlanHash: s.PlanHash, BaselineHash: s.Plan.FrozenBaselineHash, InputHash: p.InputHash, Reserved: p.Reserved, Engineering: Inconclusive, Behavioral: Inconclusive, HumanValidity: NotTested, CrossModel: NotTested}
	event, e := sealStudyEvent(StudyEvent{Version: "study-event.v1", Sequence: s.Sequence + 1, Kind: "reported", Previous: s.Hash, At: report.At, PlanHash: s.PlanHash, Day: p.Day, Claim: p.Claim, Report: &report})
	if e != nil {
		return s, e
	}
	next, e := RebuildStudy(append(events, event))
	if e != nil {
		return s, e
	}
	if e = c.Journal.Append(ctx, scope, s.Sequence, event); e != nil {
		return s, e
	}
	return next, nil
}
