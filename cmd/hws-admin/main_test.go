package main

import (
	"bytes"
	"context"
	"testing"
)

func TestAdminRequiresExplicitActionAndInputs(t *testing.T) {
	t.Setenv("DREAM_DATABASE_URL", "")
	for _, tc := range []struct {
		args []string
		code int
	}{
		{[]string{"--help"}, 0}, {nil, 2}, {[]string{"unknown"}, 2}, {[]string{"migrate"}, 1},
		{[]string{"quarantine-restore"}, 2}, {[]string{"apply-revocations"}, 2},
		{[]string{"--file", "unused", "migrate"}, 2}, {[]string{"--expected-sha256", "ignored", "apply-revocations"}, 2},
	} {
		var out bytes.Buffer
		if got := run(context.Background(), tc.args, &out); got != tc.code {
			t.Fatal(tc.args, got, tc.code)
		}
	}
}
