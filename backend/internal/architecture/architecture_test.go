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
		{code: "ARCH_FORBIDDEN_IMPORT", path: "article/forbidden.go"},
		{code: "ARCH_FORBIDDEN_IMPORT", path: "application/gin.go"},
		{code: "ARCH_FORBIDDEN_IMPORT", path: "api_generated.go"},
		{code: "ARCH_INTERNAL_IMPORT", path: "external_internal.go"},
		{code: "ARCH_DEPENDENCY_DIRECTION", path: "article/sibling.go"},
		{code: "ARCH_DEPENDENCY_DIRECTION", path: "application/cross_module.go"},
		{code: "ARCH_LEGACY_FILE", path: "not_shared.go"},
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

func Test_Architecture_RejectsInitSideEffects(t *testing.T) {
	// Given
	root := t.TempDir()
	path := filepath.Join(root, "initialization.go")
	if err := os.WriteFile(path, []byte("package fixture\n\nfunc init() {}\n"), 0o600); err != nil {
		t.Fatalf("write init fixture: %v", err)
	}

	// When
	violations, err := Scan(root)
	// Then
	if err != nil {
		t.Fatalf("scan init fixture: %v", err)
	}
	if !containsViolation(violations, "ARCH_INIT_SIDE_EFFECT", "initialization.go") {
		t.Fatalf("expected init side-effect violation; got %v", violations)
	}
}

func Test_Architecture_FutureProtocolDirectories_areReviewOnly(t *testing.T) {
	// Given
	root := t.TempDir()
	paths := []string{
		"internal/transport/grpc/server.go",
		"internal/transport/websocket/handler.go",
	}
	for _, relative := range paths {
		absolute := filepath.Join(root, filepath.FromSlash(relative))
		if err := os.MkdirAll(filepath.Dir(absolute), 0o750); err != nil {
			t.Fatalf("create future protocol fixture directory: %v", err)
		}
		if err := os.WriteFile(absolute, []byte("package protocol\n"), 0o600); err != nil {
			t.Fatalf("write future protocol fixture: %v", err)
		}
	}

	// When
	violations, err := Scan(root)
	// Then
	if err != nil {
		t.Fatalf("scan future protocol fixtures: %v", err)
	}
	for _, relative := range paths {
		if containsViolation(violations, "ARCH_FORBIDDEN_FUTURE", relative) {
			t.Errorf("organizational violation ARCH_FORBIDDEN_FUTURE at %s remains enforced: %v", relative, violations)
		}
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

func containsViolation(violations []Violation, code, path string) bool {
	for _, violation := range violations {
		if violation.Code == code && strings.Contains(violation.Path, path) {
			return true
		}
	}
	return false
}
