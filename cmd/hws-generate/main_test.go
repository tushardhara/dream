package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestWorkerRejectsLabelsAndOversizedRequests(t *testing.T) {
	for _, input := range []string{`[{"input":{},"labels":[true]}]`, `[]`, `[] []`, strings.Repeat("x", (32<<20)+1)} {
		var output bytes.Buffer
		if run(strings.NewReader(input), &output) == nil || output.Len() != 0 {
			t.Fatal("worker accepted unauthorized/malformed request")
		}
	}
}
