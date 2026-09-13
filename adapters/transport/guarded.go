package transport

import (
	"context"
	pb "github.com/tushardhara/dream/adapters/transport/gen/dream/v1"
)

type guardedBackend struct {
	pb.UnimplementedResearchServer
	backend pb.ResearchServer
	auth    *Authenticator
}

func (g *guardedBackend) ValidateScenario(ctx context.Context, r *pb.ScenarioRequest) (*pb.Document, error) {
	if err := authorizeWire(ctx, g.auth, "ValidateScenario", r); err != nil {
		return nil, sanitized(err)
	}
	out, err := g.backend.ValidateScenario(ctx, r)
	if err != nil {
		return nil, sanitized(err)
	}
	if _, err = CurrentIdentity(ctx, g.auth); err != nil {
		return nil, sanitized(err)
	}
	return out, nil
}
func (g *guardedBackend) CreateWorld(ctx context.Context, r *pb.CreateWorldRequest) (*pb.Document, error) {
	if err := authorizeWire(ctx, g.auth, "CreateWorld", r); err != nil {
		return nil, sanitized(err)
	}
	out, err := g.backend.CreateWorld(ctx, r)
	if err != nil {
		return nil, sanitized(err)
	}
	if _, err = CurrentIdentity(ctx, g.auth); err != nil {
		return nil, sanitized(err)
	}
	return out, nil
}
func (g *guardedBackend) Control(ctx context.Context, r *pb.ControlRequest) (*pb.Document, error) {
	if err := authorizeWire(ctx, g.auth, "Control", r); err != nil {
		return nil, sanitized(err)
	}
	out, err := g.backend.Control(ctx, r)
	if err != nil {
		return nil, sanitized(err)
	}
	if _, err = CurrentIdentity(ctx, g.auth); err != nil {
		return nil, sanitized(err)
	}
	return out, nil
}
func (g *guardedBackend) GetOperation(ctx context.Context, r *pb.OperationRequest) (*pb.Document, error) {
	if err := authorizeWire(ctx, g.auth, "GetOperation", r); err != nil {
		return nil, sanitized(err)
	}
	out, err := g.backend.GetOperation(ctx, r)
	if err != nil {
		return nil, sanitized(err)
	}
	if _, err = CurrentIdentity(ctx, g.auth); err != nil {
		return nil, sanitized(err)
	}
	return out, nil
}
func (g *guardedBackend) CaptureSnapshot(ctx context.Context, r *pb.SnapshotRequest) (*pb.SnapshotHandle, error) {
	if err := authorizeWire(ctx, g.auth, "CaptureSnapshot", r); err != nil {
		return nil, sanitized(err)
	}
	out, err := g.backend.CaptureSnapshot(ctx, r)
	if err != nil {
		return nil, sanitized(err)
	}
	if _, err = CurrentIdentity(ctx, g.auth); err != nil {
		return nil, sanitized(err)
	}
	return out, nil
}
func (g *guardedBackend) Fork(ctx context.Context, r *pb.ForkRequest) (*pb.Document, error) {
	if err := authorizeWire(ctx, g.auth, "Fork", r); err != nil {
		return nil, sanitized(err)
	}
	out, err := g.backend.Fork(ctx, r)
	if err != nil {
		return nil, sanitized(err)
	}
	if _, err = CurrentIdentity(ctx, g.auth); err != nil {
		return nil, sanitized(err)
	}
	return out, nil
}
func (g *guardedBackend) Replay(ctx context.Context, r *pb.ReplayRequest) (*pb.Document, error) {
	if err := authorizeWire(ctx, g.auth, "Replay", r); err != nil {
		return nil, sanitized(err)
	}
	out, err := g.backend.Replay(ctx, r)
	if err != nil {
		return nil, sanitized(err)
	}
	if _, err = CurrentIdentity(ctx, g.auth); err != nil {
		return nil, sanitized(err)
	}
	return out, nil
}
func (g *guardedBackend) ActorView(ctx context.Context, r *pb.Query) (*pb.Document, error) {
	if err := authorizeWire(ctx, g.auth, "ActorView", r); err != nil {
		return nil, sanitized(err)
	}
	out, err := g.backend.ActorView(ctx, r)
	if err != nil {
		return nil, sanitized(err)
	}
	if _, err = CurrentIdentity(ctx, g.auth); err != nil {
		return nil, sanitized(err)
	}
	return out, nil
}
func (g *guardedBackend) ResearchView(ctx context.Context, r *pb.Query) (*pb.Document, error) {
	if err := authorizeWire(ctx, g.auth, "ResearchView", r); err != nil {
		return nil, sanitized(err)
	}
	out, err := g.backend.ResearchView(ctx, r)
	if err != nil {
		return nil, sanitized(err)
	}
	if _, err = CurrentIdentity(ctx, g.auth); err != nil {
		return nil, sanitized(err)
	}
	return out, nil
}
func (g *guardedBackend) ExternalObserve(ctx context.Context, r *pb.ExternalRequest) (*pb.Document, error) {
	if err := authorizeWire(ctx, g.auth, "ExternalObserve", r); err != nil {
		return nil, sanitized(err)
	}
	out, err := g.backend.ExternalObserve(ctx, r)
	if err != nil {
		return nil, sanitized(err)
	}
	if _, err = CurrentIdentity(ctx, g.auth); err != nil {
		return nil, sanitized(err)
	}
	return out, nil
}
func (g *guardedBackend) ExternalAct(ctx context.Context, r *pb.ControlRequest) (*pb.Document, error) {
	if err := authorizeWire(ctx, g.auth, "ExternalAct", r); err != nil {
		return nil, sanitized(err)
	}
	out, err := g.backend.ExternalAct(ctx, r)
	if err != nil {
		return nil, sanitized(err)
	}
	if _, err = CurrentIdentity(ctx, g.auth); err != nil {
		return nil, sanitized(err)
	}
	return out, nil
}
func (g *guardedBackend) SubmitExport(ctx context.Context, r *pb.ExportRequest) (*pb.Document, error) {
	if err := authorizeWire(ctx, g.auth, "SubmitExport", r); err != nil {
		return nil, sanitized(err)
	}
	out, err := g.backend.SubmitExport(ctx, r)
	if err != nil {
		return nil, sanitized(err)
	}
	if _, err = CurrentIdentity(ctx, g.auth); err != nil {
		return nil, sanitized(err)
	}
	return out, nil
}
func (g *guardedBackend) DownloadExport(ctx context.Context, r *pb.DownloadRequest) (*pb.ExportPage, error) {
	if err := authorizeWire(ctx, g.auth, "DownloadExport", r); err != nil {
		return nil, sanitized(err)
	}
	out, err := g.backend.DownloadExport(ctx, r)
	if err != nil {
		return nil, sanitized(err)
	}
	if _, err = CurrentIdentity(ctx, g.auth); err != nil {
		return nil, sanitized(err)
	}
	return out, nil
}
