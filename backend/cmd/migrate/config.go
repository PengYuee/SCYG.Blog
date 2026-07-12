package main

import (
	"fmt"

	"github.com/PengYuee/SCYG.Blog/backend/internal/platform/config"
)

const defaultConfigFile = "config.local.yaml"

// loadMigrationConfig 解析迁移参数并只从指定 YAML 读取数据库连接。
func loadMigrationConfig(args []string) (string, []string, error) {
	configFile := defaultConfigFile
	command := args
	if len(command) > 0 && command[0] == "-config" {
		if len(command) < 2 {
			return "", nil, fmt.Errorf("-config 参数缺少 YAML 文件路径")
		}
		configFile, command = command[1], command[2:]
	} else if len(command) > 0 && len(command[0]) > len("-config=") && command[0][:len("-config=")] == "-config=" {
		configFile, command = command[0][len("-config="):], command[1:]
	}
	if len(command) > 0 && (command[0] == "--dsn" || command[0] == "-dsn") {
		return "", nil, fmt.Errorf("不支持 DSN 参数，请填写 YAML 的 database.dsn")
	}
	cfg, err := config.Load(config.Options{File: configFile, DisableEnvironment: true})
	if err != nil {
		return "", nil, fmt.Errorf("加载迁移配置失败：%w", err)
	}
	return cfg.Database().DSN().Value(), command, nil
}
