package bootstrap_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/PengYuee/SCYG.Blog/backend/internal/bootstrap"
	module "github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/blobstorage"
	"github.com/PengYuee/SCYG.Blog/backend/migrations"
)

const fixedDevelopmentAuthorID = "0123456789abcdef0123456789abcdef"

func Test_Application_development_fixed_author_enables_real_write_actions(t *testing.T) {
	// Given
	telemetry, database := &fakeTelemetry{}, &fakeDatabase{}
	dependencies := validDependencies(telemetry, database, &fakeMigration{version: migrations.CurrentVersion}, &fakeServer{})
	var capturedAuthorizer module.Authorizer
	var capturedAuthor module.CurrentAuthorProvider
	var capturedPolicy module.ArticleImagePolicy
	dependencies.NewContent = func(_ bootstrap.Database, authorizer module.Authorizer, author module.CurrentAuthorProvider, _ *blobstorage.Filesystem, policy module.ArticleImagePolicy) (*module.Module, error) {
		capturedAuthorizer, capturedAuthor, capturedPolicy = authorizer, author, policy
		return &module.Module{}, nil
	}
	path := filepath.Join(t.TempDir(), "development.yaml")
	yaml := "app:\n  env: development\ndatabase:\n  dsn: postgres://postgres:postgres@localhost:5432/scyg?sslmode=disable\narticle_images:\n  development_author_id: " + fixedDevelopmentAuthorID + "\n  pending_ttl: 2h\n  orphan_grace: 3h\n  cleanup_interval: 30m\n  upload_request_bytes: 2048\n  max_file_bytes: 1024\n  max_pixels: 100\n  max_dimension: 10\n"
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}

	// When
	app, err := bootstrap.New(context.Background(), bootstrap.Options{ConfigFile: path, LogWriter: &bytes.Buffer{}}, dependencies)
	// Then
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []module.Action{module.ActionCreateArticle, module.ActionUploadArticleImage} {
		if authorizeErr := capturedAuthorizer.Authorize(context.Background(), action, module.Resource{Kind: "article"}); authorizeErr != nil {
			t.Fatalf("development 写入动作 %s 被拒绝：%v", action, authorizeErr)
		}
	}
	author, authorErr := capturedAuthor.CurrentAuthor(context.Background())
	if authorErr != nil || author.String() != fixedDevelopmentAuthorID {
		t.Fatalf("固定作者未注入：author=%s err=%v", author.String(), authorErr)
	}
	if capturedPolicy.MaxFileBytes() != 1024 || capturedPolicy.MaxPixels() != 100 || capturedPolicy.MaxDimension() != 10 || capturedPolicy.PendingTTL() != 2*time.Hour || capturedPolicy.OrphanGrace() != 3*time.Hour {
		t.Fatalf("图片策略未完整注入：%+v", capturedPolicy)
	}
	if shutdownErr := app.Shutdown(context.Background()); shutdownErr != nil {
		t.Fatal(shutdownErr)
	}
}

func Test_Application_production_without_fixed_author_keeps_deny_all(t *testing.T) {
	// Given
	telemetry, database := &fakeTelemetry{}, &fakeDatabase{}
	dependencies := validDependencies(telemetry, database, &fakeMigration{version: migrations.CurrentVersion}, &fakeServer{})
	var capturedAuthorizer module.Authorizer
	dependencies.NewContent = func(_ bootstrap.Database, authorizer module.Authorizer, _ module.CurrentAuthorProvider, _ *blobstorage.Filesystem, _ module.ArticleImagePolicy) (*module.Module, error) {
		capturedAuthorizer = authorizer
		return &module.Module{}, nil
	}
	path := filepath.Join(t.TempDir(), "production.yaml")
	yaml := "app:\n  env: production\ndatabase:\n  dsn: postgres://postgres:postgres@localhost:5432/scyg?sslmode=disable\n"
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}

	// When
	app, err := bootstrap.New(context.Background(), bootstrap.Options{ConfigFile: path, LogWriter: &bytes.Buffer{}}, dependencies)
	// Then
	if err != nil {
		t.Fatal(err)
	}
	authorizeErr := module.AuthorizerOrDeny(capturedAuthorizer).Authorize(context.Background(), module.ActionCreateArticle, module.Resource{Kind: "article"})
	if !errors.Is(authorizeErr, module.ErrPermissionDenied) {
		t.Fatalf("production 应保持 DenyAll，实际 %v", authorizeErr)
	}
	if shutdownErr := app.Shutdown(context.Background()); shutdownErr != nil {
		t.Fatal(shutdownErr)
	}
}

func Test_Application_development_and_test_without_fixed_author_keep_deny_all(t *testing.T) {
	for _, environment := range []string{"development", "test"} {
		t.Run(environment, func(t *testing.T) {
			// Given
			telemetry, database := &fakeTelemetry{}, &fakeDatabase{}
			dependencies := validDependencies(telemetry, database, &fakeMigration{version: migrations.CurrentVersion}, &fakeServer{})
			var capturedAuthorizer module.Authorizer
			dependencies.NewContent = func(_ bootstrap.Database, authorizer module.Authorizer, _ module.CurrentAuthorProvider, _ *blobstorage.Filesystem, _ module.ArticleImagePolicy) (*module.Module, error) {
				capturedAuthorizer = authorizer
				return &module.Module{}, nil
			}
			path := filepath.Join(t.TempDir(), environment+".yaml")
			yaml := "app:\n  env: " + environment + "\ndatabase:\n  dsn: postgres://postgres:postgres@localhost:5432/scyg?sslmode=disable\n"
			if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
				t.Fatal(err)
			}

			// When
			app, err := bootstrap.New(context.Background(), bootstrap.Options{ConfigFile: path, LogWriter: &bytes.Buffer{}}, dependencies)
			// Then
			if err != nil {
				t.Fatal(err)
			}
			authorizeErr := module.AuthorizerOrDeny(capturedAuthorizer).Authorize(context.Background(), module.ActionUploadArticleImage, module.Resource{Kind: "article_image"})
			if !errors.Is(authorizeErr, module.ErrPermissionDenied) {
				t.Fatalf("%s 缺少固定作者时应保持 DenyAll，实际 %v", environment, authorizeErr)
			}
			if shutdownErr := app.Shutdown(context.Background()); shutdownErr != nil {
				t.Fatal(shutdownErr)
			}
		})
	}
}
