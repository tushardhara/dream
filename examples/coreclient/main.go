// Command coreclient demonstrates an importable core with no simulation identity.
package main

import (
	"fmt"

	"github.com/tushardhara/dream/core"
)

func main() {
	p, err := core.NewPrincipal("fictional-observer")
	if err != nil {
		panic(err)
	}
	encoded, err := core.Canonical(core.State{Version: 1, Principals: []core.Principal{p}})
	if err != nil {
		panic(err)
	}
	fmt.Println(string(encoded))
}
