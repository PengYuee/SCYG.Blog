// Package database 为集成测试提供独享 PostgreSQL 数据库生命周期。
package database

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	_ "github.com/jackc/pgx/v5/stdlib"

	qaconfig "github.com/PengYuee/SCYG.Blog/backend/internal/qa/config"
)

const (
	randomNameLength      = 13
	runCapabilityLength   = 25
	base36                = "0123456789abcdefghijklmnopqrstuvwxyz"
	maintenanceDatabase   = "postgres"
	capabilityTokenLength = 32
	capabilityProofSuffix = ".capability"
)

// CapabilityTokenLength 返回编排器 capability token 的固定长度。
func CapabilityTokenLength() int { return capabilityTokenLength }

// CapabilityProofSuffix 返回 run 配置旁 capability proof 的文件后缀。
func CapabilityProofSuffix() string { return capabilityProofSuffix }

// Isolated 持有一个随机 QA 数据库及其管理连接。
type Isolated struct {
	admin    *sql.DB
	dsn      string
	name     string
	prefix   string
	exact    bool
	endpoint endpoint
}

type endpoint struct {
	address string
	port    int
}

// New 创建显式 QA 配置下的独立随机数据库。未由编排器调用时，为该 fixture 生成独立 run 前缀。
func New(ctx context.Context, configPath, marker string) (*Isolated, error) {
	if token := os.Getenv("QA_CAPABILITY_TOKEN"); token != "" {
		return NewWithCapability(ctx, configPath, marker, token)
	}
	return newDirect(ctx, configPath, marker)
}

func newDirect(ctx context.Context, configPath, marker string) (*Isolated, error) {
	if err := validateMarker(marker); err != nil {
		return nil, err
	}
	runPrefix, err := newDirectRunPrefix(configPath, runCapabilityLength)
	if err != nil {
		return nil, err
	}
	return newDatabase(ctx, configPath, func(prefix string, max int) (string, error) {
		suffix, err := randomBase36(randomNameLength)
		if err != nil {
			return "", fmt.Errorf("生成 QA 数据库名：%w", err)
		}
		name := prefix + marker + suffix
		return name, validateName(name, prefix, max)
	}, false, true, "", runPrefix[len(runPrefix)-runCapabilityLength:])
}

func newDirectRunPrefix(configPath string, length int) (string, error) {
	config, err := qaconfig.Load(configPath)
	if err != nil {
		return "", fmt.Errorf("加载 QA 配置：%w", err)
	}
	runID, err := randomBase36(length)
	if err != nil {
		return "", fmt.Errorf("生成 QA run capability：%w", err)
	}
	return config.DatabasePrefix() + runID, nil
}

// NewWithCapability 创建由编排器签发 capability proof 的随机数据库。
func NewWithCapability(ctx context.Context, configPath, marker, capabilityToken string) (*Isolated, error) {
	if err := validateMarker(marker); err != nil {
		return nil, err
	}
	return newDatabase(ctx, configPath, func(prefix string, max int) (string, error) {
		suffix, err := randomBase36(randomNameLength)
		if err != nil {
			return "", fmt.Errorf("生成 QA 数据库名：%w", err)
		}
		name := prefix + marker + suffix
		return name, validateName(name, prefix, max)
	}, false, true, capabilityToken, "")
}

func validateMarker(marker string) error {
	if marker == "" || strings.IndexFunc(marker, func(r rune) bool { return (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' }) >= 0 {
		return errors.New("QA 数据库标记不合法")
	}
	return nil
}

// NewExact creates the orchestrator's registered migration database.
func NewExact(ctx context.Context, configPath, name string) (*Isolated, error) {
	return NewExactWithCapability(ctx, configPath, name, os.Getenv("QA_CAPABILITY_TOKEN"))
}

// NewExactWithCapability creates an exact database using an explicit run proof.
func NewExactWithCapability(ctx context.Context, configPath, name, capabilityToken string) (*Isolated, error) {
	return newExact(ctx, configPath, name, true, capabilityToken)
}

// RegisterExact registers an exact database name for deferred creation and cleanup.
func RegisterExact(ctx context.Context, configPath, name string) (*Isolated, error) {
	return RegisterExactWithCapability(ctx, configPath, name, os.Getenv("QA_CAPABILITY_TOKEN"))
}

// RegisterExactWithCapability registers an exact name without creating it.
func RegisterExactWithCapability(ctx context.Context, configPath, name, capabilityToken string) (*Isolated, error) {
	return newExact(ctx, configPath, name, false, capabilityToken)
}

func newExact(ctx context.Context, configPath, name string, create bool, capabilityToken string) (*Isolated, error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("QA 数据库名不能为空")
	}
	return newDatabase(ctx, configPath, func(prefix string, max int) (string, error) {
		if err := validateExactName(name, prefix, max); err != nil {
			return "", err
		}
		return name, nil
	}, true, create, capabilityToken, "")
}

func newDatabase(ctx context.Context, configPath string, nameFn func(string, int) (string, error), exact, create bool, capabilityToken, runID string) (*Isolated, error) {
	if strings.TrimSpace(configPath) == "" {
		return nil, errors.New("QA_CONFIG 不能为空")
	}
	config, err := qaconfig.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("加载 QA 配置：%w", err)
	}
	runPrefix := config.DatabasePrefix()
	if runID == "" {
		if err = validateRunCapability(configPath, runPrefix, capabilityToken); err != nil {
			return nil, err
		}
	} else {
		if len(runID) != runCapabilityLength || strings.IndexFunc(runID, func(r rune) bool { return !strings.ContainsRune(base36, r) }) >= 0 {
			return nil, errors.New("QA direct fixture run capability 不合法")
		}
		runPrefix += runID
	}
	admin, err := sql.Open("pgx", config.AdminDSN().Value())
	if err != nil {
		return nil, fmt.Errorf("打开 QA 管理连接：%w", err)
	}
	adminEndpoint, maxNameLength, err := verifyAdmin(ctx, admin)
	if err != nil {
		_ = admin.Close()
		return nil, err
	}
	name, err := nameFn(runPrefix, maxNameLength)
	if err != nil {
		_ = admin.Close()
		return nil, err
	}
	dsn, err := dsnForDatabase(config.AdminDSN().Value(), name)
	if err != nil {
		_ = admin.Close()
		return nil, err
	}
	if err = verifyDSNEndpoint(config.AdminDSN().Value(), dsn); err != nil {
		_ = admin.Close()
		return nil, err
	}
	if create {
		if _, err = admin.ExecContext(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
			createErr := fmt.Errorf("创建 QA 数据库：%w", err)
			cleanupErr := cleanupCreatedDatabase(ctx, config, admin, name, runPrefix, adminEndpoint, exact)
			closeErr := admin.Close()
			return nil, errors.Join(createErr, cleanupErr, closeErr)
		}
		if err = verifyTarget(ctx, dsn, name, adminEndpoint); err != nil {
			verifyErr := err
			cleanupErr := cleanupCreatedDatabase(ctx, config, admin, name, runPrefix, adminEndpoint, exact)
			closeErr := admin.Close()
			return nil, errors.Join(verifyErr, cleanupErr, closeErr)
		}
	}
	return &Isolated{admin: admin, dsn: dsn, name: name, prefix: runPrefix, exact: exact, endpoint: adminEndpoint}, nil
}

func cleanupCreatedDatabase(ctx context.Context, config qaconfig.Config, admin *sql.DB, name, prefix string, expected endpoint, exact bool) error {
	var cleanupErr error
	for attempt := range 2 {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), config.CommandTimeout())
		err := cleanupOne(cleanupCtx, admin, name, prefix, expected, exact)
		cancel()
		if err == nil {
			return cleanupErr
		}
		cleanupErr = errors.Join(cleanupErr, fmt.Errorf("清理 QA 数据库（第 %d 次）：%w", attempt+1, err))
	}
	return cleanupErr
}

func randomBase36(length int) (string, error) {
	limit := big.NewInt(int64(len(base36)))
	out := make([]byte, length)
	for i := range out {
		n, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", err
		}
		out[i] = base36[n.Int64()]
	}
	return string(out), nil
}

// DSN 返回隔离数据库连接。
func (database *Isolated) DSN() string {
	if database == nil {
		return ""
	}
	return database.dsn
}

// Name 返回隔离数据库名。
func (database *Isolated) Name() string {
	if database == nil {
		return ""
	}
	return database.name
}

// Close 终止独享数据库连接、删除数据库并证明不存在残留。
func (database *Isolated) Close(ctx context.Context) error {
	if database == nil {
		return nil
	}
	cleanupErr := cleanupOne(ctx, database.admin, database.name, database.prefix, database.endpoint, database.exact)
	var remaining int
	countErr := database.admin.QueryRowContext(ctx, `SELECT count(*) FROM pg_database WHERE datname=$1`, database.name).Scan(&remaining)
	if countErr == nil && remaining != 0 {
		countErr = fmt.Errorf("QA 数据库清理后仍残留：%d", remaining)
	}
	return errors.Join(cleanupErr, countErr, database.admin.Close())
}

// CleanupPrefix 删除本次编排 run capability 创建的所有残留数据库。
func CleanupPrefix(ctx context.Context, configPath, prefix, basePrefix string) error {
	return CleanupPrefixWithCapability(ctx, configPath, prefix, basePrefix, os.Getenv("QA_CAPABILITY_TOKEN"))
}

// CleanupPrefixWithCapability 删除本次编排 run capability 创建的所有残留数据库。
func CleanupPrefixWithCapability(ctx context.Context, configPath, prefix, basePrefix, capabilityToken string) error {
	config, err := qaconfig.Load(configPath)
	if err != nil {
		return fmt.Errorf("加载 QA 配置：%w", err)
	}
	if config.DatabasePrefix() != prefix {
		return errors.New("QA 配置 capability 与清理参数不一致")
	}
	if err = validateRunPrefix(prefix, basePrefix); err != nil {
		return err
	}
	if err = validateRunCapability(configPath, prefix, capabilityToken); err != nil {
		return err
	}
	admin, err := sql.Open("pgx", config.AdminDSN().Value())
	if err != nil {
		return fmt.Errorf("打开 QA 管理连接：%w", err)
	}
	defer func() { _ = admin.Close() }()
	adminEndpoint, _, err := verifyAdmin(ctx, admin)
	if err != nil {
		return err
	}
	rows, err := admin.QueryContext(ctx, `SELECT datname FROM pg_database WHERE starts_with(datname, $1)`, prefix)
	if err != nil {
		return fmt.Errorf("枚举 QA 数据库：%w", err)
	}
	defer func() { _ = rows.Close() }()
	var names []string
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			return err
		}
		names = append(names, name)
	}
	if err = rows.Err(); err != nil {
		return err
	}
	if err = rows.Close(); err != nil {
		return err
	}

	// Validate every candidate before issuing any terminate or DROP statement.
	var ownedNames []string
	for _, name := range names {
		exists, validateErr := validateCleanupCandidate(ctx, admin, name, prefix, adminEndpoint, false)
		if validateErr != nil {
			return fmt.Errorf("验证 QA 清理候选 %q：%w", name, validateErr)
		}
		if exists {
			ownedNames = append(ownedNames, name)
		}
	}
	var cleanupErr error
	for _, name := range ownedNames {
		if err = validateAndDrop(ctx, admin, name, prefix, adminEndpoint); err != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("删除 QA 数据库 %q 失败：%w", name, err))
		}
	}
	var remaining int
	countErr := admin.QueryRowContext(ctx, `SELECT count(*) FROM pg_database WHERE starts_with(datname, $1)`, prefix).Scan(&remaining)
	if countErr == nil && remaining != 0 {
		countErr = fmt.Errorf("QA run 数据库清理后仍残留：%d", remaining)
	}
	return errors.Join(cleanupErr, countErr)
}

func cleanupOne(ctx context.Context, admin *sql.DB, name, prefix string, expected endpoint, exact bool) error {
	exists, err := validateCleanupCandidate(ctx, admin, name, prefix, expected, exact)
	if err != nil || !exists {
		return err
	}
	return dropDatabase(ctx, admin, name)
}

func validateAndDrop(ctx context.Context, admin *sql.DB, name, prefix string, expected endpoint) error {
	exists, err := validateCleanupCandidate(ctx, admin, name, prefix, expected, false)
	if err != nil || !exists {
		return err
	}
	return dropDatabase(ctx, admin, name)
}

func validateCleanupCandidate(ctx context.Context, admin *sql.DB, name, prefix string, expected endpoint, exact bool) (bool, error) {
	actual, maxNameLength, err := verifyAdmin(ctx, admin)
	if err != nil {
		return false, err
	}
	if actual != expected {
		return false, errors.New("QA 管理服务器端点发生变化")
	}
	if exact {
		if name == "" || strings.HasPrefix(name, prefix) || len(name) > maxNameLength {
			return false, errors.New("QA 精确数据库名不合法")
		}
	} else if err = validateGeneratedName(name, prefix, maxNameLength); err != nil {
		return false, err
	}
	var owned bool
	err = admin.QueryRowContext(ctx, `SELECT datdba=(SELECT usesysid FROM pg_user WHERE usename=current_user) FROM pg_database WHERE datname=$1`, name).Scan(&owned)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("验证 QA 数据库所有权：%w", err)
	}
	if !owned {
		return false, errors.New("QA 数据库不属于当前管理用户")
	}
	return true, nil
}

func dropDatabase(ctx context.Context, admin *sql.DB, name string) error {
	if _, err := admin.ExecContext(ctx, `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname=$1`, name); err != nil {
		return err
	}
	_, err := admin.ExecContext(ctx, "DROP DATABASE IF EXISTS "+pgx.Identifier{name}.Sanitize())
	return err
}

func validateRunPrefix(prefix, base string) error {
	if len(prefix) != len(base)+runCapabilityLength || !strings.HasPrefix(prefix, base) {
		return errors.New("QA 清理 capability 不合法")
	}
	for _, value := range prefix[len(base):] {
		if !strings.ContainsRune(base36, value) {
			return errors.New("QA 清理 capability 不合法")
		}
	}
	return nil
}

func verifyAdmin(ctx context.Context, admin *sql.DB) (endpoint, int, error) {
	if admin == nil {
		return endpoint{}, 0, errors.New("QA 管理连接为空")
	}
	var database string
	if err := admin.QueryRowContext(ctx, `SELECT current_database()`).Scan(&database); err != nil {
		return endpoint{}, 0, fmt.Errorf("验证 QA 管理库：%w", err)
	}
	if database != maintenanceDatabase {
		return endpoint{}, 0, errors.New("QA 管理连接必须连接 postgres 数据库")
	}
	var canCreateDB, canCreateRole bool
	if err := admin.QueryRowContext(ctx, `SELECT rolcreatedb, rolcreaterole FROM pg_roles WHERE rolname=current_user`).Scan(&canCreateDB, &canCreateRole); err != nil {
		return endpoint{}, 0, fmt.Errorf("验证 QA 管理员能力：%w", err)
	}
	if !canCreateDB || !canCreateRole {
		return endpoint{}, 0, errors.New("QA 管理员必须具备 rolcreatedb 和 rolcreaterole 能力")
	}
	server, err := serverEndpoint(ctx, admin)
	if err != nil {
		return endpoint{}, 0, err
	}
	var rawMaxNameLength string
	if err = admin.QueryRowContext(ctx, `SHOW max_identifier_length`).Scan(&rawMaxNameLength); err != nil {
		return endpoint{}, 0, fmt.Errorf("读取 PostgreSQL max_identifier_length：%w", err)
	}
	maxNameLength, err := parseIdentifierLimit(rawMaxNameLength)
	if err != nil {
		return endpoint{}, 0, err
	}
	return server, maxNameLength, nil
}

func parseIdentifierLimit(raw string) (int, error) {
	value, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || value <= 0 {
		if err == nil {
			err = errors.New("值必须为正整数")
		}
		return 0, fmt.Errorf("PostgreSQL max_identifier_length 不合法：%w", err)
	}
	return value, nil
}

// OpenVerifiedAdmin opens a maintenance connection only after checking its database,
// server endpoint, CREATEDB and CREATEROLE capabilities, and identifier limit.
func OpenVerifiedAdmin(ctx context.Context, dsn string) (*sql.DB, int, error) {
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, 0, fmt.Errorf("打开 QA 管理连接：%w", err)
	}
	_, maxIdentifierLength, err := verifyAdmin(ctx, admin)
	if err != nil {
		_ = admin.Close()
		return nil, 0, err
	}
	return admin, maxIdentifierLength, nil
}

func verifyTarget(ctx context.Context, dsn, name string, expected endpoint) error {
	target, err := sql.Open("pgx", dsn)
	if err != nil {
		return fmt.Errorf("打开新建 QA 数据库：%w", err)
	}
	defer func() { _ = target.Close() }()
	var database string
	if err = target.QueryRowContext(ctx, `SELECT current_database()`).Scan(&database); err != nil {
		return fmt.Errorf("验证新建 QA 数据库：%w", err)
	}
	if database != name {
		return errors.New("新建 QA 数据库名称不匹配")
	}
	actual, err := serverEndpoint(ctx, target)
	if err != nil {
		return err
	}
	if actual != expected {
		return errors.New("新建 QA 数据库服务器端点不匹配")
	}
	return nil
}

func validateGeneratedName(name, prefix string, maxNameLength int) error {
	if err := validateName(name, prefix, maxNameLength); err != nil {
		return err
	}
	remainder := name[len(prefix):]
	if len(remainder) <= randomNameLength {
		return errors.New("QA 数据库名不符合生成规则")
	}
	marker := remainder[:len(remainder)-randomNameLength]
	if strings.IndexFunc(marker, func(r rune) bool { return (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' }) >= 0 {
		return errors.New("QA 数据库名不符合生成规则")
	}
	for _, value := range remainder[len(remainder)-randomNameLength:] {
		if !strings.ContainsRune(base36, value) {
			return errors.New("QA 数据库名不符合生成规则")
		}
	}
	return nil
}

func serverEndpoint(ctx context.Context, db *sql.DB) (endpoint, error) {
	var value endpoint
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(inet_server_addr()::text, ''), inet_server_port()`).Scan(&value.address, &value.port); err != nil {
		return endpoint{}, fmt.Errorf("验证 PostgreSQL 服务器端点：%w", err)
	}
	return value, nil
}

func validateName(name, prefix string, maxNameLength int) error {
	if !strings.HasPrefix(name, prefix) || len(name) > maxNameLength {
		return errors.New("QA 数据库名超出服务器标识符限制或不属于当前 capability")
	}
	return nil
}

func dsnForDatabase(adminDSN, name string) (string, error) {
	parsed, err := url.Parse(adminDSN)
	if err != nil {
		return "", fmt.Errorf("解析 QA 管理连接：%w", err)
	}
	parsed.Path = "/" + name
	return parsed.String(), nil
}

// IssueCapabilityProof 为指定 run 配置签发一次性 capability proof。
func IssueCapabilityProof(configPath, prefix, capabilityToken string) error {
	if strings.TrimSpace(configPath) == "" {
		return errors.New("QA_CONFIG 不能为空")
	}
	if err := validateCapabilityToken(capabilityToken); err != nil {
		return err
	}
	if len(prefix) <= runCapabilityLength {
		return errors.New("QA 配置必须包含完整 run capability")
	}
	config, err := qaconfig.Load(configPath)
	if err != nil {
		return fmt.Errorf("加载 QA 配置：%w", err)
	}
	if config.DatabasePrefix() != prefix {
		return errors.New("QA 配置 capability 与签发参数不一致")
	}
	//nolint:gosec // configPath is the explicit, capability-checked QA configuration path.
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("读取 QA 配置：%w", err)
	}
	proof := capabilityMAC(data, capabilityToken)
	content := []byte(prefix + "\n" + hex.EncodeToString(proof) + "\n")
	if err = os.WriteFile(configPath+capabilityProofSuffix, content, 0o600); err != nil {
		return fmt.Errorf("写入 QA capability proof：%w", err)
	}
	return nil
}

func validateRunCapability(configPath, prefix, capabilityToken string) error {
	if len(prefix) <= runCapabilityLength {
		return errors.New("QA 配置必须包含完整 run capability")
	}
	capability := prefix[len(prefix)-runCapabilityLength:]
	if strings.IndexFunc(capability, func(r rune) bool { return !strings.ContainsRune(base36, r) }) >= 0 {
		return errors.New("QA 配置必须包含完整 run capability")
	}
	if err := validateCapabilityToken(capabilityToken); err != nil {
		return err
	}
	//nolint:gosec // configPath is the explicit, capability-checked QA configuration path.
	data, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("读取 QA capability 配置：%w", err)
	}
	//nolint:gosec // the proof is adjacent to the same explicit QA configuration path.
	proofData, err := os.ReadFile(configPath + capabilityProofSuffix)
	if err != nil {
		return errors.New("QA 配置缺少 capability proof")
	}
	fields := strings.Split(strings.TrimSpace(string(proofData)), "\n")
	if len(fields) != 2 || fields[0] != prefix {
		return errors.New("QA capability proof 与配置不匹配")
	}
	actual, err := hex.DecodeString(fields[1])
	if err != nil || !hmac.Equal(actual, capabilityMAC(data, capabilityToken)) {
		return errors.New("QA capability proof 无效")
	}
	return nil
}

func validateCapabilityToken(token string) error {
	if len(token) != capabilityTokenLength || strings.IndexFunc(token, func(r rune) bool { return !strings.ContainsRune(base36, r) }) >= 0 {
		return errors.New("QA capability token 不合法")
	}
	return nil
}

func capabilityMAC(data []byte, token string) []byte {
	mac := hmac.New(sha256.New, []byte(token))
	_, _ = mac.Write(data)
	return mac.Sum(nil)
}

func validateExactName(name, prefix string, maxNameLength int) error {
	if strings.TrimSpace(name) == "" || len(name) > maxNameLength || len(prefix) <= runCapabilityLength {
		return errors.New("QA 精确数据库名不合法")
	}
	runID := prefix[len(prefix)-runCapabilityLength:]
	if strings.HasPrefix(name, prefix) || !strings.HasPrefix(name, runID) {
		return errors.New("QA 精确数据库名不合法")
	}
	suffix := strings.TrimPrefix(name, runID)
	if suffix != "migration" && suffix != `migrate_quoted"` {
		return errors.New("QA 精确数据库名不合法")
	}
	return nil
}

func verifyDSNEndpoint(adminDSN, targetDSN string) error {
	admin, err := url.Parse(adminDSN)
	if err != nil {
		return fmt.Errorf("解析 QA 管理连接：%w", err)
	}
	target, err := url.Parse(targetDSN)
	if err != nil {
		return fmt.Errorf("解析 QA 目标连接：%w", err)
	}
	if admin.Hostname() != target.Hostname() || admin.Port() != target.Port() {
		return errors.New("QA 目标连接服务端地址或端口与管理连接不一致")
	}
	return nil
}
