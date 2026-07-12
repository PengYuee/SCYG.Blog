// Package config 为集成测试和 F3 工具加载隔离的敏感 QA 配置。
package config

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

const (
	redacted        = "[REDACTED]"
	localConfigFile = "config.local.yaml"
	maxParentDepth  = 8
)

// DSN 保存数据库连接，并阻止常见格式化泄露。
type DSN struct{ value string }

// Value 仅向 QA 数据库适配器返回连接值。
func (dsn DSN) Value() string { return dsn.value }

// String 返回数据库连接的脱敏表示。
func (DSN) String() string { return redacted }

// GoString 返回数据库连接的 Go 语法脱敏表示。
func (DSN) GoString() string { return redacted }

// Config 是仅供 review、integration 和 e2e 工具使用的不可变 QA 配置。
type Config struct {
	adminDSN       DSN
	databaseDSN    DSN
	databasePrefix string
	commandTimeout time.Duration
}

// AdminDSN 返回 PostgreSQL 管理连接的安全值对象。
func (config Config) AdminDSN() DSN { return config.adminDSN }

// DatabaseDSN 返回普通集成测试数据库连接的安全值对象。
func (config Config) DatabaseDSN() DSN { return config.databaseDSN }

// DatabasePrefix 返回隔离测试数据库名称前缀。
func (config Config) DatabasePrefix() string { return config.databasePrefix }

// CommandTimeout 返回每个 QA 命令的最长执行时间。
func (config Config) CommandTimeout() time.Duration { return config.commandTimeout }

// String 返回不包含数据库连接的 QA 配置摘要。
func (config Config) String() string {
	return fmt.Sprintf("admin_dsn=%s database_dsn=%s database_prefix=%s command_timeout=%s", redacted, redacted, config.databasePrefix, config.commandTimeout)
}

// GoString 返回不包含数据库连接的 Go 语法配置摘要。
func (config Config) GoString() string { return config.String() }

type rawFile struct {
	App       yaml.Node   `yaml:"app"`
	HTTP      yaml.Node   `yaml:"http"`
	Database  rawDatabase `yaml:"database"`
	Docs      yaml.Node   `yaml:"docs"`
	Telemetry yaml.Node   `yaml:"telemetry"`
	QA        rawQA       `yaml:"qa"`
}

type rawDatabase struct {
	DSN             string        `yaml:"dsn"`
	MaxOpenConns    int           `yaml:"max_open_conns"`
	MaxIdleConns    int           `yaml:"max_idle_conns"`
	ConnMaxLifetime time.Duration `yaml:"conn_max_lifetime"`
}

type rawQA struct {
	PostgresAdminDSN string        `yaml:"postgres_admin_dsn"`
	DatabasePrefix   string        `yaml:"database_prefix"`
	CommandTimeout   time.Duration `yaml:"command_timeout"`
}

// LoadLocal 从当前目录向上查找 config.local.yaml，供不同测试包统一使用。
func LoadLocal() (Config, error) {
	directory, err := os.Getwd()
	if err != nil {
		return Config{}, fmt.Errorf("获取 QA 工作目录失败：%w", err)
	}
	for range maxParentDepth {
		path := filepath.Join(directory, localConfigFile)
		if _, statErr := os.Stat(path); statErr == nil {
			return Load(path)
		} else if !os.IsNotExist(statErr) {
			return Config{}, fmt.Errorf("检查 QA 配置文件失败 %q：%w", path, statErr)
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			break
		}
		directory = parent
	}
	return Config{}, fmt.Errorf("读取 QA 配置文件失败：从当前目录向上未找到 %s", localConfigFile)
}

// Load 从显式 YAML 文件解析并验证 QA 专用配置，不读取环境变量。
func Load(path string) (Config, error) {
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
	if err = validateDSN("database.dsn", raw.Database.DSN); err != nil {
		return Config{}, err
	}
	if err = validateDSN("qa.postgres_admin_dsn", raw.QA.PostgresAdminDSN); err != nil {
		return Config{}, err
	}
	if strings.TrimSpace(raw.QA.DatabasePrefix) == "" {
		return Config{}, fmt.Errorf("配置字段 qa.database_prefix：不能为空")
	}
	if raw.QA.CommandTimeout <= 0 {
		return Config{}, fmt.Errorf("配置字段 qa.command_timeout：必须大于零")
	}
	return Config{adminDSN: DSN{value: raw.QA.PostgresAdminDSN}, databaseDSN: DSN{value: raw.Database.DSN}, databasePrefix: raw.QA.DatabasePrefix, commandTimeout: raw.QA.CommandTimeout}, nil
}

func validateDSN(field, value string) error {
	if strings.Contains(value, "请填写密码") {
		return fmt.Errorf("配置字段 %s：请先填写数据库密码，不能使用占位值", field)
	}
	parsed, err := url.Parse(value)
	if err != nil || (parsed.Scheme != "postgres" && parsed.Scheme != "postgresql") || parsed.Host == "" || strings.Trim(parsed.Path, "/") == "" {
		return fmt.Errorf("配置字段 %s：必须是含主机和数据库名的 PostgreSQL DSN", field)
	}
	return nil
}
