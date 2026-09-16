package architecture

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepositoryBoundaries(t *testing.T) {
	violations, err := Check("../..")
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 0 {
		t.Fatalf("forbidden imports: %v", violations)
	}
}

func TestIllegalTaggedImport(t *testing.T) {
	violations, err := Check("testdata/illegal")
	if err != nil {
		t.Fatal(err)
	}
	// One fixture per protected package. Each is named so a regression says which
	// rule stopped being enforced rather than only that the count moved.
	want := map[string]string{
		"testdata/illegal/core/bad.go":                 "simulator",
		"testdata/illegal/examples/coreclient/bad.go":  "simulator",
		"testdata/illegal/examples/agentclient/bad.go": "app/hws",
	}
	if len(violations) != len(want) {
		t.Fatalf("expected %d tagged illegal imports, got %v", len(want), violations)
	}
	for file, target := range want {
		found := false
		for _, v := range violations {
			if strings.Contains(v, file) && strings.Contains(v, target) {
				found = true
			}
		}
		if !found {
			t.Fatalf("%s importing %s was not reported; got %v", file, target, violations)
		}
	}
}

func TestRules(t *testing.T) {
	for _, tc := range []struct {
		from, target string
		want         bool
	}{
		{"core/claim", "github.com/tushardhara/dream/simulator", true},
		{"core", "github.com/tushardhara/dream/app/graph", true},
		{"core", "github.com/tushardhara/dream/adapters/db", true},
		{"core", "github.com/tushardhara/dream/api/gen", true},
		{"core", "google.golang.org/protobuf/proto", true},
		{"core", "github.com/provider/sdk", true},
		{"core", "net/http", true},
		{"core", "time", false},
		{"core", "os", true},
		{"core", "os/exec", true},
		{"core", "syscall", true},
		{"core", "database/sql", true},
		{"core", "plugin", true},
		{"core", "github.com/tushardhara/dream/core/claim", false},
		{"app/graph", "github.com/tushardhara/dream/simulator", true},
		{"app/assistance", "github.com/tushardhara/dream/simulator", true},
		{"app/assistance", "github.com/tushardhara/dream/app/hws", true},
		{"app/assistance", "github.com/tushardhara/dream/evals", true},
		{"examples/assistanceclient", "github.com/tushardhara/dream/simulator", true},
		{"examples/assistanceclient", "github.com/tushardhara/dream/app/assistance", false},
		{"app/graph", "github.com/tushardhara/dream/adapters", true},
		{"app/hws", "github.com/tushardhara/dream/adapters/postgres", true},
		{"simulator", "github.com/tushardhara/dream/adapters", true},
		{"simulator", "github.com/tushardhara/dream/evals", true},
		{"simulator/experiment", "github.com/tushardhara/dream/evals", true},
		{"adapters/model", "github.com/tushardhara/dream/evals", true},
		{"adapters/transport", "github.com/tushardhara/dream/adapters/evaluation", true},
		{"cmd/hws-api", "github.com/tushardhara/dream/evals", true},
		{"cmd/hws-eval", "github.com/tushardhara/dream/evals", false},
		{"app/hws", "github.com/tushardhara/dream/evals", true},
		{"app/graph", "github.com/tushardhara/dream/core", false},
		{"examples/graphclient", "github.com/tushardhara/dream/simulator", true},
		{"examples/graphclient", "github.com/tushardhara/dream/app/hws", true},
		{"examples/graphclient", "github.com/tushardhara/dream/adapters/postgres", true},
		{"examples/graphclient", "github.com/tushardhara/dream/app/graph", false},
		{"examples/graphclient", "github.com/tushardhara/dream/core", false},
		{"adapters", "github.com/tushardhara/dream/core", false},
		// #82: both example clients held their contract by accident until now,
		// falling through to the default case which permits anything.
		{"examples/coreclient", "github.com/tushardhara/dream/core", false},
		{"examples/coreclient", "fmt", false},
		{"examples/coreclient", "github.com/tushardhara/dream/simulator", true},
		{"examples/coreclient", "github.com/tushardhara/dream/app/graph", true},
		{"examples/coreclient", "github.com/tushardhara/dream/app/hws", true},
		{"examples/coreclient", "github.com/tushardhara/dream/adapters/postgres", true},
		{"examples/coreclient", "google.golang.org/grpc", true},
		{"examples/agentclient", "github.com/tushardhara/dream/adapters/transport/gen/dream/v1", false},
		{"examples/agentclient", "google.golang.org/grpc", false},
		{"examples/agentclient", "google.golang.org/protobuf/proto", false},
		{"examples/agentclient", "context", false},
		{"examples/agentclient", "github.com/tushardhara/dream/core", true},
		{"examples/agentclient", "github.com/tushardhara/dream/simulator", true},
		{"examples/agentclient", "github.com/tushardhara/dream/app/hws", true},
		{"examples/agentclient", "github.com/provider/sdk", true},
	} {
		t.Run(tc.from+"/"+tc.target, func(t *testing.T) {
			if got := forbidden(tc.from, tc.target); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestPlatformFilesAndMalformedSource(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "app", "graph"), 0755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(root, "app", "graph", "bad_windows.go")
	if err := os.WriteFile(p, []byte("//go:build windows && custom\n\npackage graph\nimport _ \"github.com/tushardhara/dream/simulator\"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	v, err := Check(root)
	if err != nil || len(v) != 1 {
		t.Fatalf("all-tag scan: %v %v", v, err)
	}
	if err := os.WriteFile(p, []byte("package !"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := Check(root); err == nil {
		t.Fatal("malformed Go must fail closed")
	}
}
