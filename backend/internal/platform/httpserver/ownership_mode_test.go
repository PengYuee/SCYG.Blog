package httpserver_test

import (
	"bytes"
	"log/slog"
	"net/http"
	"reflect"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/config"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/httpserver"
)

func Test_HTTPServer_New_preserves_global_Gin_mode_under_concurrency(t *testing.T) {
	previous := gin.Mode()
	gin.SetMode(gin.TestMode)
	t.Cleanup(func() { gin.SetMode(previous) })
	cfg, err := config.Load(config.Options{})
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	logger := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	var wait sync.WaitGroup
	constructionErrors := make(chan error, 64)

	for range 64 {
		wait.Go(func() {
			_, newErr := httpserver.New(httpserver.Options{HTTP: cfg.HTTP(), Logger: logger, Mount: func(*gin.Engine) error { return nil }})
			constructionErrors <- newErr
		})
	}
	wait.Wait()
	close(constructionErrors)

	for constructionErr := range constructionErrors {
		if constructionErr != nil {
			t.Fatalf("new: %v", constructionErr)
		}
	}
	if gin.Mode() != gin.TestMode {
		t.Fatalf("Gin mode changed to %q", gin.Mode())
	}
}

func Test_HTTPServer_public_API_exposes_no_mutable_http_Server(t *testing.T) {
	serverType := reflect.TypeFor[*httpserver.Server]()
	forbidden := reflect.TypeFor[*http.Server]()
	for index := range serverType.NumMethod() {
		method := serverType.Method(index)
		for output := 0; output < method.Type.NumOut(); output++ {
			if method.Type.Out(output) == forbidden {
				t.Fatalf("method %s exposes *http.Server", method.Name)
			}
		}
	}
}

func Test_HTTPServer_Configuration_returns_value_snapshot(t *testing.T) {
	server, _ := testServer(t, func(*gin.Engine) error { return nil })
	snapshot := server.Configuration()
	if snapshot.MaxHeaderBytes() != httpserver.MaxHeaderBytes || snapshot.ReadTimeout() <= 0 || snapshot.Address() == "" {
		t.Fatalf("invalid snapshot: %+v", snapshot)
	}
}
