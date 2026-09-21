package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	invalidCatalogSQLState    = "3D000"
	duplicateDatabaseSQLState = "42P04"
	adminDatabaseName         = "postgres"
)

type serverEndpoint struct {
	address string
	port    int
}

// ensureDatabaseForUp 仅在目标库不存在时读取 QA 管理连接并创建它。
func ensureDatabaseForUp(ctx context.Context, targetDSN string, loadAdminDSN func() (string, error)) error {
	targetName, err := parseDatabaseName(targetDSN)
	if err != nil {
		return err
	}
	targetConfig, err := pgx.ParseConfig(targetDSN)
	if err != nil {
		return fmt.Errorf("database.dsn 不是有效的 PostgreSQL URL：%w", err)
	}
	adminDSN, err := loadAdminDSN()
	if err != nil {
		return err
	}
	adminConfig, err := parseAdminDSN(adminDSN)
	if err != nil {
		return err
	}
	if targetConfig.Host != adminConfig.Host || targetConfig.Port != adminConfig.Port {
		return errors.New("database.dsn 与 qa.postgres_admin_dsn 的 PostgreSQL 主机或端口不匹配")
	}

	target, err := pgx.ConnectConfig(ctx, targetConfig)
	if err == nil {
		defer func() { _ = target.Close(ctx) }()
		if err = verifyCurrentDatabase(ctx, target, targetName); err != nil {
			return err
		}
		admin, adminErr := pgx.ConnectConfig(ctx, adminConfig)
		if adminErr != nil {
			return fmt.Errorf("连接 PostgreSQL 管理库失败：%w", adminErr)
		}
		defer func() { _ = admin.Close(ctx) }()
		adminEndpoint, _, adminErr := verifyAdmin(ctx, admin)
		if adminErr != nil {
			return adminErr
		}
		actualEndpoint, endpointErr := endpointFor(ctx, target)
		if endpointErr != nil {
			return endpointErr
		}
		if actualEndpoint != adminEndpoint {
			return errors.New("目标数据库服务器端点不匹配")
		}
		return nil
	}
	if !hasSQLState(err, invalidCatalogSQLState) {
		return fmt.Errorf("连接目标数据库失败：%w", err)
	}

	admin, err := pgx.ConnectConfig(ctx, adminConfig)
	if err != nil {
		return fmt.Errorf("连接 PostgreSQL 管理库失败：%w", err)
	}
	defer func() { _ = admin.Close(ctx) }()
	adminEndpoint, maxNameLength, err := verifyAdmin(ctx, admin)
	if err != nil {
		return err
	}
	if len(targetName) > maxNameLength {
		return errors.New("目标数据库名超出服务器 max_identifier_length")
	}
	if err = createDatabase(ctx, admin, targetName); err != nil && !isDuplicateDatabase(err) {
		return fmt.Errorf("创建目标数据库失败：%w", err)
	}
	created, err := pgx.ConnectConfig(ctx, targetConfig)
	if err != nil {
		return fmt.Errorf("连接新建目标数据库失败：%w", err)
	}
	defer func() { _ = created.Close(ctx) }()
	if err = verifyCurrentDatabase(ctx, created, targetName); err != nil {
		return err
	}
	actualEndpoint, err := endpointFor(ctx, created)
	if err != nil {
		return err
	}
	if actualEndpoint != adminEndpoint {
		return errors.New("新建目标数据库服务器端点不匹配")
	}
	return nil
}

// createDatabase 使用 pgx 标识符转义执行不可参数化的 CREATE DATABASE。
func createDatabase(ctx context.Context, admin *pgx.Conn, name string) error {
	_, err := admin.Exec(ctx, createDatabaseSQL(name))
	return err
}

// createDatabaseSQL 生成只包含安全标识符的建库语句。
func createDatabaseSQL(name string) string {
	return "CREATE DATABASE " + pgx.Identifier{name}.Sanitize()
}

// parseDatabaseName 从 PostgreSQL URL 的唯一解码路径段提取目标库名。
func parseDatabaseName(dsn string) (string, error) {
	parsed, err := url.Parse(dsn)
	if err != nil || parsed.Host == "" || parsed.Fragment != "" || parsed.RawFragment != "" || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") {
		return "", fmt.Errorf("database.dsn 必须是含主机和单段数据库名的 PostgreSQL URL")
	}
	name, err := url.PathUnescape(strings.TrimPrefix(parsed.EscapedPath(), "/"))
	if err != nil || name == "" || strings.ContainsAny(name, "/\x00") || parsed.EscapedPath() == "" || strings.Count(parsed.EscapedPath(), "/") != 1 {
		return "", fmt.Errorf("database.dsn 必须包含唯一且非空的数据库名路径段")
	}
	return name, nil
}

// parseAdminDSN 解析管理连接并强制其目标库为 postgres。
func parseAdminDSN(dsn string) (*pgx.ConnConfig, error) {
	name, err := parseDatabaseName(dsn)
	if err != nil {
		return nil, fmt.Errorf("qa.postgres_admin_dsn 无效：%w", err)
	}
	if name != adminDatabaseName {
		return nil, fmt.Errorf("qa.postgres_admin_dsn 必须连接 postgres 数据库")
	}
	config, err := pgx.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("qa.postgres_admin_dsn 不是有效的 PostgreSQL URL")
	}
	return config, nil
}

func verifyAdmin(ctx context.Context, admin *pgx.Conn) (serverEndpoint, int, error) {
	if err := verifyCurrentDatabase(ctx, admin, adminDatabaseName); err != nil {
		return serverEndpoint{}, 0, err
	}
	var canCreateDB, canCreateRole bool
	if err := admin.QueryRow(ctx, `SELECT rolcreatedb, rolcreaterole FROM pg_roles WHERE rolname=current_user`).Scan(&canCreateDB, &canCreateRole); err != nil {
		return serverEndpoint{}, 0, fmt.Errorf("验证 PostgreSQL 管理员能力：%w", err)
	}
	if !canCreateDB || !canCreateRole {
		return serverEndpoint{}, 0, errors.New("PostgreSQL 管理员必须具备 rolcreatedb 和 rolcreaterole 能力")
	}
	endpoint, err := endpointFor(ctx, admin)
	if err != nil {
		return serverEndpoint{}, 0, err
	}
	var rawMaxNameLength string
	if err = admin.QueryRow(ctx, `SHOW max_identifier_length`).Scan(&rawMaxNameLength); err != nil {
		return serverEndpoint{}, 0, fmt.Errorf("读取 PostgreSQL max_identifier_length：%w", err)
	}
	maxNameLength, err := parseIdentifierLimit(rawMaxNameLength)
	if err != nil {
		return serverEndpoint{}, 0, err
	}
	return endpoint, maxNameLength, nil
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

func verifyCurrentDatabase(ctx context.Context, connection *pgx.Conn, expected string) error {
	var database string
	if err := connection.QueryRow(ctx, `SELECT current_database()`).Scan(&database); err != nil {
		return fmt.Errorf("验证 PostgreSQL 当前数据库：%w", err)
	}
	if database != expected {
		return errors.New("PostgreSQL 当前数据库不匹配")
	}
	return nil
}

func endpointFor(ctx context.Context, connection *pgx.Conn) (serverEndpoint, error) {
	var endpoint serverEndpoint
	if err := connection.QueryRow(ctx, `SELECT COALESCE(inet_server_addr()::text, ''), inet_server_port()`).Scan(&endpoint.address, &endpoint.port); err != nil {
		return serverEndpoint{}, fmt.Errorf("验证 PostgreSQL 服务器端点：%w", err)
	}
	return endpoint, nil
}

// hasSQLState 判断错误链是否包含指定 PostgreSQL SQLSTATE。
func hasSQLState(err error, state string) bool {
	var postgresError *pgconn.PgError
	return errors.As(err, &postgresError) && postgresError.Code == state
}

// isDuplicateDatabase 仅将并发建库产生的 42P04 视为成功。
func isDuplicateDatabase(err error) bool {
	return hasSQLState(err, duplicateDatabaseSQLState)
}
