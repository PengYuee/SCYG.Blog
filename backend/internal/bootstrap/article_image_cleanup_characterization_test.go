package bootstrap_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/PengYuee/SCYG.Blog/backend/internal/bootstrap"
	module "github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/blobstorage"
	"github.com/PengYuee/SCYG.Blog/backend/migrations"
)

// Test_Application_creates_image_directory_before_content_construction 锁定现有目录构造顺序。
func Test_Application_creates_image_directory_before_content_construction(t *testing.T) {
	// Given
	storageDirectory := filepath.Join(t.TempDir(), "nested", "images")
	dependencies := validDependencies(&fakeTelemetry{}, &fakeDatabase{}, &fakeMigration{version: migrations.CurrentVersion}, &fakeServer{})
	directoryObserved := false
	dependencies.NewContent = func(_ bootstrap.Database, _ module.Authorizer, _ module.CurrentAuthorProvider, filesystem *blobstorage.Filesystem, _ module.ArticleImagePolicy) (*module.Module, error) {
		if filesystem == nil {
			t.Fatal("内容构造前图片存储为空")
		}
		if info, err := os.Stat(storageDirectory); err != nil || !info.IsDir() {
			t.Fatalf("内容构造前图片目录不可用：info=%v err=%v", info, err)
		}
		directoryObserved = true
		return &module.Module{}, nil
	}
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	configBody := "database:\n  dsn: postgres://postgres:postgres@localhost:5432/scyg?sslmode=disable\narticle_images:\n  directory: " + filepath.ToSlash(storageDirectory) + "\n"
	if err := os.WriteFile(configPath, []byte(configBody), 0o600); err != nil {
		t.Fatal(err)
	}

	// When
	app, err := bootstrap.New(context.Background(), bootstrap.Options{ConfigFile: configPath, LogWriter: &bytes.Buffer{}}, dependencies)

	// Then
	if err != nil || !directoryObserved {
		t.Fatalf("app=%v err=%v directoryObserved=%t", app, err, directoryObserved)
	}
	if shutdownErr := app.Shutdown(context.Background()); shutdownErr != nil {
		t.Fatal(shutdownErr)
	}
}
