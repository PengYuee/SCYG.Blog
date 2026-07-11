package httpserver_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func Test_Recovery_aborts_committed_flush_without_appending_error(t *testing.T) {
	server, _ := testServer(t, func(router *gin.Engine) error {
		router.GET("/stream-panic", func(ctx *gin.Context) {
			if _, writeErr := ctx.Writer.WriteString("stream-prefix"); writeErr != nil {
				t.Errorf("write stream: %v", writeErr)
			}
			ctx.Writer.Flush()
			panic("secret after flush")
		})
		return nil
	})
	response := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/stream-panic", nil)
	var recovered any

	func() {
		defer func() { recovered = recover() }()
		server.Handler().ServeHTTP(response, request)
	}()

	if recovered != http.ErrAbortHandler {
		t.Fatalf("recovered=%v", recovered)
	}
	if response.Body.String() != "stream-prefix" || strings.Contains(response.Body.String(), "internal_error") || strings.Contains(response.Body.String(), "secret") {
		t.Fatalf("unsafe streaming body=%q", response.Body.String())
	}
}

func Test_RequestID_replaces_oversized_untrusted_value(t *testing.T) {
	server, _ := testServer(t, func(router *gin.Engine) error {
		router.GET("/id", func(ctx *gin.Context) { ctx.Status(http.StatusNoContent) })
		return nil
	})
	request := httptest.NewRequest(http.MethodGet, "/id", nil)
	request.Header.Set("X-Request-ID", strings.Repeat("x", 129))
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	requestID := response.Header().Get("X-Request-ID")
	if response.Code != http.StatusNoContent || len(requestID) != 32 || requestID == strings.Repeat("x", 129) {
		t.Fatalf("status=%d request_id=%q", response.Code, requestID)
	}
}

func Test_RequestID_preserves_reasonable_token(t *testing.T) {
	server, _ := testServer(t, func(router *gin.Engine) error {
		router.GET("/id", func(ctx *gin.Context) { ctx.Status(http.StatusNoContent) })
		return nil
	})
	request := httptest.NewRequest(http.MethodGet, "/id", nil)
	request.Header.Set("X-Request-ID", "valid.Request_ID-123~")
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Header().Get("X-Request-ID") != "valid.Request_ID-123~" {
		t.Fatalf("request id=%q", response.Header().Get("X-Request-ID"))
	}
}
