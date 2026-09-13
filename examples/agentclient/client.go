// Package agentclient demonstrates the restricted external-agent transport port.
// It has no research-query/export method and never downloads future schedules.
package agentclient

import (
	"context"
	"errors"

	pb "github.com/tushardhara/dream/adapters/transport/gen/dream/v1"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"
)

// The host configures TLS and credentials on its connection. This wrapper never
// accepts user-provided role/identity headers or exposes a raw research client.
type Client struct {
	client pb.ResearchClient
	scope  *pb.Scope
}

func New(connection grpc.ClientConnInterface, scope *pb.Scope) (*Client, error) {
	if connection == nil || scope == nil {
		return nil, errors.New("external connection/scope required")
	}
	return &Client{pb.NewResearchClient(connection), proto.Clone(scope).(*pb.Scope)}, nil
}
func (c *Client) Observe(ctx context.Context, sources []string) (*pb.Document, error) {
	return c.client.ExternalObserve(ctx, &pb.ExternalRequest{Scope: proto.Clone(c.scope).(*pb.Scope), SourceIds: append([]string{}, sources...)})
}

// RequestWait is a dummy intent, not a claim that an action succeeded. at comes
// from an authorized current observation, never from a future schedule.
func (c *Client) RequestWait(ctx context.Context, operation, event, principal string, at int64) (*pb.Document, error) {
	return c.client.ExternalAct(ctx, &pb.ControlRequest{Scope: proto.Clone(c.scope).(*pb.Scope), OperationId: operation, Command: "inject", Input: &pb.Input{Id: event, Actor: principal, At: at, Kind: "observation", Text: "wait"}})
}
