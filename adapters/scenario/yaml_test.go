package scenario

import (
	"bytes"
	"encoding/json"
	"errors"
	domain "github.com/tushardhara/dream/simulator/scenario"
	"io"
	"os"
	"strings"
	"testing"
)

func input(t testing.TB) []byte {
	t.Helper()
	b, err := os.ReadFile("../../examples/scenarios/quiet-overlap.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func TestFixtureGoldenAndBoundary(t *testing.T) {
	s, err := Parse(bytes.NewReader(input(t)))
	if err != nil {
		t.Fatal(err)
	}
	hash, err := s.Hash()
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("testdata/quiet-overlap.sha256")
	if err != nil {
		t.Fatal(err)
	}
	if hash != strings.TrimSpace(string(want)) {
		t.Fatalf("hash = %s", hash)
	}
	b, err := s.Canonical()
	if err != nil {
		t.Fatal(err)
	}
	golden, err := os.ReadFile("testdata/quiet-overlap.json")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(b, bytes.TrimSpace(golden)) {
		t.Fatal("canonical golden changed")
	}
	again, err := Parse(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	h, _ := again.Hash()
	if h != hash {
		t.Fatal("YAML/JSON hash mismatch")
	}
	v, err := s.View("ada")
	if err != nil {
		t.Fatal(err)
	}
	view, _ := json.Marshal(v)
	if bytes.Contains(view, []byte("CANARY")) {
		t.Fatalf("boundary leak: %s", view)
	}
	if s.Future[0].At <= 0 {
		t.Fatal("no quiet initial period")
	}
}
func TestYAMLRejects(t *testing.T) {
	base := string(input(t))
	cases := map[string]string{
		"unknown": strings.Replace(base, "seed: 42", "sead: 42", 1), "duplicate": strings.Replace(base, "seed: 42", "seed: 42, seed: 5", 1), "missing": strings.Replace(base, "seed: 42, ", "", 1), "string seed": strings.Replace(base, "seed: 42", "seed: '42'", 1), "overflow": strings.Replace(base, "seed: 42", "seed: 18446744073709551616", 1), "hex": strings.Replace(base, "seed: 42", "seed: 0x2a", 1), "null": strings.Replace(base, "memories: []", "memories: null", 1), "alias": "a: &a [1]\nb: *a", "recursive": "&a [*a]", "merge": "<<: {}", "tag": "!!map {}", "documents": base + "\n---\n{}", "empty": "", "depth": strings.Repeat("[", MaxDepth+3) + "0" + strings.Repeat("]", MaxDepth+3), "size": strings.Repeat(" ", domain.MaxBytes+1), "nodes": "[" + strings.Repeat("0,", MaxNodes) + "0]", "items": strings.Replace(base, "requires: [genesis.v1, resources.v1, memory.v1, relationships.v1, latent.v1, schedule.v1]", "requires: ["+strings.Repeat("x,", domain.MaxItems)+"x]", 1), "nan": strings.Replace(base, "confidence: 1", "confidence: .nan", 1), "yaml timestamp": strings.Replace(base, "name: Synthetic Ada", "name: 2026-01-01", 1),
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse(strings.NewReader(in))
			var e *domain.Error
			if !errors.As(err, &e) || e.Path == "" || e.Code == "" {
				t.Fatalf("expected structured error, got %v", err)
			}
		})
	}
	_, err := Parse(strings.NewReader(cases["unknown"]))
	var e *domain.Error
	_ = errors.As(err, &e)
	if e.Path != "$.world.sead" {
		t.Fatal(e)
	}
}

type endless struct{ read int }

func (r *endless) Read(b []byte) (int, error) {
	for i := range b {
		b[i] = ' '
	}
	r.read += len(b)
	return len(b), nil
}
func TestReaderBound(t *testing.T) {
	r := &endless{}
	_, err := Parse(r)
	if err == nil || r.read != domain.MaxBytes+1 {
		t.Fatalf("unbounded input: %d %v", r.read, err)
	}
	_, err = Parse(errorReader{})
	if err == nil {
		t.Fatal("read error ignored")
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) { return 0, io.ErrUnexpectedEOF }
func FuzzYAML(f *testing.F) {
	f.Add(input(f))
	f.Add([]byte("&a [*a]"))
	f.Add([]byte("version: 1\nversion: 2"))
	f.Fuzz(func(t *testing.T, b []byte) {
		s, err := Parse(bytes.NewReader(b))
		if err != nil {
			return
		}
		canonical, err := s.Canonical()
		if err != nil {
			t.Fatal(err)
		}
		again, err := Parse(bytes.NewReader(canonical))
		if err != nil {
			t.Fatal(err)
		}
		a, _ := s.Hash()
		c, _ := again.Hash()
		if a != c {
			t.Fatal("hash changed")
		}
	})
}
