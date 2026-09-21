// Command qa-database runs the fixed, isolated QA gate.
package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"math/big"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	qaconfig "github.com/PengYuee/SCYG.Blog/backend/internal/qa/config"
	qadatabase "github.com/PengYuee/SCYG.Blog/backend/internal/qa/database"
)

const (
	runIDLength    = 25
	base36Alphabet = "0123456789abcdefghijklmnopqrstuvwxyz"
)

var secretPattern = regexp.MustCompile(`(?i)(postgres(?:ql)?://[^\s"']+|password\s*[:=]\s*[^\s,;]+|dsn\s*[:=]\s*[^\s,;]+|qa_capability_token\s*[:=]\s*[^\s,;]+)`)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, redact(err.Error()))
		os.Exit(1)
	}
}

func run(args []string) (runErr error) {
	fs := flag.NewFlagSet("qa-database", flag.ContinueOnError)
	configPath := fs.String("config", "", "explicit QA configuration path")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *configPath == "" || fs.NArg() != 0 {
		return errors.New("必须提供 -config <QA_CONFIG>，且不接受额外参数")
	}
	cfg, err := qaconfig.Load(*configPath)
	if err != nil {
		return err
	}
	runID, err := randomBase36(runIDLength)
	if err != nil {
		return err
	}
	capabilityToken, err := randomBase36(qadatabase.CapabilityTokenLength())
	if err != nil {
		return err
	}
	prefix := cfg.DatabasePrefix() + runID
	root, err := resolveModuleRoot()
	if err != nil {
		return err
	}
	runConfig, err := materializeQAConfig(*configPath, root, prefix)
	if err != nil {
		return err
	}
	proofPath := runConfig + qadatabase.CapabilityProofSuffix()
	if err = qadatabase.IssueCapabilityProof(runConfig, prefix, capabilityToken); err != nil {
		_ = os.Remove(runConfig)
		return err
	}
	defer func() { _ = os.Remove(runConfig) }()
	defer func() { _ = os.Remove(proofPath) }()
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), cfg.CommandTimeout())
		defer cancel()
		runErr = errors.Join(runErr, qadatabase.CleanupPrefixWithCapability(cleanupCtx, runConfig, prefix, cfg.DatabasePrefix(), capabilityToken))
	}()
	creationCtx, creationCancel := context.WithTimeout(context.Background(), cfg.CommandTimeout())
	migrationName := runID + "migration"
	migration, err := qadatabase.NewExactWithCapability(creationCtx, runConfig, migrationName, capabilityToken)
	creationCancel()
	if err != nil {
		return err
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), cfg.CommandTimeout())
		defer cancel()
		runErr = errors.Join(runErr, migration.Close(cleanupCtx))
	}()
	migrationConfig, err := materializeMigrationConfig(root, migration.DSN(), cfg.AdminDSN().Value())
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(migrationConfig) }()
	quotedName := runID + `migrate_quoted"`
	quotedCtx, quotedCancel := context.WithTimeout(context.Background(), cfg.CommandTimeout())
	quotedMigration, err := qadatabase.RegisterExactWithCapability(quotedCtx, runConfig, quotedName, capabilityToken)
	quotedCancel()
	if err != nil {
		return err
	}
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), cfg.CommandTimeout())
		defer cancel()
		runErr = errors.Join(runErr, quotedMigration.Close(cleanupCtx))
	}()
	quotedConfig, err := materializeMigrationConfig(root, quotedMigration.DSN(), cfg.AdminDSN().Value())
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(quotedConfig) }()

	migrationEnvironment := fixedEnvironment(os.Environ(), nil)
	for _, action := range []string{"up", "down", "up", "version"} {
		ctx, cancel := context.WithTimeout(context.Background(), cfg.CommandTimeout())
		err = runChild(ctx, root, migrationEnvironment, "migration "+action, "go", "run", "./cmd/migrate", "-config", migrationConfig, action)
		cancel()
		if err != nil {
			return err
		}
	}
	integrationEnvironment := fixedEnvironment(os.Environ(), map[string]string{
		"QA_CONFIG":                 runConfig,
		"QA_CAPABILITY_TOKEN":       capabilityToken,
		"PACKAGES":                  "",
		"MIGRATION_QUOTED_DATABASE": quotedName,
		"MIGRATION_QUOTED_CONFIG":   quotedConfig,
	})
	e2eEnvironment := fixedEnvironment(os.Environ(), map[string]string{
		"QA_CONFIG":           runConfig,
		"QA_CAPABILITY_TOKEN": capabilityToken,
		"PACKAGES":            "",
	})
	ctx, cancel := context.WithTimeout(context.Background(), cfg.CommandTimeout())
	err = runChild(ctx, root, integrationEnvironment, "integration", "task", "integration")
	cancel()
	if err != nil {
		return err
	}
	ctx, cancel = context.WithTimeout(context.Background(), cfg.CommandTimeout())
	runErr = runChild(ctx, root, e2eEnvironment, "e2e", "task", "e2e")
	cancel()
	return runErr
}

func requireModuleRoot(root string) error {
	if strings.TrimSpace(root) == "" {
		return errors.New("QA module root 不能为空")
	}
	//nolint:gosec // root is resolved and validated as the backend module root before this read.
	moduleFile, err := os.ReadFile(filepath.Join(root, "go.mod"))
	if err != nil {
		return fmt.Errorf("QA 必须从 backend module root 启动：读取 go.mod 失败：%w", err)
	}
	if !hasModuleDirective(string(moduleFile), "github.com/PengYuee/SCYG.Blog/backend") {
		return errors.New("QA module root 的 go.mod module 不匹配")
	}
	for _, name := range []string{"Taskfile.yml", filepath.Join("cmd", "migrate", "main.go"), filepath.Join("cmd", "qa-database", "main.go")} {
		info, statErr := os.Stat(filepath.Join(root, name))
		if statErr != nil {
			return fmt.Errorf("QA 必须从 backend module root 启动：缺少 %s", name)
		}
		if info.IsDir() {
			return fmt.Errorf("QA module root 的 %s 不能是目录", name)
		}
	}
	return nil
}

func hasModuleDirective(source, expected string) bool {
	for _, line := range strings.Split(strings.ReplaceAll(source, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "module ") && strings.TrimSpace(strings.TrimPrefix(line, "module ")) == expected {
			return true
		}
	}
	return false
}

func resolveModuleRoot() (string, error) {
	_, source, _, ok := runtime.Caller(0)
	if !ok || strings.TrimSpace(source) == "" {
		return "", errors.New("无法定位 QA module root")
	}
	root, err := filepath.Abs(filepath.Join(filepath.Dir(source), "..", ".."))
	if err != nil {
		return "", fmt.Errorf("解析 QA module root 失败：%w", err)
	}
	if err = requireModuleRoot(root); err != nil {
		return "", err
	}
	return root, nil
}

func runChild(ctx context.Context, dir string, env []string, name, command string, args ...string) error {
	//nolint:gosec // command and arguments are selected by the bounded QA runner.
	cmd := exec.Command(command, args...)
	cmd.Dir = dir
	cmd.Env = env
	if err := configureChild(cmd); err != nil {
		return fmt.Errorf("%s configure failed: %w", name, err)
	}
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%s start failed: %w", name, err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if clean := redact(output.String()); clean != "" {
			fmt.Printf("[%s] %s", name, clean)
		}
		if err != nil {
			return fmt.Errorf("%s failed: %s", name, redact(err.Error()))
		}
		return nil
	case <-ctx.Done():
		terminationContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
		terminateErr := terminateChild(terminationContext, cmd)
		cancel()
		<-done
		if clean := redact(output.String()); clean != "" {
			fmt.Printf("[%s] %s", name, clean)
		}
		if terminateErr != nil {
			return fmt.Errorf("%s timed out: %w (terminate: %w)", name, ctx.Err(), terminateErr)
		}
		return fmt.Errorf("%s timed out: %w", name, ctx.Err())
	}
}

func fixedEnvironment(parent []string, extra map[string]string) []string {
	out := make([]string, 0, len(parent)+len(extra))
	for _, value := range parent {
		key, _, _ := strings.Cut(value, "=")
		key = strings.ToUpper(key)
		if !strings.HasPrefix(key, "SCYG_") && key != "QA_CONFIG" && key != "QA_CAPABILITY_TOKEN" && key != "PACKAGES" && key != "MIGRATION_QUOTED_DATABASE" && key != "MIGRATION_QUOTED_CONFIG" {
			out = append(out, value)
		}
	}
	for key, value := range extra {
		out = append(out, key+"="+value)
	}
	return out
}

func redact(value string) string { return secretPattern.ReplaceAllString(value, "[REDACTED]") }

func randomBase36(length int) (string, error) {
	out := make([]byte, length)
	for i := range out {
		n, err := rand.Int(rand.Reader, big.NewInt(36))
		if err != nil {
			return "", err
		}
		out[i] = base36Alphabet[n.Int64()]
	}
	return string(out), nil
}

func materializeQAConfig(source, dir, prefix string) (string, error) {
	config, err := qaconfig.Load(source)
	if err != nil {
		return "", err
	}
	content, err := yaml.Marshal(map[string]any{"qa": map[string]any{"postgres_admin_dsn": config.AdminDSN().Value(), "database_prefix": prefix, "command_timeout": config.CommandTimeout().String()}})
	if err != nil {
		return "", err
	}
	return writePrivateConfig(dir, "qa-run-*.yaml", content)
}

func materializeMigrationConfig(dir, dsn, adminDSN string) (string, error) {
	content, err := yaml.Marshal(map[string]any{"database": map[string]string{"dsn": dsn}, "qa": map[string]string{"postgres_admin_dsn": adminDSN}})
	if err != nil {
		return "", err
	}
	return writePrivateConfig(dir, "migration-*.yaml", content)
}

func writePrivateConfig(dir, pattern string, content []byte) (string, error) {
	file, err := os.CreateTemp(dir, pattern)
	if err != nil {
		return "", err
	}
	path := file.Name()
	if err = file.Chmod(0o600); err == nil {
		_, err = file.Write(content)
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

func dsnForDatabase(adminDSN, name string) string {
	parsed, err := url.Parse(adminDSN)
	if err != nil {
		return ""
	}
	parsed.Path = "/" + name
	return parsed.String()
}
