package deliverytest_test

import (
	"regexp"
	"strings"
	"testing"
)

func Test_Dockerfile_uses_pinned_multistage_nonroot_runtime(t *testing.T) {
	// Given
	dockerfile := readDeliveryFile(t, "Dockerfile")

	// When
	fromLines := regexp.MustCompile(`(?m)^FROM\s+\S+@sha256:[0-9a-f]{64}`).FindAllString(dockerfile, -1)

	// Then
	if len(fromLines) != 2 {
		t.Fatalf("Dockerfile 必须包含两个摘要固定的构建阶段，实际为 %d", len(fromLines))
	}
	for _, required := range []string{"AS build", "USER 65532:65532", "COPY --from=build", `HEALTHCHECK`, `["/healthcheck"]`} {
		if !strings.Contains(dockerfile, required) {
			t.Fatalf("Dockerfile 缺少安全契约 %q", required)
		}
	}
	for _, forbidden := range []string{"COPY . .", "golang:latest", ":latest"} {
		if strings.Contains(dockerfile, forbidden) {
			t.Fatalf("Dockerfile 包含禁止配置 %q", forbidden)
		}
	}
}

func Test_Dockerignore_excludes_sources_and_local_secrets(t *testing.T) {
	// Given
	dockerignore := readDeliveryFile(t, ".dockerignore")

	// When
	required := []string{".git", ".env", "bin", "*_test.go"}

	// Then
	for _, pattern := range required {
		if !strings.Contains(dockerignore, pattern) {
			t.Fatalf(".dockerignore 缺少 %q", pattern)
		}
	}
}
