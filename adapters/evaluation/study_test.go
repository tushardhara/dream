package evaluation

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tushardhara/dream/adapters/postgres"
	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/evals"
	"github.com/tushardhara/dream/simulator/experiment"
)

type protocolClock struct{ at time.Time }

func (c *protocolClock) Now() time.Time { return c.at }

type protocolFunc func(context.Context, []experiment.Request) ([]experiment.Projection, error)

func (f protocolFunc) Generate(c context.Context, r []experiment.Request) ([]experiment.Projection, error) {
	return f(c, r)
}
func TestStudyJournalIntegration(t *testing.T) {
	dsn := os.Getenv("DREAM_TEST_DSN")
	if dsn == "" {
		t.Skip("mandatory make demo-check supplies a fresh owned PostgreSQL cluster")
	}
	ctx := context.Background()
	admin, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer admin.Close()
	if e = postgres.Migrate(ctx, admin); e != nil {
		t.Fatal(e)
	}
	if _, e = admin.Exec(ctx, `CREATE ROLE dream_study_test LOGIN PASSWORD 'disposable_study_only' IN ROLE dream_writer`); e != nil {
		t.Fatal(e)
	}
	cfg, e := pgxpool.ParseConfig(dsn)
	if e != nil {
		t.Fatal(e)
	}
	cfg.ConnConfig.User = "dream_study_test"
	cfg.ConnConfig.Password = "disposable_study_only"
	cfg.MaxConns = 1
	db, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer db.Close()
	journal, e := NewStudyJournal(db)
	if e != nil {
		t.Fatal(e)
	}
	clock := &protocolClock{time.Now().UTC()}
	first := evals.StudyProviderRef{ID: "fake-primary", Kind: "fake", Version: experiment.Version, Artifact: "sha256:" + strings.Repeat("0", 64)}
	second := first
	second.ID = "fake-second"
	baseline, _ := evals.Digest("synthetic protocol baseline")
	plan := evals.StudyPlan{Seed: 11, Variant: experiment.Stateful, Version: "study-protocol.v1", Scope: evals.StudyScope{Owner: "evaluator", Namespace: "study-test", ID: "frozen"}, StartUTC: clock.at, Days: 30, FrozenBaselineHash: baseline, CandidateVersion: experiment.Version, Providers: [2]evals.StudyProviderRef{first, second}, DailyPredictions: 2, TotalPredictions: 60}
	// Multiple independent journals share a one-connection pool. No operation
	// may hold it while waiting for a second connection or a provider callback.
	parallelCtx, parallelCancel := context.WithTimeout(ctx, 5*time.Second)
	defer parallelCancel()
	registered := make(chan error, 8)
	for i := range 8 {
		go func(i int) {
			other := plan
			other.Scope.ID = core.ID(fmt.Sprintf("parallel:%d", i))
			j, err := NewStudyJournal(db)
			if err == nil {
				_, err = (evals.StudyController{Journal: j, Clock: clock}).Register(parallelCtx, other)
			}
			registered <- err
		}(i)
	}
	for range 8 {
		if err := <-registered; err != nil {
			t.Fatal("bounded pool concurrent registration", err)
		}
	}
	controller := evals.StudyController{Journal: journal, Clock: clock, Providers: [2]evals.StudyProvider{{Ref: first, Generator: experiment.Generator{}}, {Ref: second, Generator: experiment.Generator{}}}}
	if _, e = controller.Register(ctx, plan); e != nil {
		t.Fatal(e)
	}
	dataset, _, e := evals.SyntheticFixture()
	if e != nil {
		t.Fatal(e)
	}
	requests := []experiment.Request{{Input: dataset.Cases[0].Input, Variant: experiment.Stateful, Seed: 11}}
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	controller.Providers[0].Generator = protocolFunc(func(ctx context.Context, r []experiment.Request) ([]experiment.Projection, error) {
		once.Do(func() { close(entered) })
		<-release
		return (experiment.Generator{}).Generate(ctx, r)
	})
	result := make(chan error, 1)
	go func() { _, e := controller.RunDay(ctx, plan.Scope, "worker-one", requests); result <- e }()
	<-entered
	// Callback is outside every database transaction/lock. A separate Store can
	// reconstruct the pending checkpoint while the first provider is still active.
	secondJournal, e := NewStudyJournal(db)
	if e != nil {
		t.Fatal(e)
	}
	bounded, cancel := context.WithTimeout(ctx, time.Second)
	events, e := secondJournal.Load(bounded, plan.Scope)
	cancel()
	if e != nil || len(events) != 2 {
		t.Fatal("journal lock held by provider", e)
	}
	if e = secondJournal.Append(ctx, plan.Scope, 1, events[1]); e == nil {
		t.Fatal("idempotent receipt became another reservation")
	}
	if _, e = controller.RunDay(ctx, plan.Scope, "competing-worker", requests); e == nil {
		t.Fatal("concurrent generation admitted")
	}
	close(release)
	if e = <-result; e != nil {
		t.Fatal(e)
	}
	recovered, e := secondJournal.Load(ctx, plan.Scope)
	if e != nil {
		t.Fatal(e)
	}
	state, e := evals.RebuildStudy(recovered)
	if e != nil || state.DaysRecorded != 1 || state.Reserved != 2 || state.Reports[0].CrossModel != evals.NotTested {
		t.Fatal("restart/scientific claim", e)
	}
	// The operator CLI uses the same frozen journal and fails before generation
	// when a real day is not due. No image is launched by this clock-gate test.
	cliPlan := plan
	cliPlan.Scope.ID = "cli-future"
	cliPlan.StartUTC = time.Now().UTC().Add(evals.RealDay)
	directory := t.TempDir()
	binary := filepath.Join(directory, "hws-eval")
	build := exec.Command("go", "build", "-trimpath", "-o", binary, "./cmd/hws-eval")
	build.Dir = "../.."
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("compile study host: %v %s", err, output)
	}
	planPath := filepath.Join(directory, "plan.json")
	raw, _ := json.Marshal(cliPlan)
	if err := os.WriteFile(planPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	dataPath := filepath.Join(directory, "dataset.json")
	raw, _ = json.Marshal(dataset)
	if err := os.WriteFile(dataPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	writerURL, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	writerURL.User = url.UserPassword(cfg.ConnConfig.User, cfg.ConnConfig.Password)
	invoke := func(action string, ok bool, extra ...string) []byte {
		t.Helper()
		args := []string{"--development", "--study-plan", planPath, "--study-action", action}
		args = append(args, extra...)
		command := exec.Command(binary, args...)
		command.Env = append(os.Environ(), "DREAM_DATABASE_URL="+writerURL.String())
		out, err := command.CombinedOutput()
		if (err == nil) != ok {
			t.Fatalf("study CLI action=%s expected_success=%v output=%s", action, ok, out)
		}
		return out
	}
	invoke("register", true)
	invoke("day", false, "--dataset", dataPath)
	status := invoke("status", true)
	if !strings.Contains(string(status), `"sequence":1`) || !strings.Contains(string(status), `"daily_records":0`) {
		t.Fatal("early CLI day mutated journal")
	}
	invoke("abandon-expired", false)
	// The mandatory demo gate also runs one actual due CLI day through two
	// networkless fake image bindings. This is one synthetic batch, not 30 days.
	if image := os.Getenv("DREAM_STUDY_TEST_IMAGE"); image != "" {
		cliPlan.Scope.ID = "cli-due"
		cliPlan.StartUTC = time.Now().UTC().Add(-time.Minute)
		cliPlan.DailyPredictions = 2 * len(dataset.Cases)
		cliPlan.TotalPredictions = 30 * cliPlan.DailyPredictions
		for i := range cliPlan.Providers {
			cliPlan.Providers[i].Artifact = image
		}
		raw, _ = json.Marshal(cliPlan)
		if err := os.WriteFile(planPath, raw, 0600); err != nil {
			t.Fatal(err)
		}
		invoke("register", true)
		out := invoke("day", true, "--dataset", dataPath)
		var result struct {
			State evals.StudyCheckpoint `json:"checkpoint"`
		}
		if json.Unmarshal(out, &result) != nil || result.State.DaysRecorded != 1 ||
			len(result.State.Reports) != 1 || result.State.Reports[0].Engineering != evals.Pass ||
			result.State.Reports[0].Finished != cliPlan.DailyPredictions {
			t.Fatal("compiled due-day fake batch did not complete")
		}
		invoke("day", false, "--dataset", dataPath)
		t.Log("compiled study CLI: one due day completed with two networkless fake bindings; duplicate denied")
	}
	stream, _ := studyStream(plan.Scope)
	revocation := graph.AppendCommand{Actor: plan.Scope.Owner, Namespace: plan.Scope.Namespace, Operation: "study.revoke", Key: "revoke-study", ExpectedVersion: 0, Class: graph.ResearchPayload, Payload: graph.Payload{Version: 1, Text: "explicit synthetic study revocation"}, Event: core.Event{Version: 1, Stream: "revocations", Type: "revoke", Subject: core.Subject{Principal: plan.Scope.Owner}, Meta: core.Metadata{ID: "revoke-study", Observer: plan.Scope.Owner, Source: "study-test", Sensitivity: core.Restricted, Confidence: 1, Valid: core.Interval{}, RecordedAt: clock.at, Rights: core.Rights{Resource: "revoke-study"}}}}
	if _, e = postgres.New(db).Revoke(ctx, revocation, studyID(stream, 1)); e != nil {
		t.Fatal(e)
	}
	if _, e = secondJournal.Load(ctx, plan.Scope); e == nil {
		t.Fatal("revoked study baseline resurrected")
	}
	t.Log(fmt.Sprintf("PASS: strict reservation CAS, callback outside journal transaction, explicit checkpoint recovery, retained quota and revocation; two FAKE ports, cross-model/human validity NOT TESTED"))
}
