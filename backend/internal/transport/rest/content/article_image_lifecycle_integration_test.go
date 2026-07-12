//go:build integration

package content_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/jackc/pgx/v5/stdlib"

	module "github.com/PengYuee/SCYG.Blog/backend/internal/modules/content"
	contentpostgres "github.com/PengYuee/SCYG.Blog/backend/internal/modules/content/postgres"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/blobstorage"
	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/database"
	qaconfig "github.com/PengYuee/SCYG.Blog/backend/internal/qa/config"
	restcontent "github.com/PengYuee/SCYG.Blog/backend/internal/transport/rest/content"
	"github.com/PengYuee/SCYG.Blog/backend/migrations"
)

type integrationAllowAll struct{}

func (integrationAllowAll) Authorize(context.Context, module.Action, module.Resource) error {
	return nil
}

func integrationImageBytes(t *testing.T) []byte {
	t.Helper()
	source := image.NewRGBA(image.Rect(0, 0, 2, 2))
	source.Set(0, 0, color.White)
	var encoded bytes.Buffer
	if err := jpeg.Encode(&encoded, source, nil); err != nil {
		t.Fatal(err)
	}
	return encoded.Bytes()
}

func integrationRouter(t *testing.T, db *database.Database, store *blobstorage.Filesystem, author string) *gin.Engine {
	t.Helper()
	authorID, err := module.NewAuthorID(author)
	if err != nil {
		t.Fatal(err)
	}
	service, err := contentpostgres.New(contentpostgres.Dependencies{Database: db, Authorizer: integrationAllowAll{}, CurrentAuthor: module.NewFixedCurrentAuthorProvider(authorID), ImageFilesystem: store, ImagePendingTTL: 24 * time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	handler, err := restcontent.NewHandler(service, service)
	if err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	if err = handler.Register(router); err != nil {
		t.Fatal(err)
	}
	return router
}

func integrationFixture(t *testing.T) (*database.Database, *blobstorage.Filesystem, string) {
	t.Helper()
	qa, err := qaconfig.LoadLocal()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), qa.CommandTimeout())
	defer cancel()
	random := make([]byte, 8)
	if _, err = rand.Read(random); err != nil {
		t.Fatal(err)
	}
	name := qa.DatabasePrefix() + "todo6_" + hex.EncodeToString(random)
	adminDSN := qa.AdminDSN().Value()
	admin, err := sql.Open("pgx", adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	quotedName := `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
	if _, err = admin.ExecContext(ctx, "CREATE DATABASE "+quotedName); err != nil {
		_ = admin.Close()
		t.Fatal(err)
	}
	if err = admin.Close(); err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	parsed.Path = "/" + name
	targetDSN := parsed.String()
	migrationDB, err := sql.Open("pgx", targetDSN)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := migrations.New(migrationDB, "")
	if err != nil {
		_ = migrationDB.Close()
		t.Fatal(err)
	}
	if err = runner.Up(); err != nil {
		_ = runner.Close()
		t.Fatal(err)
	}
	if err = runner.Close(); err != nil {
		t.Fatal(err)
	}
	db, err := database.New(ctx, database.Options{DSN: targetDSN, Logger: slog.New(slog.NewTextHandler(os.Stderr, nil)), MaxOpenConns: 5, MaxIdleConns: 2, ConnMaxLifetime: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	store, err := blobstorage.New(root)
	if err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if closeErr := store.Close(); closeErr != nil {
			t.Error(closeErr)
		}
		if closeErr := db.Close(); closeErr != nil {
			t.Error(closeErr)
		}
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), qa.CommandTimeout())
		defer cleanupCancel()
		cleanup, openErr := sql.Open("pgx", adminDSN)
		if openErr != nil {
			t.Error(openErr)
			return
		}
		defer cleanup.Close()
		_, _ = cleanup.ExecContext(cleanupCtx, `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1`, name)
		if _, dropErr := cleanup.ExecContext(cleanupCtx, "DROP DATABASE "+quotedName); dropErr != nil {
			t.Error(dropErr)
			return
		}
		var count int
		if queryErr := cleanup.QueryRowContext(cleanupCtx, `SELECT count(*) FROM pg_database WHERE datname = $1`, name).Scan(&count); queryErr != nil || count != 0 {
			t.Errorf("临时数据库残留 count=%d err=%v", count, queryErr)
		}
	})
	return db, store, root
}

func uploadIntegrationImage(t *testing.T, router http.Handler) map[string]any {
	t.Helper()
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, multipartRequest(t, []struct {
		name    string
		payload []byte
	}{{"file", integrationImageBytes(t)}}))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("upload status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body
}

func Test_ArticleImageLifecycle_real_HTTP_Postgres_and_filesystem(t *testing.T) {
	db, store, root := integrationFixture(t)
	owner := "abcdef0123456789abcdef0123456789"
	router := integrationRouter(t, db, store, owner)
	first := uploadIntegrationImage(t, router)
	firstID := first["id"].(string)
	firstKey := first["storageKey"].(string)
	var pendingCount int64
	if err := db.GORM().Table("article_images").Where("id = ? AND status = 'pending'", firstID).Count(&pendingCount).Error; err != nil || pendingCount != 1 {
		t.Fatalf("pending=%d err=%v", pendingCount, err)
	}
	cross := integrationRouter(t, db, store, "11111111111111111111111111111111")
	crossDelete := httptest.NewRecorder()
	cross.ServeHTTP(crossDelete, httptest.NewRequest(http.MethodDelete, "/api/v1/article-images/"+firstID, nil))
	if crossDelete.Code != http.StatusNotFound {
		t.Fatalf("cross delete=%d", crossDelete.Code)
	}
	pendingGet := httptest.NewRecorder()
	router.ServeHTTP(pendingGet, httptest.NewRequest(http.MethodGet, "/media/article-images/"+firstKey, nil))
	if pendingGet.Code != http.StatusOK || pendingGet.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("pending get=%d headers=%v", pendingGet.Code, pendingGet.Header())
	}
	ownerDelete := httptest.NewRecorder()
	router.ServeHTTP(ownerDelete, httptest.NewRequest(http.MethodDelete, "/api/v1/article-images/"+firstID, nil))
	if ownerDelete.Code != http.StatusNoContent {
		t.Fatalf("owner delete=%d body=%s", ownerDelete.Code, ownerDelete.Body.String())
	}
	orphanGet := httptest.NewRecorder()
	router.ServeHTTP(orphanGet, httptest.NewRequest(http.MethodGet, "/media/article-images/"+firstKey, nil))
	if orphanGet.Code != http.StatusNotFound {
		t.Fatalf("orphan get=%d", orphanGet.Code)
	}
	second := uploadIntegrationImage(t, router)
	secondID := second["id"].(string)
	secondKey := second["storageKey"].(string)
	now := time.Now().UTC()
	if err := db.GORM().Table("article_images").Where("id = ?", secondID).Updates(map[string]any{"status": "committed", "committed_at": now}).Error; err != nil {
		t.Fatal(err)
	}
	committedGet := httptest.NewRecorder()
	router.ServeHTTP(committedGet, httptest.NewRequest(http.MethodGet, "/media/article-images/"+secondKey, nil))
	if committedGet.Code != http.StatusOK || committedGet.Header().Get("Cache-Control") != "public, max-age=31536000, immutable" || committedGet.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("committed=%d headers=%v", committedGet.Code, committedGet.Header())
	}
	etag := committedGet.Header().Get("ETag")
	cached := httptest.NewRecorder()
	cachedRequest := httptest.NewRequest(http.MethodGet, "/media/article-images/"+secondKey, nil)
	cachedRequest.Header.Set("If-None-Match", " W/"+etag+", \"other\" ")
	router.ServeHTTP(cached, cachedRequest)
	if cached.Code != http.StatusNotModified || cached.Body.Len() != 0 || cached.Header().Get("ETag") != etag {
		t.Fatalf("cached=%d etag=%q body=%q", cached.Code, cached.Header().Get("ETag"), cached.Body.String())
	}
	committedDelete := httptest.NewRecorder()
	router.ServeHTTP(committedDelete, httptest.NewRequest(http.MethodDelete, "/api/v1/article-images/"+secondID, nil))
	if committedDelete.Code != http.StatusConflict {
		t.Fatalf("committed delete=%d body=%s", committedDelete.Code, committedDelete.Body.String())
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 2 {
		t.Fatalf("files=%d err=%v", len(entries), err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".article-image-") {
			t.Fatalf("temp remains: %s", entry.Name())
		}
	}
	if !strings.HasPrefix(etag, "\"") || !strings.HasSuffix(etag, "\"") {
		t.Fatalf("etag=%q", etag)
	}
}
