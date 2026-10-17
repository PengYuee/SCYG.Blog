//go:build integration

package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/PengYuee/SCYG.Blog/backend/internal/generated/proto/scyg/agent/v1"
	identityauth "github.com/PengYuee/SCYG.Blog/backend/internal/modules/identity/auth"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/authentication"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/config"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/publicsearch"
)

type publicBoundaryClient struct {
	mutex   sync.Mutex
	request *pb.CreateRunRequest
}

func (c *publicBoundaryClient) CreateRun(_ context.Context, request *pb.CreateRunRequest) (*pb.Run, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	c.request = request
	return c.run(), nil
}

func (c *publicBoundaryClient) run() *pb.Run {
	run := testAgentRun()
	run.ContentScope = pb.ContentScope_CONTENT_SCOPE_PUBLIC
	run.Capability = pb.AgentCapability_AGENT_CAPABILITY_SEARCH
	return run
}

func (c *publicBoundaryClient) GetRun(_ context.Context, request *pb.GetRunRequest) (*pb.Run, error) {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	if c.request == nil || request.UserId != c.request.UserId || request.ContentScope != c.request.ContentScope {
		return nil, status.Error(codes.NotFound, "not found")
	}
	return c.run(), nil
}

func (c *publicBoundaryClient) CancelRun(ctx context.Context, request *pb.CancelRunRequest) (*pb.Run, error) {
	return c.GetRun(ctx, &pb.GetRunRequest{UserId: request.UserId, RunId: request.RunId, ContentScope: request.ContentScope})
}

func (*publicBoundaryClient) ResumeRun(context.Context, *pb.ResumeRunRequest) (*pb.Run, error) {
	return nil, status.Error(codes.Unimplemented, "not needed")
}

func (*publicBoundaryClient) StreamRunEvents(context.Context, *pb.StreamRunEventsRequest) (grpc.ServerStreamingClient[pb.RunEventFrame], error) {
	return nil, status.Error(codes.Unimplemented, "not needed")
}

func TestPublicSearchHTTPRedisFailurePolicy(t *testing.T) {
	redisURL := os.Getenv("PUBLIC_SEARCH_TEST_REDIS_URL")
	if redisURL == "" {
		t.Fatal("PUBLIC_SEARCH_TEST_REDIS_URL must point to an isolated real Redis")
	}
	path := filepath.Join(t.TempDir(), "runtime.yaml")
	body := fmt.Sprintf("public_search:\n  redis_url: %q\n  prefix: scyg:http-test:%s\n  timeout: 100ms\n  search_rate: 100\n  search_burst: 10\n", redisURL, uuid.NewString())
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(config.Options{File: path, DisableEnvironment: true})
	if err != nil {
		t.Fatal(err)
	}
	control, err := publicsearch.New(context.Background(), cfg.PublicSearch())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = control.Close() }()
	secret := []byte(strings.Repeat("s", 32))
	tokens, err := authentication.NewPublicSearchTokens(secret, "blog", time.Hour, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	author, err := identityauth.NewTokenService(secret, "blog", time.Hour, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	client := &publicBoundaryClient{}
	engine := gin.New()
	if err := engine.SetTrustedProxies(nil); err != nil {
		t.Fatal(err)
	}
	engine.Use(OptionalBearerAuthentication(author, tokens), RequireAuthenticatedRoute())
	agentHandler{AgentOptions{Client: client, ReadyTimeout: time.Second, IdleTimeout: time.Second, Public: &PublicSearchOptions{Tokens: tokens, Control: control, Config: cfg.PublicSearch()}}}.register(engine)
	host := httptest.NewServer(engine)
	defer host.Close()
	call := func(method, path, token, raw string) *httptest.ResponseRecorder {
		req, err := http.NewRequest(method, host.URL+path, bytes.NewBufferString(raw))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Idempotency-Key", uuid.NewString())
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		result, err := host.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(result.Body)
		_ = result.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		return &httptest.ResponseRecorder{Code: result.StatusCode, HeaderMap: result.Header, Body: bytes.NewBuffer(data)}
	}
	session := call("POST", "/api/v1/public/ai/session", "", "")
	if session.Code != 200 || session.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("session: %d", session.Code)
	}
	var credential struct {
		AccessToken   string `json:"accessToken"`
		QueryMaxChars int    `json:"queryMaxChars"`
		MaxResults    int    `json:"maxResults"`
	}
	if err := json.Unmarshal(session.Body.Bytes(), &credential); err != nil {
		t.Fatal(err)
	}
	if credential.QueryMaxChars != 2000 || credential.MaxResults != 5 {
		t.Fatal("session form restrictions missing")
	}
	created := call("POST", "/api/v1/public/ai/search", credential.AccessToken, `{"query":"真实公共检索"}`)
	if created.Code != 202 {
		t.Fatalf("create: %d %s", created.Code, created.Body.String())
	}
	client.mutex.Lock()
	request := client.request
	client.mutex.Unlock()
	if request.ContentScope != pb.ContentScope_CONTENT_SCOPE_PUBLIC || !strings.HasPrefix(request.UserId, "anonymous:") || string(request.JsonPayload) != `{"query":"真实公共检索","max_results":5}` {
		t.Fatal("public identity/scope/default not frozen at trusted entry")
	}
	other, _, err := tokens.Issue()
	if err != nil {
		t.Fatal(err)
	}
	if got := call("GET", "/api/v1/runs/safe-run", other, ""); got.Code != 404 {
		t.Fatalf("other owner read: %d", got.Code)
	}
	if got := call("POST", "/api/v1/runs/safe-run/resume", credential.AccessToken, `{}`); got.Code != 401 {
		t.Fatalf("anonymous resume: %d", got.Code)
	}
	if got := call("POST", "/api/v1/public/ai/search", credential.AccessToken, `{"query":"valid","max_results":6}`); got.Code != 400 {
		t.Fatalf("public result bound: %d", got.Code)
	}
	if err := control.Close(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		method, path, token, raw string
		status                   int
	}{
		{"POST", "/api/v1/public/ai/session", "", "", 503},
		{"POST", "/api/v1/public/ai/search", credential.AccessToken, `{"query":"valid"}`, 503},
		{"GET", "/api/v1/runs/safe-run/events", credential.AccessToken, "", 503},
		{"GET", "/api/v1/runs/safe-run", credential.AccessToken, "", 200},
		{"POST", "/api/v1/runs/safe-run/cancel", credential.AccessToken, "", 200},
		{"GET", "/api/v1/runs/safe-run", other, "", 404},
		{"POST", "/api/v1/runs/safe-run/cancel", other, "", 404},
	} {
		if got := call(tc.method, tc.path, tc.token, tc.raw); got.Code != tc.status {
			t.Fatalf("Redis failure %s %s: %d wanted %d", tc.method, tc.path, got.Code, tc.status)
		}
	}
}
