package reviewtest_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var lateBindingADRRequirements = []string{
	"**取代：** supersedes ADR-002",
	"**后补理由：**",
	"**哈希：** SHA-256",
	"**许可证：** MIT",
	"**回退：**",
	"**非改写理由：**",
}

// repositoryRoot 返回包含 backend 的仓库根目录。
func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("无法定位 reviewtest 源文件")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

// readFile 读取明确路径，不假设执行元数据被提交进产品。
func readFile(t *testing.T, path string) string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("读取交付文件 %s 失败：%v", path, err)
	}
	return string(content)
}

// metadataPath 优先读取显式环境输入，仅在开发工作区默认路径真实存在时回退。
func metadataPath(t *testing.T, environment, fallback string) string {
	t.Helper()
	if explicit := os.Getenv(environment); explicit != "" {
		// 显式相对路径始终以仓库根为基准，避免测试包工作目录改变解析结果。
		if filepath.IsAbs(explicit) {
			return filepath.Clean(explicit)
		}
		return filepath.Join(repositoryRoot(t), explicit)
	}
	candidate := filepath.Join(repositoryRoot(t), filepath.FromSlash(fallback))
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	t.Fatalf("缺少 %s；干净产品检出必须由 Task/CI 显式提供外部 artifact 路径", environment)
	return ""
}

func Test_MetadataPath_resolves_repository_relative_environment_from_arbitrary_cwd(t *testing.T) {
	// Given
	isolationWorkingDirectory := t.TempDir()
	originalWorkingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("读取当前工作目录失败：%v", err)
	}
	t.Cleanup(func() {
		if restoreErr := os.Chdir(originalWorkingDirectory); restoreErr != nil {
			t.Errorf("恢复当前工作目录失败：%v", restoreErr)
		}
	})
	if err := os.Chdir(isolationWorkingDirectory); err != nil {
		t.Fatalf("切换到隔离工作目录失败：%v", err)
	}
	t.Setenv("PLAN_PATH", filepath.FromSlash(".omo/plans/go-service-architecture-foundation.md"))

	// When
	resolved := metadataPath(t, "PLAN_PATH", ".omo/plans/go-service-architecture-foundation.md")

	// Then
	expected := filepath.Join(repositoryRoot(t), ".omo", "plans", "go-service-architecture-foundation.md")
	if resolved != expected {
		t.Fatalf("计划路径应基于仓库根解析，得到 %q，期望 %q", resolved, expected)
	}
}
func Test_PlanCompliance_reads_explicit_artifacts_without_product_commit(t *testing.T) {
	planPath := metadataPath(t, "PLAN_PATH", ".omo/plans/go-service-architecture-foundation.md")
	evidenceRoot := metadataPath(t, "EVIDENCE_ROOT", ".omo/evidence")
	plan := readFile(t, planPath)
	if !strings.Contains(plan, "Complete real-system architecture proof and developer handoff") {
		t.Fatal("计划 artifact 缺少 Todo13")
	}
	if _, err := os.Stat(filepath.Join(evidenceRoot, "task-13-go-service-architecture-foundation.txt")); err != nil {
		t.Fatalf("证据 artifact 缺少 Todo13：%v", err)
	}
	for _, relative := range []string{"backend/README.md", "backend/docs/guides/module-extension.md", "backend/docs/guides/protocol-integration-extension.md"} {
		if _, err := os.Stat(filepath.Join(repositoryRoot(t), filepath.FromSlash(relative))); err != nil {
			t.Fatalf("产品交付文件缺失 %s：%v", relative, err)
		}
	}
}

func Test_PlanCompliance_preserves_PowerShell_environment_variables_in_Task_template(t *testing.T) {
	// Given
	taskfile := readFile(t, filepath.Join(repositoryRoot(t), "backend", "Taskfile.yml"))

	// When
	planPathEscaped := strings.Contains(taskfile, "$$env:PLAN_PATH")
	evidenceRootEscaped := strings.Contains(taskfile, "$$env:EVIDENCE_ROOT")

	// Then
	if !planPathEscaped || !evidenceRootEscaped {
		t.Fatal("qa:plan 必须转义 PowerShell 环境变量，避免 Task 提前展开为 :PLAN_PATH")
	}
}

func Test_PlanCompliance_accepts_late_binding_ADR_without_history_rewrite(t *testing.T) {
	// Given
	adr := readFile(t, filepath.Join(repositoryRoot(t), "backend", "docs", "architecture", "adr-010-scalar-asset-pin.md"))

	// When
	err := validateLateBindingADR(adr)

	// Then
	if err != nil {
		t.Fatalf("后补 ADR 应作为共享历史的显式纠偏记录：%v", err)
	}
}

func Test_PlanCompliance_rejects_late_binding_ADR_when_required_governance_is_missing(t *testing.T) {
	// Given
	adr := readFile(t, filepath.Join(repositoryRoot(t), "backend", "docs", "architecture", "adr-010-scalar-asset-pin.md"))

	for _, requirement := range lateBindingADRRequirements {
		t.Run("缺少_"+requirement, func(t *testing.T) {
			// When
			err := validateLateBindingADR(strings.Replace(adr, requirement, "", 1))

			// Then
			if err == nil {
				t.Fatalf("缺少治理字段 %q 时不应通过审计", requirement)
			}
		})
	}
}

// validateLateBindingADR 校验后补绑定变更 ADR 的取代、可验证性与不改写共享历史理由。
func validateLateBindingADR(adr string) error {
	for _, requirement := range lateBindingADRRequirements {
		if !strings.Contains(adr, requirement) {
			return fmt.Errorf("后补 ADR 缺少治理字段 %q", requirement)
		}
	}
	return nil
}

func Test_ReviewE2E_AST_proves_nine_real_scenarios(t *testing.T) {
	path := filepath.Join(repositoryRoot(t), "backend", "internal", "e2e", "foundation_e2e_test.go")
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("解析 tagged E2E 失败：%v", err)
	}
	for _, requirement := range scenarioRequirements() {
		if failures := validateScenario(file, requirement); len(failures) != 0 {
			t.Fatalf("E2E 场景验证失败：%v", failures)
		}
	}
}

func Test_ReviewE2E_AST_rejects_string_catalog(t *testing.T) {
	fixture, err := parser.ParseFile(token.NewFileSet(), "catalog_test.go", `package x; import "testing"; func Test_E2E_catalog(t *testing.T){ names:=[]string{"db"}; if len(names)!=1 { t.Fatal("x") } }`, 0)
	if err != nil {
		t.Fatalf("解析 E2E 失败夹具失败：%v", err)
	}
	function := fixture.Decls[1].(*ast.FuncDecl)
	if functionCalls(function, "newHarness") {
		t.Fatal("字符串清单被错误识别为真实 E2E")
	}
}

func Test_ReviewE2E_AST_rejects_missing_concrete_comparison(t *testing.T) {
	fixture, err := parser.ParseFile(token.NewFileSet(), "weak_test.go", `package x; import "testing"; func Test_E2E_weak(t *testing.T){ snapshotDatabase(); request(); if true { t.Fatal("任意条件") } }`, 0)
	if err != nil {
		t.Fatalf("解析弱断言夹具失败：%v", err)
	}
	function := fixture.Decls[1].(*ast.FuncDecl)
	symbols := functionSymbolSet(function)
	if symbols["DeepEqual"] || symbols["StatusForbidden"] {
		t.Fatal("弱断言夹具错误包含具体状态比较")
	}
}
func functionSymbolSet(function *ast.FuncDecl) map[string]bool {
	symbols := make(map[string]bool)
	ast.Inspect(function.Body, func(node ast.Node) bool {
		if call, ok := node.(*ast.CallExpr); ok {
			symbols[callName(call.Fun)] = true
		}
		if selector, ok := node.(*ast.SelectorExpr); ok {
			symbols[selector.Sel.Name] = true
		}
		return true
	})
	return symbols
}

func functionCalls(function *ast.FuncDecl, name string) bool { return functionCallsAny(function, name) }
func functionCallsAny(function *ast.FuncDecl, names ...string) bool {
	found := false
	ast.Inspect(function.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		for _, name := range names {
			if callName(call.Fun) == name {
				found = true
			}
		}
		return true
	})
	return found
}
func callName(expression ast.Expr) string {
	switch value := expression.(type) {
	case *ast.Ident:
		return value.Name
	case *ast.SelectorExpr:
		return value.Sel.Name
	default:
		return ""
	}
}
