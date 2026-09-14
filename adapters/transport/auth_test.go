package transport

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tushardhara/dream/app/hws"
)

const testSecret = "synthetic-fixture-credential-never-production-0001"

func credentialFixture() Credential {
	sum := sha256.Sum256([]byte(testSecret))
	return Credential{ID: "fixture", TokenSHA256: hex.EncodeToString(sum[:]), Caller: "researcher", Role: hws.ResearchViewKind, Scope: hws.Scope{Actor: "operator", Namespace: "ns", World: "world", Branch: "branch", Run: "run"}, Principal: "a", Purpose: "research", Expires: time.Unix(10000, 0), RequestsPerMinute: 5, TotalRequests: 7}
}
func TestAuthenticationFailsClosedAndCannotForgeSession(t *testing.T) {
	now := func() time.Time { return time.Unix(100, 0) }
	if _, err := NewAuthenticator(nil, now); err == nil {
		t.Fatal("empty auth accepted")
	}
	for _, mutate := range []func(*Credential){func(c *Credential) { c.TokenSHA256 = "bad" }, func(c *Credential) { c.Role = "admin" }, func(c *Credential) { c.Expires = now() }, func(c *Credential) { c.Role = hws.ActorViewKind }, func(c *Credential) { c.RequestsPerMinute = 0 }, func(c *Credential) { c.Scope.Namespace = "" }} {
		c := credentialFixture()
		mutate(&c)
		if _, err := NewAuthenticator([]Credential{c}, now); err == nil {
			t.Fatal("invalid config accepted")
		}
	}
	c := credentialFixture()
	a, err := NewAuthenticator([]Credential{c}, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, header := range []string{"", "researcher", "Bearer short", "Basic " + testSecret, "Bearer " + testSecret + "\n", "Bearer " + testSecret + ", Bearer " + testSecret, "Bearer " + testSecret + "x"} {
		if _, err := a.Authenticate(header); !errors.Is(err, ErrDenied) {
			t.Fatal("bypass", err)
		}
	}
	session, err := a.Authenticate("Bearer " + testSecret)
	if err != nil {
		t.Fatal(err)
	}
	got, err := a.Revalidate(session)
	if err != nil || got.TokenSHA256 != "" || got.Scope != c.Scope || got.Role != c.Role {
		t.Fatal("identity mapping", err)
	}
	other, _ := NewAuthenticator([]Credential{c}, now)
	if _, err = other.Revalidate(session); err == nil {
		t.Fatal("foreign issuer accepted")
	}
	a.Revoke(c.ID)
	if _, err = a.Revalidate(session); err == nil {
		t.Fatal("stale session accepted")
	}
	if _, err = a.Authenticate("Bearer " + testSecret); err == nil {
		t.Fatal("revoked secret accepted")
	}
}
func TestQuotaLedgerConcurrentAcrossWindowsAndRollback(t *testing.T) {
	var seconds atomic.Int64
	seconds.Store(100)
	c := credentialFixture()
	a, err := NewAuthenticator([]Credential{c}, func() time.Time { return time.Unix(seconds.Load(), 0) })
	if err != nil {
		t.Fatal(err)
	}
	run := func(want int64) {
		t.Helper()
		var accepted atomic.Int64
		var wg sync.WaitGroup
		for range 20 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := a.Authenticate("Bearer " + testSecret)
				if err == nil {
					accepted.Add(1)
				} else if !errors.Is(err, ErrLimited) {
					t.Errorf("unexpected auth error: %v", err)
				}
			}()
		}
		wg.Wait()
		if accepted.Load() != want {
			t.Fatalf("quota accepted %d want %d", accepted.Load(), want)
		}
	}
	run(5)
	seconds.Store(20)
	run(0)
	seconds.Store(160)
	run(2)
	seconds.Store(220)
	run(0)
}
func TestConcurrentRevocationAndExpiry(t *testing.T) {
	var seconds atomic.Int64
	seconds.Store(100)
	c := credentialFixture()
	c.RequestsPerMinute = 100
	c.TotalRequests = 100
	a, _ := NewAuthenticator([]Credential{c}, func() time.Time { return time.Unix(seconds.Load(), 0) })
	s, _ := a.Authenticate("Bearer " + testSecret)
	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() { defer wg.Done(); _, _ = a.Revalidate(s); a.Revoke(c.ID) }()
	}
	wg.Wait()
	if _, err := a.Revalidate(s); err == nil {
		t.Fatal("revoked session survives concurrent access")
	}
	b, _ := NewAuthenticator([]Credential{c}, func() time.Time { return time.Unix(seconds.Load(), 0) })
	s, _ = b.Authenticate("Bearer " + testSecret)
	seconds.Store(c.Expires.Unix())
	if _, err := b.Revalidate(s); err == nil {
		t.Fatal("expired download session")
	}
}
