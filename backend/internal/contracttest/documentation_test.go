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

var hanText = regexp.MustCompile(`\p{Han}`)

func Test_OpenAPI_documentation_is_complete_in_Chinese(t *testing.T) {
	// Given
	document := loadAuthoritativeSpec(t)
	issues := make([]string, 0)
	checkChinese := func(label, text string) {
		if strings.TrimSpace(text) == "" || !hanText.MatchString(text) {
			issues = append(issues, label+" 缺少中文说明")
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
	for name, schema := range document.Components.Schemas {
		checkSchemaDocumentation(&issues, "schema "+name, schema.Value)
	}

	// Then
	sort.Strings(issues)
	if len(issues) != 0 {
		t.Fatalf("OpenAPI 中文文档缺少 %d 项:\n%s", len(issues), strings.Join(issues, "\n"))
	}
}

func Test_OpenAPI_enum_descriptions_cover_exact_values(t *testing.T) {
	// Given
	document := loadAuthoritativeSpec(t)
	issues := make([]string, 0)

	// When
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

func responseDescription(description *string) string {
	if description == nil {
		return ""
	}
	return *description
}

func checkParameterDocumentation(issues *[]string, label string, parameter *openapi3.ParameterRef) {
	if parameter == nil || parameter.Value == nil {
		*issues = append(*issues, label+" 无法解析")
		return
	}
	if strings.TrimSpace(parameter.Value.Description) == "" || !hanText.MatchString(parameter.Value.Description) {
		*issues = append(*issues, label+" 缺少中文说明")
	}
}

func checkSchemaDocumentation(issues *[]string, label string, schema *openapi3.Schema) {
	if schema == nil {
		*issues = append(*issues, label+" 无法解析")
		return
	}
	if strings.TrimSpace(schema.Description) == "" || !hanText.MatchString(schema.Description) {
		*issues = append(*issues, label+" 缺少中文说明")
	}
	for name, property := range schema.Properties {
		resolved := property.Value
		if resolved == nil || strings.TrimSpace(resolved.Description) == "" || !hanText.MatchString(resolved.Description) {
			*issues = append(*issues, label+" property "+name+" 缺少中文说明")
		}
		if resolved != nil && resolved.Type != nil && slices.Contains(*resolved.Type, "array") && resolved.Items != nil && resolved.Items.Value != nil {
			if strings.TrimSpace(resolved.Items.Value.Description) == "" || !hanText.MatchString(resolved.Items.Value.Description) {
				*issues = append(*issues, label+" property "+name+" items 缺少中文说明")
			}
		}
	}
}

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
