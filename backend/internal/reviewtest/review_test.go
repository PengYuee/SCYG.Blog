package reviewtest_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// repositoryRoot 返回包含 backend 与计划证据的仓库根目录。
func repositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("无法定位 reviewtest 源文件")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

// readRepositoryFile 读取仓库交付文件。
func readRepositoryFile(t *testing.T, relative string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(repositoryRoot(t), filepath.FromSlash(relative)))
	if err != nil {
		t.Fatalf("读取交付文件 %s 失败：%v", relative, err)
	}
	return string(content)
}

func Test_PlanCompliance_has_todo13_handoff_surfaces(t *testing.T) {
	// Given
	plan := readRepositoryFile(t, ".omo/plans/go-service-architecture-foundation.md")
	taskfile := readRepositoryFile(t, "backend/Taskfile.yml")

	// When
	requiredPlan := []string{"13. Complete real-system architecture proof and developer handoff", "F1 plan compliance: PASS", "F2 quality: PASS", "F3 real system: PASS", "F4 scope: PASS"}
	requiredFiles := []string{"backend/README.md", "backend/docs/guides/module-extension.md", "backend/docs/guides/protocol-integration-extension.md", ".omo/evidence/task-13-go-service-architecture-foundation.txt"}

	// Then
	for _, marker := range requiredPlan {
		if !strings.Contains(plan, marker) {
			t.Fatalf("计划缺少 Todo13/Final 标记 %q", marker)
		}
	}
	for _, relative := range requiredFiles {
		if _, err := os.Stat(filepath.Join(repositoryRoot(t), filepath.FromSlash(relative))); err != nil {
			t.Fatalf("Todo13 交付文件缺失 %s：%v", relative, err)
		}
	}
	for _, target := range []string{"qa:plan:", "qa:quality:", "qa:foundation:", "qa:scope:"} {
		if !strings.Contains(taskfile, target) {
			t.Fatalf("Taskfile 缺少最终门禁 %q", target)
		}
	}
}

func Test_ReviewDocumentation_preserves_protocol_ownership(t *testing.T) {
	// Given
	architecture := readRepositoryFile(t, "backend/docs/architecture/go-backend-architecture.md")
	protocolGuide := readRepositoryFile(t, "backend/docs/guides/protocol-integration-extension.md")
	moduleGuide := readRepositoryFile(t, "backend/docs/guides/module-extension.md")

	// When
	required := []string{"consumer-owned", "module.go", "api.go", "internal/domain", "internal/application", "internal/postgres", "250", "manual"}

	// Then
	combined := strings.ToLower(architecture + moduleGuide)
	for _, marker := range required {
		if !strings.Contains(combined, strings.ToLower(marker)) {
			t.Fatalf("模块扩展指南缺少约束 %q", marker)
		}
	}
	for _, marker := range []string{"api/proto/<domain>/v1", "scyg.realtime.protobuf.v1", "ClientMessage", "ServerMessage", "internal/integration/<service>", "Outbox", "universal envelope", "RFC 9457"} {
		if !strings.Contains(protocolGuide, marker) {
			t.Fatalf("协议扩展指南缺少决策 %q", marker)
		}
	}
}

func Test_ReviewTaskfile_bounds_external_system_cleanup(t *testing.T) {
	// Given
	taskfile := readRepositoryFile(t, "backend/Taskfile.yml")

	// When / Then
	for _, marker := range []string{"--wait-timeout 120", "defer: task compose:down", "-timeout 30s", "foundation QA: PASS", "F3 real system: PASS"} {
		if !strings.Contains(taskfile, marker) {
			t.Fatalf("最终门禁缺少 timeout/finally/结果标记 %q", marker)
		}
	}
}
