package contracttest

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

const (
	missingChineseDescription = " 缺少中文说明"
	unresolvedDocumentation   = " 无法解析"
)

// hanText 匹配中文汉字，用于全量文档完整性检查。
var hanText = regexp.MustCompile(`\p{Han}`)

// Test_OpenAPI_documentation_is_complete_in_Chinese 验证所有用户可见 OpenAPI 说明均包含中文。
func Test_OpenAPI_documentation_is_complete_in_Chinese(t *testing.T) {
	// Given
	document := loadAuthoritativeSpec(t)
	issues := make([]string, 0)
	// checkChinese 收集缺少中文说明的文档位置，便于一次修复全部回退。
	checkChinese := func(label, text string) {
		if strings.TrimSpace(text) == "" || !hanText.MatchString(text) {
			issues = append(issues, label+missingChineseDescription)
		}
	}

	// When
	checkChinese("info.description", document.Info.Description)
	for _, tag := range document.Tags {
		checkChinese("tag "+tag.Name, tag.Description)
	}
	for label, operation := range operations(document) {
		checkChinese(label+" summary", operation.Summary)
		checkChinese(label+" description", operation.Description)
		for index, parameter := range operation.Parameters {
			checkParameterDocumentation(&issues, fmt.Sprintf("%s parameter[%d]", label, index), parameter)
		}
		if operation.RequestBody != nil {
			checkChinese(label+" requestBody", operation.RequestBody.Value.Description)
		}
		for status, response := range operation.Responses.Map() {
			checkChinese(label+" response "+status, responseDescription(response.Value.Description))
		}
	}
	for path, item := range document.Paths.Map() {
		for index, parameter := range item.Parameters {
			checkParameterDocumentation(&issues, fmt.Sprintf("path %s parameter[%d]", path, index), parameter)
		}
	}
	for name, parameter := range document.Components.Parameters {
		checkParameterDocumentation(&issues, "component parameter "+name, parameter)
	}
	for name, header := range document.Components.Headers {
		checkChinese("component header "+name, header.Value.Description)
	}
	for name, response := range document.Components.Responses {
		checkChinese("component response "+name, responseDescription(response.Value.Description))
	}
	// 遍历顶层 Schema 及其属性，防止嵌套文档绕过完整性门禁。
	for name, schema := range document.Components.Schemas {
		checkSchemaDocumentation(&issues, "schema "+name, schema.Value)
	}

	// Then
	sort.Strings(issues)
	if len(issues) != 0 {
		t.Fatalf("OpenAPI 中文文档缺少 %d 项:\n%s", len(issues), strings.Join(issues, "\n"))
	}
}

// Test_OpenAPI_enum_descriptions_cover_exact_values 验证每个枚举值都在中文说明中逐项出现。
func Test_OpenAPI_enum_descriptions_cover_exact_values(t *testing.T) {
	// Given
	document := loadAuthoritativeSpec(t)
	issues := make([]string, 0)

	// When
	// 遍历顶层 Schema 及其属性，防止嵌套文档绕过完整性门禁。
	for name, schema := range document.Components.Schemas {
		checkEnumDocumentation(&issues, "schema "+name, schema.Value)
		for propertyName, property := range schema.Value.Properties {
			checkEnumDocumentation(&issues, "schema "+name+" property "+propertyName, property.Value)
		}
	}
	for name, parameter := range document.Components.Parameters {
		checkEnumDocumentation(&issues, "component parameter "+name, parameter.Value.Schema.Value)
	}

	// Then
	sort.Strings(issues)
	if len(issues) != 0 {
		t.Fatalf("OpenAPI 枚举说明不完整 %d 项:\n%s", len(issues), strings.Join(issues, "\n"))
	}
}

// responseDescription 将可空响应说明转换为门禁可检查的文本。
func responseDescription(description *string) string {
	if description == nil {
		return ""
	}
	return *description
}

// checkParameterDocumentation 检查参数引用可解析且包含中文说明。
func checkParameterDocumentation(issues *[]string, label string, parameter *openapi3.ParameterRef) {
	if parameter == nil || parameter.Value == nil {
		*issues = append(*issues, label+unresolvedDocumentation)
		return
	}
	if strings.TrimSpace(parameter.Value.Description) == "" || !hanText.MatchString(parameter.Value.Description) {
		*issues = append(*issues, label+missingChineseDescription)
	}
}

// checkSchemaDocumentation 递归检查 Schema、属性及数组元素的中文说明。
func checkSchemaDocumentation(issues *[]string, label string, schema *openapi3.Schema) {
	if schema == nil {
		*issues = append(*issues, label+unresolvedDocumentation)
		return
	}
	if strings.TrimSpace(schema.Description) == "" || !hanText.MatchString(schema.Description) {
		*issues = append(*issues, label+missingChineseDescription)
	}
	for name, property := range schema.Properties {
		resolved := property.Value
		if resolved == nil || strings.TrimSpace(resolved.Description) == "" || !hanText.MatchString(resolved.Description) {
			*issues = append(*issues, label+" property "+name+missingChineseDescription)
		}
		if resolved != nil && resolved.Type != nil && slices.Contains(*resolved.Type, "array") && resolved.Items != nil && resolved.Items.Value != nil {
			if strings.TrimSpace(resolved.Items.Value.Description) == "" || !hanText.MatchString(resolved.Items.Value.Description) {
				*issues = append(*issues, label+" property "+name+" items"+missingChineseDescription)
			}
		}
	}
}

// checkEnumDocumentation 精确检查枚举说明覆盖每一个反引号包裹的值。
func checkEnumDocumentation(issues *[]string, label string, schema *openapi3.Schema) {
	if schema == nil || len(schema.Enum) == 0 {
		return
	}
	for _, value := range schema.Enum {
		literal := fmt.Sprintf("`%v`", value)
		if !strings.Contains(schema.Description, literal) {
			*issues = append(*issues, label+" 未说明枚举值 "+literal)
		}
	}
}
