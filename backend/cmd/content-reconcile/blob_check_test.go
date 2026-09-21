package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"testing"
)

func TestCheckBlobMatchesAndRejectsMetadata(t *testing.T) {
	rootPath := t.TempDir()
	key := "0123456789abcdef0123456789abcdef.jpg"
	payload := []byte("reconciliation payload")
	if err := os.WriteFile(rootPath+string(os.PathSeparator)+key, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	digest := sha256.Sum256(payload)
	row := imageRow{ID: "abcdef0123456789abcdef0123456789", StorageKey: key, ByteSize: int64(len(payload)), SHA256: hex.EncodeToString(digest[:])}
	report := reconciliationReport{BlockingInconsistencies: []finding{}}
	if err := checkBlob(context.Background(), &report, root, row); err != nil {
		t.Fatal(err)
	}
	if len(report.BlockingInconsistencies) != 0 {
		t.Fatalf("matching metadata produced findings: %#v", report.BlockingInconsistencies)
	}

	row.SHA256 = "0000000000000000000000000000000000000000000000000000000000000000"
	if err := checkBlob(context.Background(), &report, root, row); err != nil {
		t.Fatal(err)
	}
	if len(report.BlockingInconsistencies) != 1 || report.BlockingInconsistencies[0].Category != "blob" {
		t.Fatalf("metadata mismatch findings = %#v", report.BlockingInconsistencies)
	}
}
