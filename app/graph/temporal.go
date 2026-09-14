package graph

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/tushardhara/dream/core"
)

const temporalPrefix = "temporal.v1:"

func EncodeTemporal(f core.TemporalFact) (string, error) {
	b, e := f.Encode()
	if e != nil {
		return "", e
	}
	return temporalPrefix + string(b), nil
}
func parseTemporal(text string) (core.TemporalFact, error) {
	var f core.TemporalFact
	if !strings.HasPrefix(text, temporalPrefix) || len(text) > 4096 {
		return f, fmt.Errorf("invalid temporal codec")
	}
	d := json.NewDecoder(strings.NewReader(strings.TrimPrefix(text, temporalPrefix)))
	d.DisallowUnknownFields()
	if d.Decode(&f) != nil || f.Validate() != nil {
		return core.TemporalFact{}, fmt.Errorf("invalid temporal fact")
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return core.TemporalFact{}, fmt.Errorf("trailing temporal data")
	}
	b, _ := EncodeTemporal(f)
	if b != text {
		return core.TemporalFact{}, fmt.Errorf("noncanonical temporal fact")
	}
	return f, nil
}
func bindTemporal(f core.TemporalFact, id, observer, reporter core.ID, subject core.Subject, parents, supporting []core.ID, occurred core.LogicalTime) error {
	if reporter.Validate() != nil || f.Basis == "self_report" && reporter != f.Person || f.Account != id || f.Observer != observer || subject.Principal != observer || subject.Reference != nil || f.ConfirmedAt > occurred || f.Kind == "contacts" && f.ObservedThrough > occurred {
		return fmt.Errorf("temporal envelope mismatch")
	}
	for _, source := range append(append([]core.ID{}, parents...), supporting...) {
		if source == f.Source {
			return nil
		}
	}
	return fmt.Errorf("temporal source not attributed")
}
func DecodeTemporal(r MemoryRecord) (core.TemporalFact, error) {
	if r.Validate() != nil || r.Content.Kind != EpisodicMemory {
		return core.TemporalFact{}, fmt.Errorf("invalid temporal memory")
	}
	f, e := parseTemporal(r.Content.Text)
	if e != nil {
		return f, e
	}
	if e = bindTemporal(f, r.Event.Meta.ID, r.Event.Meta.Observer, r.Event.Meta.Source, r.Event.Subject, r.Event.Meta.Parents, r.Event.Meta.Supporting, r.Event.OccurredAt); e != nil {
		return core.TemporalFact{}, e
	}
	return f, nil
}

type SafeTemporal struct {
	Fact core.TemporalFact
	Item SafeContextItem
}

func (s SafeContext) Temporal() ([]SafeTemporal, error) {
	out := []SafeTemporal{}
	for _, item := range s.items {
		if !strings.HasPrefix(item.Text, temporalPrefix) {
			continue
		}
		f, e := parseTemporal(item.Text)
		if e != nil || item.Kind != EpisodicMemory {
			return nil, fmt.Errorf("invalid approved temporal fact")
		}
		if e = bindTemporal(f, item.Source, item.Observer, item.Reporter, item.Subject, item.Parents, item.Supporting, item.OccurredAt); e != nil {
			return nil, e
		}
		out = append(out, SafeTemporal{f, item})
	}
	return out, nil
}

// TemporalEvidenceFromApproved requires the account and supporting record to be
// explicit approved items. It does not derive confidence from salience/recency.
func TemporalEvidenceFromApproved(items []SafeContextItem, at core.LogicalTime) ([]core.TemporalEvidence, error) {
	safe := SafeContext{items: items}
	facts, e := safe.Temporal()
	if e != nil {
		return nil, e
	}
	out := []core.TemporalEvidence{}
	for _, projection := range facts {
		f, item := projection.Fact, projection.Item
		found := false
		for _, source := range items {
			if source.Source != f.Source || source.Observer != f.Observer {
				continue
			}
			if source.LearnedAt > at || source.OccurredAt > at || source.Valid.Start > at || source.Valid.End != nil && at >= *source.Valid.End {
				return nil, fmt.Errorf("temporal source unavailable now")
			}
			evidence := core.TemporalEvidence{Reporter: item.Reporter, SourceReporter: source.Reporter, SourceOccurredAt: source.OccurredAt, SourceLearnedAt: source.LearnedAt, Fact: f, OccurredAt: item.OccurredAt, LearnedAt: item.LearnedAt, Valid: item.Valid, Confidence: item.Confidence, SourceConfidence: source.Confidence}
			if evidence.Validate(at) != nil {
				return nil, fmt.Errorf("invalid temporal projection")
			}
			out = append(out, evidence)
			found = true
			break
		}
		if !found {
			return nil, fmt.Errorf("temporal source metadata missing")
		}
	}
	return out, nil
}

type TemporalService struct{ Memory MemoryService }
type TemporalProjection struct {
	Fact   core.TemporalFact
	Record MemoryRecord
}

func (s TemporalService) Query(ctx context.Context, q MemoryQuery) ([]TemporalProjection, error) {
	records, e := s.Memory.Retrieve(ctx, q)
	if e != nil {
		return nil, e
	}
	out := []TemporalProjection{}
	for _, record := range records {
		if !strings.HasPrefix(record.Record.Content.Text, temporalPrefix) {
			continue
		}
		f, e := DecodeTemporal(record.Record)
		if e != nil {
			return nil, e
		}
		out = append(out, TemporalProjection{f, record.Record})
	}
	return out, nil
}
func (s TemporalService) Put(ctx context.Context, scope MemoryScope, key core.ID, expected int64, r MemoryRecord, f core.TemporalFact, purpose core.ID) (AppendResult, error) {
	encoded, e := EncodeTemporal(f)
	if e != nil {
		return AppendResult{}, e
	}
	raw, _ := json.Marshal(r)
	var owned MemoryRecord
	if json.Unmarshal(raw, &owned) != nil {
		return AppendResult{}, fmt.Errorf("invalid temporal record")
	}
	owned.Content.Kind = EpisodicMemory
	owned.Content.Text = encoded
	if _, e = DecodeTemporal(owned); e != nil {
		return AppendResult{}, e
	}
	if owned.Supersedes != "" {
		if s.Memory.Journal == nil {
			return AppendResult{}, fmt.Errorf("temporal journal required")
		}
		entries, e := s.Memory.Journal.ReadMemory(ctx, scope)
		if e != nil {
			return AppendResult{}, e
		}
		found := false
		for _, old := range entries {
			if old.Event.Meta.ID != owned.Supersedes {
				continue
			}
			if old.Revoked || old.Content == nil {
				return AppendResult{}, fmt.Errorf("temporal correction target unavailable")
			}
			prior, e := DecodeTemporal(old.record())
			if e != nil {
				return AppendResult{}, e
			}
			if prior.Observer != f.Observer || prior.Person != f.Person || prior.With != f.With || prior.Channel != f.Channel || prior.Kind != f.Kind || prior.Basis != f.Basis {
				return AppendResult{}, fmt.Errorf("temporal correction changes identity")
			}
			found = true
		}
		if !found {
			return AppendResult{}, fmt.Errorf("unknown temporal correction target")
		}
	}
	return s.Memory.Put(ctx, scope, key, expected, owned, purpose)
}
