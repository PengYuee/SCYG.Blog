package contract

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	generated "github.com/PengYuee/SCYG.Blog/backend/internal/generated/openapi"
)

// tagPatchProbe implements one strict operation and inherits unreachable methods for boundary tests.
type tagPatchProbe struct {
	generated.StrictServerInterface
	reached bool
}

// PatchTag records successful traversal through validation and generated binding.
func (probe *tagPatchProbe) PatchTag(_ context.Context, _ generated.PatchTagRequestObject) (generated.PatchTagResponseObject, error) {
	probe.reached = true
	return generated.PatchTag200JSONResponse{
		Headers: generated.PatchTag200ResponseHeaders{ETag: `"2"`},
	}, nil
}

func Test_Middleware_rejects_invalid_patch_before_generated_handler(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		ifMatch string
	}{
		{name: "empty object", body: `{}`, ifMatch: `"1"`},
		{name: "unknown field", body: `{"unexpected":"value"}`, ifMatch: `"1"`},
		{name: "malformed JSON", body: `{"name":`, ifMatch: `"1"`},
		{name: "missing If-Match", body: `{"name":"x"}`},
		{name: "weak If-Match", body: `{"name":"x"}`, ifMatch: `W/"1"`},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			// Given
			probe, engine := newValidationEngine(t, Options{})
			request := patchTagRequest(testCase.body, testCase.ifMatch)
			response := httptest.NewRecorder()

			// When
			engine.ServeHTTP(response, request)

			// Then
			if response.Code != http.StatusBadRequest {
				t.Fatalf("expected 400, got %d: %s", response.Code, response.Body.String())
			}
			if probe.reached {
				t.Fatal("invalid request reached generated strict handler")
			}
		})
	}
}

func Test_Middleware_allows_valid_patch_to_generated_handler(t *testing.T) {
	// Given
	probe, engine := newValidationEngine(t, Options{})
	request := patchTagRequest(`{"name":"x"}`, `"1"`)
	response := httptest.NewRecorder()

	// When
	engine.ServeHTTP(response, request)

	// Then
	if response.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", response.Code, response.Body.String())
	}
	if !probe.reached {
		t.Fatal("valid request did not reach generated strict handler")
	}
}

func Test_Middleware_classifies_missing_IfMatch_for_future_428_mapping(t *testing.T) {
	// Given
	var captured Failure
	probe, engine := newValidationEngine(t, Options{
		ErrorHandler: func(ctx *gin.Context, failure Failure) {
			captured = failure
			ctx.AbortWithStatus(http.StatusBadRequest)
		},
	})
	request := patchTagRequest(`{"name":"x"}`, "")
	response := httptest.NewRecorder()

	// When
	engine.ServeHTTP(response, request)

	// Then
	if captured.Kind != FailureVersionRequired {
		t.Fatalf("expected version-required classification, got %q", captured.Kind)
	}
	if probe.reached {
		t.Fatal("missing If-Match reached generated strict handler")
	}
}

// newValidationEngine mounts the authoritative validator before generated routes.
func newValidationEngine(t *testing.T, options Options) (*tagPatchProbe, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	middleware, err := Middleware(options)
	if err != nil {
		t.Fatalf("construct contract middleware: %v", err)
	}
	probe := &tagPatchProbe{}
	engine := gin.New()
	engine.Use(middleware)
	generated.RegisterHandlers(engine, generated.NewStrictHandler(probe, nil))
	return probe, engine
}

// patchTagRequest builds one real Gin request against the generated route.
func patchTagRequest(body string, ifMatch string) *http.Request {
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/tags/1", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if ifMatch != "" {
		request.Header.Set("If-Match", ifMatch)
	}
	return request
}
