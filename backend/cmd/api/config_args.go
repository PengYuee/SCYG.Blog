package main

import (
	"fmt"
	"strings"
)

const defaultConfigFile = "config.local.yaml"

// parseConfigFile 解析 API 专用配置参数，避免注册全局 flag 干扰信号参数。
func parseConfigFile(args []string) (string, error) {
	configFile := defaultConfigFile
	for index := 0; index < len(args); index++ {
		argument := args[index]
		if strings.HasPrefix(argument, "-config=") {
			configFile = strings.TrimPrefix(argument, "-config=")
			continue
		}
		if argument == "-config" {
			if index+1 >= len(args) {
				return "", fmt.Errorf("-config 参数缺少 YAML 文件路径")
			}
			index++
			configFile = args[index]
			continue
		}
		return "", fmt.Errorf("不支持的 API 参数：%s；仅支持 -config <YAML路径>", argument)
	}
	return configFile, nil
}
