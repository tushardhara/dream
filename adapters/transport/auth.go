// Package transport translates authenticated wire requests at the host boundary.
package transport

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/tushardhara/dream/app/hws"
	"github.com/tushardhara/dream/core"
)

var ErrDenied = errors.New("request not authorized")
var ErrLimited = errors.New("request budget exhausted")

// Credential is trusted host configuration, never a client request. Only a
// SHA-256 digest of a randomly generated bearer secret is retained. Secrets must
// have at least 32 bytes of entropy; length checks cannot prove entropy.
type Credential struct {
	ID                core.ID
	TokenSHA256       string
	Caller            core.ID
	Role              hws.ViewKind
	Scope             hws.Scope
	Principal         core.ID
	Purpose           core.ID
	Expires           time.Time
	RequestsPerMinute uint32
	TotalRequests     uint64
}

type credentialState struct {
	config   Credential
	used     uint64
	window   time.Time
	inWindow uint32
	revision uint64
}

// Session is minted only by Authenticate. It grants no authority after revoke
// or expiry. Revalidate must be called again at execution and download.
type Session struct {
	issuer   *Authenticator
	id       core.ID
	revision uint64
}

type Authenticator struct {
	mu       sync.Mutex
	byDigest map[[32]byte]*credentialState
	byID     map[core.ID]*credentialState
	now      func() time.Time
}

func NewAuthenticator(config []Credential, now func() time.Time) (*Authenticator, error) {
	if now == nil || len(config) == 0 || len(config) > 128 {
		return nil, ErrDenied
	}
	a := &Authenticator{byDigest: map[[32]byte]*credentialState{}, byID: map[core.ID]*credentialState{}, now: now}
	for _, c := range config {
		raw, err := hex.DecodeString(c.TokenSHA256)
		if err != nil || len(raw) != 32 || c.ID.Validate() != nil || c.Caller.Validate() != nil || c.Principal.Validate() != nil || c.Purpose.Validate() != nil || c.Scope.Validate() != nil || !now().Before(c.Expires) || c.RequestsPerMinute == 0 || c.TotalRequests == 0 || (hws.RequestBudget{Credential: c.ID, Scope: c.Scope, PerMinute: c.RequestsPerMinute, Total: c.TotalRequests}).Validate() != nil {
			return nil, ErrDenied
		}
		if c.Role != hws.ActorViewKind && c.Role != hws.ExternalViewKind && c.Role != hws.ResearchViewKind {
			return nil, ErrDenied
		}
		if c.Role == hws.ActorViewKind && c.Caller != c.Principal {
			return nil, ErrDenied
		}
		var digest [32]byte
		copy(digest[:], raw)
		if digest == ([32]byte{}) || a.byDigest[digest] != nil || a.byID[c.ID] != nil {
			return nil, ErrDenied
		}
		s := &credentialState{config: c, revision: 1}
		a.byDigest[digest] = s
		a.byID[c.ID] = s
	}
	return a, nil
}

// Authenticate accepts the entire Authorization value. Duplicate headers must
// be rejected by the adapter. No role, actor or forwarded identity is accepted.
func (a *Authenticator) Authenticate(authorization string) (Session, error) {
	if a == nil || len(authorization) < 7+32 || len(authorization) > 7+256 || authorization[:7] != "Bearer " {
		return Session{}, ErrDenied
	}
	token := authorization[7:]
	for _, r := range token {
		if r < 33 || r > 126 {
			return Session{}, ErrDenied
		}
	}
	digest := sha256.Sum256([]byte(token))
	a.mu.Lock()
	defer a.mu.Unlock()
	s := a.byDigest[digest]
	if s == nil || !a.now().Before(s.config.Expires) {
		return Session{}, ErrDenied
	}
	now := a.now()
	// Fixed windows never move backwards; a clock rollback cannot refund quota.
	window := now.Truncate(time.Minute)
	if s.window.IsZero() || window.After(s.window) {
		s.window = window
		s.inWindow = 0
	}
	if s.used >= s.config.TotalRequests || s.inWindow >= s.config.RequestsPerMinute {
		return Session{}, ErrLimited
	}
	s.used++
	s.inWindow++
	return Session{a, s.config.ID, s.revision}, nil
}
func (a *Authenticator) Revalidate(session Session) (Credential, error) {
	if a == nil || session.issuer != a {
		return Credential{}, ErrDenied
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	s := a.byID[session.id]
	if s == nil || s.revision != session.revision || !a.now().Before(s.config.Expires) {
		return Credential{}, ErrDenied
	}
	c := s.config
	c.TokenSHA256 = ""
	return c, nil
}
func (a *Authenticator) Revoke(id core.ID) {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if s := a.byID[id]; s != nil {
		raw, _ := hex.DecodeString(s.config.TokenSHA256)
		var digest [32]byte
		copy(digest[:], raw)
		delete(a.byDigest, digest)
		delete(a.byID, id)
	}
}

// configuredChild requires an existing trusted mapping for the same researcher
// and principal. Possession of a parent credential cannot invent a child scope.
func (a *Authenticator) configuredChild(parent Credential, child hws.Scope) bool {
	if a == nil {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, entry := range a.byID {
		c := entry.config
		if c.Scope == child && c.Caller == parent.Caller && c.Principal == parent.Principal && c.Role == hws.ResearchViewKind && c.Purpose == parent.Purpose && a.now().Before(c.Expires) {
			return true
		}
	}
	return false
}
