package agent_test

import (
	"bytes"
	"context"
	"net"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	outbound "github.com/PengYuee/SCYG.Blog/backend/internal/adapters/agent"
	pb "github.com/PengYuee/SCYG.Blog/backend/internal/generated/proto/scyg/agent/v1"
	"github.com/PengYuee/SCYG.Blog/backend/internal/transport/rest"
)

var _ rest.AgentClient = (*outbound.Client)(nil)

type lifetimeServer struct {
	pb.UnimplementedAgentControlServiceServer
}

func (lifetimeServer) GetRun(ctx context.Context, _ *pb.GetRunRequest) (*pb.Run, error) {
	<-ctx.Done()
	return nil, status.FromContextError(ctx.Err()).Err()
}

func (lifetimeServer) StreamRunEvents(_ *pb.StreamRunEventsRequest, stream grpc.ServerStreamingServer[pb.RunEventFrame]) error {
	if err := stream.SendHeader(metadata.Pairs("scyg-subscription-ready", "1")); err != nil {
		return err
	}
	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-stream.Context().Done():
		return status.FromContextError(stream.Context().Err()).Err()
	case <-timer.C:
	}
	return stream.Send(&pb.RunEventFrame{Frame: []byte(": heartbeat\n\n")})
}

func TestClientBoundsUnaryButPreservesSubscriptionLifetime(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	pb.RegisterAgentControlServiceServer(server, lifetimeServer{})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	client, err := outbound.New(listener.Addr().String(), 20*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// Establish the connection via a subscription, so unary timing does not depend on dial startup.
	stream, err := client.StreamRunEvents(ctx, &pb.StreamRunEventsRequest{UserId: "owner", RunId: "safe-run"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := stream.Header(); err != nil {
		t.Fatal(err)
	}
	if _, err := client.GetRun(ctx, &pb.GetRunRequest{UserId: "owner", RunId: "safe-run"}); status.Code(err) != codes.DeadlineExceeded {
		t.Fatalf("unary not bounded: %v", err)
	}
	frame, err := stream.Recv()
	if err != nil {
		t.Fatalf("subscription inherited unary deadline: %v", err)
	}
	if string(frame.Frame) != ": heartbeat\n\n" {
		t.Fatalf("invalid frame %q", frame.Frame)
	}
}

type largeResultServer struct {
	pb.UnimplementedAgentControlServiceServer
	result []byte
	frame  []byte
}

func (server largeResultServer) GetRun(context.Context, *pb.GetRunRequest) (*pb.Run, error) {
	return &pb.Run{RunId: "safe-run", Status: pb.RunStatus_RUN_STATUS_SUCCEEDED, Capability: pb.AgentCapability_AGENT_CAPABILITY_CHAT, ResultJson: server.result}, nil
}

func (server largeResultServer) StreamRunEvents(_ *pb.StreamRunEventsRequest, stream grpc.ServerStreamingServer[pb.RunEventFrame]) error {
	if err := stream.SendHeader(metadata.Pairs("scyg-subscription-ready", "1")); err != nil {
		return err
	}
	return stream.Send(&pb.RunEventFrame{Frame: server.frame})
}

func TestClientReceivesLargeRunResultAndCompleteEventFrame(t *testing.T) {
	result := []byte(`{"body":"` + strings.Repeat("x", 5<<20) + `"}`)
	frame := append([]byte("id: opaque-cursor\nevent: run_succeeded\ndata: "), result...)
	frame = append(frame, '\n', '\n')
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	pb.RegisterAgentControlServiceServer(server, largeResultServer{result: result, frame: frame})
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	client, err := outbound.New(listener.Addr().String(), 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	run, err := client.GetRun(ctx, &pb.GetRunRequest{UserId: "owner", RunId: "safe-run"})
	if err != nil || run.GetStatus() != pb.RunStatus_RUN_STATUS_SUCCEEDED || !bytes.Equal(run.GetResultJson(), result) {
		t.Fatalf("large Run result lost: %v", err)
	}
	stream, err := client.StreamRunEvents(ctx, &pb.StreamRunEventsRequest{UserId: "owner", RunId: "safe-run"})
	if err != nil {
		t.Fatal(err)
	}
	received, err := stream.Recv()
	if err != nil || !bytes.Equal(received.GetFrame(), frame) {
		t.Fatalf("large SSE frame lost: %v", err)
	}
}
