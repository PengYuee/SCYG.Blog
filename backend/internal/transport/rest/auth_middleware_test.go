package rest

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	identityauth "github.com/PengYuee/SCYG.Blog/backend/internal/modules/identity/auth"
)

type testVerifier struct {
	principal identityauth.Principal
	err       error
}

func (verifier testVerifier) Verify(string) (identityauth.Principal, error) {
	return verifier.principal, verifier.err
}

func TestOptionalBearerAuthentication_rejectsMalformedAndAcceptsValid(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name   string
		header string
		status int
	}{
		{name: "malformed", header: "Basic token", status: 401},
		{name: "invalid", header: "Bearer bad", status: 401},
		{name: "valid", header: "Bearer good", status: 200},
	} {
		t.Run(test.name, func(t *testing.T) {
			engine := gin.New()
			engine.Use(OptionalBearerAuthentication(testVerifier{err: errors.New("invalid")}))
			if test.name == "valid" {
				engine = gin.New()
				engine.Use(OptionalBearerAuthentication(testVerifier{principal: identityauth.Principal{Username: "admin"}}))
			}
			engine.GET("/", func(ctx *gin.Context) { ctx.Status(200) })
			request := httptest.NewRequest("GET", "/", nil)
			request.Header.Set("Authorization", test.header)
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)
			if response.Code != test.status {
				t.Fatalf("status=%d, want %d", response.Code, test.status)
			}
		})
	}
}

func TestRequireAuthenticatedRoute_enforces_public_and_protected_paths(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, test := range []struct {
		name   string
		path   string
		status int
	}{
		{name: "public article list", path: "/api/v1/articles", status: 200},
		{name: "public documentation", path: "/openapi.yaml", status: 200},
		{name: "protected management list", path: "/api/v1/manage/articles", status: 401},
		{name: "unknown get route", path: "/unknown", status: 401},
	} {
		t.Run(test.name, func(t *testing.T) {
			engine := gin.New()
			engine.Use(RequireAuthenticatedRoute())
			engine.GET(test.path, func(ctx *gin.Context) { ctx.Status(http.StatusOK) })
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, test.path, nil))
			if response.Code != test.status {
				t.Fatalf("status=%d, want %d", response.Code, test.status)
			}
			if test.status == http.StatusUnauthorized && response.Header().Get("Content-Type") != "application/problem+json" {
				t.Fatalf("content type=%q", response.Header().Get("Content-Type"))
			}
		})
	}
}

func TestRequireAuthenticatedRoute_accepts_authenticated_protected_request(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(func(ctx *gin.Context) {
		ctx.Request = ctx.Request.WithContext(identityauth.WithPrincipal(ctx.Request.Context(), identityauth.Principal{Username: "admin"}))
		ctx.Next()
	})
	engine.Use(RequireAuthenticatedRoute())
	engine.GET("/api/v1/manage/articles", func(ctx *gin.Context) { ctx.Status(http.StatusOK) })
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/manage/articles", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d, want 200", response.Code)
	}
}
