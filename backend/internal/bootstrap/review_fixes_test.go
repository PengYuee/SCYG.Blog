package bootstrap_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
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

type orderedTelemetry struct{ events *[]string }

func (resource *orderedTelemetry) Shutdown(context.Context) error {
	*resource.events = append(*resource.events, "telemetry")
	return nil
}

type orderedDatabase struct{ events *[]string }

func (*orderedDatabase) Ping(context.Context) error { return nil }
func (resource *orderedDatabase) Close() error {
	*resource.events = append(*resource.events, "database")
	return nil
}

type orderedMigration struct{ events *[]string }

func (*orderedMigration) Version() (uint, bool, error) { return migrations.CurrentVersion, false, nil }
func (resource *orderedMigration) Close() error {
	*resource.events = append(*resource.events, "migration")
	return nil
}

func Test_Application_New_keeps_readiness_inactive_before_Start(t *testing.T) {
	// Given
	dependencies := validDependencies(&fakeTelemetry{}, &fakeDatabase{}, &fakeMigration{version: migrations.CurrentVersion}, &fakeServer{})
	var health *observability.Health
	dependencies.NewREST = func(_ *module.Module, candidate *observability.Health, _ bool) (func(*gin.Engine) error, error) {
		health = candidate
		return func(*gin.Engine) error { return nil }, nil
	}

	// When
	app, err := bootstrap.New(context.Background(), bootstrap.Options{LogWriter: &bytes.Buffer{}}, dependencies)
	if err != nil {
		t.Fatalf("构造应用: %v", err)
	}
	ready, readyErr := health.Ready(context.Background())

	// Then
	if app == nil || ready || !errors.Is(readyErr, observability.ErrShuttingDown) {
		t.Fatalf("app=%v ready=%v err=%v", app, ready, readyErr)
	}
}

func Test_Application_constructor_failure_cleans_in_exact_reverse_order(t *testing.T) {
	// Given
	events := make([]string, 0, 3)
	dependencies := validDependencies(&fakeTelemetry{}, &fakeDatabase{}, &fakeMigration{}, &fakeServer{})
	dependencies.NewTelemetry = func(config.Telemetry) (bootstrap.Telemetry, error) { return &orderedTelemetry{events: &events}, nil }
	dependencies.NewDatabase = func(context.Context, database.Options) (bootstrap.Database, error) {
		return &orderedDatabase{events: &events}, nil
	}
	dependencies.NewMigration = func(config.DSN) (bootstrap.Migration, error) { return &orderedMigration{events: &events}, nil }
	dependencies.NewHTTP = func(httpserver.Options) (bootstrap.HTTPServer, error) { return nil, errors.New("HTTP 构造失败") }

	// When
	_, err := bootstrap.New(context.Background(), bootstrap.Options{LogWriter: &bytes.Buffer{}}, dependencies)

	// Then
	if err == nil {
		t.Fatal("期望 HTTP 构造失败")
	}
	want := "migration,database,telemetry"
	got := strings.Join(events, ",")
	if got != want {
		t.Fatalf("清理顺序=%s，期望=%s", got, want)
	}
}

func Test_Application_rejects_typed_nil_telemetry_without_panic(t *testing.T) {
	// Given
	dependencies := validDependencies(&fakeTelemetry{}, &fakeDatabase{}, &fakeMigration{}, &fakeServer{})
	var telemetry *fakeTelemetry
	dependencies.NewTelemetry = func(config.Telemetry) (bootstrap.Telemetry, error) { return telemetry, nil }

	// When
	app, err := bootstrap.New(context.Background(), bootstrap.Options{LogWriter: &bytes.Buffer{}}, dependencies)

	// Then
	if app != nil || err == nil || !strings.Contains(err.Error(), "遥测构造器返回空结果") {
		t.Fatalf("app=%v err=%v", app, err)
	}
}
