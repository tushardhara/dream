// hws-assistance runs bounded synthetic helper arms. No live provider or outreach.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/tushardhara/dream/app/assistance"
	"github.com/tushardhara/dream/examples/helperexperiment"
)

func main() {
	seed := flag.Uint64("seed", 11, "matched synthetic seed")
	flag.Parse()
	type summary struct {
		Arm               assistance.Arm
		Humans, Delivered int
		FirstIntervention int
		Digest            string
		Validity          string
	}
	out := []summary{}
	for _, arm := range []assistance.Arm{assistance.None, assistance.Simple, assistance.Single, assistance.Multi} {
		run, e := helperexperiment.Run(context.Background(), arm, assistance.Coordinate, *seed, nil)
		if e != nil {
			fmt.Fprintln(os.Stderr, "synthetic experiment failed")
			os.Exit(1)
		}
		delivered := 0
		for _, h := range run.Helper {
			if h.Delivered {
				delivered++
			}
		}
		out = append(out, summary{arm, len(run.Humans), delivered, run.FirstIntervention, assistance.Digest(run), "SYNTHETIC; benefit and human validity NOT_TESTED"})
	}
	if e := json.NewEncoder(os.Stdout).Encode(out); e != nil {
		os.Exit(1)
	}
}
