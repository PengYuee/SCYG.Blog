package reviewtest_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

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
		return explicit
	}
	candidate := filepath.Join(repositoryRoot(t), filepath.FromSlash(fallback))
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	t.Fatalf("缺少 %s；干净产品检出必须由 Task/CI 显式提供外部 artifact 路径", environment)
	return ""
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
