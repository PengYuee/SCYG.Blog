package reviewtest_test

import (
	"path/filepath"
	"strings"
	"testing"
)

func Test_ModuleExtensionGuide_documents_binding_file_organization(t *testing.T) {
	// Given
	root := repositoryRoot(t)
	guide := readFile(t, filepath.Join(root, "backend", "docs", "guides", "module-extension.md"))
	agentGuide := readFile(t, filepath.Join(root, "backend", "AGENTS.md"))
	requirements := []string{
		"目录 = 技术层，前缀 = 业务主体，后缀 = 职责",
		"<subject>_<role>.go",
		"<subject>_<role>_<subrole>.go",
		"`api.go`、`module.go`",
		"数据库数据模型",
		"`<subject>_model.go`",
		"禁止实体 Go 子包",
		"禁止任何 Go 子 package",
		"技术性子目录也需要独立架构决策",
		"完整 stem",
		"最终后缀组合",
		"准确语义例外按 layer 限定",
		"模块根、`internal/domain`、`internal/application`、`internal/postgres`、`postgres`", "测试跟随行为",
		"不要求与生产文件一对一",
		"`command`、`query`、`result`、`usecase`、`port`、`view`、`model`、`repository`、`read_model`、`mapper`、`validation`、`error`",
	}

	// When / Then
	for _, requirement := range requirements {
		if !strings.Contains(guide, requirement) {
			t.Errorf("模块扩展指南缺少命名契约 %q", requirement)
		}
	}
	for _, requirement := range []string{"实体前缀", "固定职责后缀", "*_record.go", "models.go", "禁止实体 Go 子包", "禁止任何 Go 子 package"} {
		if !strings.Contains(agentGuide, requirement) {
			t.Errorf("Backend Agent Guide 缺少强制规则 %q", requirement)
		}
	}
}
