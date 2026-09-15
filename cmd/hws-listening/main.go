// hws-listening runs only the bounded synthetic, recorded listening fixture.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/tushardhara/dream/app/assistance"
	"github.com/tushardhara/dream/core"
	"github.com/tushardhara/dream/examples/listeningclient"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "hws-listening: offline synthetic budget-fight listening fixture; no live provider or outreach. Semantic/human quality NOT_TESTED.")
	}
	flag.Parse()
	if flag.NArg() != 0 {
		flag.Usage()
		os.Exit(2)
	}
	responses, err := listeningclient.Run(context.Background())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	output := struct {
		Version, Evidence, SemanticQuality string
		Responses                          map[core.ID]assistance.ListeningResponse
	}{assistance.ListeningFlowVersion, "synthetic_recorded_fixture", "NOT_TESTED", responses}
	if json.NewEncoder(os.Stdout).Encode(output) != nil {
		os.Exit(1)
	}
}
