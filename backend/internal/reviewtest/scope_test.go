package reviewtest_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
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

func Test_ScopeScanner_rejects_semantic_fixtures_and_ignores_comments(t *testing.T) {
	cases := map[string]map[string]string{
		"模块语法":   {"go.mod": "module fixture\nrequire (\n google.golang.org/grpc v1.0.0\n)"},
		"导入别名":   {"runtime.go": "package fixture\nimport socket \"github.com/gorilla/websocket\"\nvar _ = socket.IsCloseError"},
		"选择器调用":  {"database.go": "package fixture\nfunc f(){ db.AutoMigrate ( &Article{} ) }"},
		"生产声明":   {"auth.go": "package fixture\ntype allow_all struct{}"},
		"调用参数拼接": {"route.go": "package fixture\nconst prefix = (\"/Article/\"); func mount(){ router.GET(prefix + \"Get\", handler) }"},
		"实体改名":   {"model.go": "package fixture\ntype Category struct{}"},
		"拼接旧路由":  {"route.go": "package fixture\nconst route = \"/Article/\" + \"Get\""},
		"运行时目录":  {"internal/outbox/worker.go": "package outbox"},
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
