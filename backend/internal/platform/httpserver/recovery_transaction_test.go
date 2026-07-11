package httpserver_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func Test_Recovery_discards_partial_response_for_all_panic_values(t *testing.T) {
	t.Run("string", func(t *testing.T) { assertSafeRecoveredResponse(t, func() { panic("secret panic string") }) })
	t.Run("error", func(t *testing.T) { assertSafeRecoveredResponse(t, func() { panic(errors.New("secret panic error")) }) })
	t.Run("nil", func(t *testing.T) {
		assertSafeRecoveredResponse(t, func() {
			var value any
			if err := json.Unmarshal([]byte("null"), &value); err != nil {
				t.Fatalf("decode nil: %v", err)
			}
			panic(value)
		})
	})
}

func assertSafeRecoveredResponse(t *testing.T, panicHandler func()) {
	t.Helper()
	server, logs := testServer(t, func(router *gin.Engine) error {
		router.GET("/partial-panic", func(ctx *gin.Context) {
			ctx.Header("X-Secret-Prefix", "secret-header")
			ctx.Status(http.StatusAccepted)
			if _, err := ctx.Writer.WriteString("secret-prefix"); err != nil {
				t.Errorf("write prefix: %v", err)
			}
			panicHandler()
		})
		return nil
	})
	request := httptest.NewRequest(http.MethodGet, "/partial-panic", nil)
	request.Header.Set("X-Request-ID", "request-safe")
	response := httptest.NewRecorder()

	server.Handler().ServeHTTP(response, request)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d body=%q", response.Code, response.Body.String())
	}
	if response.Body.String() != `{"error":"internal_error"}` {
		t.Fatalf("body=%q", response.Body.String())
	}
	if response.Header().Get("X-Secret-Prefix") != "" {
		t.Fatalf("secret header leaked: %q", response.Header().Get("X-Secret-Prefix"))
	}
	if strings.Contains(response.Body.String(), "secret") || strings.Contains(logs.String(), "secret panic") {
		t.Fatalf("secret leaked response=%q logs=%s", response.Body.String(), logs.String())
	}
	if !strings.Contains(logs.String(), "request-safe") {
		t.Fatalf("request fields missing: %s", logs.String())
	}
}
