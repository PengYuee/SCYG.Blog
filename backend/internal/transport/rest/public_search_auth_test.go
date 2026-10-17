package rest

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pb "github.com/PengYuee/SCYG.Blog/backend/internal/generated/proto/scyg/agent/v1"
	identityauth "github.com/PengYuee/SCYG.Blog/backend/internal/modules/identity/auth"
	"github.com/PengYuee/SCYG.Blog/backend/internal/modules/identity/user"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/authentication"
)

func TestPublicSearchHTTPAuthenticationDomains(t *testing.T) {
	secret := []byte(strings.Repeat("s", 32))
	author, err := identityauth.NewTokenService(secret, "blog", time.Hour, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	public, err := authentication.NewPublicSearchTokens(secret, "blog", time.Hour, time.Now)
	if err != nil {
		t.Fatal(err)
	}
	authorToken, _, err := author.Issue(identityauth.Principal{UserID: user.ID(strings.Repeat("a", 32)), Username: "author"})
	if err != nil {
		t.Fatal(err)
	}
	publicToken, _, err := public.Issue()
	if err != nil {
		t.Fatal(err)
	}
	engine := gin.New()
	engine.Use(OptionalBearerAuthentication(author, public), RequireAuthenticatedRoute())
	handler := func(ctx *gin.Context) {
		_, isAuthor := identityauth.PrincipalFromContext(ctx.Request.Context())
		_, isPublic := authentication.AnonymousSubject(ctx.Request.Context())
		if isAuthor == isPublic {
			t.Error("request must use exactly one identity domain")
		}
		ctx.Status(http.StatusNoContent)
	}
	engine.POST("/api/v1/public/ai/search", handler)
	engine.GET("/api/v1/runs/:runId", handler)
	engine.GET("/api/v1/runs/:runId/events", handler)
	engine.POST("/api/v1/runs/:runId/cancel", handler)
	engine.POST("/api/v1/runs/:runId/resume", handler)
	engine.POST("/api/v1/ai/write", handler)
	engine.POST("/api/v1/manage/articles", handler)
	for _, tc := range []struct {
		method, path, token string
		want                int
	}{
		{"POST", "/api/v1/public/ai/search", publicToken, 204},
		{"POST", "/api/v1/public/ai/search", authorToken, 401},
		{"GET", "/api/v1/runs/one", publicToken, 204},
		{"GET", "/api/v1/runs/one/events", publicToken, 204},
		{"POST", "/api/v1/runs/one/cancel", publicToken, 204},
		{"POST", "/api/v1/runs/one/resume", publicToken, 401},
		{"POST", "/api/v1/ai/write", publicToken, 401},
		{"POST", "/api/v1/manage/articles", publicToken, 401},
		{"GET", "/api/v1/runs/one", authorToken, 204},
		{"POST", "/api/v1/runs/one/resume", authorToken, 204},
		{"GET", "/api/v1/runs/one", "", 401},
	} {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		if tc.token != "" {
			req.Header.Set("Authorization", "Bearer "+tc.token)
		}
		response := httptest.NewRecorder()
		engine.ServeHTTP(response, req)
		if response.Code != tc.want {
			t.Fatalf("%s %s: %d wanted %d", tc.method, tc.path, response.Code, tc.want)
		}
	}
}

func TestAgentScopeIsCheckedAndRetryAfterIsTransportOnly(t *testing.T) {
	engine := gin.New()
	engine.GET("/scope", func(ctx *gin.Context) {
		ctx.Request = ctx.Request.WithContext(authentication.WithAnonymousSubject(ctx.Request.Context(), "anonymous:subject"))
		run := testAgentRun()
		agentHandler{}.respond(ctx, 200, run, nil)
	})
	engine.GET("/rate", func(ctx *gin.Context) {
		seconds := uint32(0)
		failure, err := status.New(codes.ResourceExhausted, "private").WithDetails(&pb.PublicError{Code: "quota_exhausted", Message: "稍后重试", RetryAfterSeconds: &seconds})
		if err != nil {
			t.Fatal(err)
		}
		agentFailure(ctx, failure.Err())
	})
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest("GET", "/scope", nil))
	if response.Code != 404 || strings.Contains(response.Body.String(), "recipeId") {
		t.Fatal("cross-scope Run exposed")
	}
	response = httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest("GET", "/rate", nil))
	body := response.Body.String()
	if response.Code != 429 || response.Header().Get("Retry-After") != "0" || !strings.Contains(body, `"code":"quota_exhausted"`) || !strings.Contains(body, "稍后重试") || strings.Contains(body, "retry_after") || strings.Contains(body, "private") {
		t.Fatalf("unsafe rate-limit mapping: %d %s", response.Code, body)
	}
}
