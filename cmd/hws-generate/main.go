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
func runRelationshipProbe(in io.Reader, out io.Writer) error {
	raw, e := io.ReadAll(io.LimitReader(in, (1<<20)+1))
	if e != nil || len(raw) > 1<<20 {
		return fmt.Errorf("probe request byte budget")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var request experiment.RelationshipProbeRequest
	if e = decoder.Decode(&request); e != nil {
		return fmt.Errorf("invalid probe request")
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF || request.Validate() != nil {
		return fmt.Errorf("invalid probe envelope")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Second)
	defer cancel()
	result, e := experiment.GenerateRelationshipProbe(ctx, request)
	if e != nil {
		return e
	}
	return json.NewEncoder(out).Encode(result)
}

func main() {
	if len(os.Args) == 2 && os.Args[1] == "--help" {
		fmt.Println("hws-generate: bounded label-blind synthetic JSON batch on stdin; --relationship-probe accepts frozen public fixture/seeds; no provider or dataset access")
		return
	}
	if len(os.Args) == 2 && os.Args[1] == "--relationship-probe" {
		if e := runRelationshipProbe(os.Stdin, os.Stdout); e != nil {
			fmt.Fprintln(os.Stderr, "relationship generation failed")
			os.Exit(1)
		}
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
