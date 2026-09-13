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
	if len(violations) != 1 || !strings.Contains(violations[0], "simulator") {
		t.Fatalf("expected tagged illegal import, got %v", violations)
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
		{"core", "github.com/tushardhara/dream/core/claim", false},
		{"app/graph", "github.com/tushardhara/dream/simulator", true},
		{"app/graph", "github.com/tushardhara/dream/adapters", true},
		{"app/hws", "github.com/tushardhara/dream/adapters/postgres", true},
		{"simulator", "github.com/tushardhara/dream/adapters", true},
		{"simulator", "github.com/tushardhara/dream/evals", true},
		{"app/hws", "github.com/tushardhara/dream/evals", true},
		{"app/graph", "github.com/tushardhara/dream/core", false},
		{"adapters", "github.com/tushardhara/dream/core", false},
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
