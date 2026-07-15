package bootstrap_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/PengYuee/SCYG.Blog/backend/internal/bootstrap"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/database"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/httpserver"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/observability"
	"github.com/PengYuee/SCYG.Blog/backend/migrations"
)

type lifecycleEvents struct {
	mutex  sync.Mutex
	values []string
}

func (events *lifecycleEvents) add(value string) {
	events.mutex.Lock()
	defer events.mutex.Unlock()
	events.values = append(events.values, value)
}

type orderedCleanupWorker struct{ events *lifecycleEvents }

func (worker orderedCleanupWorker) Start(context.Context) error {
	worker.events.add("worker-start")
	return nil
}

// retryableCleanupWorker 首次停止超时，随后确认退出。
type retryableCleanupWorker struct{ stops int }

func (*retryableCleanupWorker) Start(context.Context) error { return nil }
func (worker *retryableCleanupWorker) Stop(ctx context.Context) error {
	worker.stops++
	if worker.stops == 1 {
		return context.DeadlineExceeded
	}
	return nil
}

func (worker orderedCleanupWorker) Stop(context.Context) error {
	worker.events.add("worker-stop")
	return nil
}

type orderedHTTPServer struct {
	events   *lifecycleEvents
	listener net.Listener
}

func (server *orderedHTTPServer) Start() (net.Listener, <-chan error, error) {
	server.events.add("http-start")
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, err
	}
	server.listener = listener
	return listener, make(chan error), nil
}

func (server *orderedHTTPServer) Shutdown(context.Context) error {
	server.events.add("http-stop")
	return server.listener.Close()
}

type cleanupOrderedDatabase struct {
	fakeDatabase
	events *lifecycleEvents
}

func (database *cleanupOrderedDatabase) Close() error {
	database.events.add("db-close")
	return nil
}

type orderedObserver struct{ events *lifecycleEvents }

func (observer orderedObserver) ReadinessWithdrawn() { observer.events.add("readiness-false") }
func (orderedObserver) HTTPClosed()                  {}
func (orderedObserver) WorkerStopped()               {}
func (orderedObserver) DatabaseClosed()              {}
func (orderedObserver) TelemetryClosed()             {}

func Test_Application_orders_cleanup_worker_around_HTTP_lifecycle(t *testing.T) {
	// Given
	events := &lifecycleEvents{}
	db := &cleanupOrderedDatabase{events: events}
	server := &orderedHTTPServer{events: events}
	dependencies := validDependencies(&fakeTelemetry{}, &db.fakeDatabase, &fakeMigration{version: migrations.CurrentVersion}, &fakeServer{})
	dependencies.NewDatabase = func(context.Context, database.Options) (bootstrap.Database, error) { return db, nil }
	dependencies.NewLogger = func(observability.LoggerOptions) (*slog.Logger, error) {
		return slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), nil
	}
	dependencies.NewHTTP = func(options httpserver.Options) (bootstrap.HTTPServer, error) { return server, nil }
	dependencies.NewCleanupWorker = func(bootstrap.CleanupRunner, time.Duration, *slog.Logger) (bootstrap.CleanupWorker, error) {
		return orderedCleanupWorker{events: events}, nil
	}

	// When
	app, err := bootstrap.New(context.Background(), withConfig(t, bootstrap.Options{LogWriter: &bytes.Buffer{}, LifecycleObserver: orderedObserver{events: events}}), dependencies)
	if err == nil {
		err = app.Start()
	}
	if err == nil {
		err = app.Shutdown(context.Background())
	}

	// Then
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"worker-start", "http-start", "readiness-false", "http-stop", "worker-stop", "db-close"}
	if !reflect.DeepEqual(events.values, want) {
		t.Fatalf("生命周期顺序=%v，期望=%v", events.values, want)
	}
}

// Test_Application_keeps_database_open_until_worker_stop_is_confirmed 防止 worker 超时后访问已关闭数据库。
func Test_Application_keeps_database_open_until_worker_stop_is_confirmed(t *testing.T) {
	// Given
	db := &fakeDatabase{}
	worker := &retryableCleanupWorker{}
	dependencies := validDependencies(&fakeTelemetry{}, db, &fakeMigration{version: migrations.CurrentVersion}, &fakeServer{})
	dependencies.NewCleanupWorker = func(bootstrap.CleanupRunner, time.Duration, *slog.Logger) (bootstrap.CleanupWorker, error) {
		return worker, nil
	}
	app, err := bootstrap.New(context.Background(), withConfig(t, bootstrap.Options{LogWriter: &bytes.Buffer{}}), dependencies)
	if err != nil {
		t.Fatal(err)
	}

	// When
	firstErr := app.Shutdown(context.Background())
	closesAfterTimeout := db.closes
	secondErr := app.Shutdown(context.Background())

	// Then
	if !errors.Is(firstErr, context.DeadlineExceeded) || closesAfterTimeout != 0 || secondErr != nil || db.closes != 1 {
		t.Fatalf("firstErr=%v closesAfterTimeout=%d secondErr=%v closes=%d", firstErr, closesAfterTimeout, secondErr, db.closes)
	}
}
