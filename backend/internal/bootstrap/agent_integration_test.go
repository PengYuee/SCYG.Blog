package bootstrap

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

func TestAgentGRPCDrainStopsActiveWatchWithinIndependentBudget(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	checker := health.NewServer()
	healthpb.RegisterHealthServer(server, checker)
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	t.Cleanup(server.Stop)
	client, err := grpc.NewClient(listener.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	watchContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	watch, err := healthpb.NewHealthClient(client).Watch(watchContext, &healthpb.HealthCheckRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := watch.Recv(); err != nil {
		t.Fatal(err)
	}
	integration := &agentIntegration{server: server, health: checker}
	started := time.Now()
	integration.stop(30 * time.Millisecond)
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("unbounded gRPC drain: %v", elapsed)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("gRPC serve did not exit")
	}
	// Shutdown publishes NOT_SERVING before Stop terminates the active watch.
	for {
		if _, err := watch.Recv(); err != nil {
			break
		}
	}
}
