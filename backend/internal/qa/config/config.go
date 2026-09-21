// Package config 为集成测试和 E2E 工具加载隔离的敏感 QA 配置。
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const redacted = "[REDACTED]"

// DSN 保存数据库连接，并阻止常见格式化泄露。
type DSN struct{ value string }

// Value returns the unredacted DSN for controlled execution.
func (dsn DSN) Value() string { return dsn.value }
func (DSN) String() string    { return redacted }

// GoString returns the redacted DSN representation.
func (DSN) GoString() string { return redacted }

// Config 是严格 QA-only 配置；不包含普通运行时 database.dsn。
type Config struct {
	adminDSN       DSN
	databasePrefix string
	commandTimeout time.Duration
}

// AdminDSN returns the QA administrator DSN.
func (config Config) AdminDSN() DSN { return config.adminDSN }

// DatabasePrefix returns the isolated QA database prefix.
func (config Config) DatabasePrefix() string { return config.databasePrefix }

// CommandTimeout returns the configured command timeout.
func (config Config) CommandTimeout() time.Duration { return config.commandTimeout }

func (config Config) String() string {
	return fmt.Sprintf("admin_dsn=%s database_prefix=%s command_timeout=%s", redacted, config.databasePrefix, config.commandTimeout)
}

// GoString returns the redacted configuration representation.
func (config Config) GoString() string { return config.String() }

type rawFile struct {
	QA rawQA `yaml:"qa"`
}
type rawQA struct {
	PostgresAdminDSN string        `yaml:"postgres_admin_dsn"`
	DatabasePrefix   string        `yaml:"database_prefix"`
	CommandTimeout   time.Duration `yaml:"command_timeout"`
}

// Load 从显式 QA-only YAML 文件解析并验证配置，不读取环境变量。
func Load(path string) (Config, error) {
	if strings.TrimSpace(path) == "" {
		return Config{}, fmt.Errorf("读取 QA 配置文件失败：路径不能为空")
	}
	//nolint:gosec // QA configuration is explicitly supplied by the operator.
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("读取 QA 配置文件失败 %q：%w", path, err)
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	var raw rawFile
	if err = decoder.Decode(&raw); err != nil {
		return Config{}, fmt.Errorf("解析 QA 配置文件失败 %q：%w", path, err)
	}
	var trailing any
	if err = decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return Config{}, fmt.Errorf("解析 QA 配置文件失败 %q：禁止包含多个 YAML 文档", path)
		}
		return Config{}, fmt.Errorf("解析 QA 配置文件失败 %q：尾随内容：%w", path, err)
	}
	if err = validateDSN("qa.postgres_admin_dsn", raw.QA.PostgresAdminDSN); err != nil {
		return Config{}, err
	}
	prefix := raw.QA.DatabasePrefix
	if strings.TrimSpace(prefix) == "" {
		return Config{}, fmt.Errorf("配置字段 qa.database_prefix：不能为空")
	}
	if strings.IndexFunc(prefix, func(r rune) bool { return (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' }) >= 0 {
		return Config{}, fmt.Errorf("配置字段 qa.database_prefix：只能包含 ASCII 小写字母、数字和下划线")
	}
	if raw.QA.CommandTimeout <= 0 {
		return Config{}, fmt.Errorf("配置字段 qa.command_timeout：必须大于零")
	}
	return Config{adminDSN: DSN{value: raw.QA.PostgresAdminDSN}, databasePrefix: prefix, commandTimeout: raw.QA.CommandTimeout}, nil
}

func validateDSN(field, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("配置字段 %s：不能为空", field)
	}
	if strings.Contains(value, "请填写密码") {
		return fmt.Errorf("配置字段 %s：请先填写数据库密码，不能使用占位值", field)
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || parsed.Host == "" || strings.Trim(parsed.Path, "/") == "" {
		return fmt.Errorf("配置字段 %s：必须是含主机和数据库名的 PostgreSQL DSN", field)
	}
	return nil
}
