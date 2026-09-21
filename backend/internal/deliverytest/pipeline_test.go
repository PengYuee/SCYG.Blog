package deliverytest_test

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func Test_Taskfile_exposes_shared_delivery_and_quality_gates(t *testing.T) {
	// Given
	taskfile := readDeliveryFile(t, "Taskfile.yml")

	// When
	tasks, err := parseTaskfileTasks(taskfile)
	required := []string{"container:build", "compose:smoke", "compose:down", "integration", "e2e", "qa:static", "qa:database", "qa:container"}

	// Then
	if err != nil {
		t.Fatalf("解析 Taskfile 失败：%v", err)
	}
	for _, task := range required {
		if _, ok := tasks[task]; !ok {
			t.Fatalf("Taskfile 缺少共享门禁 %q", task)
		}
	}
}

func Test_Taskfile_quality_tiers_have_approved_machine_structure(t *testing.T) {
	// Given
	taskfile := readDeliveryFile(t, "Taskfile.yml")

	// When
	tasks, err := parseTaskfileTasks(taskfile)
	// Then
	if err != nil {
		t.Fatalf("解析 Taskfile 失败：%v", err)
	}
	required := []string{"qa:feature", "qa:contract", "qa:static", "qa:database", "qa:container"}
	missing := make([]string, 0, len(required))
	for _, taskName := range required {
		if _, ok := tasks[taskName]; !ok {
			missing = append(missing, taskName)
		}
	}
	if len(missing) != 0 {
		t.Fatalf("Taskfile 缺少质量层任务：%s", strings.Join(missing, ", "))
	}

	feature := tasks["qa:feature"]
	if !taskHasReferences(feature, "task:version:check", "fmt:check", "mod:verify") {
		t.Fatal("qa:feature 必须委托版本、格式和模块校验任务")
	}
	if !taskHasCommand(feature, "go test", "./internal/reviewtest", "./internal/architecture") {
		t.Fatal("qa:feature 必须运行聚焦 architecture/review 测试")
	}
	for _, entry := range feature.entries {
		lower := strings.ToLower(entry)
		for _, forbidden := range []string{"database", "container", "docker", "compose", "postgres", "migrate", "integration", "e2e", "service"} {
			if strings.Contains(lower, forbidden) {
				t.Fatalf("qa:feature 不得包含数据库、容器或服务命令：%q", entry)
			}
		}
	}

	contract := tasks["qa:contract"]
	if !taskHasReferences(contract, "api:lint", "api:generate:check") {
		t.Fatal("qa:contract 必须委托 OpenAPI lint 和生成漂移检查")
	}
	if !taskHasCommand(contract, "go test", "./internal/contracttest") {
		t.Fatal("qa:contract 必须运行聚焦 contract 测试")
	}

	static := tasks["qa:static"]
	if !taskHasReferences(static, "vet", "lint", "unit", "build", "vuln") {
		t.Fatal("qa:static 必须保留 vet、lint、unit、build 和 vuln 交接保护")
	}
	if !taskInvokesOrReproduces(static, taskReproduction{
		reference:            "qa:feature",
		reproducedReferences: []string{"task:version:check", "fmt:check", "mod:verify"},
		commandParts:         []string{"go test", "./internal/reviewtest", "./internal/architecture"},
	}) {
		t.Fatal("qa:static 必须调用或复现 qa:feature 检查")
	}
	if !taskInvokesOrReproduces(static, taskReproduction{
		reference:            "qa:contract",
		reproducedReferences: []string{"api:lint", "api:generate:check"},
		commandParts:         []string{"go test", "./internal/contracttest"},
	}) {
		t.Fatal("qa:static 必须调用或复现 qa:contract 检查")
	}
}

type taskfileTask struct {
	entries    []string
	references []string
}

type taskReproduction struct {
	reference            string
	reproducedReferences []string
	commandParts         []string
}

func parseTaskfileTasks(taskfile string) (map[string]taskfileTask, error) {
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(taskfile), &document); err != nil {
		return nil, fmt.Errorf("解析 Taskfile 失败：%w", err)
	}
	if len(document.Content) != 1 {
		return nil, fmt.Errorf("Taskfile 根节点无效")
	}
	tasksNode := taskfileMappingValue(document.Content[0], "tasks")
	if tasksNode == nil || tasksNode.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("Taskfile 缺少 tasks 映射")
	}
	tasks := make(map[string]taskfileTask, len(tasksNode.Content)/2)
	for index := 0; index+1 < len(tasksNode.Content); index += 2 {
		name, definition := tasksNode.Content[index], tasksNode.Content[index+1]
		parsed := taskfileTask{}
		if cmds := taskfileMappingValue(definition, "cmds"); cmds != nil && cmds.Kind == yaml.SequenceNode {
			for _, item := range cmds.Content {
				if item.Kind == yaml.ScalarNode {
					parsed.entries = append(parsed.entries, item.Value)
					continue
				}
				if item.Kind != yaml.MappingNode {
					continue
				}
				for itemIndex := 0; itemIndex+1 < len(item.Content); itemIndex += 2 {
					key, value := item.Content[itemIndex].Value, item.Content[itemIndex+1].Value
					switch key {
					case "task":
						parsed.references = append(parsed.references, value)
						parsed.entries = append(parsed.entries, value)
					case "cmd", "defer":
						parsed.entries = append(parsed.entries, value)
					}
				}
			}
		}
		tasks[name.Value] = parsed
	}
	return tasks, nil
}

func taskfileMappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for index := 0; index+1 < len(node.Content); index += 2 {
		if node.Content[index].Value == key {
			return node.Content[index+1]
		}
	}
	return nil
}

func taskHasReferences(task taskfileTask, references ...string) bool {
	for _, reference := range references {
		if !slices.Contains(task.references, reference) {
			return false
		}
	}
	return true
}

func taskHasCommand(task taskfileTask, parts ...string) bool {
	for _, entry := range task.entries {
		matches := true
		for _, part := range parts {
			if !strings.Contains(entry, part) {
				matches = false
				break
			}
		}
		if matches {
			return true
		}
	}
	return false
}

func taskInvokesOrReproduces(task taskfileTask, expectation taskReproduction) bool {
	if slices.Contains(task.references, expectation.reference) {
		return true
	}
	for _, reproducedReference := range expectation.reproducedReferences {
		if !slices.Contains(task.references, reproducedReference) {
			return false
		}
	}
	return taskHasCommand(task, expectation.commandParts...)
}

const (
	qaPlanSuccessMarker = "F1 plan compliance: PASS"
	qaPlanSuccessOutput = `Write-Output "` + qaPlanSuccessMarker + `"`
)

func Test_Taskfile_qa_plan_delegates_to_isolated_PowerShell_runner(t *testing.T) {
	// Given
	taskfile := readDeliveryFile(t, "Taskfile.yml")
	runnerPath := filepath.Join(backendRoot(t), "scripts", "qa-plan.ps1")

	// When
	runner := readDeliveryFile(t, filepath.Join("scripts", "qa-plan.ps1"))
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
	if err := validateQAPlanRunner(runner); err != nil {
		t.Fatal(err)
	}
}

func Test_QAPlanRunner_rejects_missing_or_incorrect_F1_marker(t *testing.T) {
	cases := []struct {
		name   string
		runner string
	}{
		{name: "缺少标记", runner: `Write-Output "QA plan complete"`},
		{name: "错误标记", runner: `Write-Output "F1 plan compliance: FAIL"`},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			// When
			err := validateQAPlanRunner(testCase.runner)

			// Then
			if err == nil {
				t.Fatalf("无效 qa:plan 脚本通过验证：%q", testCase.runner)
			}
		})
	}
}

// validateQAPlanRunner 确保 QA 计划运行器在成功路径输出精确 F1 标记。
func validateQAPlanRunner(runner string) error {
	if !strings.Contains(runner, qaPlanSuccessOutput) {
		return fmt.Errorf("qa:plan 脚本必须在成功路径输出精确标记 %q", qaPlanSuccessMarker)
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
