package config_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/config"
)

const developmentAuthorSentinel = "0123456789abcdef0123456789abcdef"

func Test_ArticleImages_defaults_are_immutable_and_safe(t *testing.T) {
	// When
	cfg, err := config.Load(config.Options{})
	// Then
	if err != nil {
		t.Fatal(err)
	}
	images := cfg.ArticleImages()
	if images.Directory() != "data/article-images" || images.PendingTTL() != 24*time.Hour || images.OrphanGrace() != 24*time.Hour || images.CleanupInterval() != time.Hour || images.UploadRequestBytes() != 6<<20 || images.MaxFileBytes() != 5<<20 || images.MaxPixels() != 25_000_000 || images.MaxDimension() != 8192 || images.DevelopmentAuthorID() != "" {
		t.Fatalf("unexpected article image defaults: %+v", images)
	}
	if reflect.TypeOf(images).NumField() == 0 {
		t.Fatal("article images config has no fields")
	}
	for index := range reflect.TypeOf(images).NumField() {
		if reflect.TypeOf(images).Field(index).IsExported() {
			t.Fatalf("exported field: %s", reflect.TypeOf(images).Field(index).Name)
		}
	}
}

func Test_ArticleImages_file_loads_exact_values_without_leaking_sensitive_fields(t *testing.T) {
	// Given
	path := filepath.Join(t.TempDir(), "config.yaml")
	yaml := "article_images:\n  directory: ./private/images\n  pending_ttl: 25h\n  orphan_grace: 26h\n  cleanup_interval: 30m\n  upload_request_bytes: 7000000\n  max_file_bytes: 5000000\n  max_pixels: 24000000\n  max_dimension: 8000\n  development_author_id: " + developmentAuthorSentinel + "\n"
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	// When
	cfg, err := config.Load(config.Options{File: path, DisableEnvironment: true})
	// Then
	if err != nil {
		t.Fatal(err)
	}
	images := cfg.ArticleImages()
	if images.Directory() != "private/images" || images.PendingTTL() != 25*time.Hour || images.CleanupInterval() != 30*time.Minute || images.UploadRequestBytes() != 7_000_000 || images.DevelopmentAuthorID() != developmentAuthorSentinel {
		t.Fatalf("unexpected values")
	}
	encoded, marshalErr := json.Marshal(cfg)
	if marshalErr != nil {
		t.Fatal(marshalErr)
	}
	for _, output := range []string{cfg.String(), fmt.Sprintf("%+v", cfg), string(encoded)} {
		if strings.Contains(output, "private/images") || strings.Contains(output, developmentAuthorSentinel) {
			t.Fatalf("sensitive config leaked: %s", output)
		}
	}
}

func Test_ArticleImages_rejects_invalid_security_values(t *testing.T) {
	tests := []struct{ name, yaml, field string }{
		{"empty directory", "article_images:\n  directory: '   '\n", "article_images.directory"},
		{"traversal directory", "article_images:\n  directory: ../outside\n", "article_images.directory"},
		{"cleanup not below pending", "article_images:\n  cleanup_interval: 24h\n", "article_images.cleanup_interval"},
		{"cleanup not below orphan", "article_images:\n  orphan_grace: 30m\n  cleanup_interval: 1h\n", "article_images.cleanup_interval"},
		{"upload below file", "article_images:\n  upload_request_bytes: 100\n", "article_images.upload_request_bytes"},
		{"zero file", "article_images:\n  max_file_bytes: 0\n", "article_images.max_file_bytes"},
		{"zero pixels", "article_images:\n  max_pixels: 0\n", "article_images.max_pixels"},
		{"zero dimension", "article_images:\n  max_dimension: 0\n", "article_images.max_dimension"},
		{"invalid author", "article_images:\n  development_author_id: invalid\n", "article_images.development_author_id"},
		{"production author", "app:\n  env: production\narticle_images:\n  development_author_id: " + developmentAuthorSentinel + "\n", "article_images.development_author_id"},
		{"test author", "app:\n  env: test\narticle_images:\n  development_author_id: " + developmentAuthorSentinel + "\n", "article_images.development_author_id"},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(testCase.yaml), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := config.Load(config.Options{File: path, DisableEnvironment: true})
			var validationErr *config.ValidationError
			if !errors.As(err, &validationErr) || validationErr.Field != testCase.field {
				t.Fatalf("err=%v", err)
			}
		})
	}
}

func Test_ArticleImages_unknown_key_fails_exact_decode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("article_images:\n  unknown: true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := config.Load(config.Options{File: path, DisableEnvironment: true})
	if err == nil || !strings.Contains(err.Error(), "解析配置失败") {
		t.Fatalf("err=%v", err)
	}
}
