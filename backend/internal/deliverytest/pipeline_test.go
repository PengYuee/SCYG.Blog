package deliverytest_test

import (
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

func Test_GitHubActions_is_backend_scoped_and_uses_immutable_actions(t *testing.T) {
	// Given
	workflow := readDeliveryFile(t, "../.github/workflows/backend-quality.yml")

	// When
	var document yaml.Node
	err := yaml.Unmarshal([]byte(workflow), &document)
	floatingAction := regexp.MustCompile(`uses:\s+[^\s]+@(v\d+|main|master)`).FindString(workflow)

	// Then
	if err != nil {
		t.Fatalf("解析 GitHub Actions 工作流失败：%v", err)
	}
	if floatingAction != "" {
		t.Fatalf("GitHub Action 必须固定提交 SHA，发现 %q", floatingAction)
	}
	for _, required := range []string{"backend/**", "task qa:static", "task qa:database", "task qa:container", "govulncheck", "sbom", "trivy"} {
		if !strings.Contains(strings.ToLower(workflow), strings.ToLower(required)) {
			t.Fatalf("后端流水线缺少门禁 %q", required)
		}
	}
}
