package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/tushardhara/dream/examples/groupexperiment"
	"os"
)

func main() {
	n := flag.Int("people", 5, "bounded synthetic group: 5 or 24")
	seed := flag.Uint64("seed", 11, "deterministic native seed")
	omitted := flag.Bool("omitted", false, "two explicitly reported prior omissions")
	burden := flag.Float64("care-burden", 4, "observed fictional caregiver burden")
	flag.Parse()
	c, e := groupexperiment.Fixture(*n, *seed, *omitted, *burden)
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	r, e := groupexperiment.Run(context.Background(), c)
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	if json.NewEncoder(os.Stdout).Encode(r) != nil {
		os.Exit(1)
	}
}
