package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	api "github.com/tushardhara/dream/adapters/transport"
	"github.com/tushardhara/dream/app/hws"
	"github.com/tushardhara/dream/core"
)

func testConfig() configuration {
	hash := sha256.Sum256([]byte("synthetic-config-token-longer-than-thirty-two"))
	scope := hws.Scope{Actor: "operator", Namespace: "ns", World: "w", Branch: "b", Run: "r"}
	return configuration{Version: 1, Mode: "management-only", GRPCAddress: "127.0.0.1:0", HTTPAddress: "127.0.0.1:0", Development: true, Credentials: []api.Credential{{ID: "researcher", TokenSHA256: hex.EncodeToString(hash[:]), Caller: "researcher", Role: hws.ResearchViewKind, Scope: scope, Principal: "a", Purpose: "research", Expires: time.Now().Add(time.Hour), RequestsPerMinute: 10, TotalRequests: 100}}, Grants: []hws.ViewGrant{{Caller: "researcher", Realm: hws.ViewRealm{Scope: scope, Principal: "a"}, Kind: hws.ResearchViewKind, Purpose: "research", Operations: []core.Operation{core.Read, core.Export}}}}
}
func TestConfigurationFailsClosedBeforeListening(t *testing.T) {
	c := testConfig()
	raw, _ := json.Marshal(c)
	if _, err := decodeConfiguration(bytes.NewReader(raw)); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*configuration){func(c *configuration) { c.Credentials = nil }, func(c *configuration) { c.Grants = nil }, func(c *configuration) { c.Mode = "fake" }, func(c *configuration) { c.Development = false }, func(c *configuration) { c.GRPCAddress = "0.0.0.0:9000" }, func(c *configuration) { c.HTTPAddress = "localhost:9000" }, func(c *configuration) { c.TLSKey = "missing" }} {
		c := testConfig()
		mutate(&c)
		raw, _ := json.Marshal(c)
		if _, err := decodeConfiguration(bytes.NewReader(raw)); err == nil {
			t.Fatal("unsafe config accepted")
		}
	}
	for _, raw := range []string{string(raw) + " {}", `{"unknown":true}`, strings.Repeat("x", (1<<20)+1)} {
		if _, err := decodeConfiguration(strings.NewReader(raw)); err == nil {
			t.Fatal("invalid document accepted")
		}
	}
}
func TestCLIHelpAndMissingConfiguration(t *testing.T) {
	t.Setenv("DREAM_DATABASE_URL", "")
	var out, errOut bytes.Buffer
	if code := run(context.Background(), []string{"--help"}, &out, &errOut); code != 0 {
		t.Fatal(code)
	}
	if code := run(context.Background(), nil, &out, &errOut); code != 2 {
		t.Fatal(code)
	}
	c := testConfig()
	raw, _ := json.Marshal(c)
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, raw, 0600); err != nil {
		t.Fatal(err)
	}
	if code := run(context.Background(), []string{"--config", path}, &out, &errOut); code != 1 {
		t.Fatal(code)
	}
	if strings.Contains(errOut.String(), c.Credentials[0].TokenSHA256) {
		t.Fatal("credential hash logged")
	}
	if strings.Contains(out.String(), "grpc=") {
		t.Fatal("started without runtime database")
	}
}
