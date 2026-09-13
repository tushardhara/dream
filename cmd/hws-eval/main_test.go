package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrivateEvaluatorFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "labels.json")
	if e := os.WriteFile(path, []byte(`{"value":"SYNTHETIC_LABEL"}`), 0600); e != nil {
		t.Fatal(e)
	}
	var v struct {
		Value string `json:"value"`
	}
	if e := readPrivate(path, &v); e != nil || v.Value != "SYNTHETIC_LABEL" {
		t.Fatal(e)
	}
	if e := os.Chmod(path, 0644); e != nil {
		t.Fatal(e)
	}
	if readPrivate(path, &v) == nil {
		t.Fatal("world-readable labels accepted")
	}
	_ = os.Chmod(path, 0600)
	link := path + ".link"
	if e := os.Symlink(path, link); e != nil {
		t.Fatal(e)
	}
	if readPrivate(link, &v) == nil {
		t.Fatal("symlink label source accepted")
	}
	for _, bad := range []string{`{"value":"ok","labels":"extra"}`, `{"value":"ok"} {}`, strings.Repeat("x", (8<<20)+1)} {
		if e := os.WriteFile(path, []byte(bad), 0600); e != nil {
			t.Fatal(e)
		}
		if readPrivate(path, &v) == nil {
			t.Fatal("malformed/unbounded evaluator file")
		}
	}
}
func TestCLIRejectsAmbiguousConfiguration(t *testing.T) {
	for _, args := range [][]string{{}, {"--synthetic"}, {"--synthetic", "--dataset", "x", "--generator-image", "sha256:bad"}, {"--generator-image", "bad", "--dataset", "x"}} {
		if run(args, &bytes.Buffer{}) == nil {
			t.Fatal("invalid configuration accepted")
		}
	}
}
