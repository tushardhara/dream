package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tushardhara/dream/adapters/postgres"
	"github.com/tushardhara/dream/app/hws"
	"github.com/tushardhara/dream/core"
)

type wallClock struct{}

func (wallClock) Now() time.Time { return time.Now() }
func readArtifact(path string) (hws.DemoArtifact, error) {
	f, e := os.OpenFile(path, os.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK, 0)
	if e != nil {
		return hws.DemoArtifact{}, fmt.Errorf("artifact unavailable")
	}
	defer f.Close()
	st, e := f.Stat()
	if e != nil || !st.Mode().IsRegular() {
		return hws.DemoArtifact{}, fmt.Errorf("artifact must be a regular file")
	}
	raw, e := io.ReadAll(io.LimitReader(f, (32<<20)+1))
	if e != nil || len(raw) > 32<<20 {
		return hws.DemoArtifact{}, fmt.Errorf("artifact byte budget")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	var a hws.DemoArtifact
	if d.Decode(&a) != nil || d.Decode(new(any)) != io.EOF {
		return a, fmt.Errorf("invalid artifact")
	}
	return a, nil
}
func run(ctx context.Context, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("hws-demo", flag.ContinueOnError)
	flags.SetOutput(out)
	people := flags.Int("people", 4, "2/4 smoke, 5-person relational fixture, or 24 fictional adults")
	months := flags.Int("months", 1, "1..12 simulated fixed 30-day months")
	seed := flags.Uint64("seed", 11, "recorded deterministic reference seed")
	namespace := flags.String("namespace", "synthetic-demo", "stable owned namespace for restart")
	boundaries := flags.Int("max-boundaries", 64, "1..64 commit boundaries, allowing a clean partial checkpoint")
	development := flags.Bool("development", false, "explicit local/disposable cleartext database")
	file := flags.String("file", "", "new owner-only output for run; existing artifact for replay/export")
	expected := flags.String("expected-sha256", "", "independently retained artifact digest for replay/export")
	actor := flags.String("actor", "", "exact synthetic actor for offline own-view export")
	flags.Usage = func() {
		fmt.Fprintln(out, "Usage: hws-demo [flags] run|replay|export\nRecorded/fake synthetic backend demo only. run requires DREAM_DATABASE_URL with the existing non-owner runtime role. replay/export are offline. No live provider, paid/deployed study, UI or model-weight update. 30 REAL-day study NOT RUN.")
		flags.PrintDefaults()
	}
	if e := flags.Parse(args); e != nil {
		return e
	}
	if flags.NArg() != 1 || *file == "" {
		return fmt.Errorf("one action and --file required")
	}
	action := flags.Arg(0)
	if action == "replay" || action == "export" {
		if *expected == "" || action == "export" && *actor == "" || action == "replay" && *actor != "" {
			return fmt.Errorf("exact artifact digest and export actor required")
		}
		a, e := readArtifact(*file)
		if e != nil {
			return e
		}
		world, e := hws.VerifyDemo(a, *expected)
		if e != nil {
			return e
		}
		if action == "export" {
			projection, e := hws.ExportDemoActor(a, *expected, core.ID(*actor))
			if e != nil {
				return e
			}
			return json.NewEncoder(out).Encode(projection)
		}
		return json.NewEncoder(out).Encode(struct {
			Exact         bool   `json:"recorded_replay_exact"`
			Periods       int    `json:"simulated_periods"`
			WorldHash     string `json:"world_hash"`
			HumanValidity string `json:"real_human_validity"`
		}{true, world.Period, a.WorldHash, "not-tested"})
	}
	if action != "run" || *expected != "" || *actor != "" {
		return fmt.Errorf("invalid run options")
	}
	if _, e := os.Lstat(*file); e == nil || !os.IsNotExist(e) {
		return fmt.Errorf("output already exists or is unavailable; use a new checkpoint export path")
	}
	dsn := os.Getenv("DREAM_DATABASE_URL")
	if dsn == "" {
		return fmt.Errorf("explicit runtime database required")
	}
	config, e := pgxpool.ParseConfig(dsn)
	if e != nil || (!*development && !postgres.ProtectedConnection(config.ConnConfig)) {
		return fmt.Errorf("protected database configuration required")
	}
	config.MaxConns = 2
	bounded, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	pool, e := pgxpool.NewWithConfig(bounded, config)
	if e != nil {
		return fmt.Errorf("database unavailable")
	}
	defer pool.Close()
	store := postgres.New(pool)
	if e = store.RuntimeReady(bounded); e != nil {
		return fmt.Errorf("non-owner runtime role and current schema required")
	}
	var nonce [12]byte
	if _, e = rand.Read(nonce[:]); e != nil {
		return e
	}
	a, e := hws.RunDemo(bounded, store, hws.DemoOptions{Namespace: core.ID(*namespace), Holder: core.ID("demo:" + hex.EncodeToString(nonce[:])), People: *people, Months: *months, Seed: *seed, MaxBoundaries: *boundaries}, wallClock{})
	if e != nil {
		return fmt.Errorf("demo failed; committed progress remains in the same namespace/run")
	}
	raw, e := json.Marshal(a)
	if e != nil {
		return e
	}
	f, e := os.OpenFile(*file, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return fmt.Errorf("cannot create artifact")
	}
	_, e = f.Write(raw)
	if e == nil {
		e = f.Sync()
	}
	closed := f.Close()
	if e != nil || closed != nil {
		return fmt.Errorf("artifact write incomplete; durable run remains resumable")
	}
	return json.NewEncoder(out).Encode(struct {
		Hash          string        `json:"artifact_sha256"`
		WorldHash     string        `json:"world_hash"`
		Complete      bool          `json:"simulated_demo_completed"`
		Elapsed       time.Duration `json:"measured_runtime_ns"`
		Platform      string        `json:"platform"`
		CPUs          int           `json:"gomaxprocs"`
		ProviderCalls int           `json:"live_provider_calls"`
		Cost          int           `json:"incurred_api_cost_micros"`
		Study         string        `json:"30_real_day_study"`
	}{a.Hash, a.WorldHash, a.Completed, a.Elapsed, runtime.GOOS + "/" + runtime.GOARCH, runtime.GOMAXPROCS(0), 0, 0, "not-run"})
}
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if e := run(ctx, os.Args[1:], os.Stdout); e != nil {
		if e == flag.ErrHelp {
			return
		}
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
