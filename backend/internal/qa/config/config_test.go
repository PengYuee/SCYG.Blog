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

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.qa.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func Test_Load_reads_strict_QA_configuration(t *testing.T) {
	cfg, err := qaconfig.Load(writeConfig(t, "qa:\n  postgres_admin_dsn: postgres://postgres:secret@localhost:5432/postgres?sslmode=disable\n  database_prefix: scyg_test_\n  command_timeout: 25s\n"))
	if err != nil || cfg.DatabasePrefix() != "scyg_test_" || cfg.CommandTimeout() != 25*time.Second {
		t.Fatalf("cfg=%v err=%v", cfg, err)
	}
}

func Test_Load_rejects_runtime_or_unknown_fields(t *testing.T) {
	for _, content := range []string{
		"database:\n  dsn: postgres://postgres:secret@localhost:5432/blog\nqa:\n  postgres_admin_dsn: postgres://postgres:secret@localhost:5432/postgres\n  database_prefix: scyg_test_\n  command_timeout: 25s\n",
		"qa:\n  postgres_admin_dsn: postgres://postgres:secret@localhost:5432/postgres\n  database_prefix: scyg_test_\n  command_timeout: 25s\n  unexpected: yes\n",
		"qa:\n  postgres_admin_dsn: postgres://postgres:secret@localhost:5432/postgres\n  database_prefix: scyg_test_\n  command_timeout: 25s\n---\nqa: {}\n",
	} {
		if _, err := qaconfig.Load(writeConfig(t, content)); err == nil {
			t.Fatal("expected strict QA schema error")
		}
	}
}

func Test_Load_rejects_invalid_values(t *testing.T) {
	cases := []string{
		"qa:\n  postgres_admin_dsn: \"\"\n  database_prefix: scyg_test_\n  command_timeout: 25s\n",
		"qa:\n  postgres_admin_dsn: postgres://postgres:secret@localhost:5432/postgres\n  database_prefix: SCYG_\n  command_timeout: 25s\n",
		"qa:\n  postgres_admin_dsn: postgres://postgres:secret@localhost:5432/postgres\n  database_prefix: scyg_test_\n  command_timeout: 0s\n",
	}
	for _, content := range cases {
		if _, err := qaconfig.Load(writeConfig(t, content)); err == nil {
			t.Fatal("expected invalid QA value error")
		}
	}
}

func Test_Config_redacts_admin_DSN_from_formatting(t *testing.T) {
	secret := "qa-secret-sentinel"
	cfg, err := qaconfig.Load(writeConfig(t, "qa:\n  postgres_admin_dsn: postgres://postgres:"+secret+"@localhost:5432/postgres\n  database_prefix: scyg_test_\n  command_timeout: 25s\n"))
	if err != nil {
		t.Fatal(err)
	}
	rendered := fmt.Sprintf("%v %+v %#v %v", cfg, cfg, cfg.AdminDSN(), cfg.AdminDSN())
	if strings.Contains(rendered, secret) || !strings.Contains(rendered, "[REDACTED]") {
		t.Fatal("QA 配置格式化泄露敏感值")
	}
}
