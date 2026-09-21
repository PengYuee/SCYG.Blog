package main

import (
	"strings"
	"testing"
)

func TestRandomBase36HasFixedSafeShape(t *testing.T) {
	value, err := randomBase36(runIDLength)
	if err != nil {
		t.Fatal(err)
	}
	if len(value) != runIDLength || strings.IndexFunc(value, func(r rune) bool { return !strings.ContainsRune(base36Alphabet, r) }) >= 0 {
		t.Fatalf("invalid run id %q", value)
	}
}

func TestFixedEnvironmentReplacesQAControls(t *testing.T) {
	got := fixedEnvironment([]string{"PATH=x", "SCYG_DATABASE_DSN=secret", "scyg_KEEP=y", "QA_CONFIG=caller.yaml", "QA_CAPABILITY_TOKEN=caller-token", "PACKAGES=./caller"}, map[string]string{"QA_CONFIG": "run.yaml", "QA_CAPABILITY_TOKEN": "run-token", "PACKAGES": ""})
	joined := strings.Join(got, "\n")
	if strings.Contains(strings.ToUpper(joined), "SCYG_") || strings.Contains(joined, "QA_CONFIG=caller.yaml") || strings.Contains(joined, "QA_CAPABILITY_TOKEN=caller-token") || strings.Contains(joined, "PACKAGES=./caller") || !strings.Contains(joined, "QA_CONFIG=run.yaml") || !strings.Contains(joined, "QA_CAPABILITY_TOKEN=run-token") || !strings.Contains(joined, "PACKAGES=") {
		t.Fatalf("environment=%q", joined)
	}
}

func TestRedactRemovesCapabilityToken(t *testing.T) {
	got := redact("child failed QA_CAPABILITY_TOKEN=0123456789abcdefghijklmnopqrstuv")
	if strings.Contains(got, "0123456789abcdefghijklmnopqrstuv") || !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("redaction=%q", got)
	}
}

func TestRedactRemovesPostgresURL(t *testing.T) {
	got := redact("failed postgres://user:secret@host/postgres")
	if strings.Contains(got, "secret") || !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("redaction=%q", got)
	}
}

func TestDSNForDatabasePreservesEndpoint(t *testing.T) {
	got := dsnForDatabase("postgres://user:secret@host:5432/postgres?sslmode=disable", `qa_quoted"`)
	if !strings.Contains(got, "host:5432") || !strings.Contains(got, "qa_quoted%22") {
		t.Fatalf("derived DSN unexpectedly changed: %q", redact(got))
	}
}
