package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestListTempCandidatesBoundsAndOrdersResults(t *testing.T) {
	rootPath := t.TempDir()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	names := make([]string, 3)
	for index := range names {
		names[index] = fmt.Sprintf(".article-image-%032x-%024x.tmp", index+1, index+1)
		path := filepath.Join(rootPath, names[index])
		if err := os.WriteFile(path, []byte("temporary"), 0o600); err != nil {
			t.Fatal(err)
		}
		modified := base.Add(time.Duration(index) * time.Minute)
		if err := os.Chtimes(path, modified, modified); err != nil {
			t.Fatal(err)
		}
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	got, truncated, err := listTempCandidates(context.Background(), root, base.Add(time.Hour), 2)
	if err != nil {
		t.Fatal(err)
	}
	if !truncated || len(got) != 2 || got[0].Name != names[0] || got[1].Name != names[1] {
		t.Fatalf("temporary candidates = %#v, truncated = %v", got, truncated)
	}
}

func TestListTempCandidatesMarksExactLimitAsTruncated(t *testing.T) {
	rootPath := t.TempDir()
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for index := range 2 {
		name := fmt.Sprintf(".article-image-%032x-%024x.tmp", index+1, index+1)
		path := filepath.Join(rootPath, name)
		if err := os.WriteFile(path, []byte("temporary"), 0o600); err != nil {
			t.Fatal(err)
		}
		modified := base.Add(time.Duration(index) * time.Minute)
		if err := os.Chtimes(path, modified, modified); err != nil {
			t.Fatal(err)
		}
	}
	root, err := os.OpenRoot(rootPath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = root.Close() }()
	got, truncated, err := listTempCandidates(context.Background(), root, base.Add(time.Hour), 2)
	if err != nil {
		t.Fatal(err)
	}
	if !truncated || len(got) != 2 {
		t.Fatalf("temporary candidates = %#v, truncated = %v", got, truncated)
	}
}

func TestIsReconciliationTempNameMatchesBlobTokenFormat(t *testing.T) {
	valid := ".article-image-0123456789abcdef0123456789abcdef-0123456789abcdef01234567.tmp"
	for _, name := range []string{valid} {
		if !isReconciliationTempName(name) {
			t.Fatalf("valid temporary name rejected: %q", name)
		}
	}
	for _, name := range []string{
		".article-image-not-a-token.tmp",
		".article-image-0123456789abcdef0123456789abcdef-0123456789abcdef0123456.tmp",
		".article-image-0123456789ABCDEF0123456789abcdef-0123456789abcdef01234567.tmp",
	} {
		if isReconciliationTempName(name) {
			t.Fatalf("malformed temporary name accepted: %q", name)
		}
	}
}

func TestSameSetComparesMembers(t *testing.T) {
	if !sameSet(map[string]struct{}{"a": {}}, map[string]struct{}{"a": {}}) {
		t.Fatal("equal sets rejected")
	}
	if sameSet(map[string]struct{}{"a": {}}, map[string]struct{}{"b": {}}) {
		t.Fatal("different sets accepted")
	}
}

func TestValidImageTimestampsMatchesLifecycleInvariants(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	expires := created.Add(24 * time.Hour)
	committed := created.Add(time.Hour)
	orphaned := expires.Add(time.Hour)
	if !validImageTimestamps(imageRow{Status: "pending", CreatedAt: created, ExpiresAt: expires}) {
		t.Fatal("valid pending timestamps rejected")
	}
	if !validImageTimestamps(imageRow{Status: "committed", CreatedAt: created, CommittedAt: &committed, ExpiresAt: expires}) {
		t.Fatal("valid committed timestamps rejected")
	}
	if validImageTimestamps(imageRow{Status: "committed", CreatedAt: created, CommittedAt: &expires, ExpiresAt: expires}) {
		t.Fatal("committed timestamp at expiry accepted")
	}
	if !validImageTimestamps(imageRow{Status: "orphaned", CreatedAt: created, CommittedAt: &committed, OrphanedAt: &orphaned, ExpiresAt: orphaned}) {
		t.Fatal("valid orphaned timestamps rejected")
	}
	badOrphaned := expires.Add(time.Hour)
	if validImageTimestamps(imageRow{Status: "orphaned", CreatedAt: created, CommittedAt: &committed, OrphanedAt: &badOrphaned, ExpiresAt: expires}) {
		t.Fatal("orphaned timestamp after expiry accepted")
	}
}
