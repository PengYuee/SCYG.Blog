package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type quiesceEvidence struct {
	SchemaVersion     int       `json:"schemaVersion"`
	EvidenceID        string    `json:"evidenceId"`
	Release           string    `json:"release"`
	Phase             string    `json:"phase"`
	Environment       string    `json:"environment"`
	PostgresInstance  string    `json:"postgresInstance"`
	BlobInstance      string    `json:"blobInstance"`
	DeploymentRunID   string    `json:"deploymentRunId"`
	Operator          string    `json:"operator"`
	ExpectedRoutes    []string  `json:"expectedRoutes"`
	RemovedRoutes     []string  `json:"removedRoutes"`
	ExpectedInstances []string  `json:"expectedInstances"`
	DrainedInstances  []string  `json:"drainedInstances"`
	ExpectedWorkers   []string  `json:"expectedCleanupWorkers"`
	StoppedWorkers    []string  `json:"stoppedCleanupWorkers"`
	WindowStartedAt   time.Time `json:"windowStartedAt"`
	DrainCompletedAt  time.Time `json:"drainCompletedAt"`
	WindowExpiresAt   time.Time `json:"windowExpiresAt"`
	GeneratedAt       time.Time `json:"generatedAt"`
}

var (
	safeSegment        = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	safeReleaseSegment = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9_.-]*[A-Za-z0-9])?$`)
)

func loadAndValidateEvidence(path string, options commandOptions, config reconcileConfig) ([]byte, quiesceEvidence, error) {
	//nolint:gosec // the command intentionally reads the explicit evidence path supplied by the operator.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, quiesceEvidence{}, fmt.Errorf("read quiesce evidence: %w", err)
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	var evidence quiesceEvidence
	if err := decoder.Decode(&evidence); err != nil {
		return nil, quiesceEvidence{}, fmt.Errorf("parse quiesce evidence: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, quiesceEvidence{}, errors.New("quiesce evidence must contain one JSON value")
		}
		return nil, quiesceEvidence{}, fmt.Errorf("parse quiesce evidence trailing data: %w", err)
	}
	if err := validateEvidence(evidence, options, config); err != nil {
		return nil, quiesceEvidence{}, err
	}
	return data, evidence, nil
}

func validateEvidence(evidence quiesceEvidence, options commandOptions, config reconcileConfig) error {
	if evidence.SchemaVersion < 1 || evidence.EvidenceID == "" || evidence.DeploymentRunID == "" || evidence.Operator == "" {
		return errors.New("quiesce evidence identity is incomplete")
	}
	if evidence.Release != options.release || evidence.Phase != options.phase || evidence.Environment != config.Reconcile.Environment || evidence.PostgresInstance != config.Reconcile.PostgresInstance || evidence.BlobInstance != config.Reconcile.BlobInstance {
		return errors.New("quiesce evidence does not match release, phase, environment, or instance")
	}
	if evidence.WindowStartedAt.IsZero() || evidence.DrainCompletedAt.IsZero() || evidence.WindowExpiresAt.IsZero() || evidence.GeneratedAt.IsZero() {
		return errors.New("quiesce evidence timestamps are incomplete")
	}
	now := time.Now().UTC()
	if evidence.WindowStartedAt.After(evidence.DrainCompletedAt) || evidence.DrainCompletedAt.After(evidence.WindowExpiresAt) || evidence.GeneratedAt.Before(evidence.DrainCompletedAt) || evidence.WindowStartedAt.After(now) || evidence.DrainCompletedAt.After(now) || evidence.GeneratedAt.After(now) || !now.Before(evidence.WindowExpiresAt) {
		return errors.New("quiesce evidence timestamps are invalid or expired")
	}
	if !sameMembersNonEmpty(evidence.ExpectedRoutes, evidence.RemovedRoutes) || !sameMembersNonEmpty(evidence.ExpectedInstances, evidence.DrainedInstances) || !sameMembersNonEmpty(evidence.ExpectedWorkers, evidence.StoppedWorkers) {
		return errors.New("quiesce evidence does not prove the requested quiet window")
	}
	return nil
}

func sameMembersNonEmpty(left, right []string) bool {
	if len(left) == 0 || len(right) == 0 {
		return false
	}
	seen := make(map[string]struct{}, len(left))
	for _, value := range left {
		if value == "" {
			return false
		}
		if _, exists := seen[value]; exists {
			return false
		}
		seen[value] = struct{}{}
	}
	return sameMembers(left, right) && len(seen) == len(right)
}

func sameMembers(left, right []string) bool {
	leftCopy, rightCopy := append([]string(nil), left...), append([]string(nil), right...)
	sort.Strings(leftCopy)
	sort.Strings(rightCopy)
	return len(leftCopy) == len(rightCopy) && strings.Join(leftCopy, "\x00") == strings.Join(rightCopy, "\x00")
}

func safeRelease(value string) bool { return safeReleaseSegment.MatchString(value) }

func evidenceHash(data []byte) string {
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

func redactValue(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:8])
}

func ensureNoSymlink(path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	volume := filepath.VolumeName(absolute)
	remaining := strings.TrimPrefix(absolute, volume)
	for remaining != "" {
		remaining = strings.TrimPrefix(remaining, string(filepath.Separator))
		if remaining == "" {
			break
		}
		part := remaining
		if index := strings.IndexRune(remaining, filepath.Separator); index >= 0 {
			part, remaining = remaining[:index], remaining[index+1:]
		} else {
			remaining = ""
		}
		volume += string(filepath.Separator) + part
		info, statErr := os.Lstat(volume)
		if statErr != nil {
			if os.IsNotExist(statErr) {
				continue
			}
			return statErr
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("path contains symlink: %s", volume)
		}
	}
	return nil
}

func prepareAttemptRoot(options commandOptions) (string, string, error) {
	if !safeRelease(options.release) || (options.phase != "before" && options.phase != "after") {
		return "", "", errors.New("release or phase is unsafe")
	}
	root, err := filepath.Abs(options.evidenceRoot)
	if err != nil {
		return "", "", err
	}
	if err = ensureNoSymlink(root); err != nil {
		return "", "", err
	}
	if err = os.MkdirAll(root, 0o700); err != nil {
		return "", "", fmt.Errorf("create evidence root: %w", err)
	}
	parent := filepath.Join(root, options.release, "reconciliation", options.phase)
	if err = ensureNoSymlink(filepath.Dir(parent)); err != nil {
		return "", "", err
	}
	if err = os.MkdirAll(parent, 0o700); err != nil {
		return "", "", fmt.Errorf("create evidence parent: %w", err)
	}
	if err = ensureNoSymlink(parent); err != nil {
		return "", "", err
	}
	attempt, err := randomAttemptID()
	if err != nil {
		return "", "", fmt.Errorf("generate attempt id: %w", err)
	}
	finalDir := filepath.Join(parent, attempt)
	if _, err = os.Lstat(finalDir); err == nil {
		return "", "", errors.New("generated attempt directory already exists")
	} else if !os.IsNotExist(err) {
		return "", "", err
	}
	tempID, err := randomAttemptID()
	if err != nil {
		return "", "", fmt.Errorf("generate temporary attempt id: %w", err)
	}
	tempDir := finalDir + ".tmp-" + tempID
	if err = os.Mkdir(tempDir, 0o700); err != nil {
		return "", "", fmt.Errorf("create attempt directory: %w", err)
	}
	return tempDir, finalDir, nil
}

func randomAttemptID() (string, error) {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}
