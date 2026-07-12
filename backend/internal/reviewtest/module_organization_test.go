package reviewtest_test

import (
	"path/filepath"
	"strings"
	"testing"
)

const agentGenericTokenContract = "Generic token set: `common` / `shared` / `utils` / `utility` / `helpers` / `models` / `usecases` / `results`."

// documentsCompleteGenericTokenSet 判断简版指南是否保留完整且顺序稳定的八词集合。
func documentsCompleteGenericTokenSet(document string) bool {
	return strings.Contains(document, agentGenericTokenContract)
}

func Test_ModuleExtensionGuide_documents_binding_file_organization(t *testing.T) {
	// Given：读取开发者实际遵循的详细指南与简版强制规则。
	root := repositoryRoot(t)
	guide := readFile(t, filepath.Join(root, "backend", "docs", "guides", "module-extension.md"))
	agentGuide := readFile(t, filepath.Join(root, "backend", "AGENTS.md"))
	genericTokens := []string{"common", "shared", "utils", "utility", "helpers", "models", "usecases", "results"}

	// Then：完整集合必须原子出现，删除任一 token 后都不得继续满足合同。
	if !documentsCompleteGenericTokenSet(agentGuide) {
		t.Errorf("Backend Agent Guide 缺少完整 generic token 集合 %q", agentGenericTokenContract)
	}
	for _, token := range genericTokens {
		incompleteContract := strings.Replace(agentGenericTokenContract, "`"+token+"`", "", 1)
		adversarialGuide := strings.Replace(agentGuide, agentGenericTokenContract, incompleteContract, 1)
		if documentsCompleteGenericTokenSet(adversarialGuide) {
			t.Errorf("删除 generic token %q 后仍错误满足完整集合合同", token)
		}
	}
	guideRequirements := []string{
		"强制机械底线",
		"推荐职责命名",
		"层级用目录、主体用前缀、职责用名称表达",
		"小写 snake_case",
		"不能为空",
		"连续使用下划线",
		"开头或结尾使用下划线",
		"`common`、`shared`、`utils`、`utility`、`helpers`、`models`、`usecases`、`results`",
		"`<subject>_<role>.go`",
		"`<subject>_<role>_<subrole>.go`",
		"Scanner 不枚举职责后缀",
		"合理的新职责无需修改 Scanner",
		"`api.go`、`module.go`",
		"数据库数据模型",
		"`<subject>_model.go`",
		"`*_record.go`",
		"模块根、`internal/domain`、`internal/application`、`internal/postgres`、`postgres`",
		"禁止任何 Go 子 package",
		"禁止实体 Go 子包",
		"测试使用业务主体与可观察行为命名",
		"`helpers`、`utils` 或 `common`",
		"不要求与生产文件一对一",
	}
	agentRequirements := []string{
		"小写 snake_case",
		"generic token",
		"`api.go` and `module.go`",
		"`<subject>_model.go`",
		"`*_record.go`",
		"Scanner does not enumerate responsibility suffixes",
		"Tests use the same generic token set",
		"禁止任何 Go 子 package",
		"禁止实体 Go 子包",
	}
	obsoleteContracts := []string{
		"完整 stem 必须",
		"固定职责后缀至少包括",
		"准确语义例外按 layer 限定",
		"这是封闭集合",
		"复杂后缀也是有限集合",
		"unknown subroles are forbidden",
		"Semantic exceptions are layer-specific",
	}

	// When / Then：新合同必须完整出现，旧有限白名单合同不得继续作为强制规则。
	for _, requirement := range guideRequirements {
		if !strings.Contains(guide, requirement) {
			t.Errorf("模块扩展指南缺少命名契约 %q", requirement)
		}
	}
	for _, requirement := range agentRequirements {
		if !strings.Contains(agentGuide, requirement) {
			t.Errorf("Backend Agent Guide 缺少强制规则 %q", requirement)
		}
	}
	combined := guide + "\n" + agentGuide
	for _, obsolete := range obsoleteContracts {
		if strings.Contains(combined, obsolete) {
			t.Errorf("文档仍将旧有限职责合同表述为强制规则 %q", obsolete)
		}
	}
}
