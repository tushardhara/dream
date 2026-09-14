package hws

import (
	_ "embed"
	"encoding/json"
	"testing"
)

//go:embed testdata/legacy-demo.json
var savedLegacy []byte

func TestSavedLegacyDemoReplay(t *testing.T) {
	raw := savedLegacy

	var a DemoArtifact
	if e := json.Unmarshal(raw, &a); e != nil {
		t.Fatal(e)
	}
	w, e := VerifyDemo(a, "a2f2ac277aedaa21cda087673dd379359aba0751e3aa64f6bf27f32945d03284")
	if e != nil {
		t.Fatal("pre-alignment recorded replay changed", e)
	}
	hash, e := w.Hash()
	if e != nil || hash != "9a3a1d524851c1499570865527fdbe2936901edc01bfc78fc85f73f677884597" {
		t.Fatal("saved world hash changed", e, hash)
	}
}
