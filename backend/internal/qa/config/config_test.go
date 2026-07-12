package config_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	qaconfig "github.com/PengYuee/SCYG.Blog/backend/internal/qa/config"
)

func Test_Load_reads_typed_QA_configuration(t *testing.T) {
	// Given
	path := filepath.Join(t.TempDir(), "config.yaml")
	content := "database:\n  dsn: postgres://postgres:app-secret@localhost:5432/blog\nqa:\n  postgres_admin_dsn: postgres://postgres:secret@localhost:5432/postgres?sslmode=disable\n  database_prefix: scyg_test_\n  command_timeout: 25s\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	// When
	cfg, err := qaconfig.Load(path)

	// Then
	if err != nil || cfg.DatabasePrefix() != "scyg_test_" || cfg.CommandTimeout() != 25*time.Second {
		t.Fatalf("cfg=%v err=%v", cfg, err)
	}
}

func Test_Config_redacts_admin_DSN_from_formatting(t *testing.T) {
	// Given
	path := filepath.Join(t.TempDir(), "config.yaml")
	secret := "qa-secret-sentinel"
	content := "database:\n  dsn: postgres://postgres:app-secret@localhost:5432/blog\nqa:\n  postgres_admin_dsn: postgres://postgres:" + secret + "@localhost:5432/postgres\n  database_prefix: scyg_test_\n  command_timeout: 25s\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := qaconfig.Load(path)
	if err != nil {
		t.Fatal(err)
	}

	// When
	rendered := fmt.Sprintf("%v %+v %#v %s", cfg, cfg, cfg, cfg.AdminDSN())

	// Then
	if strings.Contains(rendered, secret) || !strings.Contains(rendered, "[REDACTED]") {
		t.Fatalf("QA 配置格式化泄露敏感值")
	}
}
