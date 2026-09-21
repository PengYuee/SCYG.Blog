package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func Test_MigrationConfig_loads_database_dsn_from_yaml(t *testing.T) {
	// Given
	path := filepath.Join(t.TempDir(), "config.yaml")
	//nolint:gosec // synthetic DSN used only to test migration config parsing.
	content := "database:\n  dsn: postgres://postgres:secret@localhost:5432/blog?sslmode=disable\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	// When
	dsn, command, err := loadMigrationConfig([]string{"-config", path, "version"})

	// Then
	if err != nil || command[0] != "version" || !strings.Contains(dsn, "localhost:5432/blog") {
		t.Fatalf("dsn=[REDACTED] command=%v err=%v", command, err)
	}
}

func Test_MigrationConfig_rejects_missing_database_dsn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte("app:\n  env: test\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadMigrationConfig([]string{"-config", path, "version"}); err == nil {
		t.Fatal("expected missing database.dsn to fail")
	}
}

func Test_MigrationConfig_rejects_legacy_dsn_flag(t *testing.T) {
	// Given / When
	_, _, err := loadMigrationConfig([]string{"--dsn", "postgres://secret", "version"})

	// Then
	if err == nil || !strings.Contains(err.Error(), "-config") {
		t.Fatalf("err=%v", err)
	}
}
