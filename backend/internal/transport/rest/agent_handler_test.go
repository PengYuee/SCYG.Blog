package rest

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	outbound "github.com/PengYuee/SCYG.Blog/backend/internal/adapters/agent"
	pb "github.com/PengYuee/SCYG.Blog/backend/internal/generated/proto/scyg/agent/v1"
	identityauth "github.com/PengYuee/SCYG.Blog/backend/internal/modules/identity/auth"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/identity/user"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/config"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/httpserver"
)

type agentBoundaryServer struct {
	pb.UnimplementedAgentControlServiceServer
	requests chan *pb.CreateRunRequest
	resumes  chan *pb.ResumeRunRequest
	ready    bool
	noMarker bool
}

func (s *agentBoundaryServer) CreateRun(_ context.Context, r *pb.CreateRunRequest) (*pb.Run, error) {
	s.requests <- r
	return testAgentRun(), nil
}

func (s *agentBoundaryServer) GetRun(context.Context, *pb.GetRunRequest) (*pb.Run, error) {
	return testAgentRun(), nil
}

func (s *agentBoundaryServer) ResumeRun(_ context.Context, r *pb.ResumeRunRequest) (*pb.Run, error) {
	s.resumes <- r
	return testAgentRun(), nil
}

func (s *agentBoundaryServer) StreamRunEvents(_ *pb.StreamRunEventsRequest, stream grpc.ServerStreamingServer[pb.RunEventFrame]) error {
	if !s.ready {
		public, _ := status.New(codes.PermissionDenied, "private database detail").WithDetails(&pb.PublicError{Code: "RUN_NOT_FOUND", Message: "Run 不存在"})
		return public.Err()
	}
	if !s.noMarker {
		if err := stream.SendHeader(metadata.Pairs("scyg-subscription-ready", "1", "private", "do not forward")); err != nil {
			return err
		}
	}
	return stream.Send(&pb.RunEventFrame{Frame: []byte(": heartbeat\n\n")})
}

func testAgentRun() *pb.Run {
	return &pb.Run{RunId: "safe-run", Status: pb.RunStatus_RUN_STATUS_QUEUED, Capability: pb.AgentCapability_AGENT_CAPABILITY_CHAT, RecipeId: "chat", RecipeVersion: "1", CreatedAt: "2026-10-07T00:00:00Z", UpdatedAt: "2026-10-07T00:00:00Z"}
}

func setupAgentBoundary(t *testing.T, s *agentBoundaryServer) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := grpc.NewServer()
	pb.RegisterAgentControlServiceServer(server, s)
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(server.Stop)
	client, err := outbound.New(listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = client.Close() })
	cfg, err := config.Load(config.Options{DisableEnvironment: true})
	if err != nil {
		t.Fatal(err)
	}
	httpServer, err := httpserver.New(httpserver.Options{Logger: slog.New(slog.DiscardHandler), HTTP: cfg.HTTP(), AgentRequestTooLarge: AgentRequestTooLarge, Mount: func(engine *gin.Engine) error {
		engine.Use(func(ctx *gin.Context) {
			ctx.Request = ctx.Request.WithContext(identityauth.WithPrincipal(ctx.Request.Context(), identityauth.Principal{UserID: user.ID("owner")}))
		})
		agentHandler{AgentOptions{Client: client, ReadyTimeout: time.Second, IdleTimeout: time.Second}}.register(engine)
		return nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	host := httptest.NewServer(httpServer.Handler())
	t.Cleanup(host.Close)
	return host.URL
}

func TestAgentCreationPreservesJSONAndRejectsOversizedBodies(t *testing.T) {
	server := &agentBoundaryServer{requests: make(chan *pb.CreateRunRequest, 1)}
	host := setupAgentBoundary(t, server)
	raw := []byte(" \n null \t")
	req, _ := http.NewRequest(http.MethodPost, host+"/api/v1/ai/chat", bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Idempotency-Key", "a9ab8550-a044-4e78-b17d-5ea7d0bb380c")
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = response.Body.Close()
	if response.StatusCode != 202 {
		t.Fatalf("status %d", response.StatusCode)
	}
	got := <-server.requests
	if !bytes.Equal(got.JsonPayload, raw) || got.Capability != pb.AgentCapability_AGENT_CAPABILITY_CHAT {
		t.Fatal("raw JSON or fixed capability changed")
	}
	for _, chunked := range []bool{false, true} {
		req, _ = http.NewRequest(http.MethodPost, host+"/api/v1/ai/chat", bytes.NewReader(bytes.Repeat([]byte("x"), 1<<20+1)))
		if chunked {
			req.ContentLength = -1
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "a9ab8550-a044-4e78-b17d-5ea7d0bb380c")
		response, err = http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if response.StatusCode != 413 || response.Header.Get("Content-Type") != "application/problem+json" || !bytes.Contains(body, []byte(`"status":413`)) {
			t.Fatalf("invalid limit response: %d %s", response.StatusCode, body)
		}
	}
}

func TestAgentSubscriptionFailureBeforeHTTPCommit(t *testing.T) {
	host := setupAgentBoundary(t, &agentBoundaryServer{requests: make(chan *pb.CreateRunRequest, 1)})
	response, err := http.Get(host + "/api/v1/runs/safe-run/events")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != 404 || !bytes.Contains(body, []byte(`"code":"RUN_NOT_FOUND"`)) || bytes.Contains(body, []byte("private database")) {
		t.Fatalf("unsafe subscription failure: %d %s", response.StatusCode, body)
	}
}

func TestAgentSubscriptionForwardsFramesWithoutMetadata(t *testing.T) {
	host := setupAgentBoundary(t, &agentBoundaryServer{requests: make(chan *pb.CreateRunRequest, 1), ready: true})
	response, err := http.Get(host + "/api/v1/runs/safe-run/events")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != 200 || string(body) != ": heartbeat\n\n" || response.Header.Get("private") != "" {
		t.Fatalf("invalid SSE transfer: %d %q", response.StatusCode, body)
	}
}

func TestAgentResumePreservesPayloadPresence(t *testing.T) {
	server := &agentBoundaryServer{resumes: make(chan *pb.ResumeRunRequest, 1)}
	host := setupAgentBoundary(t, server)
	for _, explicitNull := range []bool{false, true} {
		raw := `{"interactionId":"交互","decision":"approve"}`
		if explicitNull {
			raw = `{"interactionId":"交互","decision":"approve","payload":null}`
		}
		req, _ := http.NewRequest(http.MethodPost, host+"/api/v1/runs/safe-run/resume", bytes.NewBufferString(raw))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "a9ab8550-a044-4e78-b17d-5ea7d0bb380c")
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != 200 {
			t.Fatalf("resume status %d", response.StatusCode)
		}
		got := <-server.resumes
		if (got.PayloadJson != nil) != explicitNull || (explicitNull && string(got.PayloadJson) != "null") {
			t.Fatal("payload presence lost")
		}
	}
	for _, raw := range []string{`null`, `[]`, `{"interactionId":"","decision":"approve"}`, `{"interactionId":"交互","decision":""}`} {
		req, _ := http.NewRequest(http.MethodPost, host+"/api/v1/runs/safe-run/resume", bytes.NewBufferString(raw))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", "a9ab8550-a044-4e78-b17d-5ea7d0bb380c")
		response, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = response.Body.Close()
		if response.StatusCode != 400 {
			t.Fatalf("invalid resume status %d", response.StatusCode)
		}
	}
}

func TestAgentSubscriptionRequiresExplicitReadiness(t *testing.T) {
	host := setupAgentBoundary(t, &agentBoundaryServer{ready: true, noMarker: true})
	response, err := http.Get(host + "/api/v1/runs/safe-run/events")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = response.Body.Close() }()
	body, _ := io.ReadAll(response.Body)
	if response.StatusCode != 500 || response.Header.Get("Content-Type") != "application/problem+json" || bytes.Contains(body, []byte("heartbeat")) {
		t.Fatalf("subscription committed without readiness: %d %s", response.StatusCode, body)
	}
}
