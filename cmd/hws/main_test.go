package main

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

func TestValidateCLI(t *testing.T) {
	b, err := os.ReadFile("../../examples/scenarios/quiet-overlap.yaml")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		args []string
		in   []byte
		code int
	}{{[]string{"scenario", "validate", "-"}, b, 0}, {[]string{"scenario", "validate", "../../examples/scenarios/quiet-overlap.yaml"}, nil, 0}, {[]string{"scenario", "validate", "-"}, []byte("version: 42"), 1}, {[]string{"scenario", "validate", "missing-file"}, nil, 1}, {[]string{"scenario", "run"}, nil, 2}, {[]string{"--help"}, nil, 0}} {
		var out, errOut bytes.Buffer
		code := run(tc.args, bytes.NewReader(tc.in), &out, &errOut)
		if code != tc.code {
			t.Fatalf("%v: %d %s", tc.args, code, out.String())
		}
		if len(tc.args) == 3 {
			var result struct {
				Valid  bool   `json:"valid"`
				Hash   string `json:"scenario_hash"`
				Errors []struct {
					Path string `json:"path"`
				} `json:"errors"`
			}
			if err = json.Unmarshal(out.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if code == 0 && (!result.Valid || len(result.Hash) != 64) {
				t.Fatal(out.String())
			}
			if code == 1 && (result.Valid || len(result.Errors) != 1 || result.Errors[0].Path == "") {
				t.Fatal(out.String())
			}
		}
	}
}
