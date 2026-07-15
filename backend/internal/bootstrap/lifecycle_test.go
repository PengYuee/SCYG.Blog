package bootstrap_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/PengYuee/SCYG.Blog/backend/internal/bootstrap"
	"github.com/PengYuee/SCYG.Blog/backend/migrations"
)

// recordingLifecycleObserver 仅记录 App 从真实关闭调用点发布的事实。
type recordingLifecycleObserver struct{ readiness, http, worker, database, telemetry bool }

func (observer *recordingLifecycleObserver) ReadinessWithdrawn() { observer.readiness = true }
func (observer *recordingLifecycleObserver) HTTPClosed()         { observer.http = true }
func (observer *recordingLifecycleObserver) WorkerStopped()      { observer.worker = true }
func (observer *recordingLifecycleObserver) DatabaseClosed()     { observer.database = true }
func (observer *recordingLifecycleObserver) TelemetryClosed()    { observer.telemetry = true }

func Test_Application_Shutdown_reports_owned_resource_outcomes(t *testing.T) {
	observer := &recordingLifecycleObserver{}
	dependencies := validDependencies(&fakeTelemetry{}, &fakeDatabase{}, &fakeMigration{version: migrations.CurrentVersion}, &fakeServer{})
	app, err := bootstrap.New(context.Background(), bootstrap.Options{LogWriter: &bytes.Buffer{}, LifecycleObserver: observer}, dependencies)
	if err != nil {
		t.Fatalf("构造生命周期观察测试应用失败：%v", err)
	}
	if err = app.Shutdown(context.Background()); err != nil {
		t.Fatalf("关闭生命周期观察测试应用失败：%v", err)
	}
	if !observer.readiness || !observer.http || !observer.worker || !observer.database || !observer.telemetry {
		t.Fatalf("生命周期事实不完整：%+v", observer)
	}
}
