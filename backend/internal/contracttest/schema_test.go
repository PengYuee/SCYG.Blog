package contracttest

import (
	"net/http"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func Test_OpenAPI_schema_contracts(t *testing.T) {
	// Given
	document := loadAuthoritativeSpec(t)

	// When / Then
	assertSchemaRequired(t, document, "ArticleCreate", []string{"title", "slug", "digest", "content", "article_type_id", "tag_ids", "status"})
	assertPatchSchema(t, document, "ArticlePatch")
	assertPatchSchema(t, document, "ArticleTypePatch")
	assertPatchSchema(t, document, "TagPatch")
	assertNoInternalDeleteFields(t, document, "Article")
	assertNoInternalDeleteFields(t, document, "ArticleType")
	assertNoInternalDeleteFields(t, document, "Tag")
	assertSchemaRequired(t, document, "Problem", []string{"type", "title", "status", "detail", "instance", "request_id", "errors"})
}

func Test_OpenAPI_pagination_and_ETag_constraints(t *testing.T) {
	// Given
	document := loadAuthoritativeSpec(t)
	page := parameterSchema(t, document, "Page")
	pageSize := parameterSchema(t, document, "PageSize")
	ifMatch := parameterSchema(t, document, "IfMatch")

	// When / Then
	if page.Default != float64(1) || page.Min == nil || *page.Min != 1 {
		t.Fatal("page must default to 1 with minimum 1")
	}
	if pageSize.Default != float64(20) || pageSize.Min == nil || *pageSize.Min != 1 || pageSize.Max == nil || *pageSize.Max != 100 {
		t.Fatal("page_size must default to 20 with range 1..100")
	}
	pattern, err := regexp.Compile(ifMatch.Pattern)
	if err != nil {
		t.Fatalf("compile If-Match pattern: %v", err)
	}
	for _, invalid := range []string{`W/"1"`, `1`, `"0"`, `"-1"`} {
		if pattern.MatchString(invalid) {
			t.Fatalf("If-Match pattern accepted %q", invalid)
		}
	}
	if !pattern.MatchString(`"42"`) {
		t.Fatal("If-Match pattern rejected strong positive version")
	}
}

func Test_OpenAPI_success_shapes_and_headers(t *testing.T) {
	// Given
	document := loadAuthoritativeSpec(t)
	all := operations(document)

	// When / Then
	for label, operation := range all {
		method, _, _ := strings.Cut(label, " ")
		switch method {
		case http.MethodGet:
			assertJSONSuccess(t, operation, "200")
			if strings.HasPrefix(operation.OperationID, "list") {
				assertResponseHeaderAbsent(t, operation, "200", "ETag")
			} else {
				assertResponseHeader(t, operation, "200", "ETag")
			}
		case http.MethodPost:
			assertJSONSuccess(t, operation, "201")
			assertResponseHeader(t, operation, "201", "ETag")
			assertResponseHeader(t, operation, "201", "Location")
		case http.MethodPatch:
			assertJSONSuccess(t, operation, "200")
			assertResponseHeader(t, operation, "200", "ETag")
		case http.MethodDelete:
			if len(response(t, operation, "204").Content) != 0 {
				t.Fatalf("operation %s delete response has a body", operation.OperationID)
			}
		default:
			t.Fatalf("unexpected operation label %s", label)
		}
	}
}

func assertSchemaRequired(t *testing.T, document *openapi3.T, name string, required []string) {
	t.Helper()
	schema := document.Components.Schemas[name].Value
	for _, property := range required {
		if !contains(schema.Required, property) {
			t.Fatalf("schema %s does not require %s", name, property)
		}
	}
}

func assertPatchSchema(t *testing.T, document *openapi3.T, name string) {
	t.Helper()
	schema := document.Components.Schemas[name].Value
	if schema.MinProps != 1 || schema.AdditionalProperties.Has == nil || *schema.AdditionalProperties.Has {
		t.Fatalf("schema %s must require one known property", name)
	}
	if err := schema.VisitJSON(map[string]any{}); err == nil {
		t.Fatalf("schema %s accepted empty patch", name)
	}
	if err := schema.VisitJSON(map[string]any{"unexpected": "value"}); err == nil {
		t.Fatalf("schema %s accepted additional property", name)
	}
}

func assertNoInternalDeleteFields(t *testing.T, document *openapi3.T, name string) {
	t.Helper()
	properties := document.Components.Schemas[name].Value.Properties
	for _, forbidden := range []string{"is_deleted", "deletion_time", "created_by", "updated_by"} {
		if _, exists := properties[forbidden]; exists {
			t.Fatalf("schema %s exposes internal field %s", name, forbidden)
		}
	}
}

func parameterSchema(t *testing.T, document *openapi3.T, name string) *openapi3.Schema {
	t.Helper()
	parameter := document.Components.Parameters[name]
	if parameter == nil || parameter.Value == nil || parameter.Value.Schema == nil || parameter.Value.Schema.Value == nil {
		t.Fatalf("parameter %s has no schema", name)
	}
	return parameter.Value.Schema.Value
}

func assertJSONSuccess(t *testing.T, operation *openapi3.Operation, status string) {
	t.Helper()
	media := response(t, operation, status).Content.Get("application/json")
	if media == nil || media.Schema == nil || media.Schema.Value == nil {
		t.Fatalf("operation %s success %s is not JSON", operation.OperationID, status)
	}
}

func assertResponseHeader(t *testing.T, operation *openapi3.Operation, status string, name string) {
	t.Helper()
	if response(t, operation, status).Headers[name] == nil {
		t.Fatalf("operation %s response %s missing %s", operation.OperationID, status, name)
	}
}

func contains(values []string, wanted string) bool {
	return slices.Contains(values, wanted)
}

func assertResponseHeaderAbsent(t *testing.T, operation *openapi3.Operation, status string, name string) {
	t.Helper()
	if response(t, operation, status).Headers[name] != nil {
		t.Fatalf("operation %s response %s unexpectedly declares %s", operation.OperationID, status, name)
	}
}
