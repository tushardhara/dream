// Command hws-worker performs bounded single-host operational reconciliation.
// It does not select a simulation policy, start providers or provision a database.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tushardhara/dream/adapters/postgres"
	"github.com/tushardhara/dream/app/hws"
	"github.com/tushardhara/dream/core"
)

type configuration struct {
	Version         int         `json:"version"`
	Scopes          []hws.Scope `json:"scopes"`
	Development     bool        `json:"development"`
	IntervalSeconds int         `json:"interval_seconds"`
	MaxCycles       int         `json:"max_cycles"`
}

func decode(raw []byte) (configuration, error) {
	var c configuration
	if len(raw) > 65536 {
		return c, hws.ErrCommand
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&c) != nil || d.Decode(new(any)) != io.EOF || c.Version != 1 || len(c.Scopes) < 1 || len(c.Scopes) > 128 || c.IntervalSeconds < 1 || c.IntervalSeconds > 60 || c.MaxCycles < 1 || c.MaxCycles > 3600 {
		return c, hws.ErrCommand
	}
	seen := map[hws.Scope]bool{}
	for _, sc := range c.Scopes {
		if sc.Validate() != nil || seen[sc] {
			return c, hws.ErrCommand
		}
		seen[sc] = true
	}
	return c, nil
}

type maintenance interface {
	RuntimeReady(context.Context) error
	ReconcileModels(context.Context, hws.Scope) (int, error)
	Project(context.Context, core.ID, int) (int, error)
}

func cycle(ctx context.Context, store maintenance, c configuration) (int, int, error) {
	if err := store.RuntimeReady(ctx); err != nil {
		return 0, 0, err
	}
	reconciled := 0
	for _, sc := range c.Scopes {
		n, err := store.ReconcileModels(ctx, sc)
		if err != nil {
			return reconciled, 0, err
		}
		reconciled += n
	}
	projected, err := store.Project(ctx, "operations.v1", 100)
	return reconciled, projected, err
}
func work(ctx context.Context, store maintenance, c configuration, out io.Writer) int {
	failures := 0
	for index := 0; index < c.MaxCycles; index++ {
		if ctx.Err() != nil {
			return 0
		}
		bounded, cancel := context.WithTimeout(ctx, 30*time.Second)
		recovered, projected, err := cycle(bounded, store, c)
		cancel()
		if ctx.Err() != nil {
			return 0
		}
		if err != nil {
			failures++
			fmt.Fprintln(out, "hws-worker: cycle unavailable; durable state retained")
		} else {
			failures = 0
			fmt.Fprintf(out, "hws-worker: cycle=%d reconciled=%d projected=%d\n", index+1, recovered, projected)
		}
		if failures == 3 {
			fmt.Fprintln(out, "hws-worker: stopped after three failed cycles; inspect configuration/database and resume explicitly")
			return 1
		}
		if index+1 == c.MaxCycles {
			if failures != 0 {
				return 1
			}
			break
		}
		delay := time.Duration(c.IntervalSeconds) * time.Second
		if failures > 0 {
			delay *= time.Duration(1 << failures)
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return 0
		case <-timer.C:
		}
	}
	fmt.Fprintln(out, "hws-worker: bounded cycle budget complete")
	return 0
}
func run(ctx context.Context, args []string, out io.Writer) int {
	flags := flag.NewFlagSet("hws-worker", flag.ContinueOnError)
	flags.SetOutput(out)
	path := flags.String("config", "", "trusted bounded maintenance JSON configuration")
	flags.Usage = func() {
		fmt.Fprintln(out, "Usage: hws-worker --config <file>\nBounded model-attempt/outbox reconciliation using DREAM_DATABASE_URL and a non-owner runtime role.\nNo provider scheduling, simulation policy, migration, deployment or credential provisioning.")
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if *path == "" || flags.NArg() != 0 {
		flags.Usage()
		return 2
	}
	file, err := os.Open(*path)
	if err != nil {
		fmt.Fprintln(out, "hws-worker: configuration unavailable")
		return 1
	}
	raw, err := io.ReadAll(io.LimitReader(file, 65537))
	file.Close()
	if err != nil {
		fmt.Fprintln(out, "hws-worker: configuration unavailable")
		return 1
	}
	c, err := decode(raw)
	if err != nil {
		fmt.Fprintln(out, "hws-worker: configuration rejected")
		return 1
	}
	dsn := os.Getenv("DREAM_DATABASE_URL")
	if dsn == "" {
		fmt.Fprintln(out, "hws-worker: database configuration required")
		return 1
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil || (!c.Development && !postgres.ProtectedConnection(cfg.ConnConfig)) {
		fmt.Fprintln(out, "hws-worker: protected database configuration required")
		return 1
	}
	cfg.MaxConns = 4
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		fmt.Fprintln(out, "hws-worker: database unavailable")
		return 1
	}
	defer pool.Close()
	bounded, cancel := context.WithTimeout(ctx, 24*time.Hour)
	defer cancel()
	return work(bounded, postgres.New(pool), c, out)
}
func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	os.Exit(run(ctx, os.Args[1:], os.Stdout))
}
