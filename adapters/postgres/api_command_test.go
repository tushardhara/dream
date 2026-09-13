package postgres

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/url"
	"os"
	osexec "os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	api "github.com/tushardhara/dream/adapters/transport"
	"github.com/tushardhara/dream/app/hws"
	"github.com/tushardhara/dream/core"
)

func TestAPICommandStartsAuthenticatedAndStops(t *testing.T) {
	dsn := os.Getenv("DREAM_TEST_DSN")
	if dsn == "" {
		t.Skip("mandatory fresh PostgreSQL cluster")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	if err = Migrate(ctx, admin); err != nil {
		t.Fatal(err)
	}
	exec(t, admin, `CREATE ROLE dream_command_fixture LOGIN PASSWORD 'disposable_command' IN ROLE dream_writer; GRANT USAGE ON SCHEMA dream TO dream_command_fixture`)
	manifest := runtimeManifest(t, "api-command")
	if _, err = New(admin).CreateRun(ctx, manifest); err != nil {
		t.Fatal(err)
	}
	secret := "synthetic-command-credential-longer-than-thirty-two"
	digest := sha256.Sum256([]byte(secret))
	credentials := []api.Credential{{ID: "command-researcher", TokenSHA256: hex.EncodeToString(digest[:]), Caller: "researcher", Role: hws.ResearchViewKind, Scope: manifest.Scope, Principal: "a", Purpose: "research", Expires: time.Now().Add(time.Hour), RequestsPerMinute: 20, TotalRequests: 100}}
	config := map[string]any{"version": 1, "mode": "management-only", "grpc_address": "127.0.0.1:0", "http_address": "127.0.0.1:0", "development": true, "credentials": credentials, "grants": []hws.ViewGrant{{Caller: "researcher", Realm: hws.ViewRealm{Scope: manifest.Scope, Principal: "a"}, Kind: hws.ResearchViewKind, Purpose: "research", Operations: []core.Operation{core.Read, core.Derive, core.Retain, core.Export}}}}
	directory := t.TempDir()
	binary := filepath.Join(directory, "hws-api")
	configPath := filepath.Join(directory, "config.json")
	raw, _ := json.Marshal(config)
	if err = os.WriteFile(configPath, raw, 0600); err != nil {
		t.Fatal(err)
	}
	build := osexec.CommandContext(ctx, "go", "build", "-trimpath", "-o", binary, "./cmd/hws-api")
	build.Dir = "../.."
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v %s", err, output)
	}
	dbConfig, _ := pgxpool.ParseConfig(dsn)
	dbConfig.ConnConfig.User = "dream_command_fixture"
	dbConfig.ConnConfig.Password = "disposable_command"
	child := osexec.CommandContext(ctx, binary, "--config", configPath)
	runtimeURL, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	runtimeURL.User = url.UserPassword(dbConfig.ConnConfig.User, dbConfig.ConnConfig.Password)
	child.Env = append(os.Environ(), "DREAM_DATABASE_URL="+runtimeURL.String())
	stdout, err := child.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	child.Stderr = &stderr
	if err = child.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if child.ProcessState == nil {
			_ = child.Process.Kill()
			_ = child.Wait()
		}
	}()
	announced := make(chan string, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		if scanner.Scan() {
			announced <- scanner.Text()
		} else {
			announced <- ""
		}
	}()
	var line string
	select {
	case line = <-announced:
	case <-ctx.Done():
		t.Fatal("startup timeout")
	}
	address := ""
	for _, field := range strings.Fields(line) {
		if strings.HasPrefix(field, "http=") {
			address = strings.TrimPrefix(field, "http=")
		}
	}
	if address == "" {
		t.Fatal("server did not announce bounded listeners")
	}
	body, _ := json.Marshal(map[string]any{"scope": map[string]string{"operator": string(manifest.Scope.Actor), "namespace": string(manifest.Scope.Namespace), "world": string(manifest.Scope.World), "branch": string(manifest.Scope.Branch), "run": string(manifest.Scope.Run)}})
	for _, authenticated := range []bool{false, true} {
		request, _ := http.NewRequestWithContext(ctx, "POST", "http://"+address+"/dream.v1.Research/ResearchView", bytes.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		if authenticated {
			request.Header.Set("Authorization", "Bearer "+secret)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if authenticated && response.StatusCode != 200 || !authenticated && response.StatusCode != 401 {
			t.Fatal("command auth boundary", authenticated, response.StatusCode)
		}
	}
	_ = child.Process.Signal(os.Interrupt)
	if err = child.Wait(); err != nil {
		t.Fatal("command failed graceful stop", err)
	}
	if strings.Contains(stderr.String(), secret) || strings.Contains(stderr.String(), dbConfig.ConnConfig.Password) {
		t.Fatal("command exposed credentials")
	}
	workerBinary := filepath.Join(directory, "hws-worker")
	build = osexec.CommandContext(ctx, "go", "build", "-trimpath", "-o", workerBinary, "./cmd/hws-worker")
	build.Dir = "../.."
	if output, e := build.CombinedOutput(); e != nil {
		t.Fatalf("worker build: %v %s", e, output)
	}
	workerConfig := map[string]any{"version": 1, "development": true, "scopes": []hws.Scope{manifest.Scope}, "interval_seconds": 1, "max_cycles": 1}
	workerRaw, _ := json.Marshal(workerConfig)
	workerPath := filepath.Join(directory, "worker.json")
	if e := os.WriteFile(workerPath, workerRaw, 0600); e != nil {
		t.Fatal(e)
	}
	worker := osexec.CommandContext(ctx, workerBinary, "--config", workerPath)
	worker.Env = child.Env
	output, e := worker.CombinedOutput()
	if e != nil || !bytes.Contains(output, []byte("bounded cycle budget complete")) {
		t.Fatal("compiled maintenance worker failed")
	}
	if bytes.Contains(output, []byte(secret)) || bytes.Contains(output, []byte(dbConfig.ConnConfig.Password)) {
		t.Fatal("maintenance log exposed credential")
	}

}
