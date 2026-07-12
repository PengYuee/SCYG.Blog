package bootstrap_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/PengYuee/SCYG.Blog/backend/internal/bootstrap"
	module "github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/config"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/database"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/httpserver"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/observability"
	"github.com/PengYuee/SCYG.Blog/backend/migrations"
)

type fakeTelemetry struct {
	closes int
	err    error
}

func (fake *fakeTelemetry) Shutdown(context.Context) error { fake.closes++; return fake.err }

type fakeDatabase struct {
	closes  int
	pingErr error
}

func (fake *fakeDatabase) Ping(context.Context) error { return fake.pingErr }
func (fake *fakeDatabase) Close() error               { fake.closes++; return nil }

type fakeMigration struct {
	version    uint
	dirty      bool
	versionErr error
	closes     int
}

func (fake *fakeMigration) Version() (uint, bool, error) {
	return fake.version, fake.dirty, fake.versionErr
}
func (fake *fakeMigration) Close() error { fake.closes++; return nil }

type fakeServer struct {
	startErr error
	closes   int
}

func (fake *fakeServer) Start() (net.Listener, <-chan error, error) { return nil, nil, fake.startErr }
func (fake *fakeServer) Shutdown(context.Context) error             { fake.closes++; return nil }

// withConfig 为 bootstrap 单元测试提供显式文件，避免依赖入口默认路径。
func withConfig(t *testing.T, options bootstrap.Options) bootstrap.Options {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("database:\n  dsn: postgres://postgres:postgres@localhost:5432/scyg?sslmode=disable\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	options.ConfigFile = path
	return options
}

func validDependencies(telemetry *fakeTelemetry, db *fakeDatabase, migration *fakeMigration, server *fakeServer) bootstrap.Dependencies {
	return bootstrap.Dependencies{
		LoadConfig: config.Load,
		NewLogger: func(observability.LoggerOptions) (*slog.Logger, error) {
			return slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil)), nil
		},
		NewTelemetry: func(config.Telemetry) (bootstrap.Telemetry, error) { return telemetry, nil },
		NewDatabase:  func(context.Context, database.Options) (bootstrap.Database, error) { return db, nil },
		NewMigration: func(config.DSN) (bootstrap.Migration, error) { return migration, nil },
		NewContent: func(bootstrap.Database, module.Authorizer, module.CurrentAuthorProvider) (*module.Module, error) {
			return &module.Module{}, nil
		},
		NewREST: func(*module.Module, *observability.Health, bool) (func(*gin.Engine) error, error) {
			return func(*gin.Engine) error { return nil }, nil
		},
		NewHTTP: func(httpserver.Options) (bootstrap.HTTPServer, error) { return server, nil },
	}
}

func Test_Application_RejectsPendingMigration_and_closes_prior_resources_once(t *testing.T) {
	// Given
	telemetry, db := &fakeTelemetry{}, &fakeDatabase{}
	migration := &fakeMigration{version: migrations.CurrentVersion - 1}
	dependencies := validDependencies(telemetry, db, migration, &fakeServer{})

	// When
	app, err := bootstrap.New(context.Background(), withConfig(t, bootstrap.Options{LogWriter: &bytes.Buffer{}}), dependencies)

	// Then
	if app != nil || err == nil {
		t.Fatalf("app=%v err=%v", app, err)
	}
	if telemetry.closes != 1 || db.closes != 1 || migration.closes != 1 {
		t.Fatalf("telemetry=%d db=%d migration=%d", telemetry.closes, db.closes, migration.closes)
	}
}

func Test_Application_RejectsDirtyMigration_and_closes_prior_resources_once(t *testing.T) {
	// Given
	telemetry, db := &fakeTelemetry{}, &fakeDatabase{}
	migration := &fakeMigration{version: migrations.CurrentVersion, dirty: true}

	// When
	_, err := bootstrap.New(context.Background(), withConfig(t, bootstrap.Options{LogWriter: &bytes.Buffer{}}), validDependencies(telemetry, db, migration, &fakeServer{}))

	// Then
	if err == nil || telemetry.closes != 1 || db.closes != 1 || migration.closes != 1 {
		t.Fatalf("err=%v telemetry=%d db=%d migration=%d", err, telemetry.closes, db.closes, migration.closes)
	}
}

func Test_Application_CleansOnBindFailure_in_reverse_once(t *testing.T) {
	// Given
	telemetry, db := &fakeTelemetry{}, &fakeDatabase{}
	migration := &fakeMigration{version: migrations.CurrentVersion}
	server := &fakeServer{startErr: errors.New("端口已占用")}
	app, err := bootstrap.New(context.Background(), withConfig(t, bootstrap.Options{LogWriter: &bytes.Buffer{}}), validDependencies(telemetry, db, migration, server))
	if err != nil {
		t.Fatalf("new app: %v", err)
	}

	// When
	startErr := app.Start()

	// Then
	if startErr == nil || telemetry.closes != 1 || db.closes != 1 {
		t.Fatalf("err=%v telemetry=%d db=%d", startErr, telemetry.closes, db.closes)
	}
}

func Test_Application_RejectsUnavailableDB_and_closes_telemetry_once(t *testing.T) {
	// Given
	telemetry := &fakeTelemetry{}
	dependencies := validDependencies(telemetry, &fakeDatabase{}, &fakeMigration{}, &fakeServer{})
	dependencies.NewDatabase = func(context.Context, database.Options) (bootstrap.Database, error) {
		return nil, errors.New("数据库不可用")
	}

	// When
	_, err := bootstrap.New(context.Background(), withConfig(t, bootstrap.Options{LogWriter: &bytes.Buffer{}}), dependencies)

	// Then
	if err == nil || telemetry.closes != 1 {
		t.Fatalf("err=%v telemetry=%d", err, telemetry.closes)
	}
}

func Test_Application_RejectsContentConstruction_and_closes_prior_resources_once(t *testing.T) {
	// Given
	telemetry, db := &fakeTelemetry{}, &fakeDatabase{}
	migration := &fakeMigration{version: migrations.CurrentVersion}
	dependencies := validDependencies(telemetry, db, migration, &fakeServer{})
	dependencies.NewContent = func(bootstrap.Database, module.Authorizer, module.CurrentAuthorProvider) (*module.Module, error) {
		return nil, errors.New("内容构造失败")
	}

	// When
	_, err := bootstrap.New(context.Background(), withConfig(t, bootstrap.Options{LogWriter: &bytes.Buffer{}}), dependencies)

	// Then
	if err == nil || telemetry.closes != 1 || db.closes != 1 {
		t.Fatalf("err=%v telemetry=%d db=%d", err, telemetry.closes, db.closes)
	}
}

func Test_Application_RejectsRESTConstruction_and_closes_prior_resources_once(t *testing.T) {
	// Given
	telemetry, db := &fakeTelemetry{}, &fakeDatabase{}
	migration := &fakeMigration{version: migrations.CurrentVersion}
	dependencies := validDependencies(telemetry, db, migration, &fakeServer{})
	dependencies.NewREST = func(*module.Module, *observability.Health, bool) (func(*gin.Engine) error, error) {
		return nil, errors.New("REST 构造失败")
	}

	// When
	_, err := bootstrap.New(context.Background(), withConfig(t, bootstrap.Options{LogWriter: &bytes.Buffer{}}), dependencies)

	// Then
	if err == nil || telemetry.closes != 1 || db.closes != 1 {
		t.Fatalf("err=%v telemetry=%d db=%d", err, telemetry.closes, db.closes)
	}
}

func Test_Application_RejectsInvalidConfig_before_resources(t *testing.T) {
	// Given
	dependencies := validDependencies(&fakeTelemetry{}, &fakeDatabase{}, &fakeMigration{}, &fakeServer{})
	dependencies.LoadConfig = func(config.Options) (config.Config, error) { return config.Config{}, errors.New("配置无效") }

	// When
	app, err := bootstrap.New(context.Background(), withConfig(t, bootstrap.Options{LogWriter: &bytes.Buffer{}}), dependencies)

	// Then
	if app != nil || err == nil {
		t.Fatalf("app=%v err=%v", app, err)
	}
}

func Test_Application_RejectsTelemetryConstruction_without_cleanup(t *testing.T) {
	// Given
	dependencies := validDependencies(&fakeTelemetry{}, &fakeDatabase{}, &fakeMigration{}, &fakeServer{})
	dependencies.NewTelemetry = func(config.Telemetry) (bootstrap.Telemetry, error) { return nil, errors.New("遥测构造失败") }

	// When
	app, err := bootstrap.New(context.Background(), withConfig(t, bootstrap.Options{LogWriter: &bytes.Buffer{}}), dependencies)

	// Then
	if app != nil || err == nil {
		t.Fatalf("app=%v err=%v", app, err)
	}
}

func Test_Application_RejectsHTTPConstruction_and_closes_prior_resources_once(t *testing.T) {
	// Given
	telemetry, db := &fakeTelemetry{}, &fakeDatabase{}
	migration := &fakeMigration{version: migrations.CurrentVersion}
	dependencies := validDependencies(telemetry, db, migration, &fakeServer{})
	dependencies.NewHTTP = func(httpserver.Options) (bootstrap.HTTPServer, error) { return nil, errors.New("HTTP 构造失败") }

	// When
	_, err := bootstrap.New(context.Background(), withConfig(t, bootstrap.Options{LogWriter: &bytes.Buffer{}}), dependencies)

	// Then
	if err == nil || telemetry.closes != 1 || db.closes != 1 {
		t.Fatalf("err=%v telemetry=%d db=%d", err, telemetry.closes, db.closes)
	}
}

func Test_Application_injects_stable_development_author_from_validated_config(t *testing.T) {
	// Given
	telemetry, db := &fakeTelemetry{}, &fakeDatabase{}
	migration := &fakeMigration{version: migrations.CurrentVersion}
	server := &fakeServer{}
	dependencies := validDependencies(telemetry, db, migration, server)
	var captured module.CurrentAuthorProvider
	dependencies.NewContent = func(_ bootstrap.Database, _ module.Authorizer, provider module.CurrentAuthorProvider) (*module.Module, error) {
		captured = provider
		return &module.Module{}, nil
	}
	path := filepath.Join(t.TempDir(), "config.yaml")
	yaml := "database:\n  dsn: postgres://postgres:postgres@localhost:5432/scyg?sslmode=disable\narticle_images:\n  development_author_id: 0123456789abcdef0123456789abcdef\n"
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}

	// When
	app, err := bootstrap.New(context.Background(), bootstrap.Options{ConfigFile: path, LogWriter: &bytes.Buffer{}}, dependencies)
	// Then
	if err != nil {
		t.Fatal(err)
	}
	if captured == nil {
		t.Fatal("development author provider was not injected")
	}
	first, firstErr := captured.CurrentAuthor(context.Background())
	second, secondErr := captured.CurrentAuthor(context.Background())
	if firstErr != nil || secondErr != nil || first != second || first.String() != "0123456789abcdef0123456789abcdef" {
		t.Fatalf("unstable author: %v %v %v %v", first, second, firstErr, secondErr)
	}
	if shutdownErr := app.Shutdown(context.Background()); shutdownErr != nil {
		t.Fatal(shutdownErr)
	}
}
