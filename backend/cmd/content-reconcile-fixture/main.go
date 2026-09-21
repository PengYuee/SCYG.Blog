// Command content-reconcile-fixture runs the disposable PostgreSQL smoke for content-reconcile.
package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
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

	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	"gopkg.in/yaml.v3"

	qaconfig "github.com/PengYuee/SCYG.Blog/backend/internal/qa/config"
	qadatabase "github.com/PengYuee/SCYG.Blog/backend/internal/qa/database"
	"github.com/PengYuee/SCYG.Blog/backend/migrations"
)

const (
	runIDLength        = 25
	fixtureSuffixLen   = 13
	roleMarker         = "reconcile_ro"
	databaseMarker     = "reconcile_"
	base36Alphabet     = "0123456789abcdefghijklmnopqrstuvwxyz"
	fixtureEnvironment = "reconcile-fixture-non-production"
)

var safeName = regexp.MustCompile(`^[a-z0-9_]+$`)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, redact(err.Error()))
		os.Exit(1)
	}
}

func run(args []string) (runErr error) {
	flags := flag.NewFlagSet("content-reconcile-fixture", flag.ContinueOnError)
	configPath := flags.String("config", "", "explicit QA configuration path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if strings.TrimSpace(*configPath) == "" || flags.NArg() != 0 {
		return errors.New("必须提供 -config <QA_CONFIG>，且不接受额外参数")
	}
	config, err := qaconfig.Load(*configPath)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), config.CommandTimeout())
	defer cancel()

	runID, err := randomBase36(runIDLength)
	if err != nil {
		return err
	}
	suffix, err := randomBase36(fixtureSuffixLen)
	if err != nil {
		return err
	}
	token, err := randomBase36(qadatabase.CapabilityTokenLength())
	if err != nil {
		return err
	}
	prefix := config.DatabasePrefix() + runID
	roleName := prefix + roleMarker + suffix
	if !safeName.MatchString(roleName) {
		return errors.New("只读角色名包含不安全字符")
	}

	admin, maxIdentifierLength, err := qadatabase.OpenVerifiedAdmin(ctx, config.AdminDSN().Value())
	if err != nil {
		return err
	}
	defer func() { runErr = errors.Join(runErr, admin.Close()) }()
	if len(roleName) > maxIdentifierLength || len(prefix)+len(roleMarker)+fixtureSuffixLen > maxIdentifierLength {
		return fmt.Errorf("fixture 标识符超过 PostgreSQL max_identifier_length=%d", maxIdentifierLength)
	}

	root, err := os.MkdirTemp("", "scyg-content-reconcile-blob-")
	if err != nil {
		return fmt.Errorf("创建 fixture Blob 根：%w", err)
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return err
	}
	defer func() { runErr = errors.Join(runErr, os.RemoveAll(root)) }()

	runConfig, err := materializeRunConfig(*configPath, prefix)
	if err != nil {
		return err
	}
	proofPath := runConfig + qadatabase.CapabilityProofSuffix()
	defer func() { runErr = errors.Join(runErr, os.Remove(runConfig), os.Remove(proofPath)) }()
	if err = qadatabase.IssueCapabilityProof(runConfig, prefix, token); err != nil {
		return err
	}

	isolated, err := qadatabase.NewWithCapability(ctx, runConfig, databaseMarker, token)
	if err != nil {
		return err
	}
	defer func() { runErr = errors.Join(runErr, isolated.Close(context.Background())) }()
	if err = migrateAndPrepare(ctx, isolated.DSN(), root); err != nil {
		return err
	}

	password, err := randomBase36(32)
	if err != nil {
		return err
	}
	roleCreated := false
	defer func() {
		if roleCreated {
			runErr = errors.Join(runErr, disconnectRoleSessions(context.Background(), admin, isolated.Name(), roleName), dropRole(context.Background(), admin, config.AdminDSN().Value(), isolated.Name(), roleName), verifyRoleAbsent(context.Background(), admin, roleName))
		}
	}()
	if err = createReadOnlyRole(ctx, admin, roleName, password); err != nil {
		return err
	}
	roleCreated = true
	if err = grantReadOnlyAccess(ctx, config.AdminDSN().Value(), isolated.Name(), roleName); err != nil {
		return err
	}

	readOnlyDSN, err := roleDSN(config.AdminDSN().Value(), isolated.Name(), roleName, password)
	if err != nil {
		return err
	}
	readOnlyConfig, err := materializeReadOnlyConfig(readOnlyDSN, root)
	if err != nil {
		return err
	}
	defer func() { runErr = errors.Join(runErr, os.Remove(readOnlyConfig)) }()
	evidenceRoot, err := os.MkdirTemp("", "scyg-content-reconcile-evidence-")
	if err != nil {
		return err
	}
	defer func() { runErr = errors.Join(runErr, os.RemoveAll(evidenceRoot)) }()
	evidencePath, err := writeFixtureEvidence(evidenceRoot)
	if err != nil {
		return err
	}
	defer func() { runErr = errors.Join(runErr, os.Remove(evidencePath)) }()
	if err = runReadOnlyReconcile(ctx, readOnlyConfig, evidencePath, evidenceRoot); err != nil {
		return err
	}
	if err = verifyReadOnlyRole(ctx, readOnlyDSN); err != nil {
		return err
	}
	return nil
}

func randomBase36(length int) (string, error) {
	result := make([]byte, length)
	limit := big.NewInt(int64(len(base36Alphabet)))
	for index := range result {
		value, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", fmt.Errorf("生成 fixture 随机标识：%w", err)
		}
		result[index] = base36Alphabet[value.Int64()]
	}
	return string(result), nil
}

func materializeRunConfig(source, prefix string) (string, error) {
	config, err := qaconfig.Load(source)
	if err != nil {
		return "", err
	}
	content, err := yaml.Marshal(map[string]any{"qa": map[string]any{
		"postgres_admin_dsn": config.AdminDSN().Value(),
		"database_prefix":    prefix,
		"command_timeout":    config.CommandTimeout().String(),
	}})
	if err != nil {
		return "", err
	}
	return writePrivateConfig("scyg-content-reconcile-qa-*.yaml", content)
}

func materializeReadOnlyConfig(dsn, root string) (string, error) {
	content, err := yaml.Marshal(map[string]any{
		"database":       map[string]any{"dsn": dsn},
		"article_images": map[string]any{"directory": root, "pending_ttl": "24h"},
		"reconcile": map[string]any{
			"environment":       fixtureEnvironment,
			"postgres_instance": "fixture-postgres",
			"blob_instance":     "fixture-blob",
			"temp_scan_limit":   10,
		},
	})
	if err != nil {
		return "", err
	}
	return writePrivateConfig("scyg-content-reconcile-readonly-*.yaml", content)
}

func writePrivateConfig(pattern string, content []byte) (string, error) {
	file, err := os.CreateTemp("", pattern)
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

func migrateAndPrepare(ctx context.Context, dsn, root string) error {
	if err := os.MkdirAll(root, 0o700); err != nil {
		return err
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("打开 fixture 数据库：%w", err)
	}
	defer func() { _ = db.Close() }()
	if err = db.PingContext(ctx); err != nil {
		return fmt.Errorf("连接 fixture 数据库：%w", err)
	}
	runner, err := migrations.New(db, "")
	if err != nil {
		return fmt.Errorf("构造 fixture migration：%w", err)
	}
	if err = runner.Up(); err != nil {
		_ = runner.Close()
		return fmt.Errorf("执行 fixture migration：%w", err)
	}
	if err = runner.Close(); err != nil {
		return fmt.Errorf("关闭 fixture migration：%w", err)
	}
	if err = db.Close(); err != nil {
		return fmt.Errorf("关闭 migration 数据库句柄：%w", err)
	}
	db, err = sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("重新打开 fixture 数据库：%w", err)
	}
	if err = db.PingContext(ctx); err != nil {
		return fmt.Errorf("重新连接 fixture 数据库：%w", err)
	}

	payload := []byte("reconciliation fixture image")
	digest := sha256.Sum256(payload)
	storageKey := "0123456789abcdef0123456789abcdef.jpg"
	imageID := "abcdef0123456789abcdef0123456789"
	ownerID := "00112233445566778899aabbccddeeff"
	if err = os.WriteFile(filepath.Join(root, storageKey), payload, 0o600); err != nil {
		return fmt.Errorf("写入 fixture Blob：%w", err)
	}
	now := time.Now().UTC().Truncate(time.Microsecond)
	var articleTypeID int64
	if err = db.QueryRowContext(ctx, `INSERT INTO article_types (name) VALUES ($1) RETURNING id`, "Reconciliation fixture").Scan(&articleTypeID); err != nil {
		return fmt.Errorf("写入 fixture ArticleType：%w", err)
	}
	content := "![fixture](/media/article-images/" + storageKey + ")"
	if _, err = db.ExecContext(ctx, `INSERT INTO articles (article_type_id, title, slug, digest, content, status, version, created_at, is_deleted) VALUES ($1, $2, $3, $4, $5, 1, 1, $6, false)`, articleTypeID, "Reconciliation fixture", "reconciliation-fixture", "fixture", content, now); err != nil {
		return fmt.Errorf("写入 fixture Article：%w", err)
	}
	if _, err = db.ExecContext(ctx, `INSERT INTO article_images (id, owner_id, storage_key, media_type, byte_size, width, height, sha256, status, created_at, committed_at, expires_at) VALUES ($1, $2, $3, 'image/jpeg', $4, 1, 1, $5, 'committed', $6, $6, $7)`, imageID, ownerID, storageKey, len(payload), hex.EncodeToString(digest[:]), now, now.Add(24*time.Hour)); err != nil {
		return fmt.Errorf("写入 fixture image：%w", err)
	}
	if _, err = db.ExecContext(ctx, `INSERT INTO article_image_references (article_id, image_id, created_at) SELECT id, $1, $2 FROM articles WHERE slug = $3`, imageID, now, "reconciliation-fixture"); err != nil {
		return fmt.Errorf("写入 fixture image reference：%w", err)
	}
	return nil
}

func createReadOnlyRole(ctx context.Context, admin *sql.DB, role, password string) error {
	identifier := pgx.Identifier{role}.Sanitize()
	if _, err := admin.ExecContext(ctx, "CREATE ROLE "+identifier+" LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS NOINHERIT PASSWORD '"+password+"'"); err != nil {
		return fmt.Errorf("创建只读角色：%w", err)
	}
	if _, err := admin.ExecContext(ctx, "ALTER ROLE "+identifier+" SET default_transaction_read_only = 'on'"); err != nil {
		return fmt.Errorf("设置只读角色默认事务：%w", err)
	}
	return nil
}

func grantReadOnlyAccess(ctx context.Context, adminDSN, databaseName, role string) error {
	admin, err := sql.Open("pgx", adminDSN)
	if err != nil {
		return err
	}
	defer func() { _ = admin.Close() }()
	roleIdentifier := pgx.Identifier{role}.Sanitize()
	databaseIdentifier := pgx.Identifier{databaseName}.Sanitize()
	if _, err = admin.ExecContext(ctx, "REVOKE ALL ON DATABASE "+databaseIdentifier+" FROM PUBLIC"); err != nil {
		return fmt.Errorf("撤销 fixture 数据库 PUBLIC 权限：%w", err)
	}
	if _, err = admin.ExecContext(ctx, "GRANT CONNECT ON DATABASE "+databaseIdentifier+" TO "+roleIdentifier); err != nil {
		return fmt.Errorf("授予 fixture 数据库连接权限：%w", err)
	}
	targetDSN, err := databaseDSN(adminDSN, databaseName)
	if err != nil {
		return err
	}
	target, err := sql.Open("pgx", targetDSN)
	if err != nil {
		return err
	}
	defer func() { _ = target.Close() }()
	if err = target.PingContext(ctx); err != nil {
		return fmt.Errorf("连接 fixture 目标数据库：%w", err)
	}
	if _, err = target.ExecContext(ctx, `REVOKE ALL PRIVILEGES ON SCHEMA public FROM PUBLIC`); err != nil {
		return fmt.Errorf("撤销 fixture schema PUBLIC 权限：%w", err)
	}
	if _, err = target.ExecContext(ctx, `REVOKE ALL PRIVILEGES ON ALL TABLES IN SCHEMA public FROM PUBLIC`); err != nil {
		return fmt.Errorf("撤销 fixture 表 PUBLIC 权限：%w", err)
	}
	if _, err = target.ExecContext(ctx, `REVOKE ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public FROM PUBLIC`); err != nil {
		return fmt.Errorf("撤销 fixture sequence PUBLIC 权限：%w", err)
	}
	if _, err = target.ExecContext(ctx, `GRANT USAGE ON SCHEMA public TO `+roleIdentifier); err != nil {
		return fmt.Errorf("授予 fixture schema 权限：%w", err)
	}
	//nolint:gosec // roleIdentifier is produced by pgx.Identifier.Sanitize before SQL identifier interpolation.
	if _, err = target.ExecContext(ctx, `GRANT SELECT ON TABLE articles, article_images, article_image_references TO `+roleIdentifier); err != nil {
		return fmt.Errorf("授予 fixture 表查询权限：%w", err)
	}
	return nil
}

func verifyReadOnlyRole(ctx context.Context, dsn string) error {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	if err = db.PingContext(ctx); err != nil {
		return fmt.Errorf("连接 fixture 只读角色：%w", err)
	}
	if _, err = db.ExecContext(ctx, `SET default_transaction_read_only = off`); err != nil {
		return fmt.Errorf("关闭只读事务设置：%w", err)
	}
	if _, err = db.ExecContext(ctx, `INSERT INTO articles (article_type_id, title, slug, digest, content) VALUES (1, 'forbidden', 'forbidden', 'forbidden', 'forbidden')`); err == nil {
		return errors.New("只读角色意外获得 Article 写权限")
	}
	if _, err = db.ExecContext(ctx, `CREATE TABLE public.reconcile_fixture_forbidden (id integer)`); err == nil {
		return errors.New("只读角色意外获得 schema DDL 权限")
	}
	return nil
}

func disconnectRoleSessions(ctx context.Context, admin *sql.DB, database, role string) error {
	_, err := admin.ExecContext(ctx, `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname = $1 AND usename = $2 AND pid <> pg_backend_pid()`, database, role)
	if err != nil {
		return fmt.Errorf("断开只读角色连接：%w", err)
	}
	return nil
}

func dropRole(ctx context.Context, admin *sql.DB, adminDSN, database, role string) error {
	identifier := pgx.Identifier{role}.Sanitize()
	targetDSN, err := databaseDSN(adminDSN, database)
	if err != nil {
		return err
	}
	target, err := sql.Open("pgx", targetDSN)
	if err != nil {
		return fmt.Errorf("打开 fixture 数据库清理连接：%w", err)
	}
	defer func() { _ = target.Close() }()
	if err = target.PingContext(ctx); err != nil {
		return fmt.Errorf("连接 fixture 数据库清理连接：%w", err)
	}
	if _, err = target.ExecContext(ctx, "DROP OWNED BY "+identifier); err != nil {
		return fmt.Errorf("清理 fixture 数据库授权：%w", err)
	}
	if _, err = admin.ExecContext(ctx, "DROP OWNED BY "+identifier); err != nil {
		return fmt.Errorf("清理管理数据库授权：%w", err)
	}
	if _, err = admin.ExecContext(ctx, "DROP ROLE IF EXISTS "+identifier); err != nil {
		return fmt.Errorf("删除只读角色：%w", err)
	}
	return nil
}

func verifyRoleAbsent(ctx context.Context, admin *sql.DB, role string) error {
	var count int
	if err := admin.QueryRowContext(ctx, `SELECT count(*) FROM pg_roles WHERE rolname = $1`, role).Scan(&count); err != nil {
		return err
	}
	if count != 0 {
		return errors.New("fixture 只读角色清理后仍残留")
	}
	return nil
}

func roleDSN(adminDSN, databaseName, role, password string) (string, error) {
	parsed, err := url.Parse(adminDSN)
	if err != nil {
		return "", fmt.Errorf("解析 QA 管理连接：%w", err)
	}
	parsed.Path = "/" + databaseName
	parsed.User = url.UserPassword(role, password)
	return parsed.String(), nil
}

func databaseDSN(adminDSN, databaseName string) (string, error) {
	parsed, err := url.Parse(adminDSN)
	if err != nil {
		return "", fmt.Errorf("解析 QA 管理连接：%w", err)
	}
	parsed.Path = "/" + databaseName
	return parsed.String(), nil
}

func writeFixtureEvidence(root string) (string, error) {
	now := time.Now().UTC()
	evidence := map[string]any{
		"schemaVersion":          1,
		"evidenceId":             "fixture-" + now.Format("20060102150405.000000000"),
		"release":                "fixture-release",
		"phase":                  "before",
		"environment":            fixtureEnvironment,
		"postgresInstance":       "fixture-postgres",
		"blobInstance":           "fixture-blob",
		"deploymentRunId":        "fixture-run",
		"operator":               "content-reconcile-fixture",
		"expectedRoutes":         []string{"fixture-api"},
		"removedRoutes":          []string{"fixture-api"},
		"expectedInstances":      []string{"fixture-api-1"},
		"drainedInstances":       []string{"fixture-api-1"},
		"expectedCleanupWorkers": []string{"fixture-cleanup-1"},
		"stoppedCleanupWorkers":  []string{"fixture-cleanup-1"},
		"windowStartedAt":        now.Add(-time.Minute),
		"drainCompletedAt":       now.Add(-30 * time.Second),
		"windowExpiresAt":        now.Add(10 * time.Minute),
		"generatedAt":            now,
	}
	data, err := yamlOrJSON(evidence)
	if err != nil {
		return "", err
	}
	path := filepath.Join(root, "quiesce-evidence.json")
	if err = os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func yamlOrJSON(value any) ([]byte, error) {
	data, err := jsonMarshal(value)
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func jsonMarshal(value any) ([]byte, error) {
	// Kept behind a small local function so evidence construction remains data-only.
	return json.Marshal(value)
}

func runReadOnlyReconcile(ctx context.Context, configPath, evidencePath, evidenceRoot string) error {
	moduleRoot, err := resolveModuleRoot()
	if err != nil {
		return err
	}
	binary := filepath.Join(os.TempDir(), "content-reconcile-fixture-"+time.Now().UTC().Format("20060102150405.000000000"))
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	defer func() { _ = os.Remove(binary) }()
	//nolint:gosec // the fixture intentionally launches the repository's fixed Go build command.
	build := exec.CommandContext(ctx, "go", "build", "-trimpath", "-o", binary, "./cmd/content-reconcile")
	build.Dir = moduleRoot
	if output, err := build.CombinedOutput(); err != nil {
		return fmt.Errorf("构建 content-reconcile 失败：%w；输出：%s", err, strings.TrimSpace(string(output)))
	} else if len(output) > 0 {
		_ = output
	}
	//nolint:gosec // the fixture launches the freshly built reconciliation binary with explicit argument values.
	command := exec.CommandContext(ctx, binary, "-config", configPath, "-quiesce-evidence", evidencePath, "-evidence-root", evidenceRoot, "-release", "fixture-release", "-phase", "before")
	command.Dir = moduleRoot
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("执行 content-reconcile 失败：%w；输出：%s", err, strings.TrimSpace(string(output)))
	} else if len(output) > 0 {
		_ = output
	}
	return nil
}

func resolveModuleRoot() (string, error) {
	_, source, _, ok := runtime.Caller(0)
	if !ok {
		return "", errors.New("无法定位 backend module root")
	}
	root, err := filepath.Abs(filepath.Join(filepath.Dir(source), "..", ".."))
	if err != nil {
		return "", err
	}
	return root, nil
}

func redact(value string) string {
	value = regexp.MustCompile(`(?i)postgres(?:ql)?://[^\s"']+`).ReplaceAllString(value, "[REDACTED]")
	return regexp.MustCompile(`(?i)(password\s*[:=]\s*)[^\s,;]+`).ReplaceAllString(value, `${1}[REDACTED]`)
}
