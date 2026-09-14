package postgres

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tushardhara/dream/app/hws"
)

func TestDemoCLIIntegration(t *testing.T) {
	dsn := os.Getenv("DREAM_TEST_DSN")
	if dsn == "" {
		t.Skip("requires owned disposable PostgreSQL; full scale also runs in make demo-check")
	}
	ctx := context.Background()
	admin, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer admin.Close()
	if e = Migrate(ctx, admin); e != nil {
		t.Fatal(e)
	}
	// A fixed public fixture password in an isolated cluster; never a real credential.
	if _, e = admin.Exec(ctx, `CREATE ROLE dream_demo_test LOGIN PASSWORD 'disposable_demo_only' IN ROLE dream_writer`); e != nil {
		t.Fatal(e)
	}
	cfg, e := pgxpool.ParseConfig(dsn)
	if e != nil {
		t.Fatal(e)
	}
	cfg.ConnConfig.User = "dream_demo_test"
	cfg.ConnConfig.Password = "disposable_demo_only"
	writerURL, e := url.Parse(dsn)
	if e != nil {
		t.Fatal(e)
	}
	writerURL.User = url.UserPassword(cfg.ConnConfig.User, cfg.ConnConfig.Password)
	scratch := t.TempDir()
	if exported := os.Getenv("DREAM_DEMO_REPORT_DIR"); exported != "" {
		scratch = exported
	}
	binary := filepath.Join(scratch, "hws-demo")
	build := osexec.Command("go", "build", "-trimpath", "-o", binary, "./cmd/hws-demo")
	build.Dir = "../.."
	if output, e := build.CombinedOutput(); e != nil {
		t.Fatalf("compile demo: %v %s", e, output)
	}
	invoke := func(expectOK bool, args ...string) []byte {
		t.Helper()
		bounded, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		command := osexec.CommandContext(bounded, binary, args...)
		command.Env = append(os.Environ(), "DREAM_DATABASE_URL="+writerURL.String())
		output, e := command.CombinedOutput()
		if (e == nil) != expectOK {
			t.Fatalf("demo command success=%v expected=%v output=%s", e == nil, expectOK, output)
		}
		return output
	}
	run := func(ns string, people, months int, seed uint64, boundaries int, file string) hws.DemoArtifact {
		t.Helper()
		output := invoke(true, "--development", "--namespace", ns, "--people", fmt.Sprint(people), "--months", fmt.Sprint(months), "--seed", fmt.Sprint(seed), "--max-boundaries", fmt.Sprint(boundaries), "--file", file, "run")
		var summary struct {
			Platform string        `json:"platform"`
			CPUs     int           `json:"gomaxprocs"`
			Hash     string        `json:"artifact_sha256"`
			Elapsed  time.Duration `json:"measured_runtime_ns"`
			Study    string        `json:"30_real_day_study"`
			Calls    int           `json:"live_provider_calls"`
		}
		if json.Unmarshal(output, &summary) != nil || summary.Study != "not-run" || summary.Calls != 0 {
			t.Fatal("invalid runtime/cost/study report")
		}
		raw, e := os.ReadFile(file)
		if e != nil {
			t.Fatal(e)
		}
		var a hws.DemoArtifact
		if json.Unmarshal(raw, &a) != nil || a.Hash != summary.Hash {
			t.Fatal("artifact digest report mismatch")
		}
		info, e := os.Stat(file)
		if e != nil || info.Mode().Perm() != 0600 {
			t.Fatal("artifact permissions")
		}
		t.Logf("demo people=%d simulated_months=%d seed=%d elapsed=%s platform=%s GOMAXPROCS=%d live_calls=0 incurred_api_cost=0 real_day_study=NOT_RUN", people, months, seed, summary.Elapsed, summary.Platform, summary.CPUs)
		return a
	}
	first := run("demo-recovery", 4, 2, 11, 1, filepath.Join(scratch, "partial.json"))
	if first.Completed {
		t.Fatal("partial run claims completed")
	}
	time.Sleep(5 * time.Millisecond)
	fullPath := filepath.Join(scratch, "resumed.json")
	full := run("demo-recovery", 4, 2, 11, 64, fullPath)
	if !full.Completed {
		t.Fatal("resumed run incomplete")
	}
	fresh := run("demo-fresh", 4, 2, 11, 64, filepath.Join(scratch, "fresh.json"))
	if full.WorldHash != fresh.WorldHash {
		t.Fatal("resume changed derived world")
	}
	replay := invoke(true, "--file", fullPath, "--expected-sha256", full.Hash, "replay")
	if !strings.Contains(string(replay), `"recorded_replay_exact":true`) {
		t.Fatal("recorded replay failed")
	}
	export := invoke(true, "--file", fullPath, "--expected-sha256", full.Hash, "--actor", "person:01", "export")
	for _, secret := range []string{"DEMO_RESEARCH_LABEL_CANARY", "OWN_FICTIONAL_NOTE:person:02"} {
		if strings.Contains(string(export), secret) {
			t.Fatal("private export leak")
		}
	}
	invoke(false, "--file", fullPath, "--expected-sha256", strings.Repeat("0", 64), "replay")
	invoke(false, "--file", fullPath, "--expected-sha256", full.Hash, "--actor", "outsider", "export")
	invoke(false, "--development", "--file", fullPath, "run") // no overwrite
	if os.Getenv("DREAM_DEMO_FULL") == "1" {
		for _, seed := range []uint64{11, 23} {
			file := filepath.Join(scratch, fmt.Sprintf("year-%d.json", seed))
			a := run(fmt.Sprintf("demo-year-%d", seed), 24, 12, seed, 64, file)
			if !a.Completed {
				t.Fatal("full simulated demo incomplete")
			}
			invoke(true, "--file", file, "--expected-sha256", a.Hash, "replay")
			invoke(true, "--file", file, "--expected-sha256", a.Hash, "--actor", "person:24", "export")
		}
	} else {
		t.Log("full 24-person multi-seed year: separately required by make demo-check")
	}
}
