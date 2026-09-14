package assistanceclient

import (
	"context"
	"github.com/tushardhara/dream/app/assistance"
	"github.com/tushardhara/dream/core"
)

func (l *Local) AppendBoundary(actor core.ID, b core.Boundary) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if actor != b.Meta.Observer || b.Validate() != nil {
		return assistance.ErrDenied
	}
	for _, old := range l.boundaries {
		if old.Meta.ID == b.Meta.ID {
			if assistance.Digest(old) == assistance.Digest(b) {
				return nil
			}
			return assistance.ErrInvalid
		}
	}
	next := append(copyValue(l.boundaries), copyValue(b))
	if core.ValidateBoundaryLog(next) != nil {
		return assistance.ErrInvalid
	}
	l.boundaries = next
	return nil
}
func (l *Local) RevokeBoundary(actor, id core.ID, at core.LogicalTime) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if at < l.now {
		return assistance.ErrInvalid
	}
	for i, b := range l.boundaries {
		if b.Meta.ID == id {
			if b.Meta.Observer != actor || at < b.LearnedAt {
				return assistance.ErrDenied
			}
			l.boundaries[i].Revoked = true
			l.boundaries[i].RevokedBy = actor
			l.boundaries[i].RevokedAt = at
			l.now = at
			return nil
		}
	}
	return assistance.ErrDenied
}
func (l *Local) Advance(at core.LogicalTime) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if at < l.now {
		return assistance.ErrInvalid
	}
	l.now = at
	return nil
}
func (l *Local) ReadBoundaries(ctx context.Context, r assistance.Request) (assistance.BoundarySnapshot, error) {
	defer l.lock(ctx)()
	old, ok := l.requests[r.ID]
	if !ok || assistance.Digest(old) != assistance.Digest(r) || !assistance.UsesScopedBoundaries(r.Version) {
		return assistance.BoundarySnapshot{}, assistance.ErrDenied
	}
	return assistance.BoundarySnapshot{Now: l.now, Records: copyValue(l.boundaries)}, nil
}
