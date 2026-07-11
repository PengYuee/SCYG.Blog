package httpserver_test

import (
	"net/http"
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
