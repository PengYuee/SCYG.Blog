package deliverytest_test

import (
	"regexp"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// composeDocument 只解析 delivery 测试需要的 Compose 安全结构。
type composeDocument struct {
	// Services 保存按名称索引的 Compose 服务。
	Services map[string]composeService `yaml:"services"`
}

// composeService 表示服务的镜像、端口、环境和依赖健康契约。
type composeService struct {
	// Image 是可选的固定摘要镜像。
	Image string `yaml:"image"`
	// Ports 是服务发布到宿主机的端口。
	Ports []string `yaml:"ports"`
	// Environment 是传入容器的环境配置。
	Environment map[string]string `yaml:"environment"`
	// DependsOn 是服务启动顺序及健康条件。
	DependsOn map[string]composeDependency `yaml:"depends_on"`
	// Healthcheck 是服务自身的健康探测配置。
	Healthcheck map[string]yaml.Node `yaml:"healthcheck"`
}

// composeDependency 表示 API 启动前必须满足的依赖状态。
type composeDependency struct {
	// Condition 是依赖服务必须达到的状态。
	Condition string `yaml:"condition"`
}

func Test_Compose_contains_only_API_and_PostgreSQL_with_health_dependency(t *testing.T) {
	// Given
	raw := readDeliveryFile(t, "compose.yaml")
	var document composeDocument

	// When
	err := yaml.Unmarshal([]byte(raw), &document)

	// Then
	if err != nil {
		t.Fatalf("解析 compose.yaml 失败：%v", err)
	}
	if len(document.Services) != 2 || document.Services["api"].DependsOn["postgres"].Condition != "service_healthy" {
		t.Fatalf("Compose 必须仅包含 API/PostgreSQL 且 API 等待数据库健康：%v", document.Services)
	}
	postgres := document.Services["postgres"]
	if len(postgres.Ports) != 0 || !strings.Contains(postgres.Image, "@sha256:") {
		t.Fatal("生产 Compose 不得暴露 PostgreSQL 端口且镜像必须固定摘要")
	}
}

func Test_Compose_rejects_missing_database_config(t *testing.T) {
	// Given
	raw := readDeliveryFile(t, "compose.yaml")
	var document composeDocument

	// When
	err := yaml.Unmarshal([]byte(raw), &document)

	// Then
	if err != nil {
		t.Fatalf("解析 compose.yaml 失败：%v", err)
	}
	requirements := map[string]string{
		"SCYG_DATABASE_DSN":      document.Services["api"].Environment["SCYG_DATABASE_DSN"],
		"SCYG_POSTGRES_PASSWORD": document.Services["postgres"].Environment["POSTGRES_PASSWORD"],
	}
	for key, value := range requirements {
		if !isRequiredEnvironment(value, key) {
			t.Fatalf("%s 必须使用带中文提示的 required 环境表达式，实际为 %q", key, value)
		}
	}
}

func Test_RequiredEnvironment_rejects_bypass_values(t *testing.T) {
	// Given
	cases := []struct {
		name  string
		value string
		valid bool
	}{
		{"接受中文 required 表达式", "${SCYG_DATABASE_DSN:?必须设置数据库连接}", true},
		{"拒绝明文", "postgres://user:secret@postgres/blog", false},
		{"拒绝空值", "", false},
		{"拒绝普通插值", "${SCYG_DATABASE_DSN}", false},
		{"拒绝默认值插值", "${SCYG_DATABASE_DSN:-postgres://fallback}", false},
		{"拒绝空提示", "${SCYG_DATABASE_DSN:?}", false},
		{"拒绝非中文提示", "${SCYG_DATABASE_DSN:?required}", false},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			// When
			valid := isRequiredEnvironment(testCase.value, "SCYG_DATABASE_DSN")

			// Then
			if valid != testCase.valid {
				t.Fatalf("required 表达式判定错误：value=%q valid=%t", testCase.value, valid)
			}
		})
	}
}

// isRequiredEnvironment 仅接受目标变量带非空中文提示的 Compose required 插值。
func isRequiredEnvironment(value, key string) bool {
	pattern := `^\$\{` + regexp.QuoteMeta(key) + `:\?[^}]*[\p{Han}][^}]*\}$`
	return regexp.MustCompile(pattern).MatchString(value)
}
