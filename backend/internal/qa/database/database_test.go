package database

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateNameRejectsOutOfCapabilityOrServerLimit(t *testing.T) {
	for _, test := range []struct {
		name    string
		prefix  string
		limit   int
		wantErr bool
	}{
		{name: "scyg_run_fixture", prefix: "scyg_run_", limit: 63},
		{name: "other_fixture", prefix: "scyg_run_", limit: 63, wantErr: true},
		{name: "scyg_run_too_long", prefix: "scyg_run_", limit: 5, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validateName(test.name, test.prefix, test.limit)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateName() error = %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}

func TestValidateRunPrefixRequiresFullBase36Capability(t *testing.T) {
	base := "scyg_run_"
	valid := base + "0123456789abcdefghijklmno"
	for name, prefix := range map[string]string{
		"valid":   valid,
		"base":    base,
		"short":   base + "abc",
		"invalid": base + "0123456789abcdefghijklmnop!",
	} {
		t.Run(name, func(t *testing.T) {
			err := validateRunPrefix(prefix, base)
			if (name == "valid") != (err == nil) {
				t.Fatalf("validateRunPrefix(%q) error=%v", prefix, err)
			}
		})
	}
}

func TestValidateRunCapabilityRejectsInvalidRunID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "qa.yaml")
	prefix := "scyg_run_0123456789abcdefghijklmno"
	badPrefix := "scyg_run_0123456789abcdefghijklmn!"
	token := strings.Repeat("a", capabilityTokenLength)
	content := "qa:\n  postgres_admin_dsn: postgres://postgres:secret@127.0.0.1:5432/postgres\n  database_prefix: " + prefix + "\n  command_timeout: 2m\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := IssueCapabilityProof(path, prefix, token); err != nil {
		t.Fatal(err)
	}
	if err := validateRunCapability(path, badPrefix, token); err == nil {
		t.Fatal("invalid run ID accepted")
	}
}

func TestValidateGeneratedNameRejectsMalformedFixtureNames(t *testing.T) {
	base := "scyg_run_0123456789abcdefghijklmno"
	valid := base + "migration_0123456789abc"
	for name, value := range map[string]string{
		"valid":        valid,
		"bad-marker":   base + "migration-0123456789abc",
		"bad-suffix":   base + "migration_0123456789ab!",
		"short-suffix": base + "migration_abc",
	} {
		t.Run(name, func(t *testing.T) {
			err := validateGeneratedName(value, base, 63)
			if (name == "valid") != (err == nil) {
				t.Fatalf("validateGeneratedName(%q) error=%v", value, err)
			}
		})
	}
}

func TestValidateExactNameRejectsRegisteredRunAndInvalidNames(t *testing.T) {
	base := "scyg_run_0123456789abcdefghijklmno"
	for name, value := range map[string]string{
		"empty":    "",
		"run":      base + "migration",
		"too-long": strings.Repeat("a", 64),
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateExactName(value, base, 63); err == nil {
				t.Fatalf("validateExactName(%q) accepted", value)
			}
		})
	}
}

func TestVerifyDSNEndpointRejectsDifferentServer(t *testing.T) {
	if err := verifyDSNEndpoint("postgres://u:p@host-a:5432/postgres", "postgres://u:p@host-b:5432/db"); err == nil {
		t.Fatal("verifyDSNEndpoint accepted a different host")
	}
}

func TestCapabilityProofBindsConfigAndToken(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "qa.yaml")
	prefix := "scyg_run_0123456789abcdefghijklmno"
	content := "qa:\n  postgres_admin_dsn: postgres://postgres:secret@127.0.0.1:5432/postgres\n  database_prefix: " + prefix + "\n  command_timeout: 2m\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("a", capabilityTokenLength)
	if err := IssueCapabilityProof(path, prefix, token); err != nil {
		t.Fatal(err)
	}
	if err := validateRunCapability(path, prefix, token); err != nil {
		t.Fatalf("valid proof rejected: %v", err)
	}
	if err := validateRunCapability(path, prefix, strings.Repeat("b", capabilityTokenLength)); err == nil {
		t.Fatal("wrong token accepted")
	}
	if err := os.WriteFile(path, append([]byte(content), '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateRunCapability(path, prefix, token); err == nil {
		t.Fatal("modified config accepted")
	}
}

func TestNewDirectRunPrefixUsesConfiguredBaseAndFullRunID(t *testing.T) {
	path := filepath.Join(t.TempDir(), "qa.yaml")
	//nolint:gosec // synthetic credentials exercise QA config parsing only.
	content := "qa:\n  postgres_admin_dsn: postgres://postgres:secret@127.0.0.1:5432/postgres\n  database_prefix: scyg_qa_\n  command_timeout: 2m\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	prefix, err := newDirectRunPrefix(path, runCapabilityLength)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(prefix, "scyg_qa_") || len(prefix) != len("scyg_qa_")+runCapabilityLength {
		t.Fatalf("direct run prefix=%q", prefix)
	}
	if err := validateRunPrefix(prefix, "scyg_qa_"); err != nil {
		t.Fatalf("direct run prefix rejected: %v", err)
	}
}
