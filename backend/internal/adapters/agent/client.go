// Package agent owns the internal AgentControl gRPC connection.
package agent

import (
	"context"
	"math"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/PengYuee/SCYG.Blog/backend/internal/generated/proto/scyg/agent/v1"
)

// Client provides bounded unary calls and cancellable long-lived subscriptions.
type Client struct {
	stub       pb.AgentControlServiceClient
	connection *grpc.ClientConn
	timeout    time.Duration
}

// New creates a non-blocking connection; Agent availability is not a startup prerequisite.
func New(target string, timeout time.Duration) (*Client, error) {
	conn, err := grpc.NewClient(
		target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(math.MaxInt)),
	)
	if err != nil {
		return nil, err
	}
	return &Client{stub: pb.NewAgentControlServiceClient(conn), connection: conn, timeout: timeout}, nil
}

// Close releases the outbound connection.
func (c *Client) Close() error { return c.connection.Close() }

// CreateRun applies the configured unary budget.
func (c *Client) CreateRun(ctx context.Context, r *pb.CreateRunRequest) (*pb.Run, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	return c.stub.CreateRun(ctx, r)
}

// GetRun applies the configured unary budget.
func (c *Client) GetRun(ctx context.Context, r *pb.GetRunRequest) (*pb.Run, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	return c.stub.GetRun(ctx, r)
}

// ResumeRun applies the configured unary budget.
func (c *Client) ResumeRun(ctx context.Context, r *pb.ResumeRunRequest) (*pb.Run, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	return c.stub.ResumeRun(ctx, r)
}

// CancelRun applies the configured unary budget.
func (c *Client) CancelRun(ctx context.Context, r *pb.CancelRunRequest) (*pb.Run, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	return c.stub.CancelRun(ctx, r)
}

// StreamRunEvents preserves the caller's subscription lifetime without a unary deadline.
func (c *Client) StreamRunEvents(ctx context.Context, r *pb.StreamRunEventsRequest) (grpc.ServerStreamingClient[pb.RunEventFrame], error) {
	return c.stub.StreamRunEvents(ctx, r)
}
