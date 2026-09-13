// Command hws provides offline scenario validation; it opens no listener.
package main

import (
	"encoding/json"
	"errors"
	"fmt"
	parser "github.com/tushardhara/dream/adapters/scenario"
	domain "github.com/tushardhara/dream/simulator/scenario"
	"io"
	"os"
)

func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }
func run(args []string, in io.Reader, out, errOut io.Writer) int {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(out, "Usage: hws scenario validate <file.yaml|->\nOffline strict v1 validation; JSON result, no database or model credentials.\nExit: 0 valid, 1 invalid/input error, 2 usage. Validation only; no execution command is exposed.")
		return 0
	}
	if len(args) != 3 || args[0] != "scenario" || args[1] != "validate" {
		fmt.Fprintln(errOut, "Usage: hws scenario validate <file.yaml|->")
		return 2
	}
	report := func(err error) int {
		e := &domain.Error{Path: "$", Code: "io", Message: "cannot read scenario file"}
		var structured *domain.Error
		if errors.As(err, &structured) {
			e = structured
		}
		_ = json.NewEncoder(out).Encode(struct {
			Valid  bool            `json:"valid"`
			Errors []*domain.Error `json:"errors"`
		}{false, []*domain.Error{e}})
		return 1
	}
	if args[2] != "-" {
		f, err := os.Open(args[2])
		if err != nil {
			return report(err)
		}
		defer f.Close()
		in = f
	}
	s, err := parser.Parse(in)
	if err != nil {
		return report(err)
	}
	hash, err := s.Hash()
	if err != nil {
		return report(err)
	}
	if err = json.NewEncoder(out).Encode(struct {
		Valid   bool   `json:"valid"`
		Version int    `json:"version"`
		Hash    string `json:"scenario_hash"`
	}{true, s.Version, hash}); err != nil {
		return 1
	}
	return 0
}
