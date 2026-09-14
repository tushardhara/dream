// Command hws provides offline scenario validation; it opens no listener.
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	parser "github.com/tushardhara/dream/adapters/scenario"
	"github.com/tushardhara/dream/app/hws"
	domain "github.com/tushardhara/dream/simulator/scenario"
	"io"
	"os"
)

func main() { os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }
func run(args []string, in io.Reader, out, errOut io.Writer) int {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Fprintln(out, "Usage: hws scenario validate <file.yaml|->\n       hws audit verify <file.json|-> --sha256 <independently-retained-hash>\nOffline strict v1 validation; JSON result, no database or model credentials.\nExit: 0 valid, 1 invalid/input error, 2 usage. Validation only; no execution command is exposed.")
		return 0
	}

	if len(args) == 5 && args[0] == "audit" && args[1] == "verify" && args[3] == "--sha256" {
		if args[2] != "-" {
			f, e := os.Open(args[2])
			if e != nil {
				fmt.Fprintln(errOut, "audit input unavailable")
				return 1
			}
			defer f.Close()
			in = f
		}
		raw, e := io.ReadAll(io.LimitReader(in, (32<<20)+1))
		if e != nil || len(raw) > 32<<20 {
			fmt.Fprintln(errOut, "audit input unavailable or oversized")
			return 1
		}
		d := json.NewDecoder(bytes.NewReader(raw))
		d.DisallowUnknownFields()
		var packet hws.AuditPacket
		if d.Decode(&packet) != nil || d.Decode(new(any)) != io.EOF || hws.VerifyAudit(packet, args[4]) != nil {
			fmt.Fprintln(errOut, "audit verification failed")
			return 1
		}
		fmt.Fprintln(out, `{"valid":true,"mode":"recorded_decision_replay","fresh_generation_equivalence":false}`)
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
