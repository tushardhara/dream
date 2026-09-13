// Command hws-worker is an explicit bootstrap scaffold; it opens no listener.
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintln(flag.CommandLine.Output(), "Usage: hws-worker --help\nBootstrap scaffold only. No simulation, server, provider or worker is implemented.")
	}
	flag.Parse()
	fmt.Fprintln(os.Stderr, "hws-worker: not implemented; use --help for scaffold status")
	os.Exit(2)
}
