package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/tushardhara/dream/core"
	"io"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tushardhara/dream/adapters/evaluation"
	"github.com/tushardhara/dream/adapters/postgres"
	"github.com/tushardhara/dream/evals"
	"github.com/tushardhara/dream/simulator/experiment"
)

type realStudyClock struct{}

func (realStudyClock) Now() time.Time { return time.Now().UTC() }

// studyAction has no scheduler and cannot launch a live provider. An explicit
// operator invocation registers, reconstructs, or advances at most one real day.
func studyAction(action, path, dataset string, development bool, out io.Writer) error {
	var plan evals.StudyPlan
	if e := readPrivate(path, &plan); e != nil {
		return e
	}
	if e := plan.Validate(); e != nil {
		return e
	}
	dsn := os.Getenv("DREAM_DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("explicit study runtime database required")
	}
	config, e := pgxpool.ParseConfig(dsn)
	if e != nil || !development && !postgres.ProtectedConnection(config.ConnConfig) {
		return fmt.Errorf("protected study database required")
	}
	config.MaxConns = 4
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	db, e := pgxpool.NewWithConfig(ctx, config)
	if e != nil {
		return fmt.Errorf("study database unavailable")
	}
	defer db.Close()
	journal, e := evaluation.NewStudyJournal(db)
	if e != nil {
		return e
	}
	controller := evals.StudyController{Journal: journal, Clock: realStudyClock{}}
	if action != "register" {
		events, err := journal.Load(ctx, plan.Scope)
		if err != nil {
			return err
		}
		frozen, err := evals.RebuildStudy(events)
		if err != nil {
			return err
		}
		hash, err := evals.Digest(plan)
		if err != nil || hash != frozen.PlanHash {
			return fmt.Errorf("study file differs from frozen registered plan")
		}
	}
	var state evals.StudyCheckpoint
	switch action {
	case "register":
		state, e = controller.Register(ctx, plan)
	case "status":
		var events []evals.StudyEvent
		events, e = journal.Load(ctx, plan.Scope)
		if e == nil {
			state, e = evals.RebuildStudy(events)
		}
	case "abandon-expired":
		state, e = controller.AbandonPending(ctx, plan.Scope)
	case "day":
		// Input is an evaluator-owned synthetic Dataset, never arbitrary generation
		// JSON or real human data. Provenance/consent/time validation runs before quota.
		var d evals.Dataset
		if dataset == "" {
			return fmt.Errorf("owner-only synthetic --dataset required for a study day")
		}
		if e = readPrivate(dataset, &d); e != nil {
			return e
		}
		if e = d.Validate(time.Now().UTC()); e != nil {
			return e
		}
		requests := []experiment.Request{}
		for _, row := range d.Cases {
			requests = append(requests, experiment.Request{Input: row.Input, Variant: plan.Variant, Seed: plan.Seed})
		}
		if len(requests) > 32 {
			return fmt.Errorf("daily input budget")
		}
		for i, ref := range plan.Providers {
			if ref.Kind != "fake" {
				return fmt.Errorf("this CLI supports only explicit networkless fake containers; other ports need a separately configured trusted host")
			}
			controller.Providers[i] = evals.StudyProvider{Ref: ref, Generator: evaluation.Container{Image: ref.Artifact}}
		}
		var claim core.ID
		claim, e = studyClaim()
		if e != nil {
			return e
		}
		state, e = controller.RunDay(ctx, plan.Scope, claim, requests)
	default:
		return fmt.Errorf("study action must be register, status, day or abandon-expired")
	}
	if e != nil {
		return e
	}
	hash, e := evals.Digest(plan)
	if e != nil || state.PlanHash != hash {
		return fmt.Errorf("study file differs from frozen registered plan")
	}
	return json.NewEncoder(out).Encode(struct {
		State         evals.StudyCheckpoint `json:"checkpoint"`
		HumanValidity evals.Status          `json:"real_human_validity"`
		CrossModel    evals.Status          `json:"cross_model_validity"`
		LiveStudy     string                `json:"live_study"`
		Promotion     string                `json:"promotion"`
	}{state, evals.NotTested, evals.NotTested, "not-run", "requires separate signed owner approval; no default changed"})
}

func studyClaim() (core.ID, error) {
	var id [12]byte
	if _, e := rand.Read(id[:]); e != nil {
		return "", e
	}
	return core.ID("study-worker:" + hex.EncodeToString(id[:])), nil
}
