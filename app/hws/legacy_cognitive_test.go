package hws

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"
)

func TestFrozenLegacyCognitiveV1(t *testing.T) {
	raw, e := os.ReadFile("testdata/legacy-cognitive-v1.txt")
	if e != nil {
		t.Fatal(e)
	}
	h := sha256.Sum256(raw)
	if hex.EncodeToString(h[:]) != "54a1d13fa9e2396113b4b414a05cd17026d927aeff58be7a6e8630d1fb4edce1" {
		t.Fatal("legacy checkpoint fixture modified")
	}
	c, e := DecodeCognitiveCheckpoint(string(raw))
	if e != nil {
		t.Fatal(e)
	}
	encoded, e := c.Encode()
	if e != nil || encoded != string(raw) {
		t.Fatal("v1 canonical encoding/hash changed", e)
	}
	if _, e = DecodeActionCheckpoint(string(raw)); e == nil {
		t.Fatal("legacy checkpoint reinterpreted as v2")
	}
}
