package reviewtest_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// scopeViolation 描述一个应阻止交付的范围越界。
type scopeViolation struct {
	// Path 是相对后端根目录的违规路径。
	Path string
	// Reason 是面向开发者的中文修复提示。
	Reason string
}

// scanScope 扫描运行时代码、依赖和目录；文档允许描述未来设计。
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
		lowerPath := strings.ToLower(normalized)
		if entry.IsDir() {
			for _, segment := range []string{"/grpc", "/websocket", "/kafka", "/outbox", "/api/proto", "/internal/integration"} {
				if strings.Contains("/"+lowerPath, segment) {
					violations = append(violations, scopeViolation{Path: normalized, Reason: "当前阶段禁止未来运行时目录"})
					return filepath.SkipDir
				}
			}
			return nil
		}
		if normalized == "README.md" || strings.HasPrefix(normalized, "docs/") || strings.HasPrefix(normalized, "bin/") || strings.HasPrefix(normalized, "internal/architecture/") || strings.Contains(normalized, "/testdata/") || strings.HasSuffix(normalized, "_test.go") || strings.HasSuffix(normalized, ".sum") {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := strings.ToLower(string(content))
		for _, token := range []string{"google.golang.org/grpc", "gorilla/websocket", "segmentio/kafka", "buf.build/", "automigrate(", "/article/get", "type category struct"} {
			if strings.Contains(text, token) {
				violations = append(violations, scopeViolation{Path: normalized, Reason: "发现禁止的依赖、旧路由、自动迁移或实体改名：" + token})
			}
		}
		if strings.Contains(text, "allowall") {
			violations = append(violations, scopeViolation{Path: normalized, Reason: "AllowAll 只能存在于测试源码或测试二进制"})
		}
		return nil
	})
	return violations, err
}

func Test_ScopeScanner_rejects_every_excluded_fixture(t *testing.T) {
	// Given
	cases := []struct {
		name string
		path string
		body string
	}{
		{"gRPC 依赖", "go.mod", "require google.golang.org/grpc v1.0.0"},
		{"WebSocket 依赖", "runtime.go", "import _ \"github.com/gorilla/websocket\""},
		{"Kafka 依赖", "runtime.go", "import _ \"github.com/segmentio/kafka-go\""},
		{"Buf 依赖", "go.mod", "require buf.build/gen/go/example v1.0.0"},
		{"Outbox 目录", "internal/outbox/worker.go", "package outbox"},
		{"Proto 目录", "api/proto/content/v1/content.proto", "syntax = \"proto3\";"},
		{"旧路由", "router.go", "const route = \"/Article/Get\""},
		{"实体改名", "model.go", "type Category struct{}"},
		{"自动迁移", "database.go", "db.AutoMigrate(&Article{})"},
		{"生产 AllowAll", "authorizer.go", "type AllowAll struct{}"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			root := t.TempDir()
			path := filepath.Join(root, filepath.FromSlash(testCase.path))
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatalf("创建失败夹具目录失败：%v", err)
			}
			if err := os.WriteFile(path, []byte(testCase.body), 0o600); err != nil {
				t.Fatalf("写入失败夹具失败：%v", err)
			}

			// When
			violations, err := scanScope(root)

			// Then
			if err != nil {
				t.Fatalf("扫描失败夹具失败：%v", err)
			}
			if len(violations) == 0 {
				t.Fatalf("范围门禁未拒绝 %s", testCase.name)
			}
		})
	}
}

func Test_ScopeScanner_allows_future_references_in_docs_and_test_AllowAll(t *testing.T) {
	// Given
	root := t.TempDir()
	fixtures := map[string]string{"docs/future.md": "future grpc websocket Buf Kafka Outbox", "authorizer_test.go": "type AllowAll struct{}"}
	for relative, body := range fixtures {
		path := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("创建允许夹具目录失败：%v", err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatalf("写入允许夹具失败：%v", err)
		}
	}

	// When
	violations, err := scanScope(root)

	// Then
	if err != nil {
		t.Fatalf("扫描允许夹具失败：%v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("范围门禁错误拒绝文档/测试引用：%v", violations)
	}
}

func Test_Scope_current_backend_has_no_excluded_runtime(t *testing.T) {
	// Given
	root := filepath.Join(repositoryRoot(t), "backend")

	// When
	violations, err := scanScope(root)

	// Then
	if err != nil {
		t.Fatalf("扫描后端范围失败：%v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("后端存在范围越界：%v", violations)
	}
	if _, err := os.Stat(filepath.Join(repositoryRoot(t), "go.mod")); !os.IsNotExist(err) {
		t.Fatal("仓库根目录禁止存在 go.mod")
	}
}
