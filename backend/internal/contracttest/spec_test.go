package contracttest

import (
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

// loadAuthoritativeSpec parses and validates the repository's authoritative OpenAPI document.
func loadAuthoritativeSpec(t *testing.T) *openapi3.T {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate contract test source")
	}
	path := filepath.Join(filepath.Dir(filename), "..", "..", "api", "openapi.yaml")
	document, err := openapi3.NewLoader().LoadFromFile(path)
	if err != nil {
		t.Fatalf("load authoritative OpenAPI document: %v", err)
	}
	if validationErr := document.Validate(context.Background()); validationErr != nil {
		t.Fatalf("validate authoritative OpenAPI document: %v", validationErr)
	}
	return document
}

// operations returns every declared HTTP operation with a stable label.
func operations(document *openapi3.T) map[string]*openapi3.Operation {
	result := make(map[string]*openapi3.Operation)
	for path, item := range document.Paths.Map() {
		for method, operation := range item.Operations() {
			result[fmt.Sprintf("%s %s", method, path)] = operation
		}
	}
	return result
}

// response returns a declared response or fails with operation context.
func response(t *testing.T, operation *openapi3.Operation, status string) *openapi3.Response {
	t.Helper()
	entry := operation.Responses.Value(status)
	if entry == nil || entry.Value == nil {
		t.Fatalf("operation %s missing response %s", operation.OperationID, status)
	}
	return entry.Value
}
