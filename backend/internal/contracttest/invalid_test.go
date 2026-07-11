package contracttest

import (
	"context"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func Test_OpenAPI_rejects_invalid_fixture(t *testing.T) {
	// Given
	fixture := []byte(`openapi: 3.0.3
info:
  title: Invalid contract
  version: 1.0.0
paths:
  /broken:
    get:
      responses: {}
`)

	// When
	document, loadErr := openapi3.NewLoader().LoadFromData(fixture)
	if loadErr != nil {
		return
	}
	validationErr := document.Validate(context.Background())

	// Then
	if validationErr == nil {
		t.Fatal("validator accepted an operation without responses")
	}
}
