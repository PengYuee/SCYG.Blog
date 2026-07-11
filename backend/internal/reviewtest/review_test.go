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
	required := map[string][]string{
		"Test_E2E_migrations_roundtrip":                   {"newHarness", "Shutdown", "migrateDown", "migrateUp", "QueryRowContext", "Scan", "Fatalf"},
		"Test_E2E_scalar_is_offline_and_self_hosted":      {"newHarness", "request", "ReadAll", "Contains", "Fatalf"},
		"Test_E2E_public_reads_hide_drafts":               {"newHarness", "createContent", "createArticle", "request", "Fatalf"},
		"Test_E2E_allow_all_performs_real_crud":           {"newHarness", "createContent", "createArticle", "request", "Fatalf"},
		"Test_E2E_production_denies_writes":               {"newHarness", "request", "Fatalf"},
		"Test_E2E_stale_etag_is_rejected":                 {"newHarness", "createContent", "Get", "request", "Fatalf"},
		"Test_E2E_readiness_fails_during_database_outage": {"newHarness", "openPool", "ExecContext", "request", "Fatal"},
		"Test_E2E_restart_preserves_committed_data":       {"newHarness", "createContent", "Shutdown", "start", "request", "ReadAll", "Contains", "Fatalf"},
		"Test_E2E_cancellation_cleans_runtime":            {"newHarness", "Run", "cancel", "After", "Fatal"},
	}
	root := filepath.Join(repositoryRoot(t), "backend", "internal", "e2e")
	paths, err := filepath.Glob(filepath.Join(root, "*_e2e_test.go"))
	if err != nil {
		t.Fatalf("枚举 tagged E2E 失败：%v", err)
	}
	seen := make(map[string]bool)
	for _, path := range paths {
		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if parseErr != nil {
			t.Fatalf("解析 tagged E2E 失败：%v", parseErr)
		}
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok {
				continue
			}
			expected, scenario := required[function.Name.Name]
			if !scenario {
				continue
			}
			seen[function.Name.Name] = true
			calls := functionCallSet(function)
			for _, name := range expected {
				if !calls[name] {
					t.Fatalf("E2E %s 缺少关键调用/断言 %s", function.Name.Name, name)
				}
			}
			if countAssertions(function) < 1 {
				t.Fatalf("E2E %s 缺少条件化可观察断言", function.Name.Name)
			}
		}
	}
	for name := range required {
		if !seen[name] {
			t.Fatalf("缺少 E2E 场景 %s", name)
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

func functionCallSet(function *ast.FuncDecl) map[string]bool {
	calls := make(map[string]bool)
	ast.Inspect(function.Body, func(node ast.Node) bool {
		if call, ok := node.(*ast.CallExpr); ok {
			calls[callName(call.Fun)] = true
		}
		return true
	})
	return calls
}

func countAssertions(function *ast.FuncDecl) int {
	count := 0
	ast.Inspect(function.Body, func(node ast.Node) bool {
		if _, ok := node.(*ast.IfStmt); ok {
			count++
		}
		return true
	})
	return count
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
