package httpserver_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/config"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/httpserver"
)

func testServer(t *testing.T, mount httpserver.MountRoutes) (*httpserver.Server, *bytes.Buffer) {
	t.Helper()
	t.Setenv("SCYG_HTTP_HOST", "127.0.0.1")
	t.Setenv("SCYG_HTTP_PORT", "18080")
	t.Setenv("SCYG_HTTP_TRUSTED_PROXIES", "192.0.2.1")
	t.Setenv("SCYG_HTTP_CORS_ALLOWED_ORIGINS", "https://allowed.example")
	cfg, err := config.Load(config.Options{})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	logs := &bytes.Buffer{}
	logger := slog.New(slog.NewJSONHandler(logs, nil))
	server, err := httpserver.New(httpserver.Options{HTTP: cfg.HTTP(), Logger: logger, Mount: mount})
	if err != nil {
		t.Fatalf("new server: %v", err)
	}
	return server, logs
}

func Test_RequestLimit_returns_413_before_handler_reads_oversize_body(t *testing.T) {
	called := false
	server, _ := testServer(t, func(router *gin.Engine) error {
		router.POST("/read", func(ctx *gin.Context) {
			called = true
			if _, readErr := io.ReadAll(ctx.Request.Body); readErr != nil {
				t.Errorf("read body: %v", readErr)
			}
			ctx.Status(http.StatusNoContent)
		})
		return nil
	})
	request := httptest.NewRequest(http.MethodPost, "/read", strings.NewReader(strings.Repeat("x", int(httpserver.MaxRequestBodyBytes)+1)))
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusRequestEntityTooLarge || called {
		t.Fatalf("status=%d called=%v", response.Code, called)
	}
}

func Test_TrustedProxy_ignores_forwarded_ip_from_untrusted_peer(t *testing.T) {
	server, _ := testServer(t, func(router *gin.Engine) error {
		router.GET("/ip", func(ctx *gin.Context) { ctx.String(http.StatusOK, ctx.ClientIP()) })
		return nil
	})
	request := httptest.NewRequest(http.MethodGet, "/ip", nil)
	request.RemoteAddr = "198.51.100.8:1234"
	request.Header.Set("X-Forwarded-For", "203.0.113.7")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Body.String() != "198.51.100.8" {
		t.Fatalf("client IP=%q", response.Body.String())
	}
}

func Test_CORS_allows_exact_origin_and_rejects_other_preflight(t *testing.T) {
	server, _ := testServer(t, func(*gin.Engine) error { return nil })
	allowed := httptest.NewRequest(http.MethodOptions, "/resource", nil)
	allowed.Header.Set("Origin", "https://allowed.example")
	allowedResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(allowedResponse, allowed)
	if allowedResponse.Code != http.StatusNoContent || allowedResponse.Header().Get("Access-Control-Allow-Origin") != "https://allowed.example" {
		t.Fatalf("allowed response=%d headers=%v", allowedResponse.Code, allowedResponse.Header())
	}
	denied := httptest.NewRequest(http.MethodOptions, "/resource", nil)
	denied.Header.Set("Origin", "https://denied.example")
	deniedResponse := httptest.NewRecorder()
	server.Handler().ServeHTTP(deniedResponse, denied)
	if deniedResponse.Code != http.StatusForbidden || deniedResponse.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("denied response=%d headers=%v", deniedResponse.Code, deniedResponse.Header())
	}
}

func Test_Recovery_returns_safe_500_and_correlated_log(t *testing.T) {
	server, logs := testServer(t, func(router *gin.Engine) error {
		router.GET("/panic", func(*gin.Context) { panic("secret panic payload") })
		return nil
	})
	request := httptest.NewRequest(http.MethodGet, "/panic", nil)
	request.Header.Set("X-Request-ID", "request-123")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusInternalServerError || strings.Contains(response.Body.String(), "secret") {
		t.Fatalf("unsafe response=%d %q", response.Code, response.Body.String())
	}
	if !strings.Contains(logs.String(), "request-123") || strings.Contains(logs.String(), "secret panic payload") {
		t.Fatalf("unsafe or uncorrelated log=%s", logs.String())
	}
}

func Test_HTTPServer_Start_shutdown_is_concurrent_safe_and_releases_port(t *testing.T) {
	server, _ := testServer(t, func(router *gin.Engine) error {
		router.GET("/ok", func(ctx *gin.Context) { ctx.Status(http.StatusNoContent) })
		return nil
	})
	server.HTTPServer().Addr = "127.0.0.1:0"
	listener, serveErrors, err := server.Start()
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	var wait sync.WaitGroup
	errors := make(chan error, 2)
	for range 2 {
		wait.Go(func() {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			errors <- server.Shutdown(ctx)
		})
	}
	wait.Wait()
	close(errors)
	for shutdownErr := range errors {
		if shutdownErr != nil {
			t.Fatalf("shutdown: %v", shutdownErr)
		}
	}
	if serveErr := <-serveErrors; serveErr != nil {
		t.Fatalf("serve: %v", serveErr)
	}
	probe, dialErr := http.Get("http://" + listener.Addr().String() + "/ok")
	if dialErr == nil {
		if closeErr := probe.Body.Close(); closeErr != nil {
			t.Errorf("close probe: %v", closeErr)
		}
		t.Fatal("port remained reachable")
	}
}
