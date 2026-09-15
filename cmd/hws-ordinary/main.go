package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/tushardhara/dream/examples/ordinaryexperiment"
	"os"
)

func main() {
	family := flag.String("family", "daily", "synthetic quiet, daily, unknown_benefit or declined_ritual")
	arm := flag.String("arm", "permitted_context", "none, generic or permitted_context")
	seed := flag.Uint64("seed", 3, "bounded reproducible native seed")
	flag.Parse()
	r, e := ordinaryexperiment.Run(context.Background(), *family, *arm, *seed)
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	if json.NewEncoder(os.Stdout).Encode(r) != nil {
		os.Exit(1)
	}
}
