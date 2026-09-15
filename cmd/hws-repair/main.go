package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/tushardhara/dream/examples/repairclient"
	"os"
)

func main() {
	flag.Parse()
	r, e := repairclient.Run(context.Background())
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
	if json.NewEncoder(os.Stdout).Encode(r) != nil {
		os.Exit(1)
	}
}
