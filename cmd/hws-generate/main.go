// hws-generate is the isolated, label-blind stdin/stdout worker. It opens no
// dataset, environment credentials, socket or policy store.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/tushardhara/dream/simulator/experiment"
	"io"
	"os"
	"time"
)

func run(in io.Reader, out io.Writer) error {
	raw, e := io.ReadAll(io.LimitReader(in, (32<<20)+1))
	if e != nil || len(raw) > 32<<20 {
		return fmt.Errorf("request byte budget")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var requests []experiment.Request
	if e = decoder.Decode(&requests); e != nil {
		return fmt.Errorf("invalid request")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF || len(requests) < 1 || len(requests) > 8192 {
		return fmt.Errorf("invalid request envelope")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
	results, e := (experiment.Generator{}).Generate(ctx, requests)
	if e != nil {
		return e
	}
	return json.NewEncoder(out).Encode(results)
}
func main() {
	if len(os.Args) == 2 && os.Args[1] == "--help" {
		fmt.Println("hws-generate: bounded label-blind synthetic JSON batch on stdin; no provider or dataset access")
		return
	}
	if len(os.Args) != 1 {
		fmt.Fprintln(os.Stderr, "unexpected argument")
		os.Exit(2)
	}
	if e := run(os.Stdin, os.Stdout); e != nil {
		fmt.Fprintln(os.Stderr, "generation failed")
		os.Exit(1)
	}
}
