// Package database 为集成测试提供独享 PostgreSQL 数据库生命周期。
package database

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib"

	qaconfig "github.com/PengYuee/SCYG.Blog/backend/internal/qa/config"
)

const randomNameBytes = 8

// Isolated 持有一个随机 QA 数据库及其管理连接。
type Isolated struct {
	admin *sql.DB
	dsn   string
	name  string
}

// New 创建严格使用 QA 前缀的随机数据库。
func New(ctx context.Context, marker string) (*Isolated, error) {
	if marker == "" || strings.IndexFunc(marker, func(value rune) bool { return !(value >= 'a' && value <= 'z' || value == '_') }) >= 0 {
		return nil, errors.New("QA 数据库标记不合法")
	}
	config, err := qaconfig.LoadLocal()
	if err != nil {
		return nil, fmt.Errorf("加载 QA 配置：%w", err)
	}
	random := make([]byte, randomNameBytes)
	if _, err = rand.Read(random); err != nil {
		return nil, fmt.Errorf("生成 QA 数据库名：%w", err)
	}
	name := config.DatabasePrefix() + marker + hex.EncodeToString(random)
	adminDSN := config.AdminDSN().Value()
	admin, err := sql.Open("pgx", adminDSN)
	if err != nil {
		return nil, fmt.Errorf("打开 QA 管理连接：%w", err)
	}
	if _, err = admin.ExecContext(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		return nil, errors.Join(fmt.Errorf("创建 QA 数据库：%w", err), admin.Close())
	}
	parsed, err := url.Parse(adminDSN)
	if err != nil {
		return nil, errors.Join(fmt.Errorf("解析 QA 管理连接：%w", err), drop(ctx, admin, name), admin.Close())
	}
	parsed.Path = "/" + name
	return &Isolated{admin: admin, dsn: parsed.String(), name: name}, nil
}

// DSN 返回仅供数据库适配器使用的独享连接字符串。
func (database *Isolated) DSN() string { return database.dsn }

// Close 终止目标连接、删除数据库并确认无残留。
func (database *Isolated) Close(ctx context.Context) error {
	if database == nil {
		return nil
	}
	cleanupErr := drop(ctx, database.admin, database.name)
	var remaining int
	countErr := database.admin.QueryRowContext(ctx, `SELECT count(*) FROM pg_database WHERE datname=$1`, database.name).Scan(&remaining)
	if countErr == nil && remaining != 0 {
		countErr = fmt.Errorf("QA 数据库清理后仍残留：%d", remaining)
	}
	return errors.Join(cleanupErr, countErr, database.admin.Close())
}

func drop(ctx context.Context, admin *sql.DB, name string) error {
	_, terminateErr := admin.ExecContext(ctx, `SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname=$1`, name)
	_, dropErr := admin.ExecContext(ctx, "DROP DATABASE IF EXISTS "+pgx.Identifier{name}.Sanitize())
	return errors.Join(terminateErr, dropErr)
}
