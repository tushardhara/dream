package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAlignmentArtifactsAreExclusiveAndPrivate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plan.json")
	want := map[string]string{"fixture": "synthetic"}
	if e := writePrivateNew(path, want); e != nil {
		t.Fatal(e)
	}
	info, e := os.Stat(path)
	if e != nil || info.Mode().Perm() != 0600 {
		t.Fatal("artifact permissions", e)
	}
	if e = writePrivateNew(path, map[string]string{"fixture": "overwritten"}); e == nil {
		t.Fatal("retained plan overwritten")
	}
	var got map[string]string
	if e = readPrivate(path, &got); e != nil || got["fixture"] != "synthetic" {
		t.Fatal("retained plan changed", e)
	}
	link := filepath.Join(dir, "alias.json")
	if e = os.Symlink(path, link); e != nil {
		t.Fatal(e)
	}
	if e = writePrivateNew(link, want); e == nil {
		t.Fatal("symlink overwrite accepted")
	}
	if e = readPrivate(link, &got); e == nil {
		t.Fatal("symlink plan read accepted")
	}
}

func TestAlignmentCLIRequiresFrozenUnambiguousMode(t *testing.T) {
	for _, args := range [][]string{
		{"--synthetic", "--alignment-plan", "missing"},
		{"--synthetic", "--generator-image", "sha256:" + strings.Repeat("a", 64), "--source-revision", strings.Repeat("b", 40)},
		{"--synthetic", "--freeze-alignment-plan", "missing", "--development"},
		{"--synthetic", "--alignment-plan", "missing", "--config", "other"},
		{"--synthetic", "--alignment-plan", "missing", "--study-action", "day"},
	} {
		var out bytes.Buffer
		if e := run(args, &out); e == nil {
			t.Fatal("unfrozen/ambiguous alignment command accepted", args)
		}
		if json.Valid(out.Bytes()) {
			t.Fatal("failed command published an apparent report")
		}
	}
}
