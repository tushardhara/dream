package transport

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"strings"

	pb "github.com/tushardhara/dream/adapters/transport/gen/dream/v1"
	"github.com/tushardhara/dream/app/graph"
	"github.com/tushardhara/dream/app/hws"
	"github.com/tushardhara/dream/core"
	"google.golang.org/protobuf/encoding/protojson"
)

type exportManifest struct {
	Version     int       `json:"version"`
	Scope       hws.Scope `json:"scope"`
	Caller      core.ID   `json:"caller"`
	Kind        string    `json:"kind"`
	RequestHash string    `json:"request_sha256"`
	BodyHash    string    `json:"body_sha256"`
	Bytes       int       `json:"bytes"`
}

func sha(raw []byte) string { sum := sha256.Sum256(raw); return hex.EncodeToString(sum[:]) }

type quoteExport struct{ from, through core.LogicalTime }

func (w quoteExport) Write(_ context.Context, safe graph.SafeContext) (graph.WriterDraft, error) {
	out := graph.WriterDraft{Spans: []graph.WriterSpan{}}
	for _, item := range safe.Items() {
		if item.OccurredAt >= w.from && item.OccurredAt <= w.through && item.Text != "" {
			out.Spans = append(out.Spans, graph.WriterSpan{Source: item.Source, End: len(item.Text)})
		}
	}
	return out, nil
}
func (b *Backend) exportBytes(ctx context.Context, c Credential, r *pb.ExportRequest) ([]byte, error) {
	if core.ID(r.ExportId).Validate() != nil || r.FromTime < 0 || r.ThroughTime < r.FromTime {
		return nil, ErrDenied
	}
	p, err := b.permit(c)
	if err != nil {
		return nil, err
	}
	if err = b.Views.CheckOperation(ctx, p, core.Export); err != nil {
		return nil, err
	}
	switch r.Kind {
	case "research":
		if c.Role != hws.ResearchViewKind || r.Source == nil || scope(r.Source.Scope) != c.Scope || len(r.SourceIds) != 0 {
			return nil, ErrDenied
		}
		if _, err = b.Views.Research(ctx, p); err != nil {
			return nil, err
		}
		bundle, err := b.Store.ReadReplay(ctx, handle(r.Source), r.ThroughRevision)
		if err != nil {
			return nil, err
		}
		frames := bundle.Frames[:0:0]
		for _, frame := range bundle.Frames {
			if int64(frame.State.At) >= r.FromTime && int64(frame.State.At) <= r.ThroughTime {
				frames = append(frames, frame)
			}
		}
		bundle.Frames = frames
		raw, err := json.Marshal(bundle)
		if err != nil {
			return nil, err
		}
		// The full source trajectory is checked again, including frames outside the
		// time filter. Filters cannot hide revoked dependencies.
		if _, err = b.Store.ReadReplay(ctx, handle(r.Source), r.ThroughRevision); err != nil {
			return nil, err
		}
		if err = b.Views.CheckOperation(ctx, p, core.Export); err != nil {
			return nil, err
		}
		return raw, nil
	case "private":
		if c.Role != hws.ActorViewKind || r.Source != nil || r.ThroughRevision != 0 || len(r.SourceIds) == 0 || len(r.SourceIds) > 16 {
			return nil, ErrDenied
		}
		sources := make([]core.ID, len(r.SourceIds))
		for i, id := range r.SourceIds {
			sources[i] = core.ID(id)
		}
		approved, decision, err := b.Views.Propose(ctx, p, sources, c.Caller, core.Export, graph.AssistantDisclosure)
		if err != nil || !decision.Allowed {
			return nil, ErrDenied
		}
		out, decision, err := b.Views.Write(ctx, p, approved, quoteExport{core.LogicalTime(r.FromTime), core.LogicalTime(r.ThroughTime)})
		if err != nil || !decision.Allowed {
			return nil, ErrDenied
		}
		return json.Marshal(out)
	default:
		return nil, ErrDenied
	}
}
func (b *Backend) SubmitExport(ctx context.Context, r *pb.ExportRequest) (*pb.Document, error) {
	c, err := b.identity(ctx, "SubmitExport", r)
	if err != nil {
		return nil, err
	}
	store, ok := b.Store.(hws.ExportStore)
	if !ok {
		return nil, ErrDenied
	}
	raw, err := b.exportBytes(ctx, c, r)
	if err != nil {
		return nil, err
	}
	request, err := protojson.Marshal(r)
	if err != nil {
		return nil, err
	}
	manifest := exportManifest{1, c.Scope, c.Caller, r.Kind, sha(request), sha(raw), len(raw)}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return nil, err
	}
	if _, err = CurrentIdentity(ctx, b.Auth); err != nil {
		return nil, err
	}
	if err = store.SaveExport(ctx, hws.ExportSubmission{Version: 1, Scope: c.Scope, Caller: c.Caller, ID: core.ID(r.ExportId), Request: request, Manifest: encoded}); err != nil {
		return nil, err
	}
	return document("export.manifest.v1", manifest)
}
func (b *Backend) DownloadExport(ctx context.Context, r *pb.DownloadRequest) (*pb.ExportPage, error) {
	c, err := b.identity(ctx, "DownloadExport", r)
	if err != nil {
		return nil, err
	}
	if r.PageSize == 0 || r.PageSize > 65536 || len(r.Cursor) > 1024 {
		return nil, ErrDenied
	}
	store, ok := b.Store.(hws.ExportStore)
	if !ok {
		return nil, ErrDenied
	}
	submission, err := store.LoadExport(ctx, c.Scope, c.Caller, core.ID(r.ExportId))
	if err != nil {
		return nil, err
	}
	var request pb.ExportRequest
	if protojson.Unmarshal(submission.Request, &request) != nil || scope(request.Scope) != c.Scope || request.ExportId != r.ExportId {
		return nil, ErrDenied
	}
	var manifest exportManifest
	if json.Unmarshal(submission.Manifest, &manifest) != nil || manifest.Version != 1 || manifest.Scope != c.Scope || manifest.Caller != c.Caller || manifest.RequestHash != sha(submission.Request) {
		return nil, ErrDenied
	}
	raw, err := b.exportBytes(ctx, c, &request)
	if err != nil {
		return nil, err
	}
	if len(raw) != manifest.Bytes || sha(raw) != manifest.BodyHash {
		return nil, ErrDenied
	}
	manifestHash := sha(submission.Manifest)
	cursor := struct {
		Manifest string
		Offset   int
	}{Manifest: manifestHash}
	if r.Cursor != "" {
		encoded, err := base64.RawURLEncoding.DecodeString(r.Cursor)
		if err != nil {
			return nil, ErrDenied
		}
		d := json.NewDecoder(strings.NewReader(string(encoded)))
		d.DisallowUnknownFields()
		if d.Decode(&cursor) != nil || cursor.Manifest != manifestHash || cursor.Offset < 0 || cursor.Offset >= len(raw) {
			return nil, ErrDenied
		}
		var extra any
		if d.Decode(&extra) != io.EOF {
			return nil, ErrDenied
		}
	}
	end := cursor.Offset + int(r.PageSize)
	if end > len(raw) {
		end = len(raw)
	}
	page := append([]byte{}, raw[cursor.Offset:end]...)
	next := ""
	if end < len(raw) {
		cursor.Offset = end
		encoded, _ := json.Marshal(cursor)
		next = base64.RawURLEncoding.EncodeToString(encoded)
	}
	if _, err = CurrentIdentity(ctx, b.Auth); err != nil {
		return nil, err
	}
	return &pb.ExportPage{ExportId: r.ExportId, ManifestSha256: manifestHash, ManifestJson: submission.Manifest, Page: page, PageSha256: sha(page), NextCursor: next}, nil
}
