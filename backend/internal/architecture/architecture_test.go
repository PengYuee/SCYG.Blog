package architecture

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func Test_Architecture_AcceptsValidGraph(t *testing.T) {
	// Given
	root := filepath.Join("testdata", "valid")

	// When
	violations, err := Scan(root)
	// Then
	if err != nil {
		t.Fatalf("scan valid graph: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("valid graph rejected: %v", violations)
	}
}

func Test_Architecture_CompilesValidGraph(t *testing.T) {
	// Given
	command := exec.Command("go", "test", "./...")
	command.Dir = filepath.Join("testdata", "valid")
	command.Env = append(os.Environ(), "GOWORK=off")

	// When
	output, err := command.CombinedOutput()
	// Then
	if err != nil {
		t.Fatalf("compile valid graph: %v: %s", err, output)
	}
}

func Test_Architecture_RejectsInvalidCompileGraph(t *testing.T) {
	// Given
	command := exec.Command("go", "test", "./...")
	command.Dir = filepath.Join("testdata", "invalid_compile")
	command.Env = append(os.Environ(), "GOWORK=off")

	// When
	output, err := command.CombinedOutput()

	// Then
	if err == nil || !strings.Contains(string(output), "MissingType") {
		t.Fatalf("expected compiler to reject MissingType; err=%v output=%s", err, output)
	}
}

func Test_Architecture_RejectsForbiddenImports(t *testing.T) {
	// Given
	cases := []struct {
		code string
		path string
	}{
		{code: "ARCH_FORBIDDEN_IMPORT", path: "domain_gorm.go"},
		{code: "ARCH_FORBIDDEN_IMPORT", path: "application_gin.go"},
		{code: "ARCH_FORBIDDEN_IMPORT", path: "api_generated.go"},
		{code: "ARCH_INTERNAL_IMPORT", path: "external_internal.go"},
		{code: "ARCH_DEPENDENCY_DIRECTION", path: "domain_outward.go"},
		{code: "ARCH_UNIVERSAL_API", path: "universal_api.go"},
		{code: "ARCH_FORBIDDEN_PACKAGE", path: "utils/grab.go"},
		{code: "ARCH_MUTABLE_GLOBAL", path: "mutable_global.go"},
		{code: "ARCH_GENERIC_ABSTRACTION", path: "generic_repository.go"},
		{code: "ARCH_GENERIC_ABSTRACTION", path: "generic_store.go"},
		{code: "ARCH_GENERIC_ABSTRACTION", path: "generic_alias.go"},
		{code: "ARCH_GENERIC_ABSTRACTION", path: "crud_repository.go"},
		{code: "ARCH_UNIVERSAL_API", path: "all_protocols.go"},
		{code: "ARCH_UNIVERSAL_API", path: "embedded_universal.go"},
		{code: "ARCH_MUTABLE_GLOBAL", path: "platform_singleton.go"},
		{code: "ARCH_MUTABLE_GLOBAL", path: "fake_errors.go"},
		{code: "ARCH_MUTABLE_GLOBAL", path: "fake_embed.go"},
		{code: "ARCH_MUTABLE_GLOBAL", path: "fake_embed_alias.go"},
		{code: "ARCH_MUTABLE_GLOBAL", path: "grouped_embed.go"},
		{code: "ARCH_MODULE_SHAPE", path: "nestedshape"},
		{code: "ARCH_FORBIDDEN_FUTURE", path: "identity/placeholder.go"},
	}

	// When
	violations, err := Scan(filepath.Join("testdata", "invalid"))
	// Then
	if err != nil {
		t.Fatalf("scan invalid fixtures: %v", err)
	}
	for _, testCase := range cases {
		t.Run(testCase.code+"/"+testCase.path, func(t *testing.T) {
			if !containsViolation(violations, testCase.code, testCase.path) {
				t.Errorf("expected %s with path %s; got %v", testCase.code, testCase.path, violations)
			}
		})
	}
}

func Test_Architecture_EnforcesModuleFileOrganization(t *testing.T) {
	// Given
	cases := []struct {
		code   string
		path   string
		detail string
	}{
		{code: "ARCH_MODULE_FILE_NAME", path: "article_record.go", detail: "数据库数据模型文件必须使用 <subject>_model.go，禁止 *_record.go"},
		{code: "ARCH_MODULE_FILE_NAME", path: "models.go", detail: "模块源码禁止泛化文件名 models.go；使用业务主体前缀和固定职责后缀"},
		{code: "ARCH_MODULE_FILE_NAME", path: "usecases.go", detail: "模块源码禁止泛化文件名 usecases.go；使用业务主体前缀和固定职责后缀"},
		{code: "ARCH_MODULE_FILE_NAME", path: "results.go", detail: "模块源码禁止泛化文件名 results.go；使用业务主体前缀和固定职责后缀"},
		{code: "ARCH_MODULE_FILE_NAME", path: "article_creation.go", detail: "模块源码文件必须完整匹配 <subject>_<role>.go 或 <subject>_<role>_<subrole>.go 的有限职责语法"},
		{code: "ARCH_MODULE_FILE_NAME", path: "article_integration.go", detail: "模块源码文件必须完整匹配 <subject>_<role>.go 或 <subject>_<role>_<subrole>.go 的有限职责语法"},
		{code: "ARCH_MODULE_FILE_NAME", path: "article_command_garbage.go", detail: "模块源码文件必须完整匹配 <subject>_<role>.go 或 <subject>_<role>_<subrole>.go 的有限职责语法"},
		{code: "ARCH_MODULE_FILE_NAME", path: "utils_query.go", detail: "业务主体必须是非泛化 snake_case 名称"},
		{code: "ARCH_MODULE_FILE_NAME", path: "article_mapper_common.go", detail: "模块源码文件必须完整匹配 <subject>_<role>.go 或 <subject>_<role>_<subrole>.go 的有限职责语法"},
		{code: "ARCH_MODULE_FILE_NAME", path: "article_command_unknown.go", detail: "模块源码文件必须完整匹配 <subject>_<role>.go 或 <subject>_<role>_<subrole>.go 的有限职责语法"},
		{code: "ARCH_MODULE_FILE_NAME", path: "internal/application/article.go", detail: "文件名 article.go 不是 application 层允许的准确语义例外"},
		{code: "ARCH_MODULE_FILE_NAME", path: "helpers_test.go", detail: "测试文件禁止泛化职责 token helpers；使用业务主体和可观察行为命名"},
		{code: "ARCH_MODULE_FILE_NAME", path: "utils_test.go", detail: "测试文件禁止泛化职责 token utils；使用业务主体和可观察行为命名"},
		{code: "ARCH_MODULE_FILE_NAME", path: "integration_helpers_test.go", detail: "测试文件禁止泛化职责 token helpers；使用业务主体和可观察行为命名"},
		{code: "ARCH_MODULE_FILE_NAME", path: "article_common_test.go", detail: "测试文件禁止泛化职责 token common；使用业务主体和可观察行为命名"},
		{code: "ARCH_MODULE_LAYER_SUBPACKAGE", path: "internal/domain/article/entity.go", detail: "模块 Go package 仅允许根、internal/domain、internal/application、internal/postgres 和 postgres；禁止额外子 package article"},
		{code: "ARCH_MODULE_LAYER_SUBPACKAGE", path: "internal/modules/bad/article/article.go", detail: "模块 Go package 仅允许根、internal/domain、internal/application、internal/postgres 和 postgres；禁止额外子 package article"},
		{code: "ARCH_MODULE_LAYER_SUBPACKAGE", path: "internal/postgres/article/article_model.go", detail: "模块 Go package 仅允许根、internal/domain、internal/application、internal/postgres 和 postgres；禁止额外子 package article"},
		{code: "ARCH_MODULE_LAYER_SUBPACKAGE", path: "internal/modules/bad/internal/application/article/article_query.go", detail: "模块 Go package 仅允许根、internal/domain、internal/application、internal/postgres 和 postgres；禁止额外子 package article"},
		{code: "ARCH_MODULE_LAYER_SUBPACKAGE", path: "internal/modules/bad/postgres/article/article_model.go", detail: "模块 Go package 仅允许根、internal/domain、internal/application、internal/postgres 和 postgres；禁止额外子 package article"},
	}

	// When
	violations, err := Scan(filepath.Join("testdata", "invalid"))

	// Then
	if err != nil {
		t.Fatalf("扫描非法命名夹具失败：%v", err)
	}
	for _, testCase := range cases {
		t.Run(testCase.path, func(t *testing.T) {
			if !containsExactViolation(violations, testCase.code, testCase.path, testCase.detail) {
				t.Errorf("期望稳定 violation code/path/detail：%+v；实际：%v", testCase, violations)
			}
		})
	}
}

func Test_Architecture_AcceptsCommentAndRawStringHeavyFiles(t *testing.T) {
	// Given and When
	violations, err := Scan(filepath.Join("testdata", "loc_legal"))
	// Then
	if err != nil {
		t.Fatalf("scan legal LOC fixtures: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("comments or raw string data counted as code: %v", violations)
	}
}

func Test_Architecture_GeneratedAllowlist_isExact(t *testing.T) {
	// Given
	source, err := os.ReadFile(filepath.Join("testdata", "oversized", "oversized.go"))
	if err != nil {
		t.Fatalf("read oversized source: %v", err)
	}
	root := t.TempDir()
	allowed := filepath.Join(root, "internal", "generated", "openapi", "openapi.gen.go")
	rejected := filepath.Join(root, "internal", "generated", "other", "other.gen.go")
	for _, target := range []string{allowed, rejected} {
		if mkdirErr := os.MkdirAll(filepath.Dir(target), 0o750); mkdirErr != nil {
			t.Fatalf("create generated fixture directory: %v", mkdirErr)
		}
		// target is constrained to this test-owned temporary directory.
		//nolint:gosec
		if writeErr := os.WriteFile(target, source, 0o600); writeErr != nil {
			t.Fatalf("write generated fixture: %v", writeErr)
		}
	}

	// When
	violations, err := Scan(root)
	// Then
	if err != nil {
		t.Fatalf("scan generated allowlist fixture: %v", err)
	}
	if containsViolation(violations, "ARCH_FILE_SIZE", "openapi.gen.go") {
		t.Fatalf("exact generated path was rejected: %v", violations)
	}
	if !containsViolation(violations, "ARCH_FILE_SIZE", "other.gen.go") {
		t.Fatalf("non-allowlisted generated-looking path was accepted: %v", violations)
	}
}

func Test_Architecture_RejectsOversizedFiles(t *testing.T) {
	// Given and When
	violations, err := Scan(filepath.Join("testdata", "oversized"))
	// Then
	if err != nil {
		t.Fatalf("scan oversized fixture: %v", err)
	}
	if !containsViolation(violations, "ARCH_FILE_SIZE", "oversized.go") {
		t.Fatalf("expected oversized fixture path; got %v", violations)
	}
}

func Test_Architecture_AcceptsCommittedBackend(t *testing.T) {
	// Given
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve architecture test path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "..", ".."))

	// When
	violations, err := Scan(root)
	// Then
	if err != nil {
		t.Fatalf("scan backend: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("committed backend rejected: %v", violations)
	}
}

func containsExactViolation(violations []Violation, code, path, detail string) bool {
	for _, violation := range violations {
		if violation.Code == code && strings.HasSuffix(violation.Path, path) && violation.Detail == detail {
			return true
		}
	}
	return false
}

func containsViolation(violations []Violation, code, path string) bool {
	for _, violation := range violations {
		if violation.Code == code && strings.Contains(violation.Path, path) {
			return true
		}
	}
	return false
}
