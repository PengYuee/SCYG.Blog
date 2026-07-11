package contracttest

import (
	"net/http"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func Test_OpenAPI_operation_policy_matrix(t *testing.T) {
	// Given
	document := loadAuthoritativeSpec(t)
	all := operations(document)
	if len(all) != 15 {
		t.Fatalf("expected 15 operations, got %d", len(all))
	}

	// When / Then
	seenIDs := make(map[string]struct{}, len(all))
	for label, operation := range all {
		if operation.OperationID == "" {
			t.Fatalf("%s has no operationId", label)
		}
		if _, exists := seenIDs[operation.OperationID]; exists {
			t.Fatalf("duplicate operationId %q", operation.OperationID)
		}
		seenIDs[operation.OperationID] = struct{}{}
		assertProblemResponse(t, operation, "500")
		method, _, _ := strings.Cut(label, " ")
		isWrite := method == http.MethodPost || method == http.MethodPatch || method == http.MethodDelete
		isConditional := method == http.MethodPatch || method == http.MethodDelete
		if isWrite {
			assertProblemResponse(t, operation, "403")
		}
		assertConditionalPolicy(t, operation, isConditional)
		if operation.Security != nil && len(*operation.Security) != 0 {
			t.Fatalf("%s declares operation security", label)
		}
	}
}

func Test_OpenAPI_paths_are_resource_only(t *testing.T) {
	// Given
	document := loadAuthoritativeSpec(t)
	want := map[string]struct{}{
		"/api/v1/articles": {}, "/api/v1/articles/{article_id}": {},
		"/api/v1/article-types": {}, "/api/v1/article-types/{article_type_id}": {},
		"/api/v1/tags": {}, "/api/v1/tags/{tag_id}": {},
	}

	// When / Then
	if len(document.Paths.Map()) != len(want) {
		t.Fatalf("expected %d resource paths, got %d", len(want), len(document.Paths.Map()))
	}
	for path := range document.Paths.Map() {
		if _, exists := want[path]; !exists {
			t.Fatalf("unexpected non-resource path %q", path)
		}
	}
	if document.Components == nil || len(document.Components.SecuritySchemes) != 0 {
		t.Fatal("components must exist without securitySchemes")
	}
	if len(document.Security) != 0 {
		t.Fatal("top-level security declaration is forbidden")
	}
}

func assertConditionalPolicy(t *testing.T, operation *openapi3.Operation, conditional bool) {
	t.Helper()
	var ifMatch *openapi3.Parameter
	for _, ref := range operation.Parameters {
		if ref.Value != nil && ref.Value.In == openapi3.ParameterInHeader && strings.EqualFold(ref.Value.Name, "If-Match") {
			ifMatch = ref.Value
		}
	}
	if conditional {
		if ifMatch == nil || !ifMatch.Required || ifMatch.Schema == nil || ifMatch.Schema.Value == nil {
			t.Fatalf("operation %s must require If-Match", operation.OperationID)
		}
		if ifMatch.Schema.Value.Pattern != `^"[1-9][0-9]*"$` {
			t.Fatalf("operation %s has non-strong If-Match schema", operation.OperationID)
		}
		assertProblemResponse(t, operation, "412")
		assertProblemResponse(t, operation, "428")
		return
	}
	if ifMatch != nil || operation.Responses.Value("412") != nil || operation.Responses.Value("428") != nil {
		t.Fatalf("operation %s must not declare conditional-write contract", operation.OperationID)
	}
}

func assertProblemResponse(t *testing.T, operation *openapi3.Operation, status string) {
	t.Helper()
	declared := response(t, operation, status)
	media := declared.Content.Get("application/problem+json")
	if media == nil || media.Schema == nil || media.Schema.Value == nil {
		t.Fatalf("operation %s response %s is not RFC 9457", operation.OperationID, status)
	}
}
