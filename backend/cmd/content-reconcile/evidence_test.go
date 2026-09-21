package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestReserveEvidenceIDIsExclusive(t *testing.T) {
	const workers = 16
	parent := t.TempDir()
	results := make(chan error, workers)
	var waitGroup sync.WaitGroup
	for attempt := range workers {
		waitGroup.Add(1)
		go func(attempt int) {
			defer waitGroup.Done()
			results <- reserveEvidenceID(parent, fmt.Sprintf("attempt-%d", attempt), "evidence")
		}(attempt)
	}
	waitGroup.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("evidence reservation successes = %d, want exactly one", successes)
	}
}

func TestWriteAttemptManifestIncludesCheckTotals(t *testing.T) {
	tempDir := t.TempDir()
	now := time.Now().UTC()
	report := reconciliationReport{ArticlesChecked: 2, ImagesChecked: 3, ReferencesChecked: 4, BlockingInconsistencies: []finding{}}
	evidence := quiesceEvidence{Environment: "env"}
	if err := writeAttempt(tempDir, []byte(`{"evidenceId":"evidence"}`), evidence, commandOptions{release: "release", phase: "before"}, report, now, now, nil); err != nil {
		t.Fatal(err)
	}
	//nolint:gosec // the manifest path is a fixed file beneath t.TempDir.
	data, err := os.ReadFile(filepath.Join(tempDir, "manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var manifestValue manifest
	if err := json.Unmarshal(data, &manifestValue); err != nil {
		t.Fatal(err)
	}
	if manifestValue.ArticlesChecked != 2 || manifestValue.ImagesChecked != 3 || manifestValue.ReferencesChecked != 4 {
		t.Fatalf("manifest totals = %#v, want 2/3/4", manifestValue)
	}
}

func TestValidateEvidenceRejectsUnsafeOrIncompleteQuietWindow(t *testing.T) {
	now := time.Now().UTC()
	config := reconcileConfig{}
	config.Reconcile.Environment = "env"
	config.Reconcile.PostgresInstance = "pg"
	config.Reconcile.BlobInstance = "blob"
	options := commandOptions{release: "release", phase: "before"}
	base := quiesceEvidence{SchemaVersion: 1, EvidenceID: "evidence", Release: "release", Phase: "before", Environment: "env", PostgresInstance: "pg", BlobInstance: "blob", DeploymentRunID: "run", Operator: "test", ExpectedRoutes: []string{"route"}, RemovedRoutes: []string{"route"}, ExpectedInstances: []string{"instance"}, DrainedInstances: []string{"instance"}, ExpectedWorkers: []string{"worker"}, StoppedWorkers: []string{"worker"}, WindowStartedAt: now.Add(-time.Minute), DrainCompletedAt: now.Add(-30 * time.Second), WindowExpiresAt: now.Add(time.Minute), GeneratedAt: now.Add(-10 * time.Second)}
	if err := validateEvidence(base, options, config); err != nil {
		t.Fatalf("valid evidence rejected: %v", err)
	}
	base.ExpectedRoutes = nil
	base.RemovedRoutes = nil
	if err := validateEvidence(base, options, config); err == nil {
		t.Fatal("empty quiet-window members accepted")
	}
	base = quiesceEvidence{SchemaVersion: 1, EvidenceID: "evidence", Release: "release", Phase: "before", Environment: "env", PostgresInstance: "pg", BlobInstance: "blob", DeploymentRunID: "run", Operator: "test", ExpectedRoutes: []string{"route"}, RemovedRoutes: []string{"route"}, ExpectedInstances: []string{"instance"}, DrainedInstances: []string{"instance"}, ExpectedWorkers: []string{"worker"}, StoppedWorkers: []string{"worker"}, WindowStartedAt: now.Add(time.Minute), DrainCompletedAt: now.Add(2 * time.Minute), WindowExpiresAt: now.Add(3 * time.Minute), GeneratedAt: now.Add(2 * time.Minute)}
	if err := validateEvidence(base, options, config); err == nil {
		t.Fatal("future quiet-window evidence accepted")
	}
	if safeRelease("..") || safeRelease("release/path") {
		t.Fatal("unsafe release segment accepted")
	}
	if !safeRelease("release.2026") || safeRelease(".") || safeRelease("..") || safeRelease("release/path") {
		t.Fatal("release segment validation mismatch")
	}
}

func TestEnsureEvidenceIDUnusedRejectsManifestReuse(t *testing.T) {
	root := t.TempDir()
	parent := root
	attempt := filepath.Join(parent, "attempt-one")
	if err := os.Mkdir(attempt, 0o700); err != nil {
		t.Fatal(err)
	}
	manifestBytes := []byte(`{"attemptId":"attempt-one","release":"release","phase":"before","evidenceId":"evidence"}`)
	if err := os.WriteFile(filepath.Join(attempt, "manifest.json"), manifestBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := ensureEvidenceIDUnused(parent, "attempt-two", "evidence"); err == nil {
		t.Fatal("reused evidence accepted")
	}
	if err := ensureEvidenceIDUnused(parent, "attempt-two", "new-evidence"); err != nil {
		t.Fatalf("distinct evidence rejected: %v", err)
	}
}
