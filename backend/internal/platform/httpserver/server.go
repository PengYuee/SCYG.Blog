// Package httpserver owns the production Gin and net/http lifecycle.
package httpserver

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"sync"

	"github.com/gin-gonic/gin"

	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/config"
)

const (
	// MaxRequestBodyBytes is the maximum accepted inbound request body.
	MaxRequestBodyBytes int64 = 1 << 20
	// MaxArticleImageUploadRequestBytes 是文章图片上传请求体的默认上限。
	MaxArticleImageUploadRequestBytes int64 = 6 << 20
	// MaxHeaderBytes is the maximum accepted HTTP header size.
	MaxHeaderBytes = 1 << 20
)

// MountRoutes installs application-owned routes exactly once during construction.
type MountRoutes func(*gin.Engine) error

// Options contains explicit server dependencies.
type Options struct {
	// Logger receives structured request and lifecycle records.
	Logger *slog.Logger
	// Mount installs routes while the composition root owns their selection.
	Mount MountRoutes
	// HTTP supplies validated immutable server settings.
	HTTP config.HTTP
	// ArticleImages 提供已验证的图片上传请求体上限；零值兼容既有构造测试并使用安全默认值。
	ArticleImages config.ArticleImages
}

// Server owns one configured HTTP server and its concurrent shutdown state.
type Server struct {
	// httpServer is the configured standard-library server.
	httpServer *http.Server
	// shutdownDone coordinates callers sharing an active shutdown attempt.
	shutdownDone chan struct{}
	// shutdownErr stores the active attempt result for concurrent callers.
	shutdownErr error
	// shutdownMu protects all shutdown state.
	shutdownMu sync.Mutex
	// shuttingDown reports whether one attempt is active.
	shuttingDown bool
	// shutdownComplete records a successful terminal shutdown.
	shutdownComplete bool
}

// New constructs a production Gin engine and HTTP server.
func New(options Options) (*Server, error) {
	if options.Logger == nil {
		return nil, fmt.Errorf("http server logger must not be nil")
	}
	if options.Mount == nil {
		return nil, fmt.Errorf("http server mount hook must not be nil")
	}
	engine := gin.New()
	if err := engine.SetTrustedProxies(options.HTTP.TrustedProxies()); err != nil {
		return nil, fmt.Errorf("set trusted proxies: %w", err)
	}
	uploadRequestBytes := options.ArticleImages.UploadRequestBytes()
	if uploadRequestBytes == 0 {
		uploadRequestBytes = MaxArticleImageUploadRequestBytes
	}
	engine.Use(requestID(), securityHeaders(), recovery(options.Logger), accessLog(options.Logger), cors(options.HTTP.CORSAllowedOrigins()), requestLimit(MaxRequestBodyBytes, uploadRequestBytes))
	if err := options.Mount(engine); err != nil {
		return nil, fmt.Errorf("mount routes: %w", err)
	}
	address := net.JoinHostPort(options.HTTP.Host(), strconv.Itoa(options.HTTP.Port()))
	configured := &http.Server{
		Addr: address, Handler: engine,
		ReadHeaderTimeout: options.HTTP.ReadHeaderTimeout(), ReadTimeout: options.HTTP.ReadTimeout(),
		WriteTimeout: options.HTTP.WriteTimeout(), IdleTimeout: options.HTTP.IdleTimeout(), MaxHeaderBytes: MaxHeaderBytes,
	}
	return &Server{httpServer: configured}, nil
}

// Handler exposes the fully configured handler for tests and embedding.
func (server *Server) Handler() http.Handler { return server.httpServer.Handler }

// Configuration returns an immutable value snapshot without exposing server ownership.
func (server *Server) Configuration() ConfigSnapshot {
	return ConfigSnapshot{
		address: server.httpServer.Addr, readHeaderTimeout: server.httpServer.ReadHeaderTimeout,
		readTimeout: server.httpServer.ReadTimeout, writeTimeout: server.httpServer.WriteTimeout,
		idleTimeout: server.httpServer.IdleTimeout, maxHeaderBytes: server.httpServer.MaxHeaderBytes,
	}
}

// Start binds synchronously so address failures are observable before return.
func (server *Server) Start() (net.Listener, <-chan error, error) {
	listener, err := net.Listen("tcp", server.httpServer.Addr)
	if err != nil {
		return nil, nil, fmt.Errorf("listen %s: %w", server.httpServer.Addr, err)
	}
	errors := make(chan error, 1)
	go func() {
		errors <- server.Serve(listener)
		close(errors)
	}()
	return listener, errors, nil
}

// Serve runs on an already-bound listener and treats graceful closure as success.
func (server *Server) Serve(listener net.Listener) error {
	err := server.httpServer.Serve(listener)
	if err == nil || err == http.ErrServerClosed {
		return nil
	}
	return fmt.Errorf("serve HTTP: %w", err)
}

// Shutdown gracefully stops the server; concurrent callers share one attempt.
func (server *Server) Shutdown(ctx context.Context) error {
	server.shutdownMu.Lock()
	if server.shutdownComplete {
		server.shutdownMu.Unlock()
		return nil
	}
	if server.shuttingDown {
		done := server.shutdownDone
		server.shutdownMu.Unlock()
		select {
		case <-done:
			server.shutdownMu.Lock()
			err := server.shutdownErr
			server.shutdownMu.Unlock()
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	server.shuttingDown = true
	server.shutdownDone = make(chan struct{})
	done := server.shutdownDone
	server.shutdownMu.Unlock()

	err := server.httpServer.Shutdown(ctx)
	server.shutdownMu.Lock()
	server.shutdownErr = err
	server.shuttingDown = false
	server.shutdownComplete = err == nil
	close(done)
	server.shutdownMu.Unlock()
	return err
}
