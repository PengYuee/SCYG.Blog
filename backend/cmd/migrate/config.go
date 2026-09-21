package main

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/config"
)

// migrationAdminFile 仅描述迁移 up 所需的私有 YAML 片段。
type migrationAdminFile struct {
	// QA 包含不进入运行时配置的数据库管理连接。
	QA struct {
		// PostgresAdminDSN 是必须连接 postgres 库的管理 URL。
		PostgresAdminDSN string `yaml:"postgres_admin_dsn"`
	} `yaml:"qa"`
}

// migrationArguments 保存迁移配置路径和位置参数。
type migrationArguments struct {
	// configFile 是必需的 YAML 路径。
	configFile string
	// command 是保持原顺序的位置参数。
	command []string
}

// parseMigrationArguments 解析现有 -config 参数且不增加新的命令接口。
func parseMigrationArguments(args []string) (migrationArguments, error) {
	result := migrationArguments{command: args}
	if len(result.command) > 0 && result.command[0] == "-config" {
		if len(result.command) < 2 {
			return migrationArguments{}, fmt.Errorf("-config 参数缺少 YAML 文件路径")
		}
		result.configFile, result.command = result.command[1], result.command[2:]
	} else if len(result.command) > 0 && strings.HasPrefix(result.command[0], "-config=") && len(result.command[0]) > len("-config=") {
		result.configFile, result.command = strings.TrimPrefix(result.command[0], "-config="), result.command[1:]
	}
	if strings.TrimSpace(result.configFile) == "" {
		return migrationArguments{}, fmt.Errorf("必须通过 -config 提供 YAML 配置路径")
	}
	if len(result.command) > 0 && (result.command[0] == "--dsn" || result.command[0] == "-dsn") {
		return migrationArguments{}, fmt.Errorf("不支持 DSN 参数，请填写 YAML 的 database.dsn")
	}
	return result, nil
}

// loadMigrationConfig 解析迁移参数并只从指定 YAML 读取数据库连接。
func loadMigrationConfig(args []string) (string, []string, error) {
	arguments, err := parseMigrationArguments(args)
	if err != nil {
		return "", nil, err
	}
	data, err := os.ReadFile(arguments.configFile)
	if err != nil {
		return "", nil, fmt.Errorf("读取迁移配置失败：%w", err)
	}
	var raw struct {
		Database struct {
			DSN string `yaml:"dsn"`
		} `yaml:"database"`
	}
	if err = yaml.Unmarshal(data, &raw); err != nil {
		return "", nil, fmt.Errorf("解析迁移配置失败：%w", err)
	}
	if strings.TrimSpace(raw.Database.DSN) == "" {
		return "", nil, fmt.Errorf("迁移配置必须显式提供 database.dsn")
	}
	cfg, err := config.Load(config.Options{File: arguments.configFile, DisableEnvironment: true})
	if err != nil {
		return "", nil, fmt.Errorf("加载迁移配置失败：%w", err)
	}
	return cfg.Database().DSN().Value(), arguments.command, nil
}

// loadMigrationAdminDSN 私有读取 up 自动建库所需的 QA 管理连接。
func loadMigrationAdminDSN(path string) (string, error) {
	//nolint:gosec // the migration command intentionally reads the explicit operator-provided config path.
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("读取迁移管理配置失败：%w", err)
	}
	var raw migrationAdminFile
	if err = yaml.Unmarshal(data, &raw); err != nil {
		return "", fmt.Errorf("解析迁移管理配置失败：%w", err)
	}
	if strings.TrimSpace(raw.QA.PostgresAdminDSN) == "" {
		return "", fmt.Errorf("up 自动建库需要配置 qa.postgres_admin_dsn")
	}
	return raw.QA.PostgresAdminDSN, nil
}
