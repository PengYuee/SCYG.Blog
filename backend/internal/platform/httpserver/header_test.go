package httpserver_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/httpserver"
)

func Test_HTTPServer_configuration_applies_MaxHeaderBytes_without_pointer_exposure(t *testing.T) {
	server, _ := testServer(t, func(router *gin.Engine) error {
		router.GET("/ok", func(ctx *gin.Context) { ctx.Status(http.StatusNoContent) })
		return nil
	})

	snapshot := server.Configuration()

	if snapshot.MaxHeaderBytes() != httpserver.MaxHeaderBytes {
		t.Fatalf("max header bytes=%d", snapshot.MaxHeaderBytes())
	}
}

func TestPublicRetryAfterIsReadableFromAllowedBrowserOrigin(t *testing.T) {
	server, _ := testServer(t, func(router *gin.Engine) error {
		router.GET("/limited", func(ctx *gin.Context) {
			ctx.Header("Retry-After", "20")
			ctx.Status(http.StatusTooManyRequests)
		})
		return nil
	})
	request := httptest.NewRequest(http.MethodGet, "/limited", nil)
	request.Header.Set("Origin", "https://allowed.example")
	response := httptest.NewRecorder()
	server.Handler().ServeHTTP(response, request)
	if response.Code != http.StatusTooManyRequests || response.Header().Get("Access-Control-Expose-Headers") != "Retry-After" || response.Header().Get("Retry-After") != "20" {
		t.Fatalf("browser cannot consume rate-limit wait: %d %v", response.Code, response.Header())
	}
}
