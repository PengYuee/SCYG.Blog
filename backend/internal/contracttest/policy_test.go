package contracttest

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func Test_OpenAPI_operation_policy_matrix(t *testing.T) {
	// Given
	document := loadAuthoritativeSpec(t)
	all := operations(document)
	public := map[string]struct{}{
		"login": {}, "listArticles": {}, "getArticle": {},
		"listPublicArticleTypes": {}, "getPublicArticleType": {}, "listPublicTags": {},
		"getPublicTag": {}, "getArticleImageMedia": {},
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
		isConditional := method == http.MethodPatch || (method == http.MethodPost && (operation.OperationID == "publishManageArticle" || operation.OperationID == "archiveManageArticle")) || (method == http.MethodDelete && operation.OperationID != "deleteArticleImage")
		if _, isPublic := public[operation.OperationID]; isPublic {
			if operation.Security == nil || len(*operation.Security) != 0 {
				t.Fatalf("%s must explicitly disable inherited security", label)
			}
		} else {
			if operation.Security != nil {
				t.Fatalf("%s must inherit top-level bearer security", label)
			}
			assertProblemResponse(t, operation, "401")
		}
		if isWrite && operation.OperationID != "login" && !slices.Contains(operation.Tags, "Agent") {
			assertProblemResponse(t, operation, "403")
		}
		assertConditionalPolicy(t, operation, isConditional)
	}
}

func Test_OpenAPI_bearer_security_contract(t *testing.T) {
	document := loadAuthoritativeSpec(t)
	if document.Components == nil || len(document.Components.SecuritySchemes) != 1 || document.Components.SecuritySchemes["bearerAuth"] == nil {
		t.Fatal("components must declare bearerAuth")
	}
	if len(document.Security) != 1 || len(document.Security[0]) != 1 {
		t.Fatal("top-level security must require bearerAuth")
	}
	if _, exists := document.Security[0]["bearerAuth"]; !exists {
		t.Fatal("top-level security must require bearerAuth")
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
