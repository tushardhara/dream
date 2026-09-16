// temporal-report executes only bounded synthetic, offline comparisons.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/tushardhara/dream/examples/temporalexperiment"
	"os"
)

func main() {
	output := flag.String("out", "", "optional report file")
	flag.Parse()
	seeds := []uint64{}
	for i := uint64(1); i <= 16; i++ {
		seeds = append(seeds, i)
	}
	report, e := temporalexperiment.Evaluate(context.Background(), seeds)
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	raw, e := json.MarshalIndent(report, "", "  ")
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	raw = append(raw, '\n')
	if *output != "" {
		e = os.WriteFile(*output, raw, 0644)
	} else {
		_, e = os.Stdout.Write(raw)
	}
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
