package reviewtest_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"golang.org/x/mod/modfile"
)

// scopeViolation 描述一个语义范围越界。
type scopeViolation struct{ Path, Reason string }

// scanScope 使用 Go AST 与 go.mod 语法树扫描运行时范围，注释不参与判定。
func scanScope(root string) ([]scopeViolation, error) {
	violations := make([]scopeViolation, 0)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		normalized := filepath.ToSlash(relative)
		if entry.IsDir() {
			if normalized != "." && forbiddenRuntimeDirectory(normalized) {
				violations = append(violations, scopeViolation{normalized, "当前阶段禁止未来运行时目录"})
				return filepath.SkipDir
			}
			return nil
		}
		if normalized == "go.mod" {
			found, parseErr := scanModule(path)
			violations = append(violations, found...)
			return parseErr
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		violations = append(violations, scanGoAST(normalized, parsed)...)
		return nil
	})
	return violations, err
}

// forbiddenModulePrefixes 返回本阶段禁止进入模块图或生产导入的前缀。
func forbiddenModulePrefixes() []string {
	return []string{"google.golang.org/grpc", "github.com/gorilla/websocket", "github.com/segmentio/kafka-go", "buf.build/"}
}

// isForbiddenDirectory 按完整路径段识别未来运行时目录。
func isForbiddenDirectory(name string) bool {
	switch name {
	case "grpc", "websocket", "kafka", "outbox", "proto", "integration":
		return true
	default:
		return false
	}
}
func forbiddenRuntimeDirectory(path string) bool {
	parts := strings.Split(strings.ToLower(filepath.ToSlash(path)), "/")
	for index, part := range parts {
		if isForbiddenDirectory(part) && !(part == "integration" && index > 0 && parts[index-1] == "testdata") {
			return true
		}
	}
	return false
}

func scanModule(path string) ([]scopeViolation, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	file, err := modfile.Parse(path, content, nil)
	if err != nil {
		return nil, err
	}
	violations := make([]scopeViolation, 0)
	for _, requirement := range file.Require {
		for _, prefix := range forbiddenModulePrefixes() {
			if strings.HasPrefix(requirement.Mod.Path, prefix) {
				violations = append(violations, scopeViolation{"go.mod", "禁止模块依赖：" + requirement.Mod.Path})
			}
		}
	}
	return violations, nil
}

func scanGoAST(path string, file *ast.File) []scopeViolation {
	violations := make([]scopeViolation, 0)
	for _, imported := range file.Imports {
		value, err := strconv.Unquote(imported.Path.Value)
		if err != nil {
			continue
		}
		for _, prefix := range forbiddenModulePrefixes() {
			if strings.HasPrefix(value, prefix) {
				violations = append(violations, scopeViolation{path, "禁止运行时导入：" + value})
			}
		}
	}
	ast.Inspect(file, func(node ast.Node) bool {
		switch value := node.(type) {
		case *ast.TypeSpec:
			if value.Name.Name == "AllowAll" {
				violations = append(violations, scopeViolation{path, "AllowAll 只能存在于测试源码"})
			}
			if value.Name.Name == "Category" {
				violations = append(violations, scopeViolation{path, "实体 ArticleType 禁止改名为 Category"})
			}
		case *ast.CallExpr:
			if selector, ok := value.Fun.(*ast.SelectorExpr); ok && selector.Sel.Name == "AutoMigrate" {
				violations = append(violations, scopeViolation{path, "禁止调用 AutoMigrate"})
			}
		case *ast.ValueSpec:
			for _, expression := range value.Values {
				if route, ok := constantString(expression); ok && legacyRoute(route) {
					violations = append(violations, scopeViolation{path, "禁止旧式 action 路由：" + route})
				}
			}
		}
		return true
	})
	return violations
}

// constantString 折叠字符串字面量拼接，避免通过空白或 `+` 绕过旧路由扫描。
func constantString(expression ast.Expr) (string, bool) {
	switch value := expression.(type) {
	case *ast.BasicLit:
		if value.Kind != token.STRING {
			return "", false
		}
		result, err := strconv.Unquote(value.Value)
		return result, err == nil
	case *ast.BinaryExpr:
		if value.Op != token.ADD {
			return "", false
		}
		left, leftOK := constantString(value.X)
		right, rightOK := constantString(value.Y)
		return left + right, leftOK && rightOK
	default:
		return "", false
	}
}

func legacyRoute(route string) bool {
	lower := strings.ToLower(route)
	return strings.Contains(lower, "/article/get") || strings.Contains(lower, "/article/create") || strings.Contains(lower, "/article/update") || strings.Contains(lower, "/article/delete")
}

func Test_ScopeScanner_rejects_semantic_fixtures_and_ignores_comments(t *testing.T) {
	cases := map[string]map[string]string{
		"模块语法":  {"go.mod": "module fixture\nrequire (\n google.golang.org/grpc v1.0.0\n)"},
		"导入别名":  {"runtime.go": "package fixture\nimport socket \"github.com/gorilla/websocket\"\nvar _ = socket.IsCloseError"},
		"选择器调用": {"database.go": "package fixture\nfunc f(){ db.AutoMigrate ( &Article{} ) }"},
		"生产声明":  {"auth.go": "package fixture\ntype AllowAll struct{}"},
		"实体改名":  {"model.go": "package fixture\ntype Category struct{}"},
		"拼接旧路由": {"route.go": "package fixture\nconst route = \"/Article/\" + \"Get\""},
		"运行时目录": {"internal/outbox/worker.go": "package outbox"},
	}
	for name, files := range cases {
		t.Run(name, func(t *testing.T) {
			root := writeFixture(t, files)
			found, err := scanScope(root)
			if err != nil {
				t.Fatalf("扫描失败夹具失败：%v", err)
			}
			if len(found) == 0 {
				t.Fatalf("范围门禁未拒绝 %s", name)
			}
		})
	}
	root := writeFixture(t, map[string]string{"safe.go": "package fixture\n// db.AutoMigrate(&x{}) type AllowAll struct{} /Article/Get\nconst route = \"/api/v1/articles\""})
	found, err := scanScope(root)
	if err != nil || len(found) != 0 {
		t.Fatalf("注释错误触发范围门禁：%v %v", err, found)
	}
}

func Test_Scope_current_backend_and_tracked_repository_are_clean(t *testing.T) {
	found, err := scanScope(filepath.Join(repositoryRoot(t), "backend"))
	if err != nil {
		t.Fatalf("扫描后端范围失败：%v", err)
	}
	if len(found) != 0 {
		t.Fatalf("后端存在范围越界：%v", found)
	}
	command := exec.Command("git", "ls-files", "go.mod", "*.cs")
	command.Dir = repositoryRoot(t)
	output, err := command.Output()
	if err != nil {
		t.Fatalf("读取 Git tracked paths 失败：%v", err)
	}
	if strings.TrimSpace(string(output)) != "" {
		t.Fatalf("仓库跟踪了禁止的根 go.mod/C# 路径：%s", output)
	}
	if baseline := os.Getenv("SCOPE_BASELINE"); baseline != "" {
		diff := exec.Command("git", "diff", "--name-only", baseline, "HEAD", "--", "*.cs")
		diff.Dir = repositoryRoot(t)
		changed, diffErr := diff.Output()
		if diffErr != nil {
			t.Fatalf("按 SCOPE_BASELINE 检查 C# 差异失败：%v", diffErr)
		}
		if strings.TrimSpace(string(changed)) != "" {
			t.Fatalf("检测到 C# 差异：%s", changed)
		}
	}
}

func writeFixture(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for relative, body := range files {
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("创建夹具目录失败：%v", err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("写入夹具失败：%v", err)
		}
	}
	return root
}
