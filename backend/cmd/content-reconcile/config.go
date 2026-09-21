package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type reconcileConfig struct {
	Database struct {
		DSN string `yaml:"dsn"`
	} `yaml:"database"`
	ArticleImages struct {
		Directory  string        `yaml:"directory"`
		PendingTTL time.Duration `yaml:"pending_ttl"`
	} `yaml:"article_images"`
	Reconcile struct {
		Environment      string `yaml:"environment"`
		PostgresInstance string `yaml:"postgres_instance"`
		BlobInstance     string `yaml:"blob_instance"`
		TempScanLimit    int    `yaml:"temp_scan_limit"`
	} `yaml:"reconcile"`
}

func loadReconcileConfig(path string) (reconcileConfig, error) {
	//nolint:gosec // the command intentionally reads the explicit operator-provided config path.
	data, err := os.ReadFile(path)
	if err != nil {
		return reconcileConfig{}, fmt.Errorf("read reconciliation config: %w", err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var config reconcileConfig
	if err := decoder.Decode(&config); err != nil {
		return reconcileConfig{}, fmt.Errorf("parse reconciliation config: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return reconcileConfig{}, fmt.Errorf("parse reconciliation config: multiple YAML documents are forbidden")
		}
		return reconcileConfig{}, fmt.Errorf("parse reconciliation config trailing document: %w", err)
	}
	if err := validateReconcileConfig(config); err != nil {
		return reconcileConfig{}, err
	}
	return config, nil
}

func validateReconcileConfig(config reconcileConfig) error {
	parsed, err := url.Parse(config.Database.DSN)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || parsed.Host == "" || strings.Trim(parsed.Path, "/") == "" {
		return fmt.Errorf("database.dsn must be a PostgreSQL DSN with host and database")
	}
	if strings.TrimSpace(config.ArticleImages.Directory) == "" {
		return fmt.Errorf("article_images.directory must not be empty")
	}
	directory, err := filepath.Abs(filepath.Clean(config.ArticleImages.Directory))
	if err != nil || !filepath.IsAbs(directory) {
		return fmt.Errorf("article_images.directory must resolve to an absolute path")
	}
	config.ArticleImages.Directory = directory
	if config.ArticleImages.PendingTTL <= 0 {
		return fmt.Errorf("article_images.pending_ttl must be positive")
	}
	if config.Reconcile.Environment == "" || config.Reconcile.PostgresInstance == "" || config.Reconcile.BlobInstance == "" {
		return fmt.Errorf("reconcile environment and instance identifiers are required")
	}
	if config.Reconcile.TempScanLimit <= 0 {
		return fmt.Errorf("reconcile.temp_scan_limit must be positive")
	}
	return nil
}
