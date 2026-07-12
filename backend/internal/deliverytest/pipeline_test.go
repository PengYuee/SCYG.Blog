package deliverytest_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func Test_Taskfile_exposes_shared_delivery_and_quality_gates(t *testing.T) {
	// Given
	taskfile := readDeliveryFile(t, "Taskfile.yml")

	// When
	var document yaml.Node
	err := yaml.Unmarshal([]byte(taskfile), &document)
	required := []string{"container:build:", "compose:smoke:", "compose:down:", "migrate:roundtrip:", "integration:", "e2e:", "qa:static:", "qa:database:", "qa:container:"}

	// Then
	if err != nil {
		t.Fatalf("解析 Taskfile 失败：%v", err)
	}
	for _, task := range required {
		if !strings.Contains(taskfile, task) {
			t.Fatalf("Taskfile 缺少共享门禁 %q", task)
		}
	}
}

func Test_Taskfile_qa_messages_use_literal_command_blocks(t *testing.T) {
	// Given
	taskfile := readDeliveryFile(t, "Taskfile.yml")
	var document yaml.Node

	// When
	err := yaml.Unmarshal([]byte(taskfile), &document)

	// Then
	if err != nil {
		t.Fatalf("解析 Taskfile 失败：%v", err)
	}
	for _, command := range []string{
		"cmd: |\n          echo 'F1 plan compliance: PASS'",
		"cmd: |\n          echo 'F2 quality: PASS'",
		"cmd: |\n          echo 'foundation QA: PASS'",
		"cmd: |\n          echo 'F3 real system: PASS'",
		"cmd: |\n          echo 'F4 scope: PASS'",
	} {
		if !strings.Contains(taskfile, command) {
			t.Fatalf("带冒号的 QA 消息必须使用 YAML 字面量命令块：%q", command)
		}
	}
	if err := validateTaskfileCommandNodes(&document); err != nil {
		t.Fatal(err)
	}
}
func Test_Taskfile_qa_plan_delegates_to_isolated_PowerShell_runner(t *testing.T) {
	// Given
	taskfile := readDeliveryFile(t, "Taskfile.yml")
	runnerPath := filepath.Join(backendRoot(t), "scripts", "qa-plan.ps1")

	// When
	_, err := os.Stat(runnerPath)

	// Then
	if err != nil {
		t.Fatalf("qa:plan PowerShell 脚本缺失：%v", err)
	}
	if strings.Contains(taskfile, "$env:") || strings.Contains(taskfile, "$$env:") {
		t.Fatal("Taskfile 的 qa:plan 不得内联 PowerShell 环境变量")
	}
	if !strings.Contains(taskfile, "powershell -NoProfile -ExecutionPolicy Bypass -File scripts/qa-plan.ps1") {
		t.Fatal("qa:plan 必须仅调用独立 PowerShell 脚本")
	}
}

// validateTaskfileCommandNodes 拒绝包含冒号但未被 YAML 显式引用的命令标量。
func validateTaskfileCommandNodes(node *yaml.Node) error {
	for index, child := range node.Content {
		if node.Kind == yaml.MappingNode && child.Value == "cmd" && index+1 < len(node.Content) {
			command := node.Content[index+1]
			if strings.Contains(command.Value, ":") && command.Style == 0 {
				return fmt.Errorf("包含冒号的 cmd 必须使用 YAML 显式引用：%q", command.Value)
			}
		}
		if err := validateTaskfileCommandNodes(child); err != nil {
			return err
		}
	}
	return nil
}

func Test_GitHubActions_is_backend_scoped_and_uses_immutable_actions(t *testing.T) {
	// Given
	workflow := readDeliveryFile(t, "../.github/workflows/backend-quality.yml")

	// When
	err := validateActionReferences(workflow)

	// Then
	if err != nil {
		t.Fatalf("GitHub Action 引用无效：%v", err)
	}
	for _, required := range []string{"backend/**", "task qa:static", "task qa:database", "task qa:container", "govulncheck", "sbom", "trivy"} {
		if !strings.Contains(strings.ToLower(workflow), strings.ToLower(required)) {
			t.Fatalf("后端流水线缺少门禁 %q", required)
		}
	}
}

func Test_ActionReference_rejects_nonimmutable_remote_references(t *testing.T) {
	// Given
	cases := []struct {
		name      string
		reference string
		valid     bool
	}{
		{"接受完整小写 SHA", "owner/repo@0123456789abcdef0123456789abcdef01234567", true},
		{"接受完整大写 SHA 和子路径", "owner/repo/path@0123456789ABCDEF0123456789ABCDEF01234567", true},
		{"接受本地 Action", "./.github/actions/local", true},
		{"拒绝分支", "owner/repo@main", false},
		{"拒绝标签", "owner/repo@v4", false},
		{"拒绝短 SHA", "owner/repo@0123456789abcdef0123456789abcdef0123456", false},
		{"拒绝四十一位 SHA", "owner/repo@0123456789abcdef0123456789abcdef012345678", false},
		{"拒绝非十六进制 SHA", "owner/repo@z123456789abcdef0123456789abcdef01234567", false},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			// When
			err := validateActionReferences("steps:\n  - uses: " + testCase.reference + "\n")

			// Then
			if (err == nil) != testCase.valid {
				t.Fatalf("Action 引用判定错误：reference=%q error=%v", testCase.reference, err)
			}
		})
	}
}

// validateActionReferences 解析工作流并校验每个 uses 引用均为本地路径或完整提交 SHA。
func validateActionReferences(workflow string) error {
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(workflow), &document); err != nil {
		return fmt.Errorf("解析工作流失败：%w", err)
	}
	return validateActionNodes(&document)
}

// validateActionNodes 递归检查 YAML 中每个 uses 标量，避免逐行正则遗漏嵌套步骤。
func validateActionNodes(node *yaml.Node) error {
	for index := 0; index < len(node.Content); index++ {
		child := node.Content[index]
		if node.Kind == yaml.MappingNode && child.Value == "uses" && index+1 < len(node.Content) {
			reference := node.Content[index+1].Value
			if !isImmutableActionReference(reference) {
				return fmt.Errorf("远程 Action 必须固定完整 40 位 SHA，实际为 %q", reference)
			}
		}
		if err := validateActionNodes(child); err != nil {
			return err
		}
	}
	return nil
}

// isImmutableActionReference 允许本地 Action；远程 Action 必须为 owner/repo 可选子路径加完整 SHA。
func isImmutableActionReference(reference string) bool {
	if strings.HasPrefix(reference, "./") {
		return true
	}
	pattern := `^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+(?:/[A-Za-z0-9_.-]+)*@[0-9a-fA-F]{40}$`
	return regexp.MustCompile(pattern).MatchString(reference)
}
